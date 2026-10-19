// Package usecase is the public contract of the asset module (content
// assets, versions and their files). Content/image/video modules create and
// version assets only through API.
package usecase

import (
	"context"
	"encoding/json"
	"time"

	"github.com/be/acs/api/internal/shared/kernel"
)

// AssetType mirrors content_assets.asset_type.
type AssetType string

const (
	TypeContent AssetType = "content"
	TypeImage   AssetType = "image"
	TypeVideo   AssetType = "video"
)

// Status mirrors content_assets.status.
type Status string

const (
	StatusDraft            Status = "draft"
	StatusPendingReview    Status = "pending_review"
	StatusChangesRequested Status = "changes_requested"
	StatusApproved         Status = "approved"
	StatusArchived         Status = "archived"
)

// FileRole mirrors asset_version_files.role.
type FileRole string

const (
	RolePrimary   FileRole = "primary"
	RoleVariant   FileRole = "variant"
	RoleThumbnail FileRole = "thumbnail"
	RoleSubtitle  FileRole = "subtitle"
	RoleAudio     FileRole = "audio"
	RolePreview   FileRole = "preview"
)

// VersionFile links a file (from the file module) to a version.
type VersionFile struct {
	FileID    kernel.ID
	Role      FileRole
	SortOrder int
}

// NewVersion is the content of a version to append.
type NewVersion struct {
	// ContentJSON follows the per-type schema documented in migration 0006.
	ContentJSON      json.RawMessage
	ChangeNote       string
	GeneratedByJobID *kernel.ID
	CreatedBy        kernel.ID
	Files            []VersionFile
	// SearchText feeds search_tsv (title is added automatically), e.g. the
	// body_text of a content asset or the prompt of an image.
	SearchText string
}

// CreateAssetInput creates an asset with its version 1.
type CreateAssetInput struct {
	CampaignID    *kernel.ID
	BrandID       kernel.ID
	Type          AssetType
	Subtype       string
	Title         string
	SourceAssetID *kernel.ID
	Tags          []string
	CreatedBy     kernel.ID
	Version       NewVersion
}

// AssetRef identifies a created asset/version.
type AssetRef struct {
	AssetID   kernel.ID
	VersionID kernel.ID
	VersionNo int
}

// VersionView is a version with its files.
type VersionView struct {
	ID               kernel.ID
	VersionNo        int
	ContentJSON      json.RawMessage
	ChangeNote       string
	GeneratedByJobID *kernel.ID
	CreatedBy        kernel.ID
	CreatedAt        time.Time
	Files            []VersionFile
}

// AssetView is an asset with its current version.
type AssetView struct {
	ID             kernel.ID
	CampaignID     *kernel.ID
	BrandID        kernel.ID
	Type           AssetType
	Subtype        string
	Title          string
	Status         Status
	SourceAssetID  *kernel.ID
	Tags           []string
	CreatedBy      kernel.ID
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CurrentVersion *VersionView
}

// API is the asset module's public interface. Methods are safe to call
// inside the caller's transaction (they join it via ctx).
type API interface {
	// CreateAsset inserts the asset and version 1 (+ files) atomically.
	CreateAsset(ctx context.Context, in CreateAssetInput) (AssetRef, error)
	// AddVersion appends version N+1 and makes it current; an approved
	// asset goes back to draft (must be reviewed again).
	AddVersion(ctx context.Context, assetID kernel.ID, v NewVersion) (AssetRef, error)
	// AttachFiles adds files to an existing version (e.g. thumbnail later).
	AttachFiles(ctx context.Context, versionID kernel.ID, files []VersionFile) error
	// Get returns the asset with its current version (NOT_FOUND if deleted).
	Get(ctx context.Context, assetID kernel.ID) (AssetView, error)
	// GetVersion returns a specific version.
	GetVersion(ctx context.Context, assetID kernel.ID, versionNo int) (VersionView, error)
	// CanEdit returns nil when actor may modify the asset (owner, campaign
	// member, or Lead/Admin), NOT_FOUND/FORBIDDEN otherwise (IDOR guard).
	CanEdit(ctx context.Context, actor kernel.Actor, assetID kernel.ID) error
}
