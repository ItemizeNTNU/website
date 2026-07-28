package discord

import "github.com/ItemizeNTNU/website/internal/config"

func testConfig() config.Discord {
	return config.Discord{
		ClientID:     "client-1",
		ClientSecret: "secret-1",
		BotToken:     "bot-1",
		GuildID:      "guild-1",
		MemberRoleID: "role-1",
	}
}
