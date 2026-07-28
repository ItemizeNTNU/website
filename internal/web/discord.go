package web

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/ItemizeNTNU/website/internal/auth"
	"github.com/ItemizeNTNU/website/internal/discord"
	"github.com/ItemizeNTNU/website/internal/users"
)

// discordStateCookie carries the OAuth state across the round trip to Discord.
const discordStateCookie = "itemize_discord_state"

// discordCallbackPath is registered with Discord as the redirect URI and must
// match byte for byte.
const discordCallbackPath = "/api/discord/callback"

func (s *Server) discordRedirectURI() string { return s.baseURL + discordCallbackPath }

// discordLink starts the account-linking flow.
func (s *Server) discordLink(w http.ResponseWriter, r *http.Request) {
	if !s.discordSvc.Available() {
		SetFlash(w, "error", "Discord-kobling er ikke tilgjengelig akkurat nå.")
		http.Redirect(w, r, "/profil", http.StatusSeeOther)
		return
	}

	// A real, unpredictable state. The previous server sent the literal
	// string "123" with a comment admitting it, which meant the callback
	// could be driven by any page that could get the visitor to load it.
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		s.log.Error("generating the Discord state failed", "err", err)
		s.ErrorPage(w, r, http.StatusInternalServerError)
		return
	}
	state := base64.RawURLEncoding.EncodeToString(raw)

	http.SetCookie(w, &http.Cookie{
		Name:     discordStateCookie,
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})

	url, err := s.discordSvc.AuthorizeURL(state, s.discordRedirectURI())
	if err != nil {
		s.ErrorPage(w, r, http.StatusServiceUnavailable)
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

// discordCallback completes the flow.
func (s *Server) discordCallback(w http.ResponseWriter, r *http.Request) {
	user := auth.FromRequest(r)

	// One attempt per cookie, whatever happens next.
	cookie, cookieErr := r.Cookie(discordStateCookie)
	http.SetCookie(w, &http.Cookie{
		Name: discordStateCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.secureCookies, SameSite: http.SameSiteLaxMode,
	})

	if r.URL.Query().Get("error") != "" {
		// They pressed cancel. Not worth an error message.
		http.Redirect(w, r, "/profil", http.StatusSeeOther)
		return
	}
	if cookieErr != nil || !auth.ConstantTimeEqual(cookie.Value, r.URL.Query().Get("state")) {
		SetFlash(w, "error", "Discord-koblingen kunne ikke bekreftes. Prøv igjen.")
		http.Redirect(w, r, "/profil", http.StatusSeeOther)
		return
	}

	link, err := s.discordSvc.Complete(r.Context(), user.ID,
		r.URL.Query().Get("code"), s.discordRedirectURI())
	switch {
	case errors.Is(err, discord.ErrDenied):
		http.Redirect(w, r, "/profil", http.StatusSeeOther)
		return
	case err != nil:
		s.log.Error("linking the Discord account failed", "err", err)
		SetFlash(w, "error", "Discord-kontoen kunne ikke kobles. Prøv igjen.")
	case !link.IsMember:
		// The common case worth explaining: linked, but not in the server yet.
		SetFlash(w, "info",
			"Discord-kontoen er koblet. Bli med på serveren, og trykk så «Oppdater».")
	default:
		SetFlash(w, "success", "Discord-kontoen er koblet.")
	}

	http.Redirect(w, r, "/profil", http.StatusSeeOther)
}

// discordRefresh re-reads the linked account and reconciles the role.
func (s *Server) discordRefresh(w http.ResponseWriter, r *http.Request) {
	user := auth.FromRequest(r)

	link, err := s.discordSvc.Refresh(r.Context(), user.ID)
	switch {
	case errors.Is(err, users.ErrNotLinked):
		SetFlash(w, "error", "Du har ingen Discord-konto koblet.")
	case errors.Is(err, users.ErrUnavailable):
		SetFlash(w, "error", "Discord-kobling er ikke tilgjengelig akkurat nå.")
	case err != nil:
		s.log.Error("refreshing the Discord link failed", "err", err)
		SetFlash(w, "error", "Kunne ikke oppdatere Discord-informasjonen.")
	case !link.IsMember:
		SetFlash(w, "warning",
			"Du er fortsatt ikke med på Discord-serveren. Bli med, og prøv igjen.")
	default:
		SetFlash(w, "success", "Discord-informasjonen er oppdatert.")
	}

	http.Redirect(w, r, "/profil", http.StatusSeeOther)
}

// discordUnlink removes the connection.
func (s *Server) discordUnlink(w http.ResponseWriter, r *http.Request) {
	user := auth.FromRequest(r)

	switch err := s.discordSvc.Unlink(r.Context(), user.ID); {
	case errors.Is(err, users.ErrNotLinked):
		SetFlash(w, "error", "Du har ingen Discord-konto koblet.")
	case err != nil:
		s.log.Error("unlinking the Discord account failed", "err", err)
		SetFlash(w, "error", "Kunne ikke fjerne Discord-koblingen.")
	default:
		SetFlash(w, "success", "Discord-koblingen er fjernet.")
	}

	http.Redirect(w, r, "/profil", http.StatusSeeOther)
}
