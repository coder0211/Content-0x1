// Package kernel holds the tiny set of types shared by every layer of every
// module: identifiers, money, domain errors, pagination and the acting user.
// It must stay framework-free (stdlib + uuid + decimal only).
package kernel

import "github.com/google/uuid"

// ID is the identifier type for every entity. It is an alias so that values
// flow between sqlc rows, domain entities and JSON without conversion.
type ID = uuid.UUID

// NilID is the zero ID.
var NilID = uuid.Nil

// NewID returns a time-ordered (UUIDv7) identifier, which keeps B-tree
// indexes on primary keys append-mostly.
func NewID() ID {
	return uuid.Must(uuid.NewV7())
}

// ParseID parses s as an ID. A malformed value yields a VALIDATION_FAILED
// error pointing at field.
func ParseID(field, s string) (ID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return NilID, Validation(FieldIssue(field, "invalid_uuid"))
	}
	return id, nil
}

// IDPtr returns a pointer to id, or nil when id is the zero value.
func IDPtr(id ID) *ID {
	if id == NilID {
		return nil
	}
	return &id
}
