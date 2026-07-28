package web

import (
	"net/http"

	"github.com/ItemizeNTNU/website/internal/auth"
)

type profileView struct {
	Page
	// AvatarURL falls back to the logo, matching the previous site's default.
	AvatarURL string
}

// profil shows the signed-in member's own details.
//
// Everything here comes from the session, which is filled from the ID token —
// no call to FusionAuth is needed to render the page. Discord linking arrives
// with the account-linking work; until then the page says so rather than
// showing a control that does nothing.
func (s *Server) profil(w http.ResponseWriter, r *http.Request) {
	user := auth.FromRequest(r)

	view := profileView{
		Page:      s.page(r, user.DisplayName(), "profil"),
		AvatarURL: user.ImageURL,
	}
	if view.AvatarURL == "" {
		view.AvatarURL = "/logo-512.png"
	}
	s.render(w, r, http.StatusOK, "profil", view)
}
