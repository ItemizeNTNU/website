// Package httpx provides the middleware chain, response helpers and static
// asset serving that a framework would otherwise supply. Everything here is
// standard library.
package httpx

import "net/http"

// Middleware wraps a handler with additional behaviour.
type Middleware func(http.Handler) http.Handler

// Chain applies middleware to h so that mw[0] is outermost — the order they
// are written is the order a request passes through them.
func Chain(h http.Handler, mw ...Middleware) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

// ChainFunc is Chain for a bare handler function.
func ChainFunc(h http.HandlerFunc, mw ...Middleware) http.Handler {
	return Chain(h, mw...)
}

// recorder wraps a ResponseWriter to capture the status code and the number
// of bytes written, so the access log can report them.
//
// Wrapping a ResponseWriter normally hides the optional interfaces the real
// one implements. Unwrap keeps [http.ResponseController] working, and Flush is
// forwarded explicitly for the same reason.
type recorder struct {
	http.ResponseWriter
	status  int
	written int64
}

func (r *recorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorder) Write(b []byte) (int, error) {
	// A handler that never calls WriteHeader still produces a 200; without
	// this the log would report status 0.
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.written += int64(n)
	return n, err
}

// Unwrap exposes the underlying writer to http.ResponseController.
func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (r *recorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Status reports the response status, defaulting to 200 for a handler that
// wrote nothing at all.
func (r *recorder) Status() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}
