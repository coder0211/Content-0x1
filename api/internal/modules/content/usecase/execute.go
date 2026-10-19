package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	assetuc "github.com/be/acs/api/internal/modules/asset/usecase"
	budgetuc "github.com/be/acs/api/internal/modules/budget/usecase"
	"github.com/be/acs/api/internal/modules/content/domain"
	"github.com/be/acs/api/internal/modules/content/usecase/prompt"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// ErrJobCancelled ends a step whose job the user cancelled before the
// provider call (nothing was charged).
var ErrJobCancelled = kernel.NewError("JOB_CANCELLED", "Tác vụ đã bị hủy")

// invalidOutput is returned when the model output does not match the
// schema; it is retried (a new sample usually fixes it).
func invalidOutput(provider, msg string) error {
	return &ports.ProviderError{Kind: ports.ErrKindTransient, Provider: provider, Code: "invalid_output", Message: msg}
}

// ExecuteStep runs one content step (worker side, SPEC-06 §2.4): call the
// text provider, record usage, then create assets / versions or return the
// plan / prompt suggestions as the job result.
func (s *Service) ExecuteStep(ctx context.Context, sc jobuc.StepContext) (jobuc.StepResult, error) {
	var in stepInput
	if err := sc.DecodeInput(&in); err != nil {
		return jobuc.StepResult{}, jobuc.Permanent(fmt.Errorf("decode step input: %w", err))
	}
	job := sc.Job()
	if cancelled, err := sc.CancelRequested(ctx); err != nil {
		return jobuc.StepResult{}, err
	} else if cancelled {
		return jobuc.StepResult{}, ErrJobCancelled
	}
	_ = sc.Progress(ctx, 10, "Đang gọi mô hình AI")
	resp, err := s.callText(ctx, sc, job, in)
	if err != nil {
		return jobuc.StepResult{}, err
	}
	_ = sc.Progress(ctx, 80, "Đang lưu kết quả")
	raw := resp.JSON
	if len(raw) == 0 {
		raw = json.RawMessage(extractJSON(resp.Content))
	}
	var res jobuc.StepResult
	switch sc.StepType() {
	case StepGeneration, StepRewrite, StepTransform:
		res, err = s.saveVariants(ctx, sc, job, in, raw)
	case StepPlan:
		res, err = s.savePlan(ctx, job, in, raw)
	case StepImagePrompts:
		res, err = imagePromptsResult(job.ProviderCode, in, raw)
	default:
		err = jobuc.Permanent(fmt.Errorf("unknown content step %q", sc.StepType()))
	}
	if err != nil {
		return jobuc.StepResult{}, err
	}
	_ = sc.Progress(ctx, 100, "")
	return res, nil
}

// callText calls the provider and records usage for every call, including
// timeouts after which the provider may have charged (Estimated=true).
func (s *Service) callText(ctx context.Context, sc jobuc.StepContext, job jobuc.JobInfo, in stepInput) (ports.TextResponse, error) {
	gen, err := s.AI.Text(job.ProviderCode)
	if err != nil {
		return ports.TextResponse{}, jobuc.Permanent(err)
	}
	resp, err := gen.Generate(ctx, ports.TextRequest{
		Model: job.ModelCode, System: in.System, Messages: []ports.Message{{Role: "user", Content: in.User}},
		JSONSchema: in.Schema, SchemaName: in.SchemaName, MaxTokens: in.MaxTokens, Temperature: in.Temperature,
	})
	stepID := sc.StepID()
	usage := budgetuc.UsageInput{
		JobID: job.ID, StepID: &stepID, CampaignID: job.CampaignID, UserID: job.CreatedBy,
		ProviderCode: job.ProviderCode, ModelCode: job.ModelCode, JobType: string(job.Type),
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
			usage.Usage = ports.TextUsage(int64(len([]rune(in.System+in.User))/3+1), int64(in.MaxTokens), 0)
			usage.Estimated = true
			if _, uerr := s.Budget.RecordUsage(context.WithoutCancel(ctx), usage); uerr != nil {
				return ports.TextResponse{}, errors.Join(err, uerr)
			}
		}
		return ports.TextResponse{}, err
	}
	usage.Usage = resp.Usage
	if _, err := s.Budget.RecordUsage(ctx, usage); err != nil {
		return ports.TextResponse{}, err
	}
	return resp, nil
}

func isTimeout(err error) bool {
	pe, ok := ports.AsProviderError(err)
	return ok && pe.Kind == ports.ErrKindTransient && strings.Contains(strings.ToLower(pe.Code), "timeout")
}

// extractJSON finds the first JSON object in free text (models without
// structured output sometimes wrap it in prose or code fences).
func extractJSON(s string) string {
	start, end := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}

func parseVariants(provider string, raw json.RawMessage, want int) ([]domain.Variant, error) {
	var out struct {
		Variants []variantJSON `json:"variants"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.Variants == nil {
		// A single object is accepted for one-variant steps.
		var one variantJSON
		if want == 1 && json.Unmarshal(raw, &one) == nil {
			out.Variants = []variantJSON{one}
		} else {
			return nil, invalidOutput(provider, "output is not {variants:[...]}")
		}
	}
	vs := make([]domain.Variant, 0, len(out.Variants))
	for _, v := range out.Variants {
		if d := v.domain(); !d.IsEmpty() {
			vs = append(vs, d)
		}
	}
	if len(vs) == 0 {
		return nil, invalidOutput(provider, "no usable variant")
	}
	if want > 0 && len(vs) > want {
		vs = vs[:want]
	}
	return vs, nil
}

func (in stepInput) provenance(job jobuc.JobInfo) *Provenance {
	ids := in.ProductIDs
	if ids == nil {
		ids = []kernel.ID{}
	}
	return &Provenance{
		JobID: job.ID, ProviderCode: job.ProviderCode, ModelCode: job.ModelCode,
		PromptTemplateCode: in.TemplateCode, PromptTemplateVersion: in.TemplateVersion, PromptTemplateID: in.TemplateID,
		BrandVersionID: in.BrandVersionID, BrandVersionNo: in.BrandVersionNo, ProductIDs: ids, Brief: in.Brief,
	}
}

// saveVariants runs the post-generation checks and stores the result:
// generation → N new assets, rewrite → a new version of the same asset,
// transform → a new asset with source_asset_id. All writes share one
// transaction so a retry never leaves half the variants behind.
func (s *Service) saveVariants(ctx context.Context, sc jobuc.StepContext, job jobuc.JobInfo, in stepInput, raw json.RawMessage) (jobuc.StepResult, error) {
	want := in.Variants
	if want == 0 {
		want = 1
	}
	vs, err := parseVariants(job.ProviderCode, raw, want)
	if err != nil {
		return jobuc.StepResult{}, err
	}
	facts := factsFrom(in.FactTexts, in.FactAmounts)
	prov := in.provenance(job)
	jobID := job.ID
	assetIDs := make([]kernel.ID, 0, len(vs))
	versions := make([]map[string]any, 0, len(vs))
	warnCount := 0
	err = s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		for i, v := range vs {
			ws := domain.RunChecks(v, in.ForbiddenWords, facts)
			warnCount += len(ws)
			cj := toContentJSON(v, in.Format, in.Platform, in.Language, ws, prov)
			body, err := json.Marshal(cj)
			if err != nil {
				return err
			}
			nv := assetuc.NewVersion{
				ContentJSON: body, GeneratedByJobID: &jobID, CreatedBy: job.CreatedBy,
				SearchText: strings.Join(v.Texts(), "\n"), ChangeNote: in.ChangeNote,
			}
			var ref assetuc.AssetRef
			if sc.StepType() == StepRewrite {
				if in.SourceAssetID == nil {
					return jobuc.Permanent(errors.New("rewrite without asset"))
				}
				ref, err = s.Assets.AddVersion(ctx, *in.SourceAssetID, nv)
			} else {
				title := strings.TrimSpace(v.Title)
				if title == "" {
					title = domain.FallbackTitle(domain.Format(in.Format), i)
				}
				if nv.ChangeNote == "" {
					nv.ChangeNote = "Tạo bởi AI"
				}
				var src *kernel.ID
				if sc.StepType() == StepTransform {
					src = in.SourceAssetID
				}
				ref, err = s.Assets.CreateAsset(ctx, assetuc.CreateAssetInput{
					CampaignID: job.CampaignID, BrandID: in.BrandID, Type: assetuc.TypeContent, Subtype: in.Format,
					Title: domain.Truncate(title, 200), SourceAssetID: src, Tags: []string{}, CreatedBy: job.CreatedBy, Version: nv,
				})
			}
			if err != nil {
				return err
			}
			assetIDs = append(assetIDs, ref.AssetID)
			versions = append(versions, map[string]any{"asset_id": ref.AssetID, "version_no": ref.VersionNo, "warnings": len(ws)})
		}
		return nil
	})
	if err != nil {
		return jobuc.StepResult{}, err
	}
	return jobuc.StepResult{
		Output:    map[string]any{"versions": versions, "warnings": warnCount},
		JobResult: map[string]any{"asset_ids": assetIDs},
	}, nil
}

// aiPlan is the content_plan output schema (prompt.planSchema).
type aiPlan struct {
	Summary string `json:"summary"`
	Themes  []struct {
		Title      string `json:"title"`
		Angle      string `json:"angle"`
		KeyMessage string `json:"key_message"`
	} `json:"themes"`
	Schedule []struct {
		Date    string `json:"date"`
		Channel string `json:"channel"`
		Format  string `json:"format"`
		Theme   string `json:"theme"`
		Idea    string `json:"idea"`
	} `json:"schedule"`
	Deliverables []struct {
		Format string `json:"format"`
		Count  int    `json:"count"`
		Notes  string `json:"notes"`
	} `json:"deliverables"`
}

// campaignPlan is the campaign module's plan shape (campaigns.content_plan):
// {"summary", "themes": [string], "items": [{topic, angle, channel, format, publish_date, notes}]}.
type campaignPlan struct {
	Summary string             `json:"summary"`
	Themes  []string           `json:"themes"`
	Items   []campaignPlanItem `json:"items"`
}

type campaignPlanItem struct {
	ID          string `json:"id"`
	Topic       string `json:"topic"`
	Angle       string `json:"angle"`
	Channel     string `json:"channel"`
	Format      string `json:"format"`
	PublishDate string `json:"publish_date"`
	Notes       string `json:"notes"`
	Done        bool   `json:"done"`
}

var nonSlug = regexp.MustCompile(`[^a-z0-9_]+`)

// planSlug turns a model-provided channel/format into the campaign slug
// format ^[a-z0-9_]{1,40}$ ("Facebook Post" → "facebook_post").
func planSlug(s string) string {
	s = strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "_"), "_")
	if len(s) > 40 {
		s = strings.Trim(s[:40], "_")
	}
	return s
}

// snapSlug maps a model-provided channel/format onto one of the campaign
// brief's slugs: an exact slug match, else the first allowed slug the value
// contains or is contained in ("Facebook Post" → "facebook"). Without a
// match it returns "" so free-form model text ("Bài đăng") never becomes a
// bogus slug. With no allowed list the plain slug is kept.
func snapSlug(s string, allowed []string) string {
	v := planSlug(s)
	if len(allowed) == 0 || v == "" {
		return v
	}
	for _, a := range allowed {
		if v == a {
			return a
		}
	}
	for _, a := range allowed {
		if strings.Contains(v, a) || strings.Contains(a, v) {
			return a
		}
	}
	return ""
}

// toCampaignPlan maps the AI plan onto the campaign plan contract: theme
// titles become themes, schedule rows become checklist items (idea → topic,
// theme + angle → angle, invalid dates moved to notes), deliverables are
// summarised in the summary. Channels and formats are snapped onto the
// brief's (channels, formats); an unknown format falls back to the brief's
// format for that row position.
func toCampaignPlan(p aiPlan, channels, formats []string) campaignPlan {
	out := campaignPlan{Summary: strings.TrimSpace(p.Summary), Themes: []string{}, Items: []campaignPlanItem{}}
	angles := map[string]string{}
	for _, t := range p.Themes {
		title := strings.TrimSpace(t.Title)
		if title == "" {
			continue
		}
		out.Themes = append(out.Themes, title)
		angles[strings.ToLower(title)] = strings.TrimSpace(t.Angle)
	}
	for i, r := range p.Schedule {
		topic := strings.TrimSpace(r.Idea)
		if topic == "" {
			topic = strings.TrimSpace(r.Theme)
		}
		if topic == "" {
			continue
		}
		angle := strings.TrimSpace(r.Theme)
		if a := angles[strings.ToLower(angle)]; a != "" {
			angle += " — " + a
		}
		format := snapSlug(r.Format, formats)
		if format == "" && len(formats) > 0 {
			format = formats[len(out.Items)%len(formats)]
		}
		if format == "" {
			format = "post"
		}
		it := campaignPlanItem{ID: fmt.Sprintf("item-%d", i+1), Topic: topic, Angle: angle, Channel: snapSlug(r.Channel, channels), Format: format}
		if d := strings.TrimSpace(r.Date); d != "" {
			if _, err := time.Parse(time.DateOnly, d); err == nil {
				it.PublishDate = d
			} else {
				it.Notes = "Thời gian: " + d
			}
		}
		out.Items = append(out.Items, it)
	}
	var del []string
	for _, d := range p.Deliverables {
		if f := strings.TrimSpace(d.Format); f != "" && d.Count > 0 {
			del = append(del, fmt.Sprintf("%s × %d", f, d.Count))
		}
	}
	if len(del) > 0 {
		line := "Đầu ra: " + strings.Join(del, ", ")
		if out.Summary == "" {
			out.Summary = line
		} else {
			out.Summary += "\n" + line
		}
	}
	return out
}

// savePlan stores the plan as a proposal on the campaign, in the campaign
// module's plan shape (campaign/domain.Plan).
func (s *Service) savePlan(ctx context.Context, job jobuc.JobInfo, in stepInput, raw json.RawMessage) (jobuc.StepResult, error) {
	var ai aiPlan
	if err := json.Unmarshal(raw, &ai); err != nil {
		return jobuc.StepResult{}, invalidOutput(job.ProviderCode, "output is not a content plan")
	}
	plan := toCampaignPlan(ai, in.PlanChannels, in.PlanFormats)
	if len(plan.Items) == 0 {
		return jobuc.StepResult{}, invalidOutput(job.ProviderCode, "output is not a content plan")
	}
	body, err := json.Marshal(plan)
	if err != nil {
		return jobuc.StepResult{}, err
	}
	if job.CampaignID == nil {
		return jobuc.StepResult{}, jobuc.Permanent(errors.New("content plan without campaign"))
	}
	if err := s.Campaigns.SaveContentPlanProposal(ctx, *job.CampaignID, body); err != nil {
		return jobuc.StepResult{}, err
	}
	return jobuc.StepResult{Output: map[string]any{"saved": true}, JobResult: map[string]any{"plan": json.RawMessage(body)}}, nil
}

// ImagePrompt is one suggested image prompt.
type ImagePrompt struct {
	Prompt         string `json:"prompt"`
	NegativePrompt string `json:"negative_prompt"`
	Rationale      string `json:"rationale"`
}

// imagePromptsResult validates 3 prompts and makes sure each names the
// brand palette and tone (ACS-052 AC), appending them when the model forgot.
func imagePromptsResult(provider string, in stepInput, raw json.RawMessage) (jobuc.StepResult, error) {
	var out struct {
		Prompts []ImagePrompt `json:"prompts"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return jobuc.StepResult{}, invalidOutput(provider, "output is not {prompts:[...]}")
	}
	ps := make([]ImagePrompt, 0, 3)
	for _, p := range out.Prompts {
		p.Prompt = strings.TrimSpace(p.Prompt)
		if p.Prompt != "" && len(ps) < 3 {
			ps = append(ps, EnsureBrandStyle(p, in.Colors, in.Tone))
		}
	}
	if len(ps) == 0 {
		return jobuc.StepResult{}, invalidOutput(provider, "no prompt")
	}
	return jobuc.StepResult{Output: map[string]any{"count": len(ps)}, JobResult: map[string]any{"prompts": ps}}, nil
}

// EnsureBrandStyle appends the brand palette (name + hex) and tone to an
// image prompt that does not mention any brand color yet.
func EnsureBrandStyle(p ImagePrompt, colors []prompt.Color, tone []string) ImagePrompt {
	lower := strings.ToLower(p.Prompt)
	mentions := false
	parts := make([]string, 0, len(colors))
	for _, c := range colors {
		if c.Hex != "" && strings.Contains(lower, strings.ToLower(c.Hex)) {
			mentions = true
		}
		parts = append(parts, strings.TrimSpace(c.Name+" "+c.Hex))
	}
	if !mentions && len(parts) > 0 {
		p.Prompt = strings.TrimRight(p.Prompt, ". ") + ". Brand color palette: " + strings.Join(parts, ", ") + "."
	}
	if len(tone) > 0 {
		toneHit := false
		for _, t := range tone {
			if t != "" && strings.Contains(lower, strings.ToLower(t)) {
				toneHit = true
			}
		}
		if !toneHit {
			p.Prompt = strings.TrimRight(p.Prompt, ". ") + ". Mood: " + strings.Join(tone, ", ") + "."
		}
	}
	return p
}
