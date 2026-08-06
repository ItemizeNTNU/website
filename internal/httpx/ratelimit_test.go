package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterAllowsBurstThenRefuses(t *testing.T) {
	l := NewRateLimiter(3, time.Hour)

	for i := 1; i <= 3; i++ {
		if !l.Allow("1.2.3.4") {
			t.Fatalf("request %d was refused inside the burst", i)
		}
	}
	if l.Allow("1.2.3.4") {
		t.Error("the fourth request was allowed; the burst is not being enforced")
	}
	// One client's exhaustion must not affect anybody else.
	if !l.Allow("5.6.7.8") {
		t.Error("a different client was refused")
	}
}

func TestRateLimiterRefills(t *testing.T) {
	// A short window so the refill is observable.
	l := NewRateLimiter(2, 100*time.Millisecond)

	l.Allow("1.2.3.4")
	l.Allow("1.2.3.4")
	if l.Allow("1.2.3.4") {
		t.Fatal("the burst was not exhausted")
	}

	time.Sleep(120 * time.Millisecond)
	if !l.Allow("1.2.3.4") {
		t.Error("the bucket did not refill")
	}
}

// Reads must not be throttled. A university network shares one address, so
// limiting ordinary browsing would take the site down for a lecture hall.
func TestRateLimiterIgnoresSafeMethods(t *testing.T) {
	l := NewRateLimiter(1, time.Hour)
	h := l.Limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 20; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/registrer", nil)
		req.RemoteAddr = "1.2.3.4:1111"
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("a GET was throttled on request %d", i)
		}
	}
}

func TestLimitRejectsWithRetryAfter(t *testing.T) {
	l := NewRateLimiter(1, time.Hour)
	h := l.Limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	post := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/registrer", nil)
		req.RemoteAddr = "1.2.3.4:1111"
		h.ServeHTTP(rec, req)
		return rec
	}

	if got := post().Code; got != http.StatusOK {
		t.Fatalf("first POST got %d, want 200", got)
	}
	second := post()
	if second.Code != http.StatusTooManyRequests {
		t.Errorf("second POST got %d, want 429", second.Code)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Error("a 429 without Retry-After leaves the client guessing")
	}
}

// X-Forwarded-For is trivially forged. Honouring it would let one client
// present a fresh address per request and bypass the limiter completely —
// worse than having none, because it would appear to be working.
func TestClientIPIgnoresForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/registrer", nil)
	req.RemoteAddr = "10.0.0.1:5555"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-IP", "5.6.7.8")

	if got := clientIP(req); got != "10.0.0.1" {
		t.Errorf("clientIP = %q; a forgeable header is being trusted", got)
	}
}
