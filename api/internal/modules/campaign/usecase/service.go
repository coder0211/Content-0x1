package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	brand "github.com/be/acs/api/internal/modules/brand/usecase"
	"github.com/be/acs/api/internal/modules/campaign/domain"
	identity "github.com/be/acs/api/internal/modules/identity/usecase"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Service implements the campaign use cases and the public API.
type Service struct {
	Tx     ports.TxManager
	Repo   Repository
	Clock  ports.Clock
	Audit  ports.AuditRecorder
	Brands Brands
	Users  Users
}

var _ API = (*Service)(nil)

const resCampaign = "campaign"

// Member is a campaign member for display.
type Member struct {
	ID    kernel.ID
	Name  string
	Email string
}

// View is a campaign with member details.
type View struct {
	domain.Campaign
	Members []Member
}

// errArchived is returned for edits of an archived campaign.
var errArchived = kernel.ErrInvalidTransition.WithMessage("Campaign đã lưu trữ, không thể chỉnh sửa")

// ---------- queries ----------

// List lists campaigns (brand.view). Mine keeps only the actor's campaigns.
func (s *Service) List(ctx context.Context, actor kernel.Actor, f Filter, mine bool, p kernel.PageRequest) (kernel.Page[domain.Campaign], error) {
	if err := actor.Require(kernel.PermBrandView); err != nil {
		return kernel.Page[domain.Campaign]{}, err
	}
	if f.Status != "" && !domain.IsStatus(f.Status) {
		return kernel.Page[domain.Campaign]{}, kernel.Validation(kernel.FieldIssue("status", "invalid"))
	}
	f.Q = strings.TrimSpace(f.Q)
	if mine {
		f.MemberID = &actor.UserID
	}
	return s.Repo.List(ctx, f, p.Normalized())
}

// Get returns one campaign with members (brand.view).
func (s *Service) Get(ctx context.Context, actor kernel.Actor, id kernel.ID) (*View, error) {
	if err := actor.Require(kernel.PermBrandView); err != nil {
		return nil, err
	}
	c, err := s.Repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.view(ctx, c)
}

func (s *Service) view(ctx context.Context, c *domain.Campaign) (*View, error) {
	v := &View{Campaign: *c, Members: []Member{}}
	if len(c.MemberIDs) == 0 {
		return v, nil
	}
	us, err := s.Users.GetUsers(ctx, c.MemberIDs)
	if err != nil {
		return nil, err
	}
	for _, u := range us {
		v.Members = append(v.Members, Member{ID: u.ID, Name: u.Name, Email: u.Email})
	}
	return v, nil
}

// MemberCandidates lists active users who can generate content, for the
// member picker (campaign.manage; /users needs user.manage).
func (s *Service) MemberCandidates(ctx context.Context, actor kernel.Actor) ([]Member, error) {
	if err := actor.Require(kernel.PermCampaignManage); err != nil {
		return nil, err
	}
	us, err := s.Users.UsersWithPermission(ctx, kernel.PermGenerateRun)
	if err != nil {
		return nil, err
	}
	out := make([]Member, 0, len(us))
	for _, u := range us {
		out = append(out, Member{ID: u.ID, Name: u.Name, Email: u.Email})
	}
	return out, nil
}

// ---------- commands ----------

// CreateInput is POST /campaigns.
type CreateInput struct {
	BrandID   kernel.ID
	Name      string
	Goal      string
	Brief     domain.Brief
	StartDate string
	EndDate   string
	MemberIDs []kernel.ID
}

// Create creates a draft campaign (campaign.manage).
func (s *Service) Create(ctx context.Context, actor kernel.Actor, in CreateInput) (*View, error) {
	if err := actor.Require(kernel.PermCampaignManage); err != nil {
		return nil, err
	}
	now := s.Clock.Now()
	c := &domain.Campaign{ID: kernel.NewID(), BrandID: in.BrandID, Name: strings.TrimSpace(in.Name), Goal: strings.TrimSpace(in.Goal),
		Brief: in.Brief.Normalize(), Status: domain.StatusDraft, StartDate: strings.TrimSpace(in.StartDate), EndDate: strings.TrimSpace(in.EndDate),
		CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now, MemberIDs: dedupe(in.MemberIDs, actor.UserID)}
	var v kernel.ValidationErrors
	if in.BrandID == kernel.NilID {
		v.Add("brand_id", "required")
	}
	domain.ValidateHeader(&v, c.Name, c.Goal, c.StartDate, c.EndDate)
	c.Brief.Validate(&v, "brief")
	if len(c.MemberIDs) > domain.MaxMembers {
		v.Add("member_ids", "too_many")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	if err := s.checkBrand(ctx, c.BrandID, c.Brief.ProductIDs); err != nil {
		return nil, err
	}
	if err := s.checkMembers(ctx, c.MemberIDs); err != nil {
		return nil, err
	}
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.Repo.Create(ctx, c); err != nil {
			return err
		}
		if err := s.Repo.SetMembers(ctx, c.ID, c.MemberIDs); err != nil {
			return err
		}
		return s.Audit.Record(ctx, "campaign.created", resCampaign, c.ID.String(), nil, auditCampaign(c))
	})
	if err != nil {
		return nil, err
	}
	return s.view(ctx, c)
}

// UpdateInput is PATCH /campaigns/{id}; nil = unchanged.
type UpdateInput struct {
	Name      *string
	Goal      *string
	Brief     *domain.Brief
	StartDate *string
	EndDate   *string
	Status    *string
	MemberIDs *[]kernel.ID
}

// Update edits the brief/header, changes status (state machine) and
// replaces members. Creators may edit only campaigns they belong to;
// members can be changed by the creator of the campaign or Lead/Admin.
func (s *Service) Update(ctx context.Context, actor kernel.Actor, id kernel.ID, in UpdateInput) (*View, error) {
	if err := actor.Require(kernel.PermCampaignManage); err != nil {
		return nil, err
	}
	var out *domain.Campaign
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		c, err := s.Repo.GetForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if !c.CanEdit(actor) {
			return kernel.ErrForbidden
		}
		before := auditCampaign(c)
		editsContent := in.Name != nil || in.Goal != nil || in.Brief != nil || in.StartDate != nil || in.EndDate != nil || in.MemberIDs != nil
		if c.Status == domain.StatusArchived && (editsContent || (in.Status != nil && *in.Status != string(domain.StatusArchived))) {
			return errArchived
		}
		if in.Name != nil {
			c.Name = strings.TrimSpace(*in.Name)
		}
		if in.Goal != nil {
			c.Goal = strings.TrimSpace(*in.Goal)
		}
		if in.StartDate != nil {
			c.StartDate = strings.TrimSpace(*in.StartDate)
		}
		if in.EndDate != nil {
			c.EndDate = strings.TrimSpace(*in.EndDate)
		}
		var v kernel.ValidationErrors
		domain.ValidateHeader(&v, c.Name, c.Goal, c.StartDate, c.EndDate)
		if in.Brief != nil {
			c.Brief = in.Brief.Normalize()
			c.Brief.Validate(&v, "brief")
		}
		if in.MemberIDs != nil {
			if !actor.IsLeadOrAdmin() && c.CreatedBy != actor.UserID {
				return kernel.ErrForbidden.WithMessage("Chỉ người tạo campaign hoặc Marketing Lead được đổi thành viên")
			}
			c.MemberIDs = dedupe(*in.MemberIDs, kernel.NilID)
			if len(c.MemberIDs) > domain.MaxMembers {
				v.Add("member_ids", "too_many")
			}
		}
		if err := v.Err(); err != nil {
			return err
		}
		if in.Status != nil {
			if err := c.TransitionTo(domain.Status(*in.Status)); err != nil {
				return err
			}
		}
		if in.Brief != nil {
			if err := s.checkBrand(ctx, c.BrandID, c.Brief.ProductIDs); err != nil {
				return err
			}
		}
		if in.MemberIDs != nil {
			if err := s.checkMembers(ctx, c.MemberIDs); err != nil {
				return err
			}
			if err := s.Repo.SetMembers(ctx, c.ID, c.MemberIDs); err != nil {
				return err
			}
		}
		c.UpdatedAt = s.Clock.Now()
		if err := s.Repo.Update(ctx, c); err != nil {
			return err
		}
		out = c
		return s.Audit.Record(ctx, "campaign.updated", resCampaign, c.ID.String(), before, auditCampaign(c))
	})
	if err != nil {
		return nil, err
	}
	return s.view(ctx, out)
}

// SavePlan stores the approved (possibly edited) content plan
// (PUT /campaigns/{id}/plan, ACS-037) and clears the pending proposal.
func (s *Service) SavePlan(ctx context.Context, actor kernel.Actor, id kernel.ID, plan domain.Plan) (*View, error) {
	if err := actor.Require(kernel.PermCampaignManage); err != nil {
		return nil, err
	}
	p := plan.Normalize()
	var v kernel.ValidationErrors
	p.Validate(&v, "plan")
	if err := v.Err(); err != nil {
		return nil, err
	}
	var out *domain.Campaign
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		c, err := s.Repo.GetForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if !c.CanEdit(actor) {
			return kernel.ErrForbidden
		}
		if c.Status == domain.StatusArchived {
			return errArchived
		}
		before := map[string]any{"content_plan": c.ContentPlan}
		now := s.Clock.Now()
		by := actor.UserID
		c.ContentPlan, c.ContentPlanApprovedBy, c.ContentPlanApprovedAt = &p, &by, &now
		c.PlanProposal, c.PlanProposedAt = nil, nil
		c.UpdatedAt = now
		if err := s.Repo.Update(ctx, c); err != nil {
			return err
		}
		out = c
		return s.Audit.Record(ctx, "campaign.plan_approved", resCampaign, c.ID.String(), before, map[string]any{"content_plan": c.ContentPlan})
	})
	if err != nil {
		return nil, err
	}
	return s.view(ctx, out)
}

// DiscardProposal drops a pending AI plan proposal.
func (s *Service) DiscardProposal(ctx context.Context, actor kernel.Actor, id kernel.ID) error {
	if err := actor.Require(kernel.PermCampaignManage); err != nil {
		return err
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		c, err := s.Repo.GetForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if !c.CanEdit(actor) {
			return kernel.ErrForbidden
		}
		if c.PlanProposal == nil {
			return nil
		}
		c.PlanProposal, c.PlanProposedAt = nil, nil
		c.UpdatedAt = s.Clock.Now()
		if err := s.Repo.Update(ctx, c); err != nil {
			return err
		}
		return s.Audit.Record(ctx, "campaign.plan_proposal_discarded", resCampaign, c.ID.String(), nil, nil)
	})
}

// checkBrand verifies the brand exists and every product belongs to it.
func (s *Service) checkBrand(ctx context.Context, brandID kernel.ID, products []kernel.ID) error {
	if _, err := s.Brands.GetBrandContext(ctx, brand.BrandContextRequest{BrandID: brandID}); err != nil {
		if errors.Is(err, kernel.ErrNotFound) {
			return kernel.Validation(kernel.FieldIssue("brand_id", "not_found"))
		}
		return err
	}
	if len(products) == 0 {
		return nil
	}
	if _, err := s.Brands.GetBrandContext(ctx, brand.BrandContextRequest{BrandID: brandID, ProductIDs: products}); err != nil {
		if errors.Is(err, kernel.ErrNotFound) {
			return kernel.Validation(kernel.FieldIssue("brief.product_ids", "not_found"))
		}
		return err
	}
	return nil
}

// checkMembers verifies every member is an existing active user.
func (s *Service) checkMembers(ctx context.Context, ids []kernel.ID) error {
	if len(ids) == 0 {
		return nil
	}
	us, err := s.Users.GetUsers(ctx, ids)
	if err != nil {
		return err
	}
	for _, id := range ids {
		i := slices.IndexFunc(us, func(u identity.UserSummary) bool { return u.ID == id })
		if i < 0 || us[i].Status != "active" {
			return kernel.Validation(kernel.Detail{"field": "member_ids", "issue": "not_found", "user_id": id})
		}
	}
	return nil
}

func dedupe(ids []kernel.ID, skip kernel.ID) []kernel.ID {
	out := make([]kernel.ID, 0, len(ids))
	for _, id := range ids {
		if id != kernel.NilID && id != skip && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

func auditCampaign(c *domain.Campaign) map[string]any {
	return map[string]any{"brand_id": c.BrandID, "name": c.Name, "goal": c.Goal, "status": c.Status, "brief": c.Brief,
		"start_date": c.StartDate, "end_date": c.EndDate, "member_ids": c.MemberIDs}
}

// ---------- public API ----------

// GetCampaignForGeneration implements API.
func (s *Service) GetCampaignForGeneration(ctx context.Context, actor kernel.Actor, id kernel.ID) (CampaignForGeneration, error) {
	if err := actor.Require(kernel.PermGenerateRun); err != nil {
		return CampaignForGeneration{}, err
	}
	c, err := s.Repo.Get(ctx, id)
	if err != nil {
		return CampaignForGeneration{}, err
	}
	if !c.Status.AcceptsGeneration() {
		return CampaignForGeneration{}, kernel.InvalidTransition(string(c.Status), "generate").
			WithMessage("Campaign đã hoàn thành hoặc lưu trữ, không thể tạo thêm nội dung")
	}
	out := CampaignForGeneration{ID: c.ID, BrandID: c.BrandID, Name: c.Name, Goal: c.Goal, Status: Status(c.Status),
		MemberIDs: slices.Clone(c.MemberIDs), CreatedBy: c.CreatedBy,
		Brief: Brief{ProductIDs: slices.Clone(c.Brief.ProductIDs), TargetAudience: c.Brief.TargetAudience, KeyMessage: c.Brief.KeyMessage,
			VerifiedOffer: c.Brief.VerifiedOffer, Channels: slices.Clone(c.Brief.Channels), Deadline: c.Brief.Deadline, Notes: c.Brief.Notes}}
	for _, f := range c.Brief.Formats {
		out.Brief.Formats = append(out.Brief.Formats, Format{Type: f.Type, Count: f.Count})
	}
	if c.ContentPlan != nil {
		out.ContentPlan, _ = json.Marshal(c.ContentPlan)
	}
	return out, nil
}

// IsMember implements API.
func (s *Service) IsMember(ctx context.Context, id, userID kernel.ID) (bool, error) {
	c, err := s.Repo.Get(ctx, id)
	if err != nil {
		return false, err
	}
	return c.IsMember(userID), nil
}

// SaveContentPlanProposal implements API: stores an AI plan (job
// content_plan) for the user to review. It never touches an approved plan.
func (s *Service) SaveContentPlanProposal(ctx context.Context, id kernel.ID, raw json.RawMessage) error {
	p, err := domain.ParsePlan(raw)
	if err != nil || p == nil {
		return kernel.Validation(kernel.FieldIssue("plan", "invalid"))
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		c, err := s.Repo.GetForUpdate(ctx, id)
		if err != nil {
			return err
		}
		now := s.Clock.Now()
		c.PlanProposal, c.PlanProposedAt, c.UpdatedAt = p, &now, now
		if err := s.Repo.Update(ctx, c); err != nil {
			return err
		}
		return s.Audit.Record(ctx, "campaign.plan_proposed", resCampaign, c.ID.String(), nil, map[string]any{"items": len(p.Items)})
	})
}
