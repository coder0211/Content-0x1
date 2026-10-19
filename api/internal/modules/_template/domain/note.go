// Package domain holds the module's entities and pure rules. Imports:
// stdlib + shared/kernel only.
package domain

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/be/acs/api/internal/shared/kernel"
)

// Status is a small state machine example.
type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusArchived  Status = "archived"
)

var transitions = map[Status][]Status{
	StatusDraft:     {StatusPublished, StatusArchived},
	StatusPublished: {StatusArchived},
}

// Note is the example entity.
type Note struct {
	ID        kernel.ID
	Title     string
	Status    Status
	CreatedBy kernel.ID
	CreatedAt time.Time
}

// NewNote validates input and builds a draft note.
func NewNote(title string, by kernel.ID, now time.Time) (*Note, error) {
	title = strings.TrimSpace(title)
	if title == "" || utf8.RuneCountInString(title) > 200 {
		return nil, kernel.Validation(kernel.FieldIssue("title", "required"))
	}
	return &Note{ID: kernel.NewID(), Title: title, Status: StatusDraft, CreatedBy: by, CreatedAt: now}, nil
}

// Transition moves the note to `to` or returns INVALID_STATE_TRANSITION.
func (n *Note) Transition(to Status) error {
	for _, s := range transitions[n.Status] {
		if s == to {
			n.Status = to
			return nil
		}
	}
	return kernel.InvalidTransition(string(n.Status), string(to))
}
