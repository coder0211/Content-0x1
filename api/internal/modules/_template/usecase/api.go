// Package usecase holds interactors (one file per use case), the ports they
// need, and api.go — the module's public contract for other modules.
package usecase

import (
	"context"

	"github.com/be/acs/api/internal/shared/kernel"
)

// NoteSummary is what other modules may read.
type NoteSummary struct {
	ID    kernel.ID
	Title string
}

// API is the public interface other modules depend on (R4).
type API interface {
	GetNote(ctx context.Context, id kernel.ID) (NoteSummary, error)
}
