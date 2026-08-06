package main

// No test in this file may call t.Parallel. TestHealthcheck relies on
// t.Setenv, which panics if the test (or any of its ancestors) is marked
// parallel, so the whole file runs sequentially.

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	healthz(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("healthz responded with status %d, want %d", rec.Code, http.StatusOK)
	}
	// version defaults to "dev" in source and is only overwritten by -ldflags
	// at build time, so under `go test` the body is always "ok dev\n".
	if got, want := rec.Body.String(), "ok dev\n"; got != want {
		t.Errorf("healthz wrote body %q, want %q", got, want)
	}

	headers := []struct {
		name string
		want string
	}{
		{"Content-Type", "text/plain; charset=utf-8"},
		{"Cache-Control", "no-store"},
	}
	for _, h := range headers {
		if got := rec.Header().Get(h.name); got != h.want {
			t.Errorf("healthz set %s header to %q, want %q", h.name, got, h.want)
		}
	}
}

// startHealthServer serves the given handler on an ephemeral 127.0.0.1 port
// and returns the bare port number, which is what healthcheck expects in its
// environment variables. The server is shut down when the test finishes.
func startHealthServer(t *testing.T, handler http.Handler) (port string, shutdown func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening on an ephemeral port failed: %v", err)
	}

	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(ln) }()

	shutdown = func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
	t.Cleanup(shutdown)

	_, port, err = net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("splitting listener address %q failed: %v", ln.Addr(), err)
	}
	return port, shutdown
}

func TestHealthcheck(t *testing.T) {
	tests := []struct {
		name string
		// status is what the test server's /healthz handler responds with.
		status int
		// closed shuts the server down before healthcheck runs, so the
		// probe's connection is refused.
		closed bool
		// useListen exercises the env fallback chain: healthcheck reads
		// PORT first and falls back to LISTEN when PORT is unset.
		useListen bool
		want      int
	}{
		{name: "healthy server via PORT", status: http.StatusOK, want: 0},
		{name: "handler returns 500", status: http.StatusInternalServerError, want: 1},
		{name: "listener closed before call", status: http.StatusOK, closed: true, want: 1},
		{name: "PORT unset but LISTEN set", status: http.StatusOK, useListen: true, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			})
			port, shutdown := startHealthServer(t, handler)

			if tt.closed {
				shutdown()
			}

			// Clear both variables first so ambient values from the
			// environment cannot leak into the fallback chain.
			t.Setenv("PORT", "")
			t.Setenv("LISTEN", "")
			if tt.useListen {
				t.Setenv("LISTEN", port)
			} else {
				t.Setenv("PORT", port)
			}

			if got := healthcheck(); got != tt.want {
				t.Errorf("healthcheck returned exit code %d, want %d", got, tt.want)
			}
		})
	}
}

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name  string
		dev   bool
		level slog.Level
		want  bool
	}{
		{name: "dev logger enables debug", dev: true, level: slog.LevelDebug, want: true},
		{name: "production logger suppresses debug", dev: false, level: slog.LevelDebug, want: false},
		{name: "production logger enables info", dev: false, level: slog.LevelInfo, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := newLogger(tt.dev)
			if got := log.Enabled(context.Background(), tt.level); got != tt.want {
				t.Errorf("newLogger(dev=%t).Enabled(%v) = %t, want %t",
					tt.dev, tt.level, got, tt.want)
			}
		})
	}
}
