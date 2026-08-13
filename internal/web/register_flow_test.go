package web_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/ItemizeNTNU/website/internal/fusionauth"
)

// validRegistrationForm mirrors base() in internal/users/register_test.go,
// filled out as a student. The finish year is derived from the clock because
// the handler validates it against time.Now.
func validRegistrationForm() url.Values {
	return url.Values{
		"fullName":                 {"Kari Nordmann"},
		"email":                    {"kari@example.no"},
		"displayName":              {"kari"},
		"type":                     {"student"},
		"study.program":            {"Kommunikasjonsteknologi"},
		"study.year":               {"3"},
		"study.expectedFinishYear": {strconv.Itoa(time.Now().Year() + 2)},
	}
}

// untouchedFusion fails the test if the handler ever reaches FusionAuth —
// used where the request must be resolved locally.
func untouchedFusion(t *testing.T) *fusionauth.Client {
	t.Helper()
	return fakeFusion(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the handler contacted FusionAuth (%s %s) when it should not have", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	})
}

// Someone already signed in has nothing to do on the signup pages.
func TestRegistrationPagesRedirectTheSignedIn(t *testing.T) {
	for _, path := range []string{"/registrer", "/registrert"} {
		t.Run(path, func(t *testing.T) {
			mux := newSite(t, siteConfig{})
			rec := get(t, mux, path, member)
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("got %d, want 303", rec.Code)
			}
			if got := rec.Header().Get("Location"); got != "/profil" {
				t.Errorf("redirected to %q, want /profil", got)
			}
		})
	}
}

// Without an API token the form must say so on submission rather than
// swallowing the registration.
func TestRegistrationUnavailableWithoutFusion(t *testing.T) {
	mux := newSite(t, siteConfig{}) // default fusion client is unconfigured

	rec := postForm(t, mux, "/registrer", validRegistrationForm(), nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503 — accepting the form would silently lose a signup", rec.Code)
	}
	if len(rec.Body.String()) == 0 {
		t.Error("the 503 has no body, so the visitor sees a blank page instead of an explanation")
	}
}

// Validation failures are resolved locally: FusionAuth is never contacted,
// the typed values come back, and the errors are in Norwegian.
func TestRegistrationValidationEchoesTheForm(t *testing.T) {
	mux := newSite(t, siteConfig{fusion: untouchedFusion(t)})

	form := validRegistrationForm()
	form.Set("fullName", "K") // too short

	rec := postForm(t, mux, "/registrer", form, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`value="kari@example.no"`,
		`value="Kommunikasjonsteknologi"`,
		"Fullt navn må være minst 3 tegn.",
	} {
		if !contains(body, want) {
			t.Errorf("the re-rendered form is missing %q — the visitor retypes everything or cannot tell what was wrong", want)
		}
	}
}

// FusionAuth's own message is the one worth showing: "email already in use"
// is something the person can act on.
func TestRegistrationSurfacesDuplicateEmail(t *testing.T) {
	fusion := fakeFusion(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"fieldErrors":{"user.email":[{"code":"[duplicate]","message":"Email already in use"}]}}`))
	})
	mux := newSite(t, siteConfig{fusion: fusion})

	rec := postForm(t, mux, "/registrer", validRegistrationForm(), nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", rec.Code)
	}
	if !contains(rec.Body.String(), "Email already in use") {
		t.Error("FusionAuth's message was not surfaced, so the visitor cannot tell the address is taken")
	}
}

// wantUpstreamDown is what the form says when the failure is ours rather than
// the visitor's: no service address, no status code, and advice worth
// following — what they typed was fine. Word for word the message the JSON
// path returns (internal/api/users.go), so one outage reads the same way
// whichever entry point a member came through.
const wantUpstreamDown = "Innloggingstjenesten svarer ikke akkurat nå. Prøv igjen om litt."

// Who is at fault decides both the status and the wording, and FusionAuth's
// error parser makes that easy to get wrong: it wraps every non-2xx reply, a
// 5xx included, in the same *APIError the handler reads validation messages
// out of. Matching on the type alone told a visitor whose registration was
// perfectly valid that it was not — a 422 on a form with nothing wrong with
// it, sometimes quoting an HTTP status at them — and hid the outage from
// anything watching for 5xx.
//
// Each case gets its own mux: five POSTs to /registrer per Server is the whole
// rate-limit allowance (webtest_test.go).
func TestRegistrationSeparatesOurFailuresFromTheVisitors(t *testing.T) {
	const duplicate = `{"fieldErrors":{"user.email":[{"code":"[duplicate]","message":"E-posten er allerede i bruk."}]}}`

	for _, tc := range []struct {
		name       string
		status     int // what FusionAuth answers
		body       string
		wantStatus int
		wantMsg    string
		wantEcho   bool // the typed values must come back to be corrected
	}{
		{
			name:       "a rejection the visitor can act on",
			status:     http.StatusBadRequest,
			body:       duplicate,
			wantStatus: http.StatusUnprocessableEntity,
			wantMsg:    "E-posten er allerede i bruk.",
			wantEcho:   true,
		},
		{
			name:       "a conflict is still the visitor's to resolve",
			status:     http.StatusConflict,
			body:       duplicate,
			wantStatus: http.StatusUnprocessableEntity,
			wantMsg:    "E-posten er allerede i bruk.",
			wantEcho:   true,
		},
		{
			// Before the fix this rendered as a 422 carrying "Uventet svar fra
			// innloggingstjenesten (HTTP 500)." — a validation error that
			// quotes an HTTP status at somebody who typed nothing wrong.
			name:       "the directory is broken",
			status:     http.StatusInternalServerError,
			body:       "",
			wantStatus: http.StatusInternalServerError,
			wantMsg:    wantUpstreamDown,
		},
		{
			name:       "the directory is restarting",
			status:     http.StatusServiceUnavailable,
			body:       "",
			wantStatus: http.StatusInternalServerError,
			wantMsg:    wantUpstreamDown,
		},
		{
			name:       "something in front of the directory is broken",
			status:     http.StatusBadGateway,
			body:       "",
			wantStatus: http.StatusInternalServerError,
			wantMsg:    wantUpstreamDown,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fusion := fakeFusion(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			mux := newSite(t, siteConfig{fusion: fusion})

			rec := postForm(t, mux, "/registrer", validRegistrationForm(), nil)

			if rec.Code != tc.wantStatus {
				t.Fatalf("FusionAuth answered %d and the page came back %d, want %d",
					tc.status, rec.Code, tc.wantStatus)
			}
			body := rec.Body.String()
			if !contains(body, tc.wantMsg) {
				t.Errorf("FusionAuth answered %d and the visitor is not told %q",
					tc.status, tc.wantMsg)
			}
			if !tc.wantEcho {
				// Nothing they typed was at fault, so nothing may suggest it was.
				if contains(body, "Uventet svar fra innloggingstjenesten") {
					t.Error("the outage is reported as an upstream HTTP status, which the visitor can do nothing with")
				}
				return
			}
			if !contains(body, `value="kari@example.no"`) {
				t.Error("the rejected form came back empty, so the visitor retypes everything to fix one field")
			}
		})
	}
}

// A transport failure — connection refused, DNS, timeout — is upstream being
// unreachable rather than anything the visitor did, so it is reported the same
// way as a 5xx from FusionAuth: a 500, and an invitation to try again.
func TestRegistrationTransportFailureIs500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	fusion := fusionauth.New(srv.URL, "test-api-key")
	srv.Close() // the address now refuses connections

	mux := newSite(t, siteConfig{fusion: fusion})
	rec := postForm(t, mux, "/registrer", validRegistrationForm(), nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500", rec.Code)
	}
	if !contains(rec.Body.String(), wantUpstreamDown) {
		t.Error("the visitor is not told the fault is ours and that retrying is worth it")
	}
}

func TestRegistrationSuccessRedirects(t *testing.T) {
	fusion := fakeFusion(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"user":{"id":"33333333-4444-4555-8666-777777777777"}}`))
	})
	mux := newSite(t, siteConfig{fusion: fusion})

	rec := postForm(t, mux, "/registrer", validRegistrationForm(), nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got %d, want 303 — rendering directly would re-register on refresh", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/registrert" {
		t.Errorf("redirected to %q, want /registrert", got)
	}
}

// With Discord configured, a successful registration goes straight into the
// linking flow instead of stopping at a button — every new member should end
// up with a linked Discord account, and every exit from that flow lands back
// on /registrert, so the detour can never cost the registration. The redirect
// carries a sealed cookie because registration itself creates no session; the
// cookie is the only thing that can vouch for the new member until the
// set-password email is acted on.
func TestRegistrationSuccessSetsDiscordOfferCookie(t *testing.T) {
	const createdID = "33333333-4444-4555-8666-777777777777"
	fusion := fakeFusion(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"user":{"id":"` + createdID + `"}}`))
	})
	mux := newSite(t, siteConfig{fusion: fusion, discordSvc: &fakeDiscordLinker{available: true}})

	rec := postForm(t, mux, "/registrer", validRegistrationForm(), nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got %d, want 303", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/registrer/discord" {
		t.Errorf("redirected to %q, want /registrer/discord — the new member should be sent straight into the linking flow", got)
	}

	c := cookieNamed(rec, "itemize_registrering")
	if c == nil {
		t.Fatal("no registration cookie was set, so /registrer/discord would bounce the new member straight back out")
	}
	if !c.HttpOnly {
		t.Error("the registration cookie is readable by script")
	}
	if c.Path != "/" {
		t.Errorf("registration cookie Path = %q, want / so both /registrert and the /api callback see it", c.Path)
	}
	if c.MaxAge != 1800 {
		t.Errorf("registration cookie MaxAge = %d, want 1800 — long enough to create a Discord account, short enough to bound the capability", c.MaxAge)
	}

	// The cookie must open to the created user, not to whatever was typed in
	// the form — the sealed id is what the callback will link Discord to.
	var reg struct {
		Purpose string    `json:"p"`
		UserID  string    `json:"u"`
		Expires time.Time `json:"exp"`
	}
	if err := testSealer.Open(c.Value, &reg); err != nil {
		t.Fatalf("the registration cookie does not open with the server's sealer: %v", err)
	}
	if reg.Purpose != "register-discord" {
		t.Errorf("sealed purpose = %q, want register-discord — anything else is replayable as some other cookie", reg.Purpose)
	}
	if reg.UserID != createdID {
		t.Errorf("sealed user id = %q, want the created user %q", reg.UserID, createdID)
	}
	if !reg.Expires.After(time.Now()) {
		t.Error("the sealed value is already expired on arrival")
	}
}

// With Discord unconfigured, a successful registration is byte-for-byte the
// old behaviour: a bare redirect, no cookie hinting at a flow that could only
// end in an error flash.
func TestRegistrationWithoutDiscordSetsNoOfferCookie(t *testing.T) {
	fusion := fakeFusion(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"user":{"id":"33333333-4444-4555-8666-777777777777"}}`))
	})
	mux := newSite(t, siteConfig{fusion: fusion}) // no discordSvc

	rec := postForm(t, mux, "/registrer", validRegistrationForm(), nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got %d, want 303", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/registrert" {
		t.Errorf("redirected to %q, want /registrert", got)
	}
	if c := cookieNamed(rec, "itemize_registrering"); c != nil {
		t.Errorf("a registration cookie %q was set with the integration off", c.Value)
	}
}

// A signed-in visitor's POST bounces to the profile before FusionAuth is ever
// contacted — no duplicate account, no email.
func TestRegistrationSignedInPostBypassesFusion(t *testing.T) {
	mux := newSite(t, siteConfig{fusion: untouchedFusion(t)})

	rec := postForm(t, mux, "/registrer", validRegistrationForm(), member)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got %d, want 303", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/profil" {
		t.Errorf("redirected to %q, want /profil", got)
	}
}

// Submitting the form makes FusionAuth send an email, which is why the route
// is rate limited. The limiter wraps the CSRF check (server.go), so even
// token-less junk counts — and httptest requests share one RemoteAddr, so all
// six POSTs land in the same bucket.
//
// This test gets its own mux: the limiter is per-Server, and these six POSTs
// spend the entire allowance.
func TestRegistrationRateLimit(t *testing.T) {
	mux := newSite(t, siteConfig{})

	for i := 1; i <= 6; i++ {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/registrer", nil))

		if i <= 5 {
			if rec.Code == http.StatusTooManyRequests {
				t.Fatalf("request %d was rate limited; a person correcting a rejected form would be locked out", i)
			}
			continue
		}
		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("request %d got %d, want 429 — a script can trigger unlimited signup emails", i, rec.Code)
		}
		if got := rec.Header().Get("Retry-After"); got != "60" {
			t.Errorf("Retry-After = %q, want %q", got, "60")
		}
	}
}
