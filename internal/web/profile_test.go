package web_test

import (
	"net/http"
	"testing"
)

func TestProfilRequiresLogin(t *testing.T) {
	mux := newSite(t, siteConfig{})

	rec := get(t, mux, "/profil", nil)
	if rec.Code != http.StatusFound {
		t.Fatalf("got %d, want a redirect to login", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !contains(loc, "return_to=%2Fprofil") {
		t.Errorf("redirected to %q; after logging in the visitor would not land back on the profile", loc)
	}
}

// With no FusionAuth token the page still renders, falling back to the logo
// for the avatar rather than a broken image.
func TestProfilAvatarFallsBackToLogo(t *testing.T) {
	mux := newSite(t, siteConfig{})

	rec := get(t, mux, "/profil", member)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	if !contains(rec.Body.String(), "/logo-512.png") {
		t.Error("no avatar fallback — a member without a picture gets a broken image")
	}
}

func TestProfilAvatarFromFusionAuth(t *testing.T) {
	fusion := fakeFusion(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/"+member.ID {
			t.Errorf("asked FusionAuth for %s, want the signed-in member's record", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"user":{"id":"` + member.ID + `","imageUrl":"https://cdn.example/pic.png"}}`))
	})
	mux := newSite(t, siteConfig{fusion: fusion})

	rec := get(t, mux, "/profil", member)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !contains(body, "https://cdn.example/pic.png") {
		t.Error("the stored avatar is not shown; a freshly linked picture would stay invisible until re-login")
	}
	if contains(body, "/logo-512.png") {
		t.Error("the logo fallback is rendered alongside a real avatar")
	}
}

// FusionAuth being down degrades the page, it does not take it down: the
// session already carries enough to render.
func TestProfilSurvivesFusionAuthFailure(t *testing.T) {
	fusion := fakeFusion(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux := newSite(t, siteConfig{fusion: fusion})

	rec := get(t, mux, "/profil", member)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 — the profile should warn-and-continue when FusionAuth fails", rec.Code)
	}
}

// The Discord link is read fresh from FusionAuth, and the refresh/unlink
// controls only appear when the integration could actually serve them.
func TestProfilShowsDiscordLink(t *testing.T) {
	fusion := fakeFusion(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"user":{"id":"` + member.ID + `",` +
			`"data":{"discord":{"id":"123","username":"kari","avatar":"","isMember":true}}}}`))
	})
	// discordSvc stays nil: the link is displayable, the controls are not.
	mux := newSite(t, siteConfig{fusion: fusion})

	rec := get(t, mux, "/profil", member)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !contains(body, "<code>kari</code>") {
		t.Error("the linked Discord username is not shown")
	}
	if contains(body, "/profil/discord/oppdater") {
		t.Error("the refresh control is rendered although the Discord integration is not configured; pressing it could only fail")
	}
}
