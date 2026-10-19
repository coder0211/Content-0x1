package usecase

import (
	"context"
	"encoding/json"

	"github.com/be/acs/api/internal/modules/provider/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

// canManageTemplates: FR-CNT-08 lets Admin and Marketing Lead manage prompt
// templates; Lead has no provider.manage, so the role is checked too.
func canManageTemplates(a kernel.Actor) error {
	if a.Can(kernel.PermProviderManage) || a.HasRole(kernel.RoleMarketingLead) {
		return nil
	}
	return kernel.ErrForbidden
}

// ListTemplates lists template versions, optionally for one code, ordered
// by code then version descending (provider.view).
func (s *Service) ListTemplates(ctx context.Context, actor kernel.Actor, code string) ([]domain.PromptTemplate, error) {
	if err := actor.Require(kernel.PermProviderView); err != nil {
		return nil, err
	}
	return s.Repo.ListTemplates(ctx, code)
}

// GetTemplate returns one version (provider.view).
func (s *Service) GetTemplate(ctx context.Context, actor kernel.Actor, id kernel.ID) (*domain.PromptTemplate, error) {
	if err := actor.Require(kernel.PermProviderView); err != nil {
		return nil, err
	}
	return s.Repo.GetTemplate(ctx, id)
}

// CreateTemplateInput creates the next version of a template code.
type CreateTemplateInput struct {
	Code         string
	SystemPrompt string
	UserTemplate string
	OutputSchema json.RawMessage
	Activate     bool
}

func auditTemplate(t domain.PromptTemplate) map[string]any {
	return map[string]any{"code": t.Code, "version": t.Version, "status": t.Status}
}

// CreateTemplateVersion allocates version max+1 of code (draft, or active
// when Activate is set — the previous active version is retired). Versions
// are immutable afterwards (DB trigger, migration 0071).
func (s *Service) CreateTemplateVersion(ctx context.Context, actor kernel.Actor, in CreateTemplateInput) (*domain.PromptTemplate, error) {
	if err := canManageTemplates(actor); err != nil {
		return nil, err
	}
	// Validate shape before taking locks.
	if _, err := domain.NewTemplateVersion(in.Code, 1, in.SystemPrompt, in.UserTemplate, in.OutputSchema, actor.UserID, s.Clock.Now()); err != nil {
		return nil, err
	}
	var out *domain.PromptTemplate
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.Repo.LockTemplateCode(ctx, in.Code); err != nil {
			return err
		}
		maxV, err := s.Repo.MaxTemplateVersion(ctx, in.Code)
		if err != nil {
			return err
		}
		t, err := domain.NewTemplateVersion(in.Code, maxV+1, in.SystemPrompt, in.UserTemplate, in.OutputSchema, actor.UserID, s.Clock.Now())
		if err != nil {
			return err
		}
		if err := s.Repo.CreateTemplate(ctx, t); err != nil {
			return err
		}
		if err := s.Audit.Record(ctx, "prompt_template.create", "prompt_template", t.ID.String(), nil, auditTemplate(*t)); err != nil {
			return err
		}
		if in.Activate {
			if err := s.activate(ctx, t); err != nil {
				return err
			}
		}
		out = t
		return nil
	})
	return out, err
}

// activate retires the current active version of t.Code and activates t.
func (s *Service) activate(ctx context.Context, t *domain.PromptTemplate) error {
	cur, err := s.Repo.ActiveTemplate(ctx, t.Code)
	switch {
	case err == nil && cur.ID != t.ID:
		before := auditTemplate(*cur)
		if err := cur.Retire(); err != nil {
			return err
		}
		if err := s.Repo.SetTemplateStatus(ctx, cur.ID, cur.Status); err != nil {
			return err
		}
		if err := s.Audit.Record(ctx, "prompt_template.retire", "prompt_template", cur.ID.String(), before, auditTemplate(*cur)); err != nil {
			return err
		}
	case err != nil && kernel.ErrorCode(err) != kernel.CodeNotFound:
		return err
	}
	before := auditTemplate(*t)
	if err := t.Activate(); err != nil {
		return err
	}
	if err := s.Repo.SetTemplateStatus(ctx, t.ID, t.Status); err != nil {
		return err
	}
	return s.Audit.Record(ctx, "prompt_template.activate", "prompt_template", t.ID.String(), before, auditTemplate(*t))
}

// ActivateTemplate makes a version the active one for its code. New jobs
// use it; jobs that already ran keep their prompt_template_id.
func (s *Service) ActivateTemplate(ctx context.Context, actor kernel.Actor, id kernel.ID) (*domain.PromptTemplate, error) {
	if err := canManageTemplates(actor); err != nil {
		return nil, err
	}
	var out *domain.PromptTemplate
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		t, err := s.Repo.GetTemplate(ctx, id)
		if err != nil {
			return err
		}
		if err := s.Repo.LockTemplateCode(ctx, t.Code); err != nil {
			return err
		}
		if t, err = s.Repo.GetTemplateForUpdate(ctx, id); err != nil {
			return err
		}
		if err := s.activate(ctx, t); err != nil {
			return err
		}
		out = t
		return nil
	})
	return out, err
}

// RetireTemplate takes a version out of use. Retiring the active version
// leaves the code without an active template (ActivePromptTemplate →
// NOT_FOUND) until another version is activated.
func (s *Service) RetireTemplate(ctx context.Context, actor kernel.Actor, id kernel.ID) (*domain.PromptTemplate, error) {
	if err := canManageTemplates(actor); err != nil {
		return nil, err
	}
	var out *domain.PromptTemplate
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		t, err := s.Repo.GetTemplateForUpdate(ctx, id)
		if err != nil {
			return err
		}
		before := auditTemplate(*t)
		if err := t.Retire(); err != nil {
			return err
		}
		if err := s.Repo.SetTemplateStatus(ctx, t.ID, t.Status); err != nil {
			return err
		}
		out = t
		return s.Audit.Record(ctx, "prompt_template.retire", "prompt_template", t.ID.String(), before, auditTemplate(*t))
	})
	return out, err
}
