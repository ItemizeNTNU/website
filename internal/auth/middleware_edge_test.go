package auth

// Edge cases for context injection, the role gates, and the User predicates
// the gates are built on.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// injector builds the smallest Authenticator that Inject needs. Inject reads
// nothing but the sealer, so a full one — which would require reaching an
// identity provider — is not warranted here.
func injector(sealer *Sealer) *Authenticator { return &Authenticator{sealer: sealer} }

// seeing records the user the wrapped handler was given, so a test can assert
// on what reached the handler rather than only on the status code.
func seeing(got **User, ran *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = FromRequest(r)
		*ran = true
		w.WriteHeader(http.StatusOK)
	})
}

// ── Inject ────────────────────────────────────────────────────────────────

// Inject must never fail a request. Whatever the cookie jar contains, the worst
// outcome is an anonymous visitor — a broken cookie turning into a 500 would
// lock everybody out of the whole site until the secret was rolled back.
func TestInjectNeverFailsARequest(t *testing.T) {
	sealer := newTestSealer(t, testSecret, true)
	other := newTestSealer(t, "ffffffffffffffffffffffffffffffff", true)

	live, err := sealer.Seal(NewSession(User{ID: "fa-1", Name: "Kari",
		Roles: []string{RoleStyret}}, time.Now().Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	expired, err := sealer.Seal(&Session{User: User{ID: "fa-1"},
		Expires: time.Now().Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := other.Seal(NewSession(User{ID: "attacker", Roles: []string{RoleStyret}},
		time.Now().Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		value    *string
		wantUser bool
		why      string
	}{
		{"no cookie", nil, false, "an ordinary first-time visitor is anonymous"},
		{"a valid session", &live, true, "the normal signed-in case"},
		{"an empty cookie", ptr(""), false, "an emptied cookie is not a session"},
		{"garbage", ptr("!!!!"), false, "a corrupted cookie must not fail the request"},
		{"a truncated cookie", ptr(live[:len(live)/2]), false,
			"a cookie clipped in transit must not decrypt into a partial user"},
		{"a session sealed under a rotated secret", &foreign, false,
			"after a secret rotation every old cookie must read as anonymous, and a " +
				"cookie sealed by anyone else must never grant a role"},
		{"an expired session", &expired, false,
			"expiry must be enforced here, not only when the cookie was written"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.value != nil {
				r.AddCookie(&http.Cookie{Name: SessionCookie, Value: *tt.value})
			}

			var got *User
			var ran bool
			rec := httptest.NewRecorder()
			injector(sealer).Inject(seeing(&got, &ran)).ServeHTTP(rec, r)

			if !ran {
				t.Fatalf("Inject swallowed the request instead of passing it on (%s)", tt.why)
			}
			if rec.Code != http.StatusOK {
				t.Errorf("status %d; Inject must never fail a request (%s)", rec.Code, tt.why)
			}
			if tt.wantUser && got == nil {
				t.Errorf("no user reached the handler: %s", tt.why)
			}
			if !tt.wantUser && got != nil {
				t.Errorf("the handler was handed %+v: %s", *got, tt.why)
			}
		})
	}
}

func ptr(s string) *string { return &s }

// Inject copies the whole user into the context, not just an identifier — every
// page renders the display name and avatar straight out of it.
func TestInjectCarriesTheWholeUser(t *testing.T) {
	sealer := newTestSealer(t, testSecret, true)

	want := User{
		ID: "fa-1", Name: "Bjørn", FullName: "Bjørn Ærlig Ødegård",
		Email: "bjorn@example.no", ImageURL: "https://itemize.no/b.png",
		Roles: []string{"Medlem", RoleStyret},
	}
	sealed, err := sealer.Seal(NewSession(want, time.Now().Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookie, Value: sealed})

	var got *User
	var ran bool
	injector(sealer).Inject(seeing(&got, &ran)).ServeHTTP(httptest.NewRecorder(), r)

	if got == nil {
		t.Fatal("no user in the context")
	}
	if got.ID != want.ID || got.Name != want.Name || got.FullName != want.FullName ||
		got.Email != want.Email || got.ImageURL != want.ImageURL {
		t.Errorf("got %+v, want %+v", *got, want)
	}
	if !got.IsStyret() {
		t.Error("roles did not survive injection, so a board member would see no admin links")
	}
}

// The doc comment on Inject says an unopenable cookie "is cleared". It is not:
// Read returns nil and the request goes on with the cookie still in the jar,
// which is re-sent on every subsequent request. That is not a security problem,
// but it is a discrepancy between the comment and the code — pinned here so
// whichever of the two changes, this test says which one moved.
func TestInjectDoesNotActuallyClearABadCookie(t *testing.T) {
	sealer := newTestSealer(t, testSecret, true)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookie, Value: "!!!!"})

	rec := httptest.NewRecorder()
	var got *User
	var ran bool
	injector(sealer).Inject(seeing(&got, &ran)).ServeHTTP(rec, r)

	if len(rec.Result().Cookies()) != 0 {
		t.Skip("Inject now clears the bad cookie, which is what its doc comment claims; " +
			"delete this test and the note about the discrepancy")
	}
}

// ── Nil users ─────────────────────────────────────────────────────────────

// A typed nil *User in the context must read as anonymous, not as a signed-in
// visitor with no roles. The difference is a redirect to login versus a 403 —
// and, if HasRole did not guard against nil, a panic.
func TestATypedNilUserIsAnonymous(t *testing.T) {
	deny := func(w http.ResponseWriter, _ *http.Request, status int) { w.WriteHeader(status) }

	handlers := map[string]struct {
		h    http.Handler
		want int
	}{
		"RequireLogin":    {RequireLogin(okHandler()), http.StatusFound},
		"RequireRole":     {RequireRole(RoleStyret, deny)(okHandler()), http.StatusFound},
		"RequireLoginAPI": {RequireLoginAPI(okHandler()), http.StatusUnauthorized},
		"RequireRoleAPI":  {RequireRoleAPI(RoleStyret)(okHandler()), http.StatusUnauthorized},
	}

	for name, tt := range handlers {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/skjult", nil)
			r = r.WithContext(WithUser(r.Context(), (*User)(nil)))

			rec := httptest.NewRecorder()
			tt.h.ServeHTTP(rec, r)
			if rec.Code != tt.want {
				t.Errorf("got %d, want %d; a nil user in the context must be treated as "+
					"anonymous rather than as a signed-in member", rec.Code, tt.want)
			}
		})
	}
}

// ── Role names ────────────────────────────────────────────────────────────

// Role comparison is exact. FusionAuth role names are free text set in an admin
// interface, so "styret" and "Styret" are different roles and only the second
// one grants access. Anything looser here would let a differently-cased or
// similarly-named role inherit board privileges.
func TestRoleMatchingIsExact(t *testing.T) {
	deny := func(w http.ResponseWriter, _ *http.Request, status int) { w.WriteHeader(status) }

	tests := []struct {
		name  string
		roles []string
		want  int
	}{
		{"the role itself", []string{RoleStyret}, http.StatusOK},
		{"among several roles", []string{"Medlem", "Infra", RoleStyret}, http.StatusOK},
		{"the same role twice", []string{RoleStyret, RoleStyret}, http.StatusOK},
		{"lower case", []string{"styret"}, http.StatusForbidden},
		{"upper case", []string{"STYRET"}, http.StatusForbidden},
		{"trailing space", []string{"Styret "}, http.StatusForbidden},
		{"leading space", []string{" Styret"}, http.StatusForbidden},
		{"a prefix", []string{"Sty"}, http.StatusForbidden},
		{"a superstring", []string{"Styretmedlem"}, http.StatusForbidden},
		{"a similar Norwegian word", []string{"Styrer"}, http.StatusForbidden},
		{"no roles at all", nil, http.StatusForbidden},
		{"an empty role name", []string{""}, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/arrangementer/ny", nil)
			r = r.WithContext(WithUser(r.Context(), &User{ID: "fa-1", Roles: tt.roles}))

			rec := httptest.NewRecorder()
			RequireRole(RoleStyret, deny)(okHandler()).ServeHTTP(rec, r)

			if rec.Code != tt.want {
				t.Errorf("a member holding %v got %d, want %d — role names must match "+
					"character for character", tt.roles, rec.Code, tt.want)
			}
		})
	}
}

// A gate on a role nobody holds must refuse everyone rather than fall open.
func TestGatingOnAnUnknownRoleRefusesEveryone(t *testing.T) {
	deny := func(w http.ResponseWriter, _ *http.Request, status int) { w.WriteHeader(status) }

	for _, role := range []string{"Admin", "Kasserer", "", "Styret\n"} {
		t.Run("role "+role, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/skjult", nil)
			r = r.WithContext(WithUser(r.Context(),
				&User{ID: "fa-1", Roles: []string{"Medlem", RoleStyret}}))

			rec := httptest.NewRecorder()
			RequireRole(role, deny)(okHandler()).ServeHTTP(rec, r)
			if rec.Code != http.StatusForbidden {
				t.Errorf("a gate on the unheld role %q returned %d rather than 403", role, rec.Code)
			}
		})
	}
}

// ── Redirect construction ─────────────────────────────────────────────────

// The login redirect has to carry the visitor back to where they were, and the
// return path has to survive the trip intact — including a query string, which
// is what "?old=1" on the events page relies on. It must also come back out of
// safeReturnTo unchanged, or the visitor lands on the front page instead.
func TestLoginRedirectPreservesTheRequestedPath(t *testing.T) {
	deny := func(w http.ResponseWriter, _ *http.Request, status int) { w.WriteHeader(status) }

	paths := []string{
		"/profil",
		"/arrangementer?old=1",
		"/arrangementer?q=pizza&old=1",
		"/arrangementer/68f0b3c1a2b3c4d5e6f70819/rediger",
		"/s%C3%B8k?q=%C3%A6%C3%B8%C3%A5",
	}

	for _, name := range []string{"RequireLogin", "RequireRole"} {
		for _, path := range paths {
			t.Run(name+" "+path, func(t *testing.T) {
				var h http.Handler = RequireLogin(okHandler())
				if name == "RequireRole" {
					h = RequireRole(RoleStyret, deny)(okHandler())
				}

				r := httptest.NewRequest(http.MethodGet, path, nil)
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, r)

				if rec.Code != http.StatusFound {
					t.Fatalf("got %d, want 302", rec.Code)
				}
				location, err := url.Parse(rec.Header().Get("Location"))
				if err != nil {
					t.Fatalf("the Location header is not a URL: %v", err)
				}
				if location.Path != "/login" {
					t.Errorf("redirected to %q rather than /login", location.Path)
				}
				if location.Host != "" || location.Scheme != "" {
					t.Errorf("the login redirect points off-site, to %q", location)
				}

				returnTo := location.Query().Get("return_to")
				if returnTo != r.URL.RequestURI() {
					t.Errorf("return_to = %q, want %q; the visitor would not land back "+
						"where they were", returnTo, r.URL.RequestURI())
				}
				// The round trip that matters: what the redirect puts in the URL
				// must be what safeReturnTo hands back after login.
				if got := safeReturnTo(returnTo); got != r.URL.RequestURI() {
					t.Errorf("safeReturnTo(%q) = %q; the escaping done here and the "+
						"validation done at callback time disagree", returnTo, got)
				}
			})
		}
	}
}

// The query string is escaped into return_to rather than concatenated raw, so a
// visitor cannot smuggle extra parameters into the login URL.
func TestLoginRedirectEscapesTheReturnPath(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x?a=1&return_to=%2F%2Fevil.example", nil)

	rec := httptest.NewRecorder()
	RequireLogin(okHandler()).ServeHTTP(rec, r)

	location, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if got := location.Query()["return_to"]; len(got) != 1 {
		t.Fatalf("the login URL carries %d return_to parameters (%v); a second one injected "+
			"through the original query string could win", len(got), got)
	}
	if got := safeReturnTo(location.Query().Get("return_to")); strings.HasPrefix(got, "//") {
		t.Errorf("the round trip produced %q, a protocol-relative URL browsers follow "+
			"off-site", got)
	}
}

// ── The JSON error contract ───────────────────────────────────────────────

// The previous API used 401 for a missing role, and its clients branch on the
// message text. Both are load-bearing compatibility, not style: changing either
// breaks callers silently.
func TestAPIErrorBodies(t *testing.T) {
	tests := []struct {
		name    string
		handler http.Handler
		user    *User
		status  int
		message string
	}{
		{"not logged in", RequireLoginAPI(okHandler()), nil,
			http.StatusUnauthorized, "You are not logged in"},
		{"logged in without the role", RequireRoleAPI(RoleStyret)(okHandler()),
			&User{ID: "fa-1", Roles: []string{"Medlem"}},
			http.StatusUnauthorized, "Permission denied"},
		{"anonymous at a role-gated endpoint", RequireRoleAPI(RoleStyret)(okHandler()), nil,
			http.StatusUnauthorized, "You are not logged in"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/events", nil)
			if tt.user != nil {
				r = r.WithContext(WithUser(r.Context(), tt.user))
			}
			rec := httptest.NewRecorder()
			tt.handler.ServeHTTP(rec, r)

			if rec.Code != tt.status {
				t.Errorf("status %d, want %d; API clients branch on this", rec.Code, tt.status)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type %q; a client parsing this as JSON would fail", ct)
			}

			var body struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("the error body %q is not valid JSON: %v", rec.Body.String(), err)
			}
			if body.Message != tt.message {
				t.Errorf("message %q, want %q; clients key off this text", body.Message, tt.message)
			}
		})
	}
}

// quoteJSON is hand-rolled to keep this package free of a dependency on the api
// package. It must agree with encoding/json for everything it is actually given.
func TestQuoteJSONMatchesTheStandardLibrary(t *testing.T) {
	for _, s := range []string{
		"You are not logged in",
		"Permission denied",
		``,
		`a "quoted" word`,
		`a\backslash`,
		"a\nnewline",
		"æøå ÆØÅ",
		"emoji 🎉",
		`{"nested":"json"}`,
	} {
		t.Run(s, func(t *testing.T) {
			want, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if got := quoteJSON(s); got != string(want) {
				t.Errorf("quoteJSON(%q) = %s, want %s", s, got, want)
			}
		})
	}
}

// quoteJSON escapes only ", \ and \n. Any other control character below 0x20
// goes out raw and makes the response invalid JSON. That is unreachable today —
// every caller passes one of two literals — but it is a trap for the next
// person who routes a database or provider error message through it.
func TestQuoteJSONLeavesOtherControlCharactersRaw(t *testing.T) {
	for _, s := range []string{"a\tb", "a\rb"} {
		if json.Valid([]byte(quoteJSON(s))) {
			t.Skipf("quoteJSON now escapes %q correctly; delete this test and the warning "+
				"it carries", s)
		}
	}
	t.Log("confirmed: quoteJSON must only ever be called with literal messages, never " +
		"with text derived from an error or from user input")
}

// ── Context plumbing ──────────────────────────────────────────────────────

func TestUserContextPlumbing(t *testing.T) {
	t.Run("an empty context has no user", func(t *testing.T) {
		if got := FromContext(context.Background()); got != nil {
			t.Errorf("got %+v from a bare context", got)
		}
	})

	t.Run("a request with no user is anonymous", func(t *testing.T) {
		if got := FromRequest(httptest.NewRequest(http.MethodGet, "/", nil)); got != nil {
			t.Errorf("got %+v from a plain request", got)
		}
	})

	t.Run("the user survives a round trip", func(t *testing.T) {
		want := &User{ID: "fa-1", Name: "Kari"}
		if got := FromContext(WithUser(context.Background(), want)); got != want {
			t.Errorf("got %v, want the same pointer back", got)
		}
	})

	t.Run("the key is unexported and cannot collide", func(t *testing.T) {
		// A context key of a named type declared in this package cannot be
		// produced by another package, so nothing outside auth can plant a user.
		// Planting an int 0 — the underlying value of userKey — must not work.
		ctx := context.WithValue(context.Background(), 0, &User{ID: "attacker", Roles: []string{RoleStyret}}) //nolint:staticcheck
		if got := FromContext(ctx); got != nil {
			t.Errorf("a value stored under a bare int key was read back as %+v; anything "+
				"in the process could then forge a signed-in board member", got)
		}
	})

	t.Run("a later WithUser wins", func(t *testing.T) {
		first := &User{ID: "fa-1"}
		second := &User{ID: "fa-2"}
		ctx := WithUser(WithUser(context.Background(), first), second)
		if got := FromContext(ctx); got != second {
			t.Errorf("got %+v, want the most recently injected user", got)
		}
	})
}

// ── User predicates ───────────────────────────────────────────────────────

func TestHasRoleTable(t *testing.T) {
	tests := []struct {
		name string
		user *User
		role string
		want bool
		why  string
	}{
		{"nil user", nil, RoleStyret, false,
			"callers rely on this so they do not need a separate nil check"},
		{"nil user, empty role", nil, "", false, "a nil user holds nothing at all"},
		{"no roles", &User{}, RoleStyret, false, "a member with no roles holds none"},
		{"empty role list", &User{Roles: []string{}}, RoleStyret, false, "same as nil"},
		{"holds it", &User{Roles: []string{RoleStyret}}, RoleStyret, true, "the ordinary case"},
		{"holds it last", &User{Roles: []string{"a", "b", RoleStyret}}, RoleStyret, true,
			"the whole list must be searched"},
		{"asked for the empty role", &User{Roles: []string{"Medlem"}}, "", false,
			"an empty role name must not match a real one"},
		{"holds the empty role", &User{Roles: []string{""}}, "", true,
			"an empty entry matches an empty query; RequireRole(\"\") would therefore let " +
				"such a member through, which is why no gate uses an empty role name"},
		{"unicode role", &User{Roles: []string{"Økonomi"}}, "Økonomi", true,
			"Norwegian role names must work"},
		{"unicode role, different form", &User{Roles: []string{"Økonomi"}}, "Okonomi", false,
			"comparison is byte-for-byte, not transliterating"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.user.HasRole(tt.role); got != tt.want {
				t.Errorf("HasRole(%q) = %v, want %v — %s", tt.role, got, tt.want, tt.why)
			}
		})
	}
}

func TestIsStyretIsHasRoleStyret(t *testing.T) {
	for _, u := range []*User{
		nil,
		{},
		{Roles: []string{RoleStyret}},
		{Roles: []string{"Medlem"}},
		{Roles: []string{"styret"}},
	} {
		if u.IsStyret() != u.HasRole(RoleStyret) {
			t.Errorf("IsStyret and HasRole(%q) disagree for %+v", RoleStyret, u)
		}
	}
	if RoleStyret != "Styret" {
		t.Errorf("RoleStyret is %q; it must match the role name configured in FusionAuth "+
			"exactly, or the board loses access to event administration", RoleStyret)
	}
}

// DisplayName falls back through the available fields so the interface never
// renders an empty name where a person should be. FusionAuth supplies "name"
// only when the lambda populates it, so the fallbacks are the normal path for
// some members, not an edge case.
func TestDisplayNameFallback(t *testing.T) {
	tests := []struct {
		name string
		user *User
		want string
	}{
		{"nil user", nil, ""},
		{"name present", &User{Name: "Kari", FullName: "Kari Nordmann",
			Email: "kari@example.no"}, "Kari"},
		{"no name, full name present", &User{FullName: "Kari Nordmann",
			Email: "kari@example.no"}, "Kari Nordmann"},
		{"only an email", &User{Email: "kari@example.no"}, "kari@example.no"},
		{"nothing at all", &User{}, ""},
		{"only an ID", &User{ID: "fa-1"}, ""},
		{"unicode name", &User{Name: "Bjørn Ærlig Ødegård"}, "Bjørn Ærlig Ødegård"},
		{"a name that is only whitespace", &User{Name: " ", FullName: "Kari Nordmann"}, " "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.user.DisplayName(); got != tt.want {
				t.Errorf("DisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}
