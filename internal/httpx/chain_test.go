package httpx

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// tag returns a middleware that records its name as a request passes inward
// and again as the response unwinds, so a test can assert the exact path a
// request takes through a chain.
func tag(order *[]string, name string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*order = append(*order, name+" in")
			next.ServeHTTP(w, r)
			*order = append(*order, name+" out")
		})
	}
}

func okHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
}

// The order middleware is written in main.go is the order a reader assumes a
// request passes through it. If Chain reversed that, Recover would sit outside
// Logger instead of inside it and a recovered panic would produce no access
// log line at all — the exact request you most want a record of.
func TestChainAppliesFirstMiddlewareOutermost(t *testing.T) {
	var order []string
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
	}), tag(&order, "a"), tag(&order, "b"), tag(&order, "c"))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	want := []string{"a in", "b in", "c in", "handler", "c out", "b out", "a out"}
	if !slices.Equal(order, want) {
		t.Errorf("request took the path %v, want %v", order, want)
	}
}

func TestChainWithNoMiddlewareServesTheHandler(t *testing.T) {
	calls := 0
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusTeapot)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if calls != 1 {
		t.Errorf("handler ran %d times through an empty chain, want exactly 1", calls)
	}
	if rec.Code != http.StatusTeapot {
		t.Errorf("an empty chain changed the status to %d; it must be transparent", rec.Code)
	}
}

func TestChainWithOneMiddlewareStillWraps(t *testing.T) {
	var order []string
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
	}), tag(&order, "only"))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	want := []string{"only in", "handler", "only out"}
	if !slices.Equal(order, want) {
		t.Errorf("request took the path %v, want %v", order, want)
	}
}

type chainKey struct{}

// Every middleware in the real chain relies on this: SecurityHeaders sets
// headers the handlers below never touch, and RequestID puts a value in the
// context that Logger reads several layers further in. If an inner wrapper
// could not observe an outer one's writes, both would silently do nothing.
func TestChainInnerMiddlewareSeesOuterEffects(t *testing.T) {
	outer := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Outer", "written")
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), chainKey{}, "from outer")))
		})
	}

	var sawHeader, sawContext string
	inner := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sawHeader = w.Header().Get("X-Outer")
			sawContext, _ = r.Context().Value(chainKey{}).(string)
			next.ServeHTTP(w, r)
		})
	}

	h := Chain(okHandler(), outer, inner)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if sawHeader != "written" {
		t.Errorf("inner middleware saw X-Outer = %q; headers set upstream are being lost", sawHeader)
	}
	if sawContext != "from outer" {
		t.Errorf("inner middleware saw context value %q; the request is not being passed down", sawContext)
	}
}

func TestChainFuncWrapsABareFunction(t *testing.T) {
	var order []string
	h := ChainFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
		w.WriteHeader(http.StatusOK)
	}, tag(&order, "a"), tag(&order, "b"))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	want := []string{"a in", "b in", "handler", "b out", "a out"}
	if !slices.Equal(order, want) {
		t.Errorf("request took the path %v, want %v", order, want)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want the handler's 200", rec.Code)
	}
}

// plainWriter is a ResponseWriter that implements nothing beyond the required
// three methods — no Flusher, no Hijacker. Production writers are richer, so
// this stands in for the case a wrapper must not assume anything extra.
type plainWriter struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func newPlainWriter() *plainWriter { return &plainWriter{header: http.Header{}} }

func (p *plainWriter) Header() http.Header         { return p.header }
func (p *plainWriter) WriteHeader(status int)      { p.code = status }
func (p *plainWriter) Write(b []byte) (int, error) { return p.body.Write(b) }

// The access log reports the status, and a handler that only calls Write never
// touches WriteHeader. Without the default the log would claim status 0 for
// every ordinary page — every successful request would look broken.
func TestRecorderReportsStatusAndBytes(t *testing.T) {
	tests := []struct {
		name       string
		handler    func(w http.ResponseWriter)
		wantStatus int
		wantBytes  int64
	}{
		{
			name:       "handler wrote nothing at all",
			handler:    func(w http.ResponseWriter) {},
			wantStatus: http.StatusOK,
			wantBytes:  0,
		},
		{
			name:       "body without an explicit WriteHeader",
			handler:    func(w http.ResponseWriter) { w.Write([]byte("hei")) },
			wantStatus: http.StatusOK,
			wantBytes:  3,
		},
		{
			name: "explicit status then body",
			handler: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte("laget"))
			},
			wantStatus: http.StatusCreated,
			wantBytes:  5,
		},
		{
			name: "several writes are summed",
			handler: func(w http.ResponseWriter) {
				w.Write([]byte("abc"))
				w.Write([]byte("de"))
				w.Write(nil)
			},
			wantStatus: http.StatusOK,
			wantBytes:  5,
		},
		{
			name:       "error status with no body",
			handler:    func(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError) },
			wantStatus: http.StatusInternalServerError,
			wantBytes:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recorder{ResponseWriter: httptest.NewRecorder()}
			tt.handler(rec)

			if got := rec.Status(); got != tt.wantStatus {
				t.Errorf("the access log would report status %d, want %d", got, tt.wantStatus)
			}
			if rec.written != tt.wantBytes {
				t.Errorf("the access log would report %d bytes, want %d", rec.written, tt.wantBytes)
			}
		})
	}
}

// Recover branches on whether anything has been written yet. If a second
// WriteHeader could overwrite the recorded status, a handler that wrote its
// 200 and then hit a superfluous WriteHeader would confuse that decision.
func TestRecorderKeepsTheFirstStatus(t *testing.T) {
	under := httptest.NewRecorder()
	rec := &recorder{ResponseWriter: under}

	rec.WriteHeader(http.StatusNotFound)
	rec.WriteHeader(http.StatusInternalServerError)

	if rec.Status() != http.StatusNotFound {
		t.Errorf("recorded status %d, want the first one written (404)", rec.Status())
	}
	if under.Code != http.StatusNotFound {
		t.Errorf("client saw status %d, want 404", under.Code)
	}
}

// Wrapping a ResponseWriter hides the optional interfaces the real one
// implements. Unwrap is what keeps http.ResponseController working through the
// wrapper; without it a streaming handler silently stops flushing.
func TestRecorderForwardsFlushAndUnwrap(t *testing.T) {
	t.Run("flush reaches a flushable writer", func(t *testing.T) {
		under := httptest.NewRecorder()
		rec := &recorder{ResponseWriter: under}
		rec.Write([]byte("chunk"))
		rec.Flush()

		if !under.Flushed {
			t.Error("Flush did not reach the underlying writer; streamed responses would stall")
		}
	})

	t.Run("flush on a plain writer is a no-op", func(t *testing.T) {
		rec := &recorder{ResponseWriter: newPlainWriter()}
		rec.Write([]byte("chunk"))
		rec.Flush() // must not panic
	})

	t.Run("Unwrap exposes the writer underneath", func(t *testing.T) {
		under := httptest.NewRecorder()
		rec := &recorder{ResponseWriter: under}
		if rec.Unwrap() != http.ResponseWriter(under) {
			t.Error("Unwrap returned a different writer; ResponseController would operate on the wrong one")
		}
		if err := http.NewResponseController(rec).Flush(); err != nil {
			t.Errorf("ResponseController.Flush through the recorder failed: %v", err)
		}
	})
}
