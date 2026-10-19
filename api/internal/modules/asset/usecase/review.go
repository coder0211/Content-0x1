package usecase

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/be/acs/api/internal/modules/asset/domain"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// SubmitInput is POST /assets/{id}/submit-review.
type SubmitInput struct {
	Comment   string
	Checklist domain.Checklist
}

// reviewPayload is the notification payload for review events.
type reviewPayload struct {
	AssetID   kernel.ID `json:"asset_id"`
	Title     string    `json:"title"`
	AssetType string    `json:"asset_type"`
	VersionNo int       `json:"version_no"`
	ActorID   kernel.ID `json:"actor_id"`
	ActorName string    `json:"actor_name"`
	Comment   *string   `json:"comment,omitempty"`
}

func trimComment(c string) (*string, error) {
	c = strings.TrimSpace(c)
	if c == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(c) > domain.MaxCommentLen {
		return nil, kernel.Validation(kernel.FieldIssue("comment", "too_long"))
	}
	return &c, nil
}

// SubmitReview moves draft → pending_review and notifies reviewers.
func (s *Service) SubmitReview(ctx context.Context, actor kernel.Actor, id kernel.ID, in SubmitInput) (Detail, error) {
	comment, err := trimComment(in.Comment)
	if err != nil {
		return Detail{}, err
	}
	var out *domain.Asset
	err = s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		a, err := s.lockEditable(ctx, actor, id)
		if err != nil {
			return err
		}
		before := snapshot(a)
		if err := a.Submit(in.Checklist, s.Clock.Now()); err != nil {
			return err
		}
		checklist, _ := json.Marshal(in.Checklist)
		if err := s.decide(ctx, actor, a, before, DecisionSubmitted, "asset.submit_review", comment, checklist); err != nil {
			return err
		}
		out = a
		return s.notifyReviewers(ctx, actor, a, comment)
	})
	if err != nil {
		return Detail{}, err
	}
	return s.detail(ctx, actor, out)
}

// Approve moves pending_review → approved (asset.review; no self-approval
// except Admin) and notifies the creator.
func (s *Service) Approve(ctx context.Context, actor kernel.Actor, id kernel.ID, commentIn string) (Detail, error) {
	return s.review(ctx, actor, id, commentIn, DecisionApproved)
}

// RequestChanges moves pending_review → changes_requested (comment
// required) and notifies the creator.
func (s *Service) RequestChanges(ctx context.Context, actor kernel.Actor, id kernel.ID, commentIn string) (Detail, error) {
	return s.review(ctx, actor, id, commentIn, DecisionChangesRequested)
}

func (s *Service) review(ctx context.Context, actor kernel.Actor, id kernel.ID, commentIn string, d Decision) (Detail, error) {
	if err := actor.Require(kernel.PermAssetReview); err != nil {
		return Detail{}, err
	}
	comment, err := trimComment(commentIn)
	if err != nil {
		return Detail{}, err
	}
	var out *domain.Asset
	err = s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		a, err := s.Repo.GetForUpdate(ctx, id)
		if err != nil {
			return err
		}
		before := snapshot(a)
		action, notifyType := "asset.approve", ports.NotifyAssetApproved
		if d == DecisionApproved {
			err = a.Approve(actor.UserID, actor.IsAdmin())
		} else {
			action, notifyType = "asset.request_changes", ports.NotifyChangesRequested
			c := ""
			if comment != nil {
				c = *comment
			}
			err = a.RequestChanges(c)
		}
		if err != nil {
			return err
		}
		if err := s.decide(ctx, actor, a, before, d, action, comment, nil); err != nil {
			return err
		}
		out = a
		return s.Notifier.Notify(ctx, []kernel.ID{a.CreatedBy}, notifyType, s.payload(actor, a, comment))
	})
	if err != nil {
		return Detail{}, err
	}
	return s.detail(ctx, actor, out)
}

// decide saves the asset, appends the approvals row and audits.
func (s *Service) decide(ctx context.Context, actor kernel.Actor, a *domain.Asset, before auditView, d Decision,
	action string, comment *string, checklist json.RawMessage,
) error {
	a.UpdatedAt = s.Clock.Now()
	if err := s.Repo.Save(ctx, a); err != nil {
		return err
	}
	if checklist == nil {
		checklist = json.RawMessage(`{}`)
	}
	if a.CurrentVersionID == nil {
		return kernel.ErrInternal.WithMessage("Asset chưa có phiên bản")
	}
	if err := s.Repo.AddApproval(ctx, Approval{
		ID: kernel.NewID(), AssetID: a.ID, VersionID: *a.CurrentVersionID, VersionNo: a.CurrentVersionNo,
		ReviewerID: actor.UserID, Decision: d, Comment: comment, Checklist: checklist, CreatedAt: s.Clock.Now(),
	}); err != nil {
		return err
	}
	after := struct {
		auditView
		Comment *string `json:"comment"`
	}{snapshot(a), comment}
	return s.Audit.Record(ctx, action, "asset", a.ID.String(), before, after)
}

func (s *Service) payload(actor kernel.Actor, a *domain.Asset, comment *string) reviewPayload {
	return reviewPayload{AssetID: a.ID, Title: a.Title, AssetType: string(a.Type), VersionNo: a.CurrentVersionNo,
		ActorID: actor.UserID, ActorName: actor.Name, Comment: comment}
}

// notifyReviewers notifies every active user holding asset.review except
// the submitter. Looking reviewers up must not block the submission.
func (s *Service) notifyReviewers(ctx context.Context, actor kernel.Actor, a *domain.Asset, comment *string) error {
	if s.Identity == nil {
		return nil
	}
	users, err := s.Identity.UsersWithPermission(ctx, kernel.PermAssetReview)
	if err != nil {
		s.log().WarnContext(ctx, "asset: list reviewers", slog.Any("err", err))
		return nil
	}
	ids := make([]kernel.ID, 0, len(users))
	for _, u := range users {
		if u.ID != actor.UserID {
			ids = append(ids, u.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return s.Notifier.Notify(ctx, ids, ports.NotifyReviewRequested, s.payload(actor, a, comment))
}
