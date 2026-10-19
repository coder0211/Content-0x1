package http

import (
	"time"

	"github.com/be/acs/api/internal/modules/image/domain"
	"github.com/be/acs/api/internal/modules/image/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

type moneyDTO struct {
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
	IsEstimate bool    `json:"is_estimate"`
}

func money(m kernel.Money, estimate bool) moneyDTO {
	cur := m.Currency
	if cur == "" {
		cur = kernel.CurrencyUSD
	}
	return moneyDTO{Amount: m.Float64(), Currency: cur, IsEstimate: estimate}
}

func moneyPtr(m *kernel.Money, estimate bool) *moneyDTO {
	if m == nil {
		return nil
	}
	v := money(*m, estimate)
	return &v
}

type generateRequest struct {
	CampaignID       *kernel.ID  `json:"campaign_id"`
	BrandID          *kernel.ID  `json:"brand_id"`
	Prompt           string      `json:"prompt"`
	Preset           string      `json:"preset"`
	ReferenceFileIDs []kernel.ID `json:"reference_file_ids"`
	AspectRatio      string      `json:"aspect_ratio"`
	Quality          string      `json:"quality"`
	Count            int         `json:"count"`
	ModelCode        *string     `json:"model_code"`
	BudgetOverride   bool        `json:"budget_override"`
}

type editRequest struct {
	FileID         kernel.ID  `json:"file_id"`
	Prompt         string     `json:"prompt"`
	MaskFileID     *kernel.ID `json:"mask_file_id"`
	Quality        string     `json:"quality"`
	BudgetOverride bool       `json:"budget_override"`
}

type variationRequest struct {
	FileID         kernel.ID `json:"file_id"`
	Count          int       `json:"count"`
	Prompt         string    `json:"prompt"`
	BudgetOverride bool      `json:"budget_override"`
}

type estimateRequest struct {
	CampaignID  *kernel.ID `json:"campaign_id"`
	Operation   string     `json:"operation"`
	AspectRatio string     `json:"aspect_ratio"`
	Quality     string     `json:"quality"`
	Count       int        `json:"count"`
	ModelCode   *string    `json:"model_code"`
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

type jobAcceptedDTO struct {
	JobID         kernel.ID `json:"job_id"`
	Status        string    `json:"status"`
	EstimatedCost moneyDTO  `json:"estimated_cost"`
}

func presentAccepted(a usecase.JobAccepted) jobAcceptedDTO {
	return jobAcceptedDTO{JobID: a.JobID, Status: a.Status, EstimatedCost: money(a.EstimatedCost, true)}
}

type estimateLineDTO struct {
	Item         string   `json:"item"`
	ProviderCode string   `json:"provider_code"`
	ModelCode    string   `json:"model_code"`
	Unit         string   `json:"unit"`
	Units        float64  `json:"units"`
	Amount       moneyDTO `json:"amount"`
}

type estimateDTO struct {
	Total             moneyDTO          `json:"total"`
	Lines             []estimateLineDTO `json:"lines"`
	PricingVersion    string            `json:"pricing_version"`
	PerJobLimit       moneyDTO          `json:"per_job_limit"`
	CampaignRemaining *moneyDTO         `json:"campaign_remaining"`
	GlobalRemaining   moneyDTO          `json:"global_remaining"`
	RequiresOverride  bool              `json:"requires_override"`
	Blocked           bool              `json:"blocked"`
}

func presentEstimate(e usecase.EstimateView) estimateDTO {
	out := estimateDTO{Total: money(e.Total, true), Lines: make([]estimateLineDTO, 0, len(e.Lines)), PricingVersion: e.PricingVersion,
		PerJobLimit: money(e.PerJobLimit, false), CampaignRemaining: moneyPtr(e.CampaignRemaining, false),
		GlobalRemaining: money(e.GlobalRemaining, false), RequiresOverride: e.RequiresOverride, Blocked: e.Blocked}
	for _, l := range e.Lines {
		out.Lines = append(out.Lines, estimateLineDTO{Item: l.Item, ProviderCode: l.ProviderCode, ModelCode: l.ModelCode,
			Unit: l.Unit, Units: l.Units, Amount: money(l.Amount, true)})
	}
	return out
}

type imageDTO struct {
	FileID          kernel.ID  `json:"file_id"`
	Role            string     `json:"role"`
	SortOrder       int        `json:"sort_order"`
	MimeType        string     `json:"mime_type"`
	Width           int        `json:"width"`
	Height          int        `json:"height"`
	ParentFileID    *kernel.ID `json:"parent_file_id"`
	URL             string     `json:"url"`
	ThumbnailFileID *kernel.ID `json:"thumbnail_file_id"`
	ThumbnailURL    string     `json:"thumbnail_url"`
	URLExpiresAt    time.Time  `json:"url_expires_at"`
}

type batchAssetDTO struct {
	ID                      kernel.ID  `json:"id"`
	Title                   string     `json:"title"`
	Status                  string     `json:"status"`
	VersionNo               int        `json:"version_no"`
	CurrentVersionNo        int        `json:"current_version_no"`
	RequiresTypographyCheck bool       `json:"requires_typography_check"`
	Images                  []imageDTO `json:"images"`
}

type batchJobDTO struct {
	ID           kernel.ID `json:"id"`
	Status       string    `json:"status"`
	Progress     int       `json:"progress"`
	ErrorCode    *string   `json:"error_code"`
	ErrorKind    *string   `json:"error_kind"`
	ErrorMessage *string   `json:"error_message"`
	CostEstimate *moneyDTO `json:"cost_estimate"`
	CostActual   *moneyDTO `json:"cost_actual"`
}

type batchDTO struct {
	ID           kernel.ID      `json:"id"`
	Operation    string         `json:"operation"`
	CampaignID   *kernel.ID     `json:"campaign_id"`
	BrandID      kernel.ID      `json:"brand_id"`
	Prompt       string         `json:"prompt"`
	Preset       string         `json:"preset"`
	AspectRatio  string         `json:"aspect_ratio"`
	Quality      string         `json:"quality"`
	Count        int            `json:"count"`
	ModelCode    string         `json:"model_code"`
	SourceFileID *kernel.ID     `json:"source_file_id"`
	CreatedBy    kernel.ID      `json:"created_by"`
	CreatedAt    time.Time      `json:"created_at"`
	Job          batchJobDTO    `json:"job"`
	Asset        *batchAssetDTO `json:"asset"`
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func presentBatch(b usecase.BatchView) batchDTO {
	o := b.Options
	out := batchDTO{ID: b.ID, Operation: string(b.Operation), CampaignID: b.CampaignID, BrandID: b.BrandID, Prompt: o.Prompt,
		Preset: string(o.Preset), AspectRatio: o.AspectRatio, Quality: o.Quality, Count: o.Count, ModelCode: b.ModelCode,
		SourceFileID: b.SourceFileID, CreatedBy: b.CreatedBy, CreatedAt: b.CreatedAt.UTC(),
		Job: batchJobDTO{ID: b.Job.ID, Status: b.Job.Status, Progress: b.Job.Progress, ErrorCode: nonEmpty(b.Job.ErrorCode),
			ErrorKind: nonEmpty(b.Job.ErrorKind), ErrorMessage: nonEmpty(b.Job.ErrorMessage),
			CostEstimate: moneyPtr(b.Job.CostEstimate, true), CostActual: moneyPtr(b.Job.CostActual, false)}}
	if a := b.Asset; a != nil {
		ad := &batchAssetDTO{ID: a.ID, Title: a.Title, Status: a.Status, VersionNo: a.VersionNo, CurrentVersionNo: a.CurrentVersionNo,
			RequiresTypographyCheck: a.RequiresTypographyCheck, Images: make([]imageDTO, 0, len(a.Images))}
		for _, im := range a.Images {
			ad.Images = append(ad.Images, imageDTO{FileID: im.FileID, Role: im.Role, SortOrder: im.SortOrder, MimeType: im.MimeType,
				Width: im.Width, Height: im.Height, ParentFileID: im.ParentFileID, URL: im.URL, ThumbnailFileID: im.ThumbnailFileID,
				ThumbnailURL: im.ThumbnailURL, URLExpiresAt: im.URLExpiresAt.UTC()})
		}
		out.Asset = ad
	}
	return out
}

type referenceDTO struct {
	FileID       kernel.ID `json:"file_id"`
	Source       string    `json:"source"`
	Label        string    `json:"label"`
	MimeType     string    `json:"mime_type"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	URL          string    `json:"url"`
	ThumbnailURL string    `json:"thumbnail_url"`
	URLExpiresAt time.Time `json:"url_expires_at"`
}

func presentReference(r usecase.ReferenceCandidate) referenceDTO {
	return referenceDTO{FileID: r.FileID, Source: r.Source, Label: r.Label, MimeType: r.MimeType, Width: r.Width, Height: r.Height,
		URL: r.URL, ThumbnailURL: r.ThumbnailURL, URLExpiresAt: r.URLExpiresAt.UTC()}
}

func preset(s string) domain.Preset { return domain.Preset(s) }
