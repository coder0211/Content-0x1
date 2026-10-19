package usecase

import (
	"context"

	"github.com/be/acs/api/internal/modules/_template/domain"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Notes groups the example interactors.
type Notes struct {
	Tx    ports.TxManager
	Repo  NoteRepository
	Clock ports.Clock
	Audit ports.AuditRecorder
}

var _ API = (*Notes)(nil)

// Create checks permission, validates via the domain and persists in a tx.
func (uc *Notes) Create(ctx context.Context, actor kernel.Actor, title string) (*domain.Note, error) {
	if err := actor.Require(kernel.PermCampaignManage); err != nil {
		return nil, err
	}
	n, err := domain.NewNote(title, actor.UserID, uc.Clock.Now())
	if err != nil {
		return nil, err
	}
	err = uc.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := uc.Repo.Create(ctx, n); err != nil {
			return err
		}
		return uc.Audit.Record(ctx, "note.create", "note", n.ID.String(), nil, n)
	})
	return n, err
}

// Publish applies a state transition with an ownership (IDOR) check.
func (uc *Notes) Publish(ctx context.Context, actor kernel.Actor, id kernel.ID) (*domain.Note, error) {
	var out *domain.Note
	err := uc.Tx.WithinTx(ctx, func(ctx context.Context) error {
		n, err := uc.Repo.GetForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if !actor.CanModifyOwned(n.CreatedBy) {
			return kernel.ErrNotFound // do not reveal others' resources
		}
		if err := n.Transition(domain.StatusPublished); err != nil {
			return err
		}
		out = n
		return uc.Repo.Save(ctx, n)
	})
	return out, err
}

// GetNote implements API.
func (uc *Notes) GetNote(ctx context.Context, id kernel.ID) (NoteSummary, error) {
	n, err := uc.Repo.Get(ctx, id)
	if err != nil {
		return NoteSummary{}, err
	}
	return NoteSummary{ID: n.ID, Title: n.Title}, nil
}
