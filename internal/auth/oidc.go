package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/ItemizeNTNU/website/internal/config"
)

// flowCookie carries the state, nonce and PKCE verifier between the redirect
// out to FusionAuth and the callback. Keeping them in a sealed cookie rather
// than server memory is what makes the login flow stateless — there is no
// session store to run, and a restart mid-login is harmless.
const flowCookie = "itemize_oidc"

// flowTTL bounds how long a login attempt may take.
const flowTTL = 10 * time.Minute

// Authenticator runs the OpenID Connect login flow against FusionAuth.
type Authenticator struct {
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    *oauth2.Config
	sealer   *Sealer
	log      *slog.Logger
}

// flowState is what the pre-login cookie holds.
type flowState struct {
	State    string    `json:"s"`
	Nonce    string    `json:"n"`
	Verifier string    `json:"v"`
	ReturnTo string    `json:"r"`
	Expires  time.Time `json:"e"`
}

// New builds an Authenticator by discovering the provider's configuration.
func New(ctx context.Context, cfg *config.Config, sealer *Sealer, log *slog.Logger) (*Authenticator, error) {
	issuer := cfg.FusionAuth.Host.String()

	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf(
			"OIDC discovery failed against %s — check that the issuer in "+
				"%s/.well-known/openid-configuration matches this URL exactly: %w",
			issuer, issuer, err)
	}

	oidcCfg := &oidc.Config{
		ClientID:             cfg.FusionAuth.ClientID,
		SupportedSigningAlgs: []string{cfg.FusionAuth.IDTokenAlg},
	}

	var verifier *oidc.IDTokenVerifier
	switch cfg.FusionAuth.IDTokenAlg {
	case "HS256":
		// Symmetric signing: there is no public key to fetch, so the token is
		// verified against the shared secret. See hs256.go, and docs/auth.md
		// for what it would take to stop doing this.
		log.Warn("verifying ID tokens with HS256; RS256 is preferred — see docs/auth.md")
		verifier = oidc.NewVerifier(issuer,
			hmacKeySet{secret: []byte(cfg.FusionAuth.IDTokenSecret())}, oidcCfg)
	default:
		// RS256 and friends: go-oidc fetches and caches the provider's JWKS
		// and handles key rotation on its own.
		verifier = provider.Verifier(oidcCfg)
	}

	return &Authenticator{
		provider: provider,
		verifier: verifier,
		oauth: &oauth2.Config{
			ClientID:     cfg.FusionAuth.ClientID,
			ClientSecret: cfg.FusionAuth.ClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  cfg.BaseURL.String() + "/callback",
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
		sealer: sealer,
		log:    log,
	}, nil
}

// Routes registers the login endpoints. The previous site had these mounted
// implicitly by its OIDC middleware; they are explicit here.
func (a *Authenticator) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /login", a.Login)
	mux.HandleFunc("GET /callback", a.Callback)
	mux.HandleFunc("GET /logout", a.Logout)
	mux.HandleFunc("POST /logout", a.Logout)
}

// Login starts the authorization-code flow.
func (a *Authenticator) Login(w http.ResponseWriter, r *http.Request) {
	state, err := randomToken()
	if err != nil {
		a.fail(w, "kunne ikke starte innlogging", err)
		return
	}
	nonce, err := randomToken()
	if err != nil {
		a.fail(w, "kunne ikke starte innlogging", err)
		return
	}
	verifier := oauth2.GenerateVerifier()

	flow := flowState{
		State:    state,
		Nonce:    nonce,
		Verifier: verifier,
		ReturnTo: safeReturnTo(r.URL.Query().Get("return_to")),
		Expires:  time.Now().Add(flowTTL),
	}
	sealed, err := a.sealer.Seal(flow)
	if err != nil {
		a.fail(w, "kunne ikke starte innlogging", err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     flowCookie,
		Value:    sealed,
		Path:     "/",
		MaxAge:   int(flowTTL.Seconds()),
		HttpOnly: true,
		Secure:   a.sealer.secure,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, a.oauth.AuthCodeURL(state,
		oidc.Nonce(nonce),
		// Always sent, whether or not FusionAuth requires it. A provider
		// validates the verifier whenever a challenge was present, so this
		// gives us the protection regardless of the application's setting.
		oauth2.S256ChallengeOption(verifier),
	), http.StatusFound)
}

// Callback completes the flow and establishes the session.
func (a *Authenticator) Callback(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(flowCookie)
	if err != nil {
		a.fail(w, "innloggingen tok for lang tid — prøv igjen", err)
		return
	}
	// One attempt per cookie, whatever the outcome.
	http.SetCookie(w, &http.Cookie{
		Name: flowCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: a.sealer.secure, SameSite: http.SameSiteLaxMode,
	})

	var flow flowState
	if err := a.sealer.Open(c.Value, &flow); err != nil {
		a.fail(w, "innloggingen kunne ikke fullføres", err)
		return
	}
	if time.Now().After(flow.Expires) {
		a.fail(w, "innloggingen tok for lang tid — prøv igjen", nil)
		return
	}
	// Constant-time compare: this is the CSRF defence for the callback.
	if !constantTimeEqual(flow.State, r.URL.Query().Get("state")) {
		a.fail(w, "innloggingen kunne ikke bekreftes", nil)
		return
	}
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		a.log.Info("identity provider declined the login", "error", errParam)
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	token, err := a.oauth.Exchange(r.Context(), r.URL.Query().Get("code"),
		oauth2.VerifierOption(flow.Verifier))
	if err != nil {
		a.fail(w, "innloggingen kunne ikke fullføres", err)
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		a.fail(w, "innloggingen kunne ikke fullføres", fmt.Errorf("no id_token in the response"))
		return
	}
	idToken, err := a.verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		a.fail(w, "innloggingen kunne ikke bekreftes", err)
		return
	}
	if idToken.Nonce != flow.Nonce {
		a.fail(w, "innloggingen kunne ikke bekreftes", fmt.Errorf("nonce mismatch"))
		return
	}

	user, err := a.userFromToken(idToken)
	if err != nil {
		a.fail(w, "kunne ikke lese brukerinformasjon", err)
		return
	}

	sess := NewSession(user, idToken.Expiry)
	if err := a.sealer.Write(w, sess); err != nil {
		a.fail(w, "kunne ikke opprette økt", err)
		return
	}

	http.Redirect(w, r, flow.ReturnTo, http.StatusFound)
}

// Logout clears the session.
//
// It does not end the session at FusionAuth. The previous site ran with
// idpLogout disabled, so signing out here has never signed you out of the
// identity provider, and changing that silently would be a surprise.
func (a *Authenticator) Logout(w http.ResponseWriter, r *http.Request) {
	a.sealer.Clear(w)
	http.Redirect(w, r, "/", http.StatusFound)
}

// claims is the shape we read out of the ID token.
//
// sub and email are standard. roles, fullName and imageUrl are not — they come
// from FusionAuth's ID token populate lambda. If that lambda is missing, roles
// arrives empty and nobody has Styret, which presents as a permissions bug
// rather than a configuration one; hence the diagnostic logging below.
type claims struct {
	Subject  string          `json:"sub"`
	Name     string          `json:"name"`
	Email    string          `json:"email"`
	FullName string          `json:"fullName"`
	ImageURL string          `json:"imageUrl"`
	Roles    json.RawMessage `json:"roles"`
}

func (a *Authenticator) userFromToken(idToken *oidc.IDToken) (User, error) {
	var c claims
	if err := idToken.Claims(&c); err != nil {
		return User{}, err
	}

	roles, err := parseRoles(c.Roles)
	if err != nil {
		return User{}, err
	}

	if len(roles) == 0 {
		// Worth a line in the log: an empty roles claim is indistinguishable
		// from "this member has no roles", and the two are debugged very
		// differently.
		a.log.Debug("ID token carries no roles claim",
			"sub", c.Subject,
			"hint", "if nobody has Styret, check the ID token populate lambda — see docs/auth.md")
	}

	return User{
		ID:       c.Subject,
		Name:     c.Name,
		FullName: c.FullName,
		Email:    c.Email,
		ImageURL: c.ImageURL,
		Roles:    roles,
	}, nil
}

// parseRoles accepts either a list or a bare string. FusionAuth emits a single
// role as a plain string in some lambda configurations, which a []string field
// would reject outright.
func parseRoles(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		if single == "" {
			return nil, nil
		}
		return []string{single}, nil
	}
	return nil, fmt.Errorf("roles claim is neither a string nor a list: %s", raw)
}

// safeReturnTo restricts post-login redirection to paths on this site.
//
// Without this the login URL is an open redirect: ?return_to=//evil.example
// is a protocol-relative URL that browsers follow off-site, and it arrives
// wearing our domain in the link the visitor clicked.
func safeReturnTo(raw string) string {
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "/"
	}
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" {
		return "/"
	}
	return u.RequestURI()
}

func (a *Authenticator) fail(w http.ResponseWriter, msg string, err error) {
	if err != nil {
		a.log.Error("login failed", "reason", msg, "err", err)
	}
	http.Error(w, msg, http.StatusBadRequest)
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
