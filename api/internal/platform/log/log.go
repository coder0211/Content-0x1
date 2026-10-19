// Package log builds the process slog.Logger: JSON output, request/job
// context fields, and redaction of secrets and signed URL parameters
// (SPEC-02 §7 Logging, NFR-06).
package log

import (
	"context"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"

	"github.com/be/acs/api/internal/shared/kernel"
)

const redacted = "[REDACTED]"

// sensitiveKeys are attribute keys whose values are always redacted.
var sensitiveKeys = map[string]bool{
	"password": true, "temp_password": true, "password_hash": true,
	"token": true, "session_token": true, "csrf_token": true,
	"api_key": true, "secret": true, "secret_key": true,
	"authorization": true, "cookie": true, "set-cookie": true, "x-csrf-token": true,
}

// sensitiveQuery are URL query parameters stripped from logged URLs.
var sensitiveQuery = []string{
	"X-Amz-Signature", "X-Amz-Credential", "X-Amz-Security-Token",
	"Signature", "signature", "token", "access_token", "key",
}

// New builds a JSON logger writing to stdout at level.
func New(level string) *slog.Logger {
	return NewWithWriter(os.Stdout, level)
}

// NewWithWriter is New with an explicit writer (tests).
func NewWithWriter(w io.Writer, level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: lvl,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			switch {
			case a.Key == slog.TimeKey:
				a.Key = "ts"
			case sensitiveKeys[strings.ToLower(a.Key)]:
				return slog.String(a.Key, redacted)
			case a.Value.Kind() == slog.KindString && looksLikeURL(a.Value.String()):
				return slog.String(a.Key, RedactURL(a.Value.String()))
			}
			return a
		},
	})
	return slog.New(&contextHandler{Handler: h})
}

func looksLikeURL(s string) bool {
	return strings.Contains(s, "?") && (strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "/"))
}

// RedactURL removes signature/credential query parameters from raw.
func RedactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return redacted
	}
	q := u.Query()
	changed := false
	for _, k := range sensitiveQuery {
		if q.Has(k) {
			q.Set(k, redacted)
			changed = true
		}
	}
	if !changed {
		return raw
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// fieldsKey stores extra log attributes (job_id, step_id, provider...) in ctx.
type fieldsKey struct{}

// With returns a ctx whose log records carry attrs (e.g. job_id, step_id).
func With(ctx context.Context, attrs ...slog.Attr) context.Context {
	prev, _ := ctx.Value(fieldsKey{}).([]slog.Attr)
	return context.WithValue(ctx, fieldsKey{}, append(append([]slog.Attr(nil), prev...), attrs...))
}

// contextHandler adds request_id, user_id and ctx fields to every record.
type contextHandler struct{ slog.Handler }

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if m := kernel.RequestMetaFrom(ctx); m.RequestID != "" {
		r.AddAttrs(slog.String("request_id", m.RequestID))
	}
	if a := kernel.ActorFrom(ctx); a.IsAuthenticated() {
		r.AddAttrs(slog.String("user_id", a.UserID.String()))
	}
	if attrs, ok := ctx.Value(fieldsKey{}).([]slog.Attr); ok {
		r.AddAttrs(attrs...)
	}
	return h.Handler.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{h.Handler.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{h.Handler.WithGroup(name)}
}
