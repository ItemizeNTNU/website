package discord

import (
	"fmt"
	"strconv"
)

// User is a Discord account.
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	// GlobalName is the display name introduced with the username migration.
	// Empty for accounts that have not set one.
	GlobalName string `json:"global_name"`
	// Discriminator is the legacy #1234 suffix. Migrated accounts report the
	// literal string "0" — see DisplayName.
	Discriminator string `json:"discriminator"`
	Avatar        string `json:"avatar"`
}

// migratedDiscriminator is what a post-migration account reports.
const migratedDiscriminator = "0"

// DisplayName renders the handle to show and store.
//
// Discord retired discriminators in the "pomelo" migration: usernames became
// globally unique and the #1234 suffix went away, with migrated accounts
// reporting "0". The previous server built "username#discriminator"
// unconditionally, so every linked member would now display as "name#0".
func (u User) DisplayName() string {
	if u.GlobalName != "" {
		return u.GlobalName
	}
	if u.Discriminator != "" && u.Discriminator != migratedDiscriminator {
		return u.Username + "#" + u.Discriminator
	}
	return u.Username
}

// AvatarURL returns the account's avatar, falling back to Discord's default.
func (u User) AvatarURL() string {
	if u.Avatar != "" {
		return fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png?size=512", u.ID, u.Avatar)
	}
	return fmt.Sprintf("https://cdn.discordapp.com/embed/avatars/%d.png", u.defaultAvatarIndex())
}

// defaultAvatarIndex picks the placeholder avatar.
//
// Legacy accounts index by discriminator modulo 5. Migrated accounts index by
// the snowflake shifted right 22 bits, modulo 6 — a different divisor as well
// as a different input.
//
// The snowflake must be parsed as an unsigned 64-bit integer. It exceeds
// 2^53, so anything that reads it as a float — which a direct port of the
// JavaScript would — silently loses precision and computes the wrong bucket.
func (u User) defaultAvatarIndex() uint64 {
	if u.Discriminator != "" && u.Discriminator != migratedDiscriminator {
		if d, err := strconv.ParseUint(u.Discriminator, 10, 64); err == nil {
			return d % 5
		}
	}
	if id, err := strconv.ParseUint(u.ID, 10, 64); err == nil {
		return (id >> 22) % 6
	}
	return 0
}

// GuildMember is a membership record in the guild.
type GuildMember struct {
	User  *User    `json:"user"`
	Roles []string `json:"roles"`
	Nick  string   `json:"nick"`
}
