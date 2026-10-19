// Package usecase implements identity use cases. api.go is the public
// contract used by other modules (R4).
package usecase

import (
	"context"

	"github.com/be/acs/api/internal/shared/kernel"
)

// UserSummary is the public view of a user.
type UserSummary struct {
	ID     kernel.ID
	Email  string
	Name   string
	Status string
	Roles  []string
}

// API is the identity module's public interface.
type API interface {
	// UsersWithPermission lists active users holding perm (e.g. reviewers
	// for review_requested notifications, budget.manage for budget alerts).
	UsersWithPermission(ctx context.Context, perm string) ([]UserSummary, error)
	// GetUsers returns summaries for ids (missing ids are skipped), for
	// displaying creator/reviewer names.
	GetUsers(ctx context.Context, ids []kernel.ID) ([]UserSummary, error)
}
