package web

import (
	"net/http"

	"github.com/ItemizeNTNU/website/internal/auth"
	"github.com/ItemizeNTNU/website/internal/users"
)

type profileView struct {
	Page
	// AvatarURL falls back to the logo, matching the previous site's default.
	AvatarURL string
	Discord   *users.Link
	// DiscordAvailable is false when the integration is not configured, so the
	// page can omit a control that could not work.
	DiscordAvailable bool
}

// profil shows the signed-in member's own details.
//
// The identity comes from the session, which is filled from the ID token. The
// Discord link is read from FusionAuth, because it changes without the session
// being reissued — someone who links their account would otherwise see stale
// information until their next login.
func (s *Server) profil(w http.ResponseWriter, r *http.Request) {
	user := auth.FromRequest(r)

	view := profileView{
		Page:             s.page(r, user.DisplayName(), "profil"),
		AvatarURL:        user.ImageURL,
		DiscordAvailable: s.discordSvc.Available(),
	}
	view.CSRF = s.csrf(w, r)
	view.Command = "whoami --verbose"

	if s.fusion.Configured() {
		if stored, err := s.fusion.GetUser(r.Context(), user.ID); err != nil {
			// Not fatal: the page is still worth showing without it.
			s.log.Warn("could not read the profile from FusionAuth", "err", err)
		} else {
			view.Discord = users.CurrentLink(stored)
			if stored.ImageURL != "" {
				view.AvatarURL = stored.ImageURL
			}
		}
	}

	if view.AvatarURL == "" {
		view.AvatarURL = "/logo-512.png"
	}
	s.render(w, r, http.StatusOK, "profil", view)
}
