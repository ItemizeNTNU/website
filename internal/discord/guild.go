package discord

import (
	"context"
	"net/http"
)

// GuildMember looks up a member of the guild.
//
// A nil member with a nil error means the account exists but has not joined —
// the ordinary case for someone who has just linked their account and not yet
// followed the invite.
//
// This endpoint needs the bot to have the guild members intent. Without it
// Discord answers 403 rather than 404, which reads as "not a member" if the
// two are not told apart.
func (c *Client) GuildMember(ctx context.Context, discordID string) (*GuildMember, error) {
	var m GuildMember
	err := c.do(ctx, http.MethodGet,
		"/guilds/"+c.cfg.GuildID+"/members/"+discordID, nil, &m)

	if apiErr, ok := err.(*APIError); ok {
		switch apiErr.Status {
		case http.StatusNotFound:
			return nil, nil // simply not in the guild
		case http.StatusForbidden:
			// A configuration problem, not an answer about this person.
			c.log.Error("the bot may be missing the guild members intent",
				"guild", c.cfg.GuildID, "err", err)
			return nil, err
		}
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// AddMemberRole grants the member role.
func (c *Client) AddMemberRole(ctx context.Context, discordID string) error {
	return c.do(ctx, http.MethodPut, c.rolePath(discordID), nil, nil)
}

// RemoveMemberRole withdraws the member role.
//
// Someone who has already left the guild, or already lost the role, is not an
// error: the desired state has been reached either way.
func (c *Client) RemoveMemberRole(ctx context.Context, discordID string) error {
	err := c.do(ctx, http.MethodDelete, c.rolePath(discordID), nil, nil)
	if apiErr, ok := err.(*APIError); ok && apiErr.Status == http.StatusNotFound {
		return nil
	}
	return err
}

func (c *Client) rolePath(discordID string) string {
	return "/guilds/" + c.cfg.GuildID + "/members/" + discordID +
		"/roles/" + c.cfg.MemberRoleID
}
