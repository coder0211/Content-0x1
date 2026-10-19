package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
		res, err = s.savePlan(ctx, job, raw)
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
				title := v.Title
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

// savePlan stores the plan as a proposal on the campaign.
func (s *Service) savePlan(ctx context.Context, job jobuc.JobInfo, raw json.RawMessage) (jobuc.StepResult, error) {
	var plan struct {
		Summary      string           `json:"summary"`
		Themes       []map[string]any `json:"themes"`
		Schedule     []map[string]any `json:"schedule"`
		Deliverables []map[string]any `json:"deliverables"`
	}
	if err := json.Unmarshal(raw, &plan); err != nil || (len(plan.Themes) == 0 && len(plan.Schedule) == 0) {
		return jobuc.StepResult{}, invalidOutput(job.ProviderCode, "output is not a content plan")
	}
	for _, l := range []*[]map[string]any{&plan.Themes, &plan.Schedule, &plan.Deliverables} {
		if *l == nil {
			*l = []map[string]any{}
		}
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
