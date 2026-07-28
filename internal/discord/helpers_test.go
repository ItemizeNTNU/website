package discord

import (
	"io"
	"log/slog"

	"github.com/ItemizeNTNU/website/internal/config"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testConfig() config.Discord {
	return config.Discord{
		ClientID:     "client-1",
		ClientSecret: "secret-1",
		BotToken:     "bot-1",
		GuildID:      "guild-1",
		MemberRoleID: "role-1",
	}
}
