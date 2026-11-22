// Package httpx holds HTTP plumbing shared by all feature packages:
// request IDs, access logging, panic recovery and security headers.
package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"
)

type ctxKey int

const requestIDKey ctxKey = iota

// RequestID returns the ID assigned to this request, or "".
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// Chain applies middleware so the first listed runs outermost.
func Chain(h http.Handler, mw ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b [8]byte
		_, _ = rand.Read(b[:])
		id := hex.EncodeToString(b[:])
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Logger logs one line per request. It logs the path only (never the query
// string or body) so student data never reaches the logs.
func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w}
			next.ServeHTTP(sw, r)
			if r.URL.Path == "/healthz" {
				return
			}
			log.Info("request",
				"id", RequestID(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"bytes", sw.bytes,
				"dur_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

// Recover turns panics into a 500 and logs them.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					if v == http.ErrAbortHandler {
						panic(v)
					}
					log.Error("panic", "id", RequestID(r.Context()), "err", v)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders sets conservative defaults. HSTS only in production (behind TLS).
func SecurityHeaders(prod bool) func(http.Handler) http.Handler {
	const csp = "default-src 'self'; " +
		"style-src 'self' https://fonts.googleapis.com 'unsafe-inline'; " +
		"font-src https://fonts.gstatic.com; " +
		"img-src 'self' data:; " +
		"script-src 'self'; " +
		"frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Content-Security-Policy", csp)
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "same-origin")
			h.Set("X-Frame-Options", "DENY")
			if prod {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}
