package users

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/ItemizeNTNU/website/internal/discord"
	"github.com/ItemizeNTNU/website/internal/fusionauth"
)

// ErrUnavailable is returned when Discord or FusionAuth is not configured.
var ErrUnavailable = errors.New("the Discord integration is not configured")

// ErrNotLinked is returned when there is no Discord account to act on.
var ErrNotLinked = errors.New("no Discord account is linked")

// Link is what the profile page shows about a member's Discord account.
type Link struct {
	ID       string
	Username string
	Avatar   string
	// IsMember reports whether they have actually joined the guild. Linking an
	// account and joining the server are separate steps, and people routinely
	// do the first and forget the second.
	IsMember bool
	// MembershipUnknown is set when Discord could not be asked at all — a
	// rejected bot token, say. That is not the same as "has not joined", and
	// conflating them tells a member to go join a server they are already in
	// while the real fault is ours.
	MembershipUnknown bool
}

// DiscordService links member accounts to Discord identities.
type DiscordService struct {
	discord *discord.Client
	fusion  *fusionauth.Client
	log     *slog.Logger
}

// NewDiscordService builds the service.
func NewDiscordService(d *discord.Client, f *fusionauth.Client, log *slog.Logger) *DiscordService {
	return &DiscordService{discord: d, fusion: f, log: log}
}

// Available reports whether the linking flow can run at all.
func (s *DiscordService) Available() bool {
	return s != nil && s.discord.Enabled() && s.fusion.Configured()
}

// AuthorizeURL builds the link a member follows to grant access.
func (s *DiscordService) AuthorizeURL(state, redirectURI string) (string, error) {
	if !s.Available() {
		return "", ErrUnavailable
	}
	return s.discord.AuthorizeURL(state, redirectURI), nil
}

// Complete finishes the OAuth flow and records the link.
func (s *DiscordService) Complete(ctx context.Context, userID, code, redirectURI string) (*Link, error) {
	if !s.Available() {
		return nil, ErrUnavailable
	}
	account, err := s.discord.Exchange(ctx, code, redirectURI)
	if err != nil {
		return nil, err
	}
	// Nothing checks that this Discord account is not already linked to another
	// member, and store writes without reading what is there first. The stored
	// links do not collide — each patch touches only its own FusionAuth record,
	// so no other member's link is detached — but the guild role is granted and
	// withdrawn by Discord id alone, guild-wide. Two members sharing one Discord
	// account therefore share one role: when either presses "fjern kobling",
	// Unlink calls RemoveMemberRole on that id and the other member loses their
	// access while their own profile still shows the account as linked and them
	// as a guild member. Enforcing uniqueness needs a FusionAuth user search on
	// data.discord.id, so it is left as a product decision rather than done here.
	return s.store(ctx, userID, account)
}

// Refresh re-reads the linked account and reconciles guild membership.
//
// This is what a member presses after joining the server, so that the role is
// granted without waiting for anything else.
func (s *DiscordService) Refresh(ctx context.Context, userID string) (*Link, error) {
	if !s.Available() {
		return nil, ErrUnavailable
	}

	current, err := s.fusion.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	discordID := linkedID(current)
	if discordID == "" {
		return nil, ErrNotLinked
	}

	account, err := s.discord.GetUser(ctx, discordID)
	if err != nil {
		return nil, err
	}
	return s.store(ctx, userID, account)
}

// store writes the link and grants the role when they are in the guild.
func (s *DiscordService) store(ctx context.Context, userID string, account *discord.User) (*Link, error) {
	member, err := s.discord.GuildMember(ctx, account.ID)
	if err != nil {
		// Being unable to check membership should not lose the link itself —
		// the member can press refresh once whatever broke is fixed. But it
		// must not be reported as "has not joined" either.
		s.log.Error("could not check guild membership; the member role cannot be "+
			"granted until this is fixed",
			"discord_id", account.ID, "err", err)
	}
	unknown := err != nil
	isMember := member != nil

	if isMember {
		if err := s.discord.AddMemberRole(ctx, account.ID); err != nil {
			s.log.Error("granting the Discord role failed", "discord_id", account.ID, "err", err)
		}
	}

	link := &Link{
		ID:                account.ID,
		Username:          account.DisplayName(),
		Avatar:            account.AvatarURL(),
		IsMember:          isMember,
		MembershipUnknown: unknown,
	}

	stored := map[string]any{
		"id":       link.ID,
		"username": link.Username,
		"avatar":   link.Avatar,
	}
	// Only a real answer is written. A failed check says nothing about this
	// member, and storing it as isMember:false would turn our own outage into a
	// permanent "has not joined": the next page load reads the record back
	// through CurrentLink, which has no notion of the check having failed, and
	// tells them to join a server they may already be in. Leaving the key out
	// of the merge patch keeps whatever was last known in place instead.
	if !unknown {
		stored["isMember"] = link.IsMember
	}

	changes := map[string]any{
		"data":     map[string]any{"discord": stored},
		"imageUrl": link.Avatar,
	}
	if _, err := s.fusion.PatchUser(ctx, userID, changes); err != nil {
		return nil, fmt.Errorf("storing the Discord link: %w", err)
	}
	return link, nil
}

// Unlink removes the connection and withdraws the role.
func (s *DiscordService) Unlink(ctx context.Context, userID string) error {
	if !s.Available() {
		return ErrUnavailable
	}

	current, err := s.fusion.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	discordID := linkedID(current)
	if discordID == "" {
		return ErrNotLinked
	}

	if err := s.discord.RemoveMemberRole(ctx, discordID); err != nil {
		// Losing the role is Discord's business; the member asked us to
		// forget the link, and that must happen regardless.
		s.log.Warn("withdrawing the Discord role failed", "discord_id", discordID, "err", err)
	}

	// An explicit null is what deletes a key in a merge patch. Omitting the
	// field would leave the link in place, which is the opposite of what was
	// asked for.
	changes := map[string]any{"data": map[string]any{"discord": nil}}
	// The avatar came from Discord, so it goes too — otherwise the profile
	// keeps showing a picture from an account that is no longer connected.
	if current.ImageURL != "" && current.ImageURL == linkedAvatar(current) {
		changes["imageUrl"] = nil
	}

	if _, err := s.fusion.PatchUser(ctx, userID, changes); err != nil {
		return fmt.Errorf("clearing the Discord link: %w", err)
	}
	return nil
}

// CurrentLink reads what is stored, without contacting Discord.
func CurrentLink(u *fusionauth.User) *Link {
	if u == nil {
		return nil
	}
	block, ok := u.Data["discord"].(map[string]any)
	if !ok {
		return nil
	}
	id, _ := block["id"].(string)
	if id == "" {
		return nil
	}
	username, _ := block["username"].(string)
	avatar, _ := block["avatar"].(string)
	isMember, _ := block["isMember"].(bool)
	return &Link{ID: id, Username: username, Avatar: avatar, IsMember: isMember}
}

func linkedID(u *fusionauth.User) string {
	if link := CurrentLink(u); link != nil {
		return link.ID
	}
	return ""
}

func linkedAvatar(u *fusionauth.User) string {
	if link := CurrentLink(u); link != nil {
		return link.Avatar
	}
	return ""
}
