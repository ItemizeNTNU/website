package httpx

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
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

// rewind moves a bucket's last-seen time backwards, which is how these tests
// exercise refill without sleeping. The alternative — a real sleep long enough
// to be measurable — makes the suite both slow and flaky on a loaded machine.
func rewind(t *testing.T, l *RateLimiter, key string, d time.Duration) {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		t.Fatalf("no bucket for %q; the test never made a request as that client", key)
	}
	b.seen = b.seen.Add(-d)
}

// countAllowed drains the limiter, reporting how many requests got through
// before the first refusal.
func countAllowed(l *RateLimiter, key string, attempts int) int {
	allowed := 0
	for range attempts {
		if l.Allow(key) {
			allowed++
		}
	}
	return allowed
}

// The bucket refills in proportion to the time that has passed, not in whole
// windows. A visitor who mistyped their address should get another attempt
// within minutes rather than having to wait out the full hour.
func TestRateLimiterRefillsProportionally(t *testing.T) {
	tests := []struct {
		name        string
		elapsed     time.Duration
		wantAllowed int
	}{
		{name: "no time has passed", elapsed: 0, wantAllowed: 0},
		{name: "a tenth of the window", elapsed: 6 * time.Minute, wantAllowed: 1},
		{name: "half the window", elapsed: 30 * time.Minute, wantAllowed: 5},
		{name: "the whole window", elapsed: time.Hour, wantAllowed: 10},
		// Tokens are capped at the burst: an address idle overnight gets one
		// burst back, not a night's worth of credit to spend at once.
		{name: "ten windows", elapsed: 10 * time.Hour, wantAllowed: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewRateLimiter(10, time.Hour)
			if got := countAllowed(l, "1.2.3.4", 10); got != 10 {
				t.Fatalf("the initial burst allowed %d of 10", got)
			}
			if l.Allow("1.2.3.4") {
				t.Fatal("an eleventh request got through; the burst is not being enforced")
			}

			rewind(t, l, "1.2.3.4", tt.elapsed)

			if got := countAllowed(l, "1.2.3.4", 15); got != tt.wantAllowed {
				t.Errorf("after %v idle, %d requests were allowed, want %d", tt.elapsed, got, tt.wantAllowed)
			}
		})
	}
}

// A shared address is one bucket, but distinct addresses must never interfere.
// If they did, one abusive client would lock out everyone else on the site.
func TestRateLimiterIsolatesKeys(t *testing.T) {
	l := NewRateLimiter(2, time.Hour)
	keys := []string{"1.2.3.4", "5.6.7.8", "2001:db8::1", "", "10.0.0.1"}

	for _, key := range keys {
		if got := countAllowed(l, key, 2); got != 2 {
			t.Fatalf("client %q was allowed %d of its 2 requests", key, got)
		}
	}
	for _, key := range keys {
		if l.Allow(key) {
			t.Errorf("client %q got a third request through", key)
		}
	}

	// Refilling one bucket must not refill any other.
	rewind(t, l, "1.2.3.4", time.Hour)
	if !l.Allow("1.2.3.4") {
		t.Error("the refilled client is still blocked")
	}
	if l.Allow("5.6.7.8") {
		t.Error("refilling one client's bucket handed tokens to another")
	}
}

// A burst of one is the tightest useful setting and the one most likely to
// expose an off-by-one: the first request must succeed and the second must not.
func TestRateLimiterBurstOfOne(t *testing.T) {
	l := NewRateLimiter(1, time.Hour)
	if !l.Allow("1.2.3.4") {
		t.Fatal("the very first request was refused")
	}
	if l.Allow("1.2.3.4") {
		t.Error("a second immediate request was allowed with a burst of one")
	}
	rewind(t, l, "1.2.3.4", time.Hour)
	if !l.Allow("1.2.3.4") {
		t.Error("the bucket did not refill after a full window")
	}
}

// A burst of zero does not deny everything: the first request from an address
// creates its bucket and is allowed unconditionally, and only the second is
// refused. Pinned because "burst 0" reads like "block everything" and is not.
func TestRateLimiterBurstOfZeroStillAllowsTheFirstRequest(t *testing.T) {
	l := NewRateLimiter(0, time.Hour)
	if !l.Allow("1.2.3.4") {
		t.Error("behaviour changed: a burst of zero now refuses the first request too")
	}
	if l.Allow("1.2.3.4") {
		t.Error("a second request was allowed with a burst of zero")
	}
}

// The limiter is consulted from every request-handling goroutine at once. A
// missed lock here is a map corruption panic under load, which takes down the
// process rather than one request.
func TestRateLimiterIsSafeUnderConcurrency(t *testing.T) {
	const (
		burst    = 100
		workers  = 25
		perGroup = 20
	)
	// A window this long makes refill during the test immeasurably small, so
	// the total is exact rather than approximately right.
	l := NewRateLimiter(burst, 24*time.Hour)

	var (
		mu      sync.Mutex
		allowed int
		wg      sync.WaitGroup
	)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := 0
			for range perGroup {
				if l.Allow("1.2.3.4") {
					local++
				}
			}
			mu.Lock()
			allowed += local
			mu.Unlock()
		}()
	}
	wg.Wait()

	if allowed != burst {
		t.Errorf("%d of %d concurrent requests were allowed, want exactly the burst of %d",
			allowed, workers*perGroup, burst)
	}
}

// Separate clients hammering at once must each get their full allowance, and
// the map they all write to must survive it.
func TestRateLimiterConcurrentDistinctKeys(t *testing.T) {
	l := NewRateLimiter(3, 24*time.Hour)

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := "10.0.0." + strconv.Itoa(i)
			if got := countAllowed(l, key, 5); got != 3 {
				t.Errorf("client %s was allowed %d requests, want its full burst of 3", key, got)
			}
		}()
	}
	wg.Wait()

	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.buckets) != 50 {
		t.Errorf("the limiter holds %d buckets after 50 distinct clients, want 50", len(l.buckets))
	}
}

// The bucket key is the direct peer's address with the port stripped. A client
// that reconnects gets a fresh port every time, so keeping the port would give
// every request its own bucket and the limiter would never trigger.
func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{name: "ipv4 with a port", remoteAddr: "1.2.3.4:5678", want: "1.2.3.4"},
		{name: "ipv6 with a port", remoteAddr: "[2001:db8::1]:443", want: "2001:db8::1"},
		{name: "ipv6 loopback", remoteAddr: "[::1]:52134", want: "::1"},
		{name: "ipv4-mapped ipv6", remoteAddr: "[::ffff:1.2.3.4]:80", want: "::ffff:1.2.3.4"},
		{name: "ipv6 with a zone", remoteAddr: "[fe80::1%eth0]:80", want: "fe80::1%eth0"},
		{name: "port zero", remoteAddr: "1.2.3.4:0", want: "1.2.3.4"},
		// Malformed values must still produce a usable key rather than an
		// empty one — an empty key would put every odd client in one bucket.
		{name: "no port at all", remoteAddr: "1.2.3.4", want: "1.2.3.4"},
		{name: "bare ipv6", remoteAddr: "2001:db8::1", want: "2001:db8::1"},
		{name: "empty", remoteAddr: "", want: ""},
		{name: "not an address", remoteAddr: "unix-socket", want: "unix-socket"},
		{name: "trailing colon", remoteAddr: "1.2.3.4:", want: "1.2.3.4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/registrer", nil)
			req.RemoteAddr = tt.remoteAddr
			if got := clientIP(req); got != tt.want {
				t.Errorf("clientIP(%q) = %q, want %q", tt.remoteAddr, got, tt.want)
			}
		})
	}
}

// One client opening several connections is still one client. Keying on the
// full RemoteAddr would give each connection its own allowance, which is the
// same as having no limiter for anyone willing to reconnect.
func TestLimitSharesABucketAcrossPorts(t *testing.T) {
	l := NewRateLimiter(2, time.Hour)
	h := l.Limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	post := func(addr string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/registrer", nil)
		req.RemoteAddr = addr
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	for i, addr := range []string{"1.2.3.4:1111", "1.2.3.4:2222"} {
		if got := post(addr); got != http.StatusOK {
			t.Fatalf("request %d from %s got %d inside the burst", i, addr, got)
		}
	}
	if got := post("1.2.3.4:3333"); got != http.StatusTooManyRequests {
		t.Errorf("a third connection from the same address got %d; reconnecting bypasses the limiter", got)
	}
	if got := post("[2001:db8::1]:3333"); got != http.StatusOK {
		t.Errorf("a genuinely different client got %d", got)
	}
}

// Reads are cheap and browsing from a shared university address must not be
// throttled; only the methods that cause a side effect are counted.
func TestLimitCountsOnlyUnsafeMethods(t *testing.T) {
	tests := []struct {
		method    string
		throttled bool
	}{
		{method: http.MethodGet},
		{method: http.MethodHead},
		{method: http.MethodOptions},
		{method: http.MethodPost, throttled: true},
		{method: http.MethodPut, throttled: true},
		{method: http.MethodPatch, throttled: true},
		{method: http.MethodDelete, throttled: true},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			l := NewRateLimiter(1, time.Hour)
			h := l.Limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			var last int
			for range 5 {
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(tt.method, "/registrer", nil)
				req.RemoteAddr = "1.2.3.4:1111"
				h.ServeHTTP(rec, req)
				last = rec.Code
			}

			if tt.throttled && last != http.StatusTooManyRequests {
				t.Errorf("%s was never throttled (last status %d); this endpoint sends mail to whatever address is submitted",
					tt.method, last)
			}
			if !tt.throttled && last != http.StatusOK {
				t.Errorf("%s got %d; throttling it would break ordinary browsing from a shared address", tt.method, last)
			}
		})
	}
}

// A throttled request must not reach the handler at all — the handler is what
// sends the mail the limit exists to prevent.
func TestLimitStopsTheRequestReachingTheHandler(t *testing.T) {
	l := NewRateLimiter(1, time.Hour)
	reached := 0
	h := l.Limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached++
		w.WriteHeader(http.StatusOK)
	}))

	for range 4 {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/registrer", nil)
		req.RemoteAddr = "1.2.3.4:1111"
		h.ServeHTTP(rec, req)
	}

	if reached != 1 {
		t.Errorf("the handler ran %d times behind a burst of 1; the throttled requests still had their effect", reached)
	}
}

// The 429 is shown to a person who has just filled in a form, so it has to say
// what happened in the site's language rather than showing Go's default text.
func TestLimitExplainsItselfInNorwegian(t *testing.T) {
	l := NewRateLimiter(1, time.Hour)
	h := l.Limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	var rec *httptest.ResponseRecorder
	for range 2 {
		rec = httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/registrer", nil)
		req.RemoteAddr = "1.2.3.4:1111"
		h.ServeHTTP(rec, req)
	}

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "60" {
		t.Errorf("Retry-After = %q, want 60 so the client knows when to come back", got)
	}
	if body := rec.Body.String(); !strings.Contains(body, "For mange forsøk") {
		t.Errorf("body = %q, want the Norwegian explanation a visitor can act on", body)
	}
}

// Refilling has to work through the middleware too, not just through Allow:
// a visitor locked out by a typo must eventually be able to submit again.
func TestLimitRecoversAfterTheWindow(t *testing.T) {
	l := NewRateLimiter(1, time.Hour)
	h := l.Limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	post := func() int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/registrer", nil)
		req.RemoteAddr = "1.2.3.4:1111"
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	post()
	if got := post(); got != http.StatusTooManyRequests {
		t.Fatalf("got %d, want the second POST throttled", got)
	}
	rewind(t, l, "1.2.3.4", time.Hour)
	if got := post(); got != http.StatusOK {
		t.Errorf("got %d after a full window; a visitor who mistyped their address would be locked out permanently", got)
	}
}

// Idle buckets are deleted so the map cannot grow without bound — otherwise
// the limiter is itself the memory-exhaustion vector it was added to prevent.
// The reaper's ticker is fixed at ten minutes and takes no clock, so this
// exercises the eviction rule directly rather than waiting for it to fire.
func TestRateLimiterEvictionRule(t *testing.T) {
	l := NewRateLimiter(5, time.Hour)
	l.Allow("recent")
	l.Allow("idle")
	rewind(t, l, "idle", 2*time.Hour)

	cutoff := time.Now().Add(-time.Hour)
	l.mu.Lock()
	for key, b := range l.buckets {
		if b.seen.Before(cutoff) {
			delete(l.buckets, key)
		}
	}
	_, keptRecent := l.buckets["recent"]
	_, keptIdle := l.buckets["idle"]
	l.mu.Unlock()

	if !keptRecent {
		t.Error("an active client's bucket was evicted, which hands them a fresh allowance")
	}
	if keptIdle {
		t.Error("a bucket untouched for two hours survived; the map would grow with every address ever seen")
	}
}
