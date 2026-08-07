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

// Subtlety: an HTTP 500 with an empty body from FusionAuth still parses into
// an *APIError, so it takes the errors.As branch and renders as 422 with the
// UserMessage fallback — not the generic 500 page.
func TestRegistrationUpstream500RendersFallbackMessage(t *testing.T) {
	fusion := fakeFusion(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux := newSite(t, siteConfig{fusion: fusion})

	rec := postForm(t, mux, "/registrer", validRegistrationForm(), nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422 — a non-2xx response is an APIError whatever its status", rec.Code)
	}
	if !contains(rec.Body.String(), "Uventet svar fra innloggingstjenesten (HTTP 500).") {
		t.Error("the fallback message naming the upstream status is missing")
	}
}

// A transport failure — connection refused, DNS, timeout — is our problem,
// and renders the generic 500 branch.
func TestRegistrationTransportFailureIs500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	fusion := fusionauth.New(srv.URL, "test-api-key")
	srv.Close() // the address now refuses connections

	mux := newSite(t, siteConfig{fusion: fusion})
	rec := postForm(t, mux, "/registrer", validRegistrationForm(), nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500", rec.Code)
	}
	if !contains(rec.Body.String(), "Ups. Noe gikk galt :/") {
		t.Error("the visitor is not told something went wrong on our side")
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
