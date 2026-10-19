package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/be/acs/api/internal/modules/campaign/domain"
	"github.com/be/acs/api/internal/modules/campaign/usecase"
	db "github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Repository implements usecase.Repository on PostgreSQL.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository returns a repository on pool.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

var _ usecase.Repository = (*Repository)(nil)

func (r *Repository) q(ctx context.Context) *Queries { return New(db.Conn(ctx, r.pool)) }

func toDate(s string) pgtype.Date {
	if s == "" {
		return pgtype.Date{}
	}
	t, err := time.Parse(domain.DateLayout, s)
	if err != nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: t, Valid: true}
}

func fromDate(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format(domain.DateLayout)
}

func planJSON(p *domain.Plan) json.RawMessage {
	if p == nil {
		return nil
	}
	b, _ := json.Marshal(p)
	return b
}

func toDomain(c Campaign) domain.Campaign {
	var brief domain.Brief
	_ = json.Unmarshal(c.Brief, &brief)
	plan, _ := domain.ParsePlan(c.ContentPlan)
	proposal, _ := domain.ParsePlan(c.ContentPlanProposal)
	return domain.Campaign{ID: c.ID, BrandID: c.BrandID, Name: c.Name, Goal: c.Goal, Brief: brief.Normalize(), Status: domain.Status(c.Status),
		StartDate: fromDate(c.StartDate), EndDate: fromDate(c.EndDate), CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
		MemberIDs: []kernel.ID{}, ContentPlan: plan, ContentPlanApprovedBy: c.ContentPlanApprovedBy, ContentPlanApprovedAt: c.ContentPlanApprovedAt,
		PlanProposal: proposal, PlanProposedAt: c.ContentPlanProposedAt}
}

func (r *Repository) withMembers(ctx context.Context, cs []domain.Campaign) error {
	if len(cs) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(cs))
	idx := map[uuid.UUID]int{}
	for i, c := range cs {
		ids[i] = c.ID
		idx[c.ID] = i
	}
	rows, err := r.q(ctx).ListMembers(ctx, ids)
	if err != nil {
		return err
	}
	for _, m := range rows {
		i := idx[m.CampaignID]
		cs[i].MemberIDs = append(cs[i].MemberIDs, m.UserID)
	}
	return nil
}

// Create implements usecase.Repository.
func (r *Repository) Create(ctx context.Context, c *domain.Campaign) error {
	brief, err := json.Marshal(c.Brief)
	if err != nil {
		return err
	}
	return r.q(ctx).InsertCampaign(ctx, InsertCampaignParams{ID: c.ID, BrandID: c.BrandID, Name: c.Name, Goal: c.Goal, Brief: brief,
		Status: string(c.Status), StartDate: toDate(c.StartDate), EndDate: toDate(c.EndDate), CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt})
}

func (r *Repository) one(ctx context.Context, row Campaign, err error) (*domain.Campaign, error) {
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	cs := []domain.Campaign{toDomain(row)}
	if err := r.withMembers(ctx, cs); err != nil {
		return nil, err
	}
	return &cs[0], nil
}

// Get implements usecase.Repository.
func (r *Repository) Get(ctx context.Context, id kernel.ID) (*domain.Campaign, error) {
	row, err := r.q(ctx).GetCampaign(ctx, id)
	return r.one(ctx, row, err)
}

// GetForUpdate implements usecase.Repository.
func (r *Repository) GetForUpdate(ctx context.Context, id kernel.ID) (*domain.Campaign, error) {
	row, err := r.q(ctx).GetCampaignForUpdate(ctx, id)
	return r.one(ctx, row, err)
}

// Update implements usecase.Repository.
func (r *Repository) Update(ctx context.Context, c *domain.Campaign) error {
	brief, err := json.Marshal(c.Brief)
	if err != nil {
		return err
	}
	return r.q(ctx).UpdateCampaign(ctx, UpdateCampaignParams{ID: c.ID, Name: c.Name, Goal: c.Goal, Brief: brief, Status: string(c.Status),
		StartDate: toDate(c.StartDate), EndDate: toDate(c.EndDate), ContentPlan: planJSON(c.ContentPlan),
		ContentPlanProposal: planJSON(c.PlanProposal), ContentPlanProposedAt: c.PlanProposedAt,
		ContentPlanApprovedBy: c.ContentPlanApprovedBy, ContentPlanApprovedAt: c.ContentPlanApprovedAt})
}

// SetMembers implements usecase.Repository.
func (r *Repository) SetMembers(ctx context.Context, id kernel.ID, members []kernel.ID) error {
	q := r.q(ctx)
	if err := q.DeleteMembers(ctx, id); err != nil {
		return err
	}
	for _, m := range members {
		if err := q.InsertMember(ctx, InsertMemberParams{CampaignID: id, UserID: m}); err != nil {
			return err
		}
	}
	return nil
}

// List implements usecase.Repository.
func (r *Repository) List(ctx context.Context, f usecase.Filter, p kernel.PageRequest) (kernel.Page[domain.Campaign], error) {
	params := ListCampaignsParams{BrandID: f.BrandID, MemberID: f.MemberID, Lim: int32(p.Limit + 1)}
	if f.Status != "" {
		params.Status = &f.Status
	}
	if f.Q != "" {
		params.Q = &f.Q
	}
	var c kernel.TimeIDCursor
	if ok, err := kernel.DecodeCursor(p.Cursor, &c); err != nil {
		return kernel.Page[domain.Campaign]{}, err
	} else if ok {
		params.CursorAt, params.CursorID = &c.At, &c.ID
	}
	rows, err := r.q(ctx).ListCampaigns(ctx, params)
	if err != nil {
		return kernel.Page[domain.Campaign]{}, err
	}
	out := make([]domain.Campaign, len(rows))
	for i, row := range rows {
		out[i] = toDomain(row)
	}
	page := kernel.PageFromRows(out, p.Limit, func(c domain.Campaign) any { return kernel.TimeIDCursor{At: c.CreatedAt, ID: c.ID} })
	if err := r.withMembers(ctx, page.Items); err != nil {
		return kernel.Page[domain.Campaign]{}, err
	}
	return page, nil
}
