package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/shared/kernel"
)

// MaxJSONBody caps JSON request bodies.
const MaxJSONBody = 1 << 20

// Decode parses a JSON body into v, rejecting unknown fields, trailing data
// and oversize bodies with VALIDATION_FAILED.
func Decode(r *http.Request, v any) error {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		return kernel.Validation(kernel.FieldIssue("body", "content_type_must_be_json"))
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, MaxJSONBody+1))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return decodeErr(err)
	}
	if dec.More() {
		return kernel.Validation(kernel.FieldIssue("body", "trailing_data"))
	}
	return nil
}

func decodeErr(err error) error {
	var typeErr *json.UnmarshalTypeError
	var syntaxErr *json.SyntaxError
	switch {
	case errors.Is(err, io.EOF):
		return kernel.Validation(kernel.FieldIssue("body", "required"))
	case errors.As(err, &typeErr):
		return kernel.Validation(kernel.FieldIssue(typeErr.Field, "invalid_type"))
	case errors.As(err, &syntaxErr), errors.Is(err, io.ErrUnexpectedEOF):
		return kernel.Validation(kernel.FieldIssue("body", "malformed_json"))
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		field := strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)
		return kernel.Validation(kernel.FieldIssue(field, "unknown_field"))
	default:
		return kernel.Validation(kernel.FieldIssue("body", "invalid"))
	}
}

// PathID parses the chi URL parameter name as an ID. A malformed ID is
// reported as NOT_FOUND (it cannot name an existing resource).
func PathID(r *http.Request, name string) (kernel.ID, error) {
	id, err := kernel.ParseID(name, chi.URLParam(r, name))
	if err != nil {
		return kernel.NilID, kernel.ErrNotFound
	}
	return id, nil
}

// PathParam returns the raw chi URL parameter.
func PathParam(r *http.Request, name string) string { return chi.URLParam(r, name) }

// QueryID parses an optional ID query parameter.
func QueryID(r *http.Request, name string) (*kernel.ID, error) {
	s := r.URL.Query().Get(name)
	if s == "" {
		return nil, nil
	}
	id, err := kernel.ParseID(name, s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// QueryInt parses an optional integer query parameter.
func QueryInt(r *http.Request, name string, def int) (int, error) {
	s := r.URL.Query().Get(name)
	if s == "" {
		return def, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, kernel.Validation(kernel.FieldIssue(name, "invalid_integer"))
	}
	return n, nil
}

// PageRequest reads ?limit and ?cursor.
func PageRequest(r *http.Request) (kernel.PageRequest, error) {
	limit, err := QueryInt(r, "limit", kernel.DefaultPageLimit)
	if err != nil {
		return kernel.PageRequest{}, err
	}
	if limit < 1 || limit > kernel.MaxPageLimit {
		return kernel.PageRequest{}, kernel.Validation(kernel.FieldIssue("limit", "out_of_range"))
	}
	return kernel.PageRequest{Limit: limit, Cursor: kernel.Cursor(r.URL.Query().Get("cursor"))}, nil
}
