package kernel

import (
	"encoding/base64"
	"encoding/json"
	"time"
)

// Pagination defaults (SPEC-04 §1: ?limit=20&cursor=<opaque>).
const (
	DefaultPageLimit = 20
	MaxPageLimit     = 100
)

// Cursor is an opaque, URL-safe continuation token.
type Cursor string

// PageRequest is the input of every list use case.
type PageRequest struct {
	Limit  int
	Cursor Cursor
}

// Normalized clamps Limit into [1, MaxPageLimit], defaulting to DefaultPageLimit.
func (p PageRequest) Normalized() PageRequest {
	switch {
	case p.Limit <= 0:
		p.Limit = DefaultPageLimit
	case p.Limit > MaxPageLimit:
		p.Limit = MaxPageLimit
	}
	return p
}

// Page is one page of results. NextCursor is nil on the last page.
type Page[T any] struct {
	Items      []T
	NextCursor *Cursor
}

// EncodeCursor serialises any JSON-able keyset position into a Cursor.
func EncodeCursor(v any) Cursor {
	b, err := json.Marshal(v)
	if err != nil {
		panic("kernel.EncodeCursor: " + err.Error())
	}
	return Cursor(base64.RawURLEncoding.EncodeToString(b))
}

// DecodeCursor parses c into v. An empty cursor leaves v untouched and
// returns false. A malformed cursor is a validation error.
func DecodeCursor(c Cursor, v any) (bool, error) {
	if c == "" {
		return false, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(string(c))
	if err != nil || json.Unmarshal(b, v) != nil {
		return false, Validation(FieldIssue("cursor", "invalid"))
	}
	return true, nil
}

// TimeIDCursor is the standard keyset position for lists ordered by
// (created_at DESC, id DESC).
type TimeIDCursor struct {
	At time.Time `json:"t"`
	ID ID        `json:"i"`
}

// PageFromRows trims rows fetched with limit+1 and builds the next cursor
// from the last returned item via key.
func PageFromRows[T any](rows []T, limit int, key func(T) any) Page[T] {
	if len(rows) <= limit {
		return Page[T]{Items: rows}
	}
	rows = rows[:limit]
	c := EncodeCursor(key(rows[len(rows)-1]))
	return Page[T]{Items: rows, NextCursor: &c}
}
