package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"github.com/ItemizeNTNU/website/internal/auth"
	"github.com/ItemizeNTNU/website/internal/discord"
	"github.com/ItemizeNTNU/website/internal/users"
)

// discordStateCookie carries the OAuth state across the round trip to Discord.
const discordStateCookie = "itemize_discord_state"

// discordTimeout bounds one trip through the linking flow.
//
// r.Context() carries no deadline of its own — http.Server's WriteTimeout does
// not cancel it — so without this the handler waits for however long Discord
// and FusionAuth between them decide to take. Each of these calls touches
// Discord two or three times plus FusionAuth twice, and thirty seconds is
// comfortably more than that needs while staying inside the server's
// sixty-second WriteTimeout. Past it the member sees an error and can press the
// button again, which is better than a goroutine held open indefinitely.
const discordTimeout = 30 * time.Second

// discordCallbackPath is registered with Discord as the redirect URI and must
// match byte for byte.
const discordCallbackPath = "/api/discord/callback"

func (s *Server) discordRedirectURI() string { return s.baseURL + discordCallbackPath }

// regLinkCookie holds a sealed proof of a just-completed registration, so the
// new member can link Discord before their first login — registration ends
// with a set-password email, not a session, and sending someone straight from
// the signup form through login would lose most of them on the way.
const regLinkCookie = "itemize_registrering"

// regLinkPurpose tags the sealed value. Everything this server seals uses the
// same key, so without a purpose check any other sealed cookie — a session, an
// OIDC flow — could be replayed here and open as garbage-or-worse.
const regLinkPurpose = "register-discord"

// regLinkTTL bounds how long the registration cookie can vouch for its owner.
//
// Thirty minutes — deliberately longer than the logged-in flow's 600-second
// state cookie, because the person this flow exists for may not have a
// Discord account yet and creating one mid-flow (email verification included)
// takes longer than pressing "authorize". Still short enough to bound the
// capability the cookie carries: whoever holds it can attach a Discord
// identity to the freshly created account, and that power should not sit in a
// cookie jar for days.
const regLinkTTL = 30 * time.Minute

// regLinkState is what the registration cookie carries.
type regLinkState struct {
	Purpose string    `json:"p"`
	UserID  string    `json:"u"`
	Expires time.Time `json:"exp"`
}

// setRegLinkCookie seals the created user's id into the registration cookie.
//
// Failure is logged and swallowed: the membership already exists and the
// confirmation must proceed — the cookie is a convenience on top of
// registration, never a step of it.
func (s *Server) setRegLinkCookie(w http.ResponseWriter, userID string) {
	if s.sealer == nil || userID == "" {
		// No sealer means no way to make the cookie trustworthy; no id means
		// nothing to vouch for. Either way the flow is simply not offered.
		return
	}
	sealed, err := s.sealer.Seal(regLinkState{
		Purpose: regLinkPurpose,
		UserID:  userID,
		Expires: time.Now().Add(regLinkTTL),
	})
	if err != nil {
		s.log.Error("sealing the registration cookie failed; the Discord offer is skipped", "err", err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     regLinkCookie,
		Value:    sealed,
		Path:     "/",
		MaxAge:   int(regLinkTTL.Seconds()),
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

// readRegLinkCookie opens the registration cookie and returns the user it
// vouches for. Anything short of a sealed, unexpired, purpose-tagged value
// with a user id in it answers false.
func (s *Server) readRegLinkCookie(r *http.Request) (userID string, ok bool) {
	if s.sealer == nil {
		return "", false
	}
	c, err := r.Cookie(regLinkCookie)
	if err != nil {
		return "", false
	}
	var reg regLinkState
	if err := s.sealer.Open(c.Value, &reg); err != nil {
		return "", false
	}
	// The Expires inside the sealed value is what counts, not the cookie's
	// MaxAge — that one is client-side and a client can lie about it.
	if reg.Purpose != regLinkPurpose || reg.UserID == "" || time.Now().After(reg.Expires) {
		return "", false
	}
	return reg.UserID, true
}

// clearRegLinkCookie expires the registration cookie once its purpose is
// fulfilled.
func (s *Server) clearRegLinkCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: regLinkCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.secureCookies, SameSite: http.SameSiteLaxMode,
	})
}

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

// discordRegisterLink starts the linking flow for somebody who just
// registered and has no session yet — the sealed registration cookie is what
// vouches for them.
func (s *Server) discordRegisterLink(w http.ResponseWriter, r *http.Request) {
	if auth.FromRequest(r) != nil {
		// Same person, better flow: with a session the ordinary profile flow
		// applies, and its outcomes land on a page they can actually open.
		http.Redirect(w, r, "/api/discord/link", http.StatusSeeOther)
		return
	}
	if !s.discordSvc.Available() {
		SetFlash(w, "error", "Discord-kobling er ikke tilgjengelig akkurat nå.")
		http.Redirect(w, r, "/registrert", http.StatusSeeOther)
		return
	}
	if _, ok := s.readRegLinkCookie(r); !ok {
		// No proof of a recent registration, so there is nothing to link the
		// Discord account to. Back to the page, which will not be offering the
		// button either.
		http.Redirect(w, r, "/registrert", http.StatusSeeOther)
		return
	}

	// The same random, browser-bound state as discordLink — the callback
	// cannot tell the two flows' round trips apart and must not need to.
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		s.log.Error("generating the Discord state failed", "err", err)
		s.ErrorPage(w, r, http.StatusInternalServerError)
		return
	}
	state := base64.RawURLEncoding.EncodeToString(raw)

	http.SetCookie(w, &http.Cookie{
		Name:  discordStateCookie,
		Value: state,
		Path:  "/",
		// Thirty minutes rather than the profile flow's 600 seconds: this
		// person may be creating a Discord account mid-flow. See regLinkTTL.
		MaxAge:   int(regLinkTTL.Seconds()),
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
//
// Not behind RequireLogin: the person arriving here is either a signed-in
// member who started on the profile page, or somebody fresh from the signup
// form holding the sealed registration cookie — they have no session, because
// registration ends with a set-password email rather than a login. Identity
// comes from whichever of the two is present; with neither, the handler fails
// closed before touching anything.
func (s *Server) discordCallback(w http.ResponseWriter, r *http.Request) {
	// Resolved once, used by every branch below, so a registrant can never be
	// bounced onto the profile page their missing session cannot open.
	var userID string
	returnTo := "/profil"
	registrant := false

	if user := auth.FromRequest(r); user != nil {
		userID = user.ID
	} else if id, ok := s.readRegLinkCookie(r); ok {
		userID = id
		returnTo = "/registrert"
		registrant = true
	} else {
		// Nobody we can vouch for. Redirect with zero side effects — not even
		// the state cookie is expired, because every action here would be
		// taken on behalf of no one in particular.
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// One attempt per cookie, whatever happens next.
	cookie, cookieErr := r.Cookie(discordStateCookie)
	http.SetCookie(w, &http.Cookie{
		Name: discordStateCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.secureCookies, SameSite: http.SameSiteLaxMode,
	})

	if r.URL.Query().Get("error") != "" {
		// They pressed cancel. For a member that is not worth a message; a
		// registrant is told the door stays open, because the page they land
		// back on just made a point of offering the link.
		if registrant {
			SetFlash(w, "info", "Du kan koble til Discord senere fra profilsiden din.")
		}
		http.Redirect(w, r, returnTo, http.StatusSeeOther)
		return
	}
	if cookieErr != nil || !auth.ConstantTimeEqual(cookie.Value, r.URL.Query().Get("state")) {
		if registrant {
			// The fallback is spelled out: their registration cookie may well
			// outlive the failed attempt, but "log in and do it from the
			// profile" works no matter what state their cookies are in.
			SetFlash(w, "error", "Discord-koblingen kunne ikke bekreftes. "+
				"Prøv igjen, eller logg inn og koble til fra profilsiden senere.")
		} else {
			SetFlash(w, "error", "Discord-koblingen kunne ikke bekreftes. Prøv igjen.")
		}
		http.Redirect(w, r, returnTo, http.StatusSeeOther)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), discordTimeout)
	defer cancel()

	link, err := s.discordSvc.Complete(ctx, userID,
		r.URL.Query().Get("code"), s.discordRedirectURI())
	if registrant && err == nil {
		// The link is stored, so the capability the registration cookie
		// carried is spent. On failure it stays, so pressing the button on
		// /registrert again still works.
		s.clearRegLinkCookie(w)
	}
	switch {
	case errors.Is(err, discord.ErrDenied):
		if registrant {
			SetFlash(w, "info", "Du kan koble til Discord senere fra profilsiden din.")
		}
		http.Redirect(w, r, returnTo, http.StatusSeeOther)
		return
	case err != nil:
		s.log.Error("linking the Discord account failed", "err", err)
		SetFlash(w, "error", "Discord-kontoen kunne ikke kobles. Prøv igjen.")
	case link.MembershipUnknown:
		// Our fault, not theirs. Telling them to go join would send them after
		// a problem they cannot fix.
		SetFlash(w, "warning",
			"Discord-kontoen er koblet, men vi fikk ikke sjekket medlemskapet på "+
				"serveren. Si fra til styret.")
	case !link.IsMember:
		// The common case worth explaining: linked, but not in the server yet.
		SetFlash(w, "info",
			"Discord-kontoen er koblet. Bli med på serveren, og trykk så «Oppdater».")
	default:
		SetFlash(w, "success", "Discord-kontoen er koblet.")
	}

	http.Redirect(w, r, returnTo, http.StatusSeeOther)
}

// discordRefresh re-reads the linked account and reconciles the role.
func (s *Server) discordRefresh(w http.ResponseWriter, r *http.Request) {
	user := auth.FromRequest(r)

	ctx, cancel := context.WithTimeout(r.Context(), discordTimeout)
	defer cancel()

	link, err := s.discordSvc.Refresh(ctx, user.ID)
	switch {
	case errors.Is(err, users.ErrNotLinked):
		SetFlash(w, "error", "Du har ingen Discord-konto koblet.")
	case errors.Is(err, users.ErrUnavailable):
		SetFlash(w, "error", "Discord-kobling er ikke tilgjengelig akkurat nå.")
	case err != nil:
		s.log.Error("refreshing the Discord link failed", "err", err)
		SetFlash(w, "error", "Kunne ikke oppdatere Discord-informasjonen.")
	case link.MembershipUnknown:
		SetFlash(w, "warning",
			"Vi fikk ikke kontakt med Discord for å sjekke medlemskapet ditt. "+
				"Si fra til styret.")
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

	ctx, cancel := context.WithTimeout(r.Context(), discordTimeout)
	defer cancel()

	switch err := s.discordSvc.Unlink(ctx, user.ID); {
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
