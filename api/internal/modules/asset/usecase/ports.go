package usecase

import (
	"context"
	"encoding/json"
	"time"

	"github.com/be/acs/api/internal/modules/asset/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

// VersionRecord is a stored asset version with its files.
type VersionRecord struct {
	ID               kernel.ID
	AssetID          kernel.ID
	VersionNo        int
	ContentJSON      json.RawMessage
	ChangeNote       *string
	GeneratedByJobID *kernel.ID
	CreatedBy        kernel.ID
	CreatedAt        time.Time
	Files            []VersionFile
}

// Decision mirrors approvals.decision.
type Decision string

const (
	DecisionSubmitted        Decision = "submitted"
	DecisionApproved         Decision = "approved"
	DecisionChangesRequested Decision = "changes_requested"
)

// Approval is one review-history row (approvals).
type Approval struct {
	ID         kernel.ID
	AssetID    kernel.ID
	VersionID  kernel.ID
	VersionNo  int
	ReviewerID kernel.ID // actor of the decision (submitter for "submitted")
	Decision   Decision
	Comment    *string
	Checklist  json.RawMessage
	CreatedAt  time.Time
}

// Provenance is how a version was generated (FR-AST-02), read from the job
// that produced it.
type Provenance struct {
	JobID                 kernel.ID
	JobType               string
	ProviderCode          *string
	ModelCode             *string
	PromptTemplateCode    *string
	PromptTemplateVersion *int
	BrandVersionID        *kernel.ID
	BrandVersionNo        *int
}

// SearchFilter is a library query. Query is a to_tsquery expression built by
// domain.PrefixQuery ("" = no full-text condition).
type SearchFilter struct {
	Query      string
	Type       *domain.Type
	Statuses   []domain.Status
	CampaignID *kernel.ID
	BrandID    *kernel.ID
	CreatedBy  *kernel.ID
	Tag        string
	From       *time.Time
	To         *time.Time
}

// Repository persists assets, versions and approvals.
type Repository interface {
	CreateAsset(ctx context.Context, a *domain.Asset) error
	// NextVersionNo returns max(version_no)+1 (call with the asset locked).
	NextVersionNo(ctx context.Context, assetID kernel.ID) (int, error)
	// InsertVersion inserts the version and its files.
	InsertVersion(ctx context.Context, v *VersionRecord) error
	// Get / GetForUpdate load a live (not deleted) asset or NOT_FOUND.
	Get(ctx context.Context, id kernel.ID) (*domain.Asset, error)
	GetForUpdate(ctx context.Context, id kernel.ID) (*domain.Asset, error)
	// Save writes title, tags, usage_rights, status, submitted_at,
	// current_version_id and deleted_at.
	Save(ctx context.Context, a *domain.Asset) error
	// SetSearch recomputes search_tsv from title (A), body (B) and tags (C).
	SetSearch(ctx context.Context, assetID kernel.ID, body string) error
	// RefreshSearchTitle recomputes the title/tag weights, keeping the body.
	RefreshSearchTitle(ctx context.Context, assetID kernel.ID) error
	GetVersion(ctx context.Context, assetID kernel.ID, versionNo int) (*VersionRecord, error)
	GetVersionByID(ctx context.Context, versionID kernel.ID) (*VersionRecord, error)
	// ListVersions returns all versions, newest first, with files.
	ListVersions(ctx context.Context, assetID kernel.ID) ([]VersionRecord, error)
	AttachFiles(ctx context.Context, versionID kernel.ID, files []VersionFile) error
	// VersionFiles returns the files of several versions.
	VersionFiles(ctx context.Context, versionIDs []kernel.ID) (map[kernel.ID][]VersionFile, error)
	AddApproval(ctx context.Context, a Approval) error
	// ListApprovals returns the review history, newest first.
	ListApprovals(ctx context.Context, assetID kernel.ID) ([]Approval, error)
	// Search lists live assets by (created_at DESC, id DESC) after cursor.
	Search(ctx context.Context, f SearchFilter, after *kernel.TimeIDCursor, limit int) ([]domain.Asset, error)
	// Queue lists pending_review assets by (submitted_at ASC, id ASC).
	Queue(ctx context.Context, f SearchFilter, after *kernel.TimeIDCursor, limit int) ([]domain.Asset, error)
	// CountByStatus counts live assets per status (createdBy nil = all).
	CountByStatus(ctx context.Context, createdBy *kernel.ID) (map[domain.Status]int, error)
	// Provenance reads the generating jobs (read-only join on ai_jobs,
	// brand_versions, prompt_templates).
	Provenance(ctx context.Context, jobIDs []kernel.ID) (map[kernel.ID]Provenance, error)
}
