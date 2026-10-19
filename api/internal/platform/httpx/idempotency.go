package httpx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/be/acs/api/internal/shared/kernel"
)

// HeaderIdempotencyKey is required on every POST that creates an AI job.
const HeaderIdempotencyKey = "Idempotency-Key"

// Idempotency identifies a create request for de-duplication. The job
// module stores (created_by, Key, RequestHash): same key + same hash returns
// the existing job, same key + different hash is 409 IDEMPOTENCY_CONFLICT.
type Idempotency struct {
	Key         string
	RequestHash string
}

type idemKey struct{}

// RequireIdempotencyKey validates the Idempotency-Key header (a UUID),
// hashes method + route path + body, and stores both in ctx for the handler
// (read with IdempotencyFrom) to pass into its use case.
func RequireIdempotencyKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get(HeaderIdempotencyKey)
		if _, err := uuid.Parse(key); err != nil {
			Error(w, r, kernel.NewError(kernel.CodeIdempotencyKeyRequired, "Thiếu hoặc sai định dạng Idempotency-Key (UUID)"))
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, MaxJSONBody+1))
		if err != nil {
			Error(w, r, kernel.Validation(kernel.FieldIssue("body", "unreadable")))
			return
		}
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(body))
		h := sha256.New()
		h.Write([]byte(r.Method + " " + r.URL.Path + "\n"))
		h.Write(body)
		ctx := context.WithValue(r.Context(), idemKey{}, Idempotency{Key: key, RequestHash: hex.EncodeToString(h.Sum(nil))})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// IdempotencyFrom returns the idempotency data set by RequireIdempotencyKey.
func IdempotencyFrom(r *http.Request) (Idempotency, bool) {
	v, ok := r.Context().Value(idemKey{}).(Idempotency)
	return v, ok
}
