package httpx

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// RateLimiter allows a burst of requests per client, refilling over time.
//
// This exists for one endpoint in particular. Registration causes FusionAuth to
// send a password-setting email to whatever address was submitted, so an
// unthrottled form is a way to send mail to arbitrary people from our domain —
// and to fill the member directory with junk. Neither needs an attacker with
// any access; the form is public, which it has to be.
//
// It is per-process and in memory. That is honest about what it is: this runs
// as a single container, and a limiter that survives a restart or spans
// replicas would need shared state this site does not otherwise have. It
// raises the cost of abuse from free to inconvenient, which is the right
// trade for a volunteer-run site — it is not a defence against a distributed
// attacker, and it is not meant to be.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket

	burst  int
	window time.Duration
}

type bucket struct {
	tokens float64
	seen   time.Time
}

// NewRateLimiter allows burst requests, refilled over window.
func NewRateLimiter(burst int, window time.Duration) *RateLimiter {
	l := &RateLimiter{
		buckets: map[string]*bucket{},
		burst:   burst,
		window:  window,
	}
	go l.reap()
	return l
}

// Allow reports whether a request from key may proceed.
func (l *RateLimiter) Allow(key string) bool {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
		l.buckets[key] = &bucket{tokens: float64(l.burst) - 1, seen: now}
		return true
	}

	// Refill in proportion to the time that has passed.
	refill := now.Sub(b.seen).Seconds() / l.window.Seconds() * float64(l.burst)
	b.tokens = min(b.tokens+refill, float64(l.burst))
	b.seen = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// reap discards idle buckets so the map cannot grow without bound — otherwise
// the limiter itself becomes the memory-exhaustion vector it was added to
// prevent.
func (l *RateLimiter) reap() {
	for range time.Tick(10 * time.Minute) {
		cutoff := time.Now().Add(-time.Hour)

		l.mu.Lock()
		for key, b := range l.buckets {
			if b.seen.Before(cutoff) {
				delete(l.buckets, key)
			}
		}
		l.mu.Unlock()
	}
}

// Limit rejects requests from a client that has exhausted its allowance.
//
// Only unsafe methods are counted: reads are cheap and throttling them would
// break ordinary browsing from a shared address, which a university network
// very much is.
func (l *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}

		if !l.Allow(clientIP(r)) {
			w.Header().Set("Retry-After", "60")
			http.Error(w,
				"For mange forsøk. Vent litt og prøv igjen.",
				http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP identifies the caller.
//
// X-Forwarded-For is deliberately ignored. It is trivially forged, so trusting
// it would let one client present a fresh address per request and bypass the
// limiter entirely — worse than no limiter, because it would look like one was
// working. The direct peer is the reverse proxy, which means the whole site
// shares one bucket for these endpoints; with a burst sized for humans that is
// acceptable, and correct beats convenient here.
//
// If the deployment ever puts a proxy we control in front and we want
// per-visitor limits, the header can be trusted then — but only alongside a
// list of trusted proxy addresses, which is the part that makes it safe.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
