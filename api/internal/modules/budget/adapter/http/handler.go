// Package http serves the budget endpoints: cost estimates, usage summary,
// budget settings and campaign budgets (SPEC-04 §2.9).
package http

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/modules/budget/domain"
	"github.com/be/acs/api/internal/modules/budget/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Handler maps HTTP to the budget use cases.
type Handler struct{ Svc *usecase.Service }

// Routes registers the module's routes. Permissions are re-checked in the
// use cases.
func (h *Handler) Routes(r chi.Router) {
	r.With(httpx.RequirePermission(kernel.PermGenerateRun)).Post("/cost-estimates", h.estimate)
	r.Group(func(r chi.Router) {
		r.Use(httpx.RequirePermission(kernel.PermUsageView))
		r.Get("/usage/summary", h.summary)
		r.Get("/settings/budgets", h.getSettings)
		r.Get("/campaigns/{id}/budget", h.getCampaignBudget)
	})
	r.Group(func(r chi.Router) {
		r.Use(httpx.RequirePermission(kernel.PermBudgetManage))
		r.Patch("/settings/budgets", h.patchSettings)
		r.Put("/campaigns/{id}/budget", h.putCampaignBudget)
	})
}

// ---------------------------------------------------------------- DTOs

type money struct {
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
	IsEstimate bool    `json:"is_estimate,omitempty"`
}

func num(d decimal.Decimal) float64 {
	f, _ := d.Round(6).Float64()
	return f
}

func toMoney(m kernel.Money) money { return money{Amount: num(m.Amount), Currency: m.Currency} }

type estimateRequest struct {
	Kind            string           `json:"kind"`
	Provider        string           `json:"provider"`
	Model           string           `json:"model"`
	CampaignID      *kernel.ID       `json:"campaign_id"`
	PromptChars     int              `json:"prompt_chars"`
	MaxOutputTokens int              `json:"max_output_tokens"`
	Count           int              `json:"count"`
	Size            string           `json:"size"`
	Quality         string           `json:"quality"`
	ClipSeconds     *decimal.Decimal `json:"clip_seconds"`
	Resolution      string           `json:"resolution"`
	VoiceoverChars  int              `json:"voiceover_chars"`
}

type estimateLine struct {
	Item      string  `json:"item"`
	Provider  string  `json:"provider"`
	Model     string  `json:"model"`
	Unit      string  `json:"unit"`
	Units     float64 `json:"units"`
	UnitPrice float64 `json:"unit_price"`
	Amount    float64 `json:"amount"`
}

type limitsDTO struct {
	PerJobLimit       float64  `json:"per_job_limit"`
	CampaignRemaining *float64 `json:"campaign_remaining"`
	GlobalRemaining   float64  `json:"global_remaining"`
}

type estimateResponse struct {
	Breakdown        []estimateLine `json:"breakdown"`
	Total            money          `json:"total"`
	PricingVersion   string         `json:"pricing_version"`
	Limits           limitsDTO      `json:"limits"`
	RequiresOverride bool           `json:"requires_override"`
	Blocked          bool           `json:"blocked"`
}

// presentEstimate renders an estimate + limits in the SPEC-04 §2.6 shape
// (breakdown/total/limits/requires_override).
func presentEstimate(est usecase.Estimate, lim usecase.Limits) any {
	out := estimateResponse{Breakdown: []estimateLine{}, PricingVersion: est.PricingVersion,
		RequiresOverride: lim.RequiresOverride, Blocked: lim.Blocked}
	for _, l := range est.Lines {
		out.Breakdown = append(out.Breakdown, estimateLine{Item: l.Item, Provider: l.ProviderCode, Model: l.ModelCode,
			Unit: l.Unit, Units: num(l.Units), UnitPrice: num(l.UnitPrice), Amount: num(l.Amount.Amount)})
	}
	out.Total = toMoney(est.Total)
	out.Total.IsEstimate = true
	out.Limits = limitsDTO{PerJobLimit: num(lim.PerJobLimit.Amount), GlobalRemaining: num(lim.GlobalRemaining.Amount)}
	if lim.CampaignRemaining != nil {
		v := num(lim.CampaignRemaining.Amount)
		out.Limits.CampaignRemaining = &v
	}
	return out
}

type budgetDTO struct {
	ID              kernel.ID  `json:"id"`
	Scope           string     `json:"scope"`
	CampaignID      *kernel.ID `json:"campaign_id"`
	CampaignName    *string    `json:"campaign_name"`
	PeriodStart     string     `json:"period_start"`
	PeriodEnd       string     `json:"period_end"`
	LimitAmount     float64    `json:"limit_amount"`
	ReservedAmount  float64    `json:"reserved_amount"`
	SpentAmount     float64    `json:"spent_amount"`
	RemainingAmount float64    `json:"remaining_amount"`
	PercentUsed     float64    `json:"percent_used"`
	Currency        string     `json:"currency"`
	AlertThresholds []int      `json:"alert_thresholds"`
	AlertsSent      []int      `json:"alerts_sent"`
	HardStop        bool       `json:"hard_stop"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func nonNil(v []int) []int {
	if v == nil {
		return []int{}
	}
	return v
}

func presentBudget(b domain.Budget) budgetDTO {
	return budgetDTO{
		ID: b.ID, Scope: string(b.Scope), CampaignID: b.CampaignID,
		PeriodStart: b.PeriodStart.Format(time.DateOnly), PeriodEnd: b.PeriodEnd.Format(time.DateOnly),
		LimitAmount: num(b.Limit), ReservedAmount: num(b.Reserved), SpentAmount: num(b.Spent),
		RemainingAmount: num(b.Remaining()), PercentUsed: num(b.PercentUsed().Round(2)), Currency: b.Currency,
		AlertThresholds: nonNil(b.Thresholds), AlertsSent: nonNil(b.AlertsSent), HardStop: b.HardStop, UpdatedAt: b.UpdatedAt.UTC(),
	}
}

type perJobDTO struct {
	Text  float64 `json:"text"`
	Image float64 `json:"image"`
	Video float64 `json:"video"`
}

type globalTemplateDTO struct {
	MonthlyLimit    float64 `json:"monthly_limit"`
	AlertThresholds []int   `json:"alert_thresholds"`
	HardStop        bool    `json:"hard_stop"`
}

type settingsDTO struct {
	Currency               string            `json:"currency"`
	PerJobLimits           perJobDTO         `json:"per_job_limits"`
	GlobalTemplate         globalTemplateDTO `json:"global_template"`
	Global                 budgetDTO         `json:"global"`
	Campaigns              []budgetDTO       `json:"campaigns"`
	MaxConcurrentVideoJobs int               `json:"max_concurrent_video_jobs_per_user"`
	UpdatedBy              *kernel.ID        `json:"updated_by"`
	UpdatedAt              time.Time         `json:"updated_at"`
}

func presentSettings(v usecase.SettingsView) settingsDTO {
	st := v.Settings
	out := settingsDTO{
		Currency:     kernel.CurrencyUSD,
		PerJobLimits: perJobDTO{Text: num(st.PerJobText), Image: num(st.PerJobImage), Video: num(st.PerJobVideo)},
		GlobalTemplate: globalTemplateDTO{MonthlyLimit: num(st.GlobalMonthlyLimit), AlertThresholds: nonNil(st.GlobalThresholds),
			HardStop: st.GlobalHardStop},
		Global: presentBudget(v.Global), Campaigns: []budgetDTO{}, MaxConcurrentVideoJobs: st.MaxConcurrentVideoJobs,
		UpdatedBy: st.UpdatedBy, UpdatedAt: st.UpdatedAt.UTC(),
	}
	for _, c := range v.Campaigns {
		d := presentBudget(c.Budget)
		if c.CampaignName != "" {
			name := c.CampaignName
			d.CampaignName = &name
		}
		out.Campaigns = append(out.Campaigns, d)
	}
	return out
}

type perJobPatch struct {
	Text  *decimal.Decimal `json:"text"`
	Image *decimal.Decimal `json:"image"`
	Video *decimal.Decimal `json:"video"`
}

type globalPatch struct {
	LimitAmount     *decimal.Decimal `json:"limit_amount"`
	AlertThresholds []int            `json:"alert_thresholds"`
	HardStop        *bool            `json:"hard_stop"`
}

type settingsPatchRequest struct {
	PerJobLimits           *perJobPatch `json:"per_job_limits"`
	Global                 *globalPatch `json:"global"`
	MaxConcurrentVideoJobs *int         `json:"max_concurrent_video_jobs_per_user"`
}

type campaignBudgetRequest struct {
	LimitAmount     *decimal.Decimal `json:"limit_amount"`
	Currency        string           `json:"currency"`
	PeriodStart     *string          `json:"period_start"`
	PeriodEnd       *string          `json:"period_end"`
	AlertThresholds []int            `json:"alert_thresholds"`
	HardStop        *bool            `json:"hard_stop"`
}

type sourceTotalsDTO struct {
	ProviderReported float64 `json:"provider_reported"`
	Calculated       float64 `json:"calculated"`
	Estimated        float64 `json:"estimated"`
	Total            float64 `json:"total"`
	Count            int64   `json:"count"`
}

func presentTotals(t usecase.SourceTotals) sourceTotalsDTO {
	return sourceTotalsDTO{ProviderReported: num(t.ProviderReported), Calculated: num(t.Calculated),
		Estimated: num(t.Estimated), Total: num(t.Total), Count: t.Count}
}

type summaryRowDTO struct {
	Key   *string `json:"key"`
	Label string  `json:"label"`
	sourceTotalsDTO
}

type summaryDTO struct {
	From     string          `json:"from"`
	To       string          `json:"to"`
	GroupBy  string          `json:"group_by"`
	Currency string          `json:"currency"`
	Rows     []summaryRowDTO `json:"rows"`
	Totals   sourceTotalsDTO `json:"totals"`
}

// ------------------------------------------------------------- handlers

func (h *Handler) estimate(w http.ResponseWriter, r *http.Request) {
	var req estimateRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	q := usecase.QuoteRequest{Kind: usecase.Category(req.Kind), ProviderCode: req.Provider, ModelCode: req.Model,
		CampaignID: req.CampaignID, PromptChars: req.PromptChars, MaxOutputTokens: req.MaxOutputTokens,
		Count: req.Count, Size: req.Size, Quality: req.Quality, ClipSeconds: decimal.Zero, Resolution: req.Resolution,
		VoiceoverChars: req.VoiceoverChars}
	if req.ClipSeconds != nil {
		q.ClipSeconds = *req.ClipSeconds
	}
	res, err := h.Svc.Quote(r.Context(), httpx.ActorFrom(r), q)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, presentEstimate(res.Estimate, res.Limits))
}

func parseDate(field, s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return nil, kernel.Validation(kernel.FieldIssue(field, "invalid_date"))
	}
	return &t, nil
}

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, err := parseDate("from", q.Get("from"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	to, err := parseDate("to", q.Get("to"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	s, err := h.Svc.UsageSummary(r.Context(), httpx.ActorFrom(r), usecase.SummaryRequest{From: from, To: to, GroupBy: q.Get("group_by")})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := summaryDTO{From: s.From.Format(time.DateOnly), To: s.To.Format(time.DateOnly), GroupBy: s.GroupBy,
		Currency: s.Currency, Rows: []summaryRowDTO{}, Totals: presentTotals(s.Totals)}
	for _, row := range s.Rows {
		d := summaryRowDTO{Label: row.Label, sourceTotalsDTO: presentTotals(row.SourceTotals)}
		if row.Key != "" {
			k := row.Key
			d.Key = &k
		}
		out.Rows = append(out.Rows, d)
	}
	httpx.OK(w, r, out)
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	v, err := h.Svc.GetSettings(r.Context(), httpx.ActorFrom(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, presentSettings(v))
}

func (h *Handler) patchSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsPatchRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	p := usecase.SettingsPatch{MaxConcurrentVideoJobs: req.MaxConcurrentVideoJobs}
	if req.PerJobLimits != nil {
		p.PerJobText, p.PerJobImage, p.PerJobVideo = req.PerJobLimits.Text, req.PerJobLimits.Image, req.PerJobLimits.Video
	}
	if req.Global != nil {
		p.GlobalMonthlyLimit, p.GlobalThresholds, p.GlobalHardStop = req.Global.LimitAmount, req.Global.AlertThresholds, req.Global.HardStop
	}
	actor := httpx.ActorFrom(r)
	if _, err := h.Svc.UpdateSettings(r.Context(), actor, p); err != nil {
		httpx.Error(w, r, err)
		return
	}
	v, err := h.Svc.GetSettings(r.Context(), actor)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, presentSettings(v))
}

func (h *Handler) getCampaignBudget(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	b, err := h.Svc.GetCampaignBudget(r.Context(), httpx.ActorFrom(r), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if b == nil {
		httpx.OK(w, r, nil)
		return
	}
	httpx.OK(w, r, presentBudget(*b))
}

func (h *Handler) putCampaignBudget(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req campaignBudgetRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if req.LimitAmount == nil {
		httpx.Error(w, r, kernel.Validation(kernel.FieldIssue("limit_amount", "required")))
		return
	}
	in := usecase.CampaignBudgetInput{Limit: *req.LimitAmount, Currency: req.Currency, Thresholds: req.AlertThresholds, HardStop: req.HardStop}
	if req.PeriodStart != nil {
		if in.PeriodStart, err = parseDate("period_start", *req.PeriodStart); err != nil {
			httpx.Error(w, r, err)
			return
		}
	}
	if req.PeriodEnd != nil {
		if in.PeriodEnd, err = parseDate("period_end", *req.PeriodEnd); err != nil {
			httpx.Error(w, r, err)
			return
		}
	}
	b, err := h.Svc.PutCampaignBudget(r.Context(), httpx.ActorFrom(r), id, in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, presentBudget(b))
}
