package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/be/acs/api/internal/shared/kernel"
)

func TestTransitions(t *testing.T) {
	cases := []struct {
		from, to Status
		ok       bool
	}{
		{StatusDraft, StatusPublished, true},
		{StatusDraft, StatusArchived, true},
		{StatusPublished, StatusArchived, true},
		{StatusPublished, StatusDraft, false},
		{StatusArchived, StatusPublished, false},
	}
	for _, c := range cases {
		n := &Note{Status: c.from}
		err := n.Transition(c.to)
		if (err == nil) != c.ok {
			t.Errorf("%s→%s: %v", c.from, c.to, err)
		}
		if !c.ok && !errors.Is(err, kernel.ErrInvalidTransition) {
			t.Errorf("want INVALID_STATE_TRANSITION, got %v", err)
		}
	}
}

func TestNewNoteValidation(t *testing.T) {
	if _, err := NewNote("  ", kernel.NewID(), time.Now()); !errors.Is(err, kernel.ErrValidation) {
		t.Fatal(err)
	}
}
