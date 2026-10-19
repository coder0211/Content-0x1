package http

import (
	"encoding/json"
	"time"

	"github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

// costDTO is SPEC-04 §2.7 "cost".
type costDTO struct {
	Estimate *json.Number `json:"estimate"`
	Actual   *json.Number `json:"actual"`
	Source   *string      `json:"source"`
	Currency string       `json:"currency"`
}

type errorDTO struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type stepDTO struct {
	ID           kernel.ID       `json:"id"`
	StepKey      string          `json:"step_key"`
	StepType     string          `json:"step_type"`
	Status       string          `json:"status"`
	AttemptCount int             `json:"attempt_count"`
	ErrorKind    *string         `json:"error_kind"`
	ErrorMessage *string         `json:"error_message"`
	Output       json.RawMessage `json:"output"`
	StartedAt    *time.Time      `json:"started_at"`
	FinishedAt   *time.Time      `json:"finished_at"`
}

// jobSummaryDTO is a row of GET /jobs.
type jobSummaryDTO struct {
	ID                  kernel.ID  `json:"id"`
	JobType             string     `json:"job_type"`
	Status              string     `json:"status"`
	Progress            int        `json:"progress"`
	CreatedBy           kernel.ID  `json:"created_by"`
	CreatedByName       string     `json:"created_by_name"`
	CampaignID          *kernel.ID `json:"campaign_id"`
	AssetID             *kernel.ID `json:"asset_id"`
	ProviderCode        *string    `json:"provider_code"`
	ModelCode           *string    `json:"model_code"`
	Cost                costDTO    `json:"cost"`
	Error               *errorDTO  `json:"error"`
	CancelRequested     bool       `json:"cancel_requested"`
	NeedsReconciliation bool       `json:"needs_reconciliation"`
	CreatedAt           time.Time  `json:"created_at"`
	StartedAt           *time.Time `json:"started_at"`
	FinishedAt          *time.Time `json:"finished_at"`
}

// jobDTO is GET /jobs/{id}.
type jobDTO struct {
	jobSummaryDTO
	BrandVersionID   *kernel.ID      `json:"brand_version_id"`
	PromptTemplateID *kernel.ID      `json:"prompt_template_id"`
	Request          json.RawMessage `json:"request"`
	Result           json.RawMessage `json:"result"`
	Steps            []stepDTO       `json:"steps"`
}

type eventDTO struct {
	ID        int64           `json:"id"`
	StepID    *kernel.ID      `json:"step_id"`
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

type moneyDTO struct {
	Amount     json.Number `json:"amount"`
	Currency   string      `json:"currency"`
	IsEstimate bool        `json:"is_estimate"`
}

type jobAcceptedDTO struct {
	JobID         kernel.ID `json:"job_id"`
	Status        string    `json:"status"`
	EstimatedCost *moneyDTO `json:"estimated_cost"`
}

type cancelAcceptedDTO struct {
	JobID           kernel.ID `json:"job_id"`
	Status          string    `json:"status"`
	CancelRequested bool      `json:"cancel_requested"`
}

type retryRequest struct {
	Force bool `json:"force"`
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func num(m *kernel.Money) *json.Number {
	if m == nil {
		return nil
	}
	n := json.Number(m.Amount.String())
	return &n
}

func rawOrNull(b json.RawMessage) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("null")
	}
	return b
}

func summaryDTO(v usecase.JobView, needsRec bool) jobSummaryDTO {
	currency := kernel.CurrencyUSD
	switch {
	case v.CostEstimate != nil && v.CostEstimate.Currency != "":
		currency = v.CostEstimate.Currency
	case v.CostActual != nil && v.CostActual.Currency != "":
		currency = v.CostActual.Currency
	}
	d := jobSummaryDTO{
		ID: v.ID, JobType: string(v.Type), Status: string(v.Status), Progress: v.Progress, CreatedBy: v.CreatedBy,
		CampaignID: v.CampaignID, AssetID: v.AssetID, ProviderCode: optStr(v.ProviderCode), ModelCode: optStr(v.ModelCode),
		Cost:            costDTO{Estimate: num(v.CostEstimate), Actual: num(v.CostActual), Source: optStr(v.CostSource), Currency: currency},
		CancelRequested: v.CancelRequested, NeedsReconciliation: needsRec,
		CreatedAt: v.CreatedAt, StartedAt: v.StartedAt, FinishedAt: v.FinishedAt,
	}
	if v.ErrorCode != "" {
		d.Error = &errorDTO{Code: v.ErrorCode, Message: v.ErrorMessage}
	}
	return d
}

func detailDTO(d usecase.JobDetail) jobDTO {
	out := jobDTO{
		jobSummaryDTO: summaryDTO(d.JobView, d.NeedsReconciliation), BrandVersionID: d.BrandVersionID,
		PromptTemplateID: d.PromptTemplateID, Request: rawOrNull(d.Request), Result: rawOrNull(d.Result),
		Steps: make([]stepDTO, 0, len(d.Steps)),
	}
	for _, s := range d.Steps {
		out.Steps = append(out.Steps, stepDTO{
			ID: s.ID, StepKey: s.Key, StepType: s.Type, Status: string(s.Status), AttemptCount: s.AttemptCount,
			ErrorKind: optStr(s.ErrorKind), ErrorMessage: optStr(s.ErrorMessage), Output: rawOrNull(s.Output),
			StartedAt: s.StartedAt, FinishedAt: s.FinishedAt,
		})
	}
	return out
}
