package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/platform/log"
	"github.com/be/acs/api/internal/platform/metrics"
	"github.com/be/acs/api/internal/shared/kernel"
)

// HeaderRequestID is echoed on every response.
const HeaderRequestID = "X-Request-Id"

type loggerKey struct{}

func loggerFrom(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// RequestID returns the current request ID.
func RequestID(ctx context.Context) string { return kernel.RequestMetaFrom(ctx).RequestID }

func newRequestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func validClientRequestID(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, c := range s {
		ok := c == '-' || c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if !ok {
			return false
		}
	}
	return true
}

// clientIP prefers X-Real-IP set by the Nginx reverse proxy, else the peer.
func clientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(ip) != nil {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// RequestMeta assigns a request ID (accepting a well-formed client one),
// records client IP / user agent in ctx and attaches the logger.
func RequestMeta(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(HeaderRequestID)
			if !validClientRequestID(id) {
				id = newRequestID()
			}
			w.Header().Set(HeaderRequestID, id)
			ctx := kernel.WithRequestMeta(r.Context(), kernel.RequestMeta{RequestID: id, IP: clientIP(r), UserAgent: r.UserAgent()})
			ctx = context.WithValue(ctx, loggerKey{}, logger)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Recover turns a panic into a 500 envelope carrying the request ID.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				if err, ok := p.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(p)
				}
				loggerFrom(r.Context()).ErrorContext(r.Context(), "panic",
					slog.String("panic", fmt.Sprint(p)), slog.String("stack", string(debug.Stack())))
				writeError(w, r, http.StatusInternalServerError, kernel.ErrInternal)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// Flush keeps SSE working through the recorder.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// AccessLog logs one line per request and records the latency histogram.
// Headers are never logged; the query string is redacted of signatures.
func AccessLog(m *metrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			if rec.status == 0 {
				rec.status = http.StatusOK
			}
			route := "unmatched"
			if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
				route = rc.RoutePattern()
			}
			dur := time.Since(start)
			if m != nil {
				m.HTTPDuration.WithLabelValues(route, r.Method, strconv.Itoa(rec.status)).Observe(dur.Seconds())
			}
			level := slog.LevelInfo
			if rec.status >= 500 {
				level = slog.LevelError
			}
			if route == "/healthz" || route == "/readyz" || route == "/metrics" {
				level = slog.LevelDebug
			}
			loggerFrom(r.Context()).LogAttrs(r.Context(), level, "http request",
				slog.String("method", r.Method),
				slog.String("path", log.RedactURL(r.URL.RequestURI())),
				slog.String("route", route),
				slog.Int("status", rec.status),
				slog.Int("bytes", rec.bytes),
				slog.Int64("duration_ms", dur.Milliseconds()),
				slog.String("ip", kernel.RequestMetaFrom(r.Context()).IP),
			)
		})
	}
}

// CORS allows credentialed requests from the configured internal origins
// only. In production the web app is same-origin behind Nginx and the list
// is typically empty.
func CORS(origins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && slices.Contains(origins, origin) {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Add("Vary", "Origin")
				if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
					h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					h.Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token, Idempotency-Key, X-Request-Id, Last-Event-ID")
					h.Set("Access-Control-Expose-Headers", "X-Request-Id")
					h.Set("Access-Control-Max-Age", "600")
					w.WriteHeader(http.StatusNoContent)
					return
				}
				h.Set("Access-Control-Expose-Headers", "X-Request-Id")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders sets conservative headers for JSON APIs.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
