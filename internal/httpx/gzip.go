package httpx

import (
	"compress/gzip"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

var gzipPool = sync.Pool{
	New: func() any {
		w, _ := gzip.NewWriterLevel(nil, gzip.BestSpeed)
		return w
	},
}

// Gzip compresses text responses for clients that ask for it.
//
// The decision is made from the response Content-Type, so a handler must set
// it before writing — every handler in this program does, and the alternative
// (sniffing the first write) buys nothing here.
//
// Static assets are pre-compressed once at startup and served with
// Content-Encoding already set, so this middleware leaves them alone.
func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !acceptsGzip(r) || r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		// Vary goes on every response, compressed or not: without it a shared
		// cache can serve a gzipped body to a client that cannot read it.
		w.Header().Add("Vary", "Accept-Encoding")

		gw := &gzipResponseWriter{ResponseWriter: w}
		defer gw.Close()
		next.ServeHTTP(gw, r)
	})
}

// acceptsGzip reports whether the client offered gzip and did not then refuse
// it with a zero q-value.
//
// A bare "*" does not count as an offer, so "*;q=0" needs no special handling:
// without an explicit gzip token the answer is already no, and with one the
// explicit entry wins over the wildcard under RFC 9110 anyway.
func acceptsGzip(r *http.Request) bool {
	for _, enc := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		// The whitespace has to come off after the cut as well as before it:
		// RFC 9110 allows it on both sides of the ";", so "gzip ; q=1.0" is a
		// perfectly legal way to ask for gzip.
		name, params, _ := strings.Cut(enc, ";")
		if strings.EqualFold(strings.TrimSpace(name), "gzip") {
			return !refusedByQValue(params)
		}
	}
	return false
}

// refusedByQValue reports whether a coding's parameters carry q=0, which
// RFC 9110 defines as "not acceptable" — compressing for such a client is
// doing the one thing it explicitly asked us not to do. Ranking the codings
// against each other is not worth it here: gzip is the only encoding we can
// produce, so the only question is whether it is allowed at all.
func refusedByQValue(params string) bool {
	for _, p := range strings.Split(params, ";") {
		v, ok := strings.CutPrefix(strings.ToLower(strings.TrimSpace(p)), "q=")
		if !ok {
			continue
		}
		// Any legal spelling of zero: "0", "0.0", "0.000".
		if q, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && q <= 0 {
			return true
		}
	}
	return false
}

// compressible reports whether a media type is worth compressing. Images,
// fonts and archives are already compressed; running them through gzip costs
// CPU and usually makes them marginally larger.
func compressible(contentType string) bool {
	mediaType, _, _ := strings.Cut(contentType, ";")
	mediaType = strings.TrimSpace(strings.ToLower(mediaType))

	switch {
	case strings.HasPrefix(mediaType, "text/"):
		return true
	case strings.HasPrefix(mediaType, "image/svg"):
		return true
	case strings.HasPrefix(mediaType, "image/"), strings.HasPrefix(mediaType, "font/"),
		strings.HasPrefix(mediaType, "audio/"), strings.HasPrefix(mediaType, "video/"):
		return false
	}

	switch mediaType {
	case "application/json", "application/javascript", "application/xml",
		"application/xhtml+xml", "application/manifest+json", "application/rss+xml",
		"application/atom+xml":
		return true
	}
	return false
}

type gzipResponseWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	wroteHeader bool
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true

	h := w.Header()
	// 204/304 have no body, and an existing Content-Encoding means the
	// handler already compressed (or otherwise encoded) the payload.
	//
	// A 206 is excluded because its Content-Range counts offsets into the body
	// the handler chose to send. Compressing it here would leave those offsets
	// describing bytes that are no longer on the wire, and a client resuming a
	// download would stitch the pieces back together wrongly.
	if status != http.StatusNoContent && status != http.StatusNotModified &&
		status != http.StatusPartialContent &&
		h.Get("Content-Encoding") == "" && compressible(h.Get("Content-Type")) {

		// The compressed length is unknown until the body is written, and an
		// ETag computed over the identity body no longer matches the bytes on
		// the wire, so it becomes a weak validator.
		h.Del("Content-Length")
		h.Set("Content-Encoding", "gzip")
		if etag := h.Get("ETag"); etag != "" && !strings.HasPrefix(etag, "W/") {
			h.Set("ETag", "W/"+etag)
		}

		w.gz = gzipPool.Get().(*gzip.Writer)
		w.gz.Reset(w.ResponseWriter)
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.gz != nil {
		return w.gz.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

// Close flushes and releases the gzip writer. It is safe to call more than
// once and is a no-op when the response was not compressed.
func (w *gzipResponseWriter) Close() {
	if w.gz == nil {
		return
	}
	w.gz.Close()
	w.gz.Reset(nil) // drop the reference to the underlying writer
	gzipPool.Put(w.gz)
	w.gz = nil
}

func (w *gzipResponseWriter) Flush() {
	if w.gz != nil {
		w.gz.Flush()
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap exposes the underlying writer to http.ResponseController.
func (w *gzipResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
