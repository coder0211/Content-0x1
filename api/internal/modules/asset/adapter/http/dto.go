package http

import (
	"encoding/json"
	"time"

	"github.com/be/acs/api/internal/modules/asset/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Response DTOs follow api/openapi/asset.yaml (snake_case, explicit nulls).

type fileDTO struct {
	FileID     kernel.ID `json:"file_id"`
	Role       string    `json:"role"`
	SortOrder  int       `json:"sort_order"`
	MimeType   string    `json:"mime_type"`
	Kind       string    `json:"kind"`
	Width      *int      `json:"width"`
	Height     *int      `json:"height"`
	DurationMs *int      `json:"duration_ms"`
	URL        *string   `json:"url"`
}

type provenanceDTO struct {
	JobID                 kernel.ID  `json:"job_id"`
	JobType               string     `json:"job_type"`
	ProviderCode          *string    `json:"provider_code"`
	ModelCode             *string    `json:"model_code"`
	PromptTemplateCode    *string    `json:"prompt_template_code"`
	PromptTemplateVersion *int       `json:"prompt_template_version"`
	BrandVersionID        *kernel.ID `json:"brand_version_id"`
	BrandVersionNo        *int       `json:"brand_version_no"`
}

type versionDTO struct {
	ID               kernel.ID       `json:"id"`
	VersionNo        int             `json:"version_no"`
	ContentJSON      json.RawMessage `json:"content_json"`
	ChangeNote       *string         `json:"change_note"`
	GeneratedByJobID *kernel.ID      `json:"generated_by_job_id"`
	CreatedBy        kernel.ID       `json:"created_by"`
	CreatedByName    string          `json:"created_by_name"`
	CreatedAt        time.Time       `json:"created_at"`
	Files            []fileDTO       `json:"files"`
	Provenance       *provenanceDTO  `json:"provenance"`
}

type summaryDTO struct {
	ID               kernel.ID  `json:"id"`
	CampaignID       *kernel.ID `json:"campaign_id"`
	BrandID          kernel.ID  `json:"brand_id"`
	Type             string     `json:"type"`
	Subtype          *string    `json:"subtype"`
	Title            string     `json:"title"`
	Status           string     `json:"status"`
	Tags             []string   `json:"tags"`
	UsageRights      *string    `json:"usage_rights"`
	SourceAssetID    *kernel.ID `json:"source_asset_id"`
	CreatedBy        kernel.ID  `json:"created_by"`
	CreatedByName    string     `json:"created_by_name"`
	CurrentVersionNo int        `json:"current_version_no"`
	ThumbnailURL     *string    `json:"thumbnail_url"`
	SubmittedAt      *time.Time `json:"submitted_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type approvalDTO struct {
	ID           kernel.ID       `json:"id"`
	VersionID    kernel.ID       `json:"version_id"`
	VersionNo    int             `json:"version_no"`
	ReviewerID   kernel.ID       `json:"reviewer_id"`
	ReviewerName string          `json:"reviewer_name"`
	Decision     string          `json:"decision"`
	Comment      *string         `json:"comment"`
	Checklist    json.RawMessage `json:"checklist"`
	CreatedAt    time.Time       `json:"created_at"`
}

type permissionsDTO struct {
	CanEdit     bool `json:"can_edit"`
	CanSubmit   bool `json:"can_submit"`
	CanReview   bool `json:"can_review"`
	CanDownload bool `json:"can_download"`
}

type detailDTO struct {
	summaryDTO
	CurrentVersion versionDTO     `json:"current_version"`
	Approvals      []approvalDTO  `json:"approvals"`
	Permissions    permissionsDTO `json:"permissions"`
}

type downloadDTO struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
	FileID    kernel.ID `json:"file_id"`
}

func optInt(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func presentVersion(v usecase.VersionDetail) versionDTO {
	out := versionDTO{
		ID: v.ID, VersionNo: v.VersionNo, ContentJSON: v.ContentJSON, ChangeNote: v.ChangeNote,
		GeneratedByJobID: v.GeneratedByJobID, CreatedBy: v.CreatedBy, CreatedByName: v.CreatedByName, CreatedAt: v.CreatedAt,
		Files: make([]fileDTO, 0, len(v.FileViews)),
	}
	if len(out.ContentJSON) == 0 {
		out.ContentJSON = json.RawMessage(`{}`)
	}
	for _, f := range v.FileViews {
		out.Files = append(out.Files, fileDTO{
			FileID: f.FileID, Role: string(f.Role), SortOrder: f.SortOrder, MimeType: f.MimeType, Kind: f.Kind,
			Width: optInt(f.Width), Height: optInt(f.Height), DurationMs: optInt(f.DurationMs), URL: f.URL,
		})
	}
	if p := v.Provenance; p != nil {
		out.Provenance = &provenanceDTO{
			JobID: p.JobID, JobType: p.JobType, ProviderCode: p.ProviderCode, ModelCode: p.ModelCode,
			PromptTemplateCode: p.PromptTemplateCode, PromptTemplateVersion: p.PromptTemplateVersion,
			BrandVersionID: p.BrandVersionID, BrandVersionNo: p.BrandVersionNo,
		}
	}
	return out
}

func presentSummary(s usecase.Summary) summaryDTO {
	a := s.Asset
	tags := a.Tags
	if tags == nil {
		tags = []string{}
	}
	return summaryDTO{
		ID: a.ID, CampaignID: a.CampaignID, BrandID: a.BrandID, Type: string(a.Type), Subtype: optStr(a.Subtype),
		Title: a.Title, Status: string(a.Status), Tags: tags, UsageRights: a.UsageRights, SourceAssetID: a.SourceAssetID,
		CreatedBy: a.CreatedBy, CreatedByName: s.CreatedByName, CurrentVersionNo: a.CurrentVersionNo,
		ThumbnailURL: s.ThumbnailURL, SubmittedAt: a.SubmittedAt, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}

func presentSummaries(items []usecase.Summary) []summaryDTO {
	out := make([]summaryDTO, 0, len(items))
	for _, s := range items {
		out = append(out, presentSummary(s))
	}
	return out
}

func presentDetail(d usecase.Detail) detailDTO {
	out := detailDTO{
		summaryDTO: presentSummary(d.Summary), CurrentVersion: presentVersion(d.Current),
		Approvals: make([]approvalDTO, 0, len(d.Approvals)),
		Permissions: permissionsDTO{CanEdit: d.Permissions.CanEdit, CanSubmit: d.Permissions.CanSubmit,
			CanReview: d.Permissions.CanReview, CanDownload: d.Permissions.CanDownload},
	}
	for _, a := range d.Approvals {
		cl := a.Checklist
		if len(cl) == 0 {
			cl = json.RawMessage(`{}`)
		}
		out.Approvals = append(out.Approvals, approvalDTO{
			ID: a.ID, VersionID: a.VersionID, VersionNo: a.VersionNo, ReviewerID: a.ReviewerID, ReviewerName: a.ReviewerName,
			Decision: string(a.Decision), Comment: a.Comment, Checklist: cl, CreatedAt: a.CreatedAt,
		})
	}
	return out
}

// Requests.

type updateRequest struct {
	Title       *string         `json:"title"`
	Tags        *[]string       `json:"tags"`
	UsageRights json.RawMessage `json:"usage_rights"` // absent = unchanged, null = clear
}

type versionCreateRequest struct {
	ContentJSON   json.RawMessage `json:"content_json"`
	ChangeNote    string          `json:"change_note"`
	BaseVersionNo int             `json:"base_version_no"`
}

type submitRequest struct {
	Comment   string `json:"comment"`
	Checklist struct {
		TypographyChecked bool `json:"typography_checked"`
	} `json:"checklist"`
}

type decisionRequest struct {
	Comment string `json:"comment"`
}

type downloadRequest struct {
	FileID *kernel.ID `json:"file_id"`
}
