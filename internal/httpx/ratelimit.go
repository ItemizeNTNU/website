package httpx

import (
	"fmt"
	"math"
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

	stopOnce sync.Once
	stop     chan struct{}
}

type bucket struct {
	tokens float64
	seen   time.Time
}

const (
	// How often idle buckets are swept, and how long a bucket has to go
	// untouched before it is swept.
	reapInterval = 10 * time.Minute
	idleTTL      = time.Hour
)

// NewRateLimiter allows burst requests, refilled over window.
//
// It panics if either is not positive. Both call sites pass constants, so a
// bad value is a programmer error that a panic surfaces at startup — before
// the process serves anything — rather than a misconfiguration arriving from
// the outside world that we should tolerate at runtime. Silently substituting
// a default would be worse than either: a window of zero used to divide by
// zero in Allow and store a NaN in the bucket, after which every comparison
// against it was false and that client was never refused again. A limiter that
// looks installed but never limits is the failure mode this whole type exists
// to avoid, so it must not be reachable at all.
func NewRateLimiter(burst int, window time.Duration) *RateLimiter {
	if burst <= 0 {
		panic(fmt.Sprintf("httpx.NewRateLimiter: burst must be positive, got %d", burst))
	}
	if window <= 0 {
		panic(fmt.Sprintf("httpx.NewRateLimiter: window must be positive, got %v", window))
	}
	l := &RateLimiter{
		buckets: map[string]*bucket{},
		burst:   burst,
		window:  window,
		stop:    make(chan struct{}),
	}
	go l.reap(reapInterval)
	return l
}

// Stop ends the reaper goroutine and releases its ticker. It is safe to call
// more than once and safe never to call: the two limiters in this program live
// for the lifetime of the process. It exists so that a limiter created per test
// or per request does not leak a goroutine and a ticker for each one.
//
// A stopped limiter still limits; only the eviction of idle buckets stops.
func (l *RateLimiter) Stop() {
	l.stopOnce.Do(func() { close(l.stop) })
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

	// Refill in proportion to the time that has passed. The window is positive
	// by construction so this cannot divide by zero; the guard is belt and
	// braces, because a NaN stored here makes every later "b.tokens < 1"
	// comparison false and that bucket is never refused again. A clock that
	// jumped backwards is treated as no time passing rather than as a debit.
	refill := now.Sub(b.seen).Seconds() / l.window.Seconds() * float64(l.burst)
	if math.IsNaN(refill) || refill < 0 {
		refill = 0
	}
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
// It runs until Stop. The interval is a parameter rather than a constant so a
// test can drive it without waiting ten minutes for the first tick.
func (l *RateLimiter) reap(every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()

	for {
		select {
		case <-l.stop:
			return
		case now := <-t.C:
			l.evictIdle(now.Add(-idleTTL))
		}
	}
}

// evictIdle drops every bucket last seen before cutoff.
func (l *RateLimiter) evictIdle(cutoff time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, b := range l.buckets {
		if b.seen.Before(cutoff) {
			delete(l.buckets, key)
		}
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
