package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"unicode/utf8"

	"github.com/shopspring/decimal"

	assetuc "github.com/be/acs/api/internal/modules/asset/usecase"
	branduc "github.com/be/acs/api/internal/modules/brand/usecase"
	budgetuc "github.com/be/acs/api/internal/modules/budget/usecase"
	campaignuc "github.com/be/acs/api/internal/modules/campaign/usecase"
	"github.com/be/acs/api/internal/modules/content/domain"
	"github.com/be/acs/api/internal/modules/content/usecase/prompt"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	provideruc "github.com/be/acs/api/internal/modules/provider/usecase"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Step types registered by the content module (unique across modules).
const (
	StepGeneration   = "content_generation"
	StepRewrite      = "content_rewrite"
	StepTransform    = "content_transform"
	StepPlan         = "content_plan"
	StepImagePrompts = "content_image_prompt_suggest"
)

// TextProviders resolves the text adapter of a provider (ports.AIProviders
// satisfies it).
type TextProviders interface {
	Text(providerCode string) (ports.TextGenerator, error)
}

// Service implements every content use case and the public API.
type Service struct {
	Tx        ports.TxManager
	Clock     ports.Clock
	Jobs      jobuc.API
	Budget    budgetuc.API
	Assets    assetuc.API
	Brands    branduc.API
	Campaigns campaignuc.API
	Providers provideruc.API
	AI        TextProviders
}

var _ API = (*Service)(nil)

// template returns the active template for code, falling back to the
// built-in v1 (identical to the seed) when the provider module has none.
func (s *Service) template(ctx context.Context, code string) (prompt.Template, error) {
	t, err := s.Providers.ActivePromptTemplate(ctx, code)
	if err == nil {
		id := t.ID
		return prompt.Template{ID: &id, Code: t.Code, Version: t.Version, System: t.SystemPrompt, User: t.UserTemplate, Schema: t.OutputSchema}, nil
	}
	if errors.Is(err, kernel.ErrNotFound) {
		if d, ok := prompt.Defaults[code]; ok {
			return d, nil
		}
	}
	return prompt.Template{}, err
}

func brandForPrompt(bc branduc.BrandContext) prompt.Brand {
	cs := make([]prompt.Color, 0, len(bc.Colors))
	for _, c := range bc.Colors {
		cs = append(cs, prompt.Color{Name: c.Name, Hex: c.Hex})
	}
	return prompt.Brand{
		Name: bc.Name, Description: bc.Description, Tone: bc.ToneOfVoice, PreferredWords: bc.PreferredWords,
		ForbiddenWords: bc.ForbiddenWords, DefaultCTAs: bc.DefaultCTAs, Colors: cs,
	}
}

// claimJSON is a serializable fact amount.
type claimJSON struct {
	Kind     string  `json:"kind"`
	Value    float64 `json:"value"`
	Currency string  `json:"currency"`
}

type productJSON struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	USP         []string    `json:"usp,omitempty"`
	Price       *priceJSON  `json:"price,omitempty"`
	Offers      []offerJSON `json:"offers,omitempty"`
	Policies    []string    `json:"policies,omitempty"`
	Proof       []string    `json:"proof,omitempty"`
}

type priceJSON struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
	Note     string `json:"note,omitempty"`
}

type offerJSON struct {
	Text    string `json:"text"`
	ValidTo string `json:"valid_to,omitempty"`
}

// factsContext holds verified facts for the prompt and the post-checks.
type factsContext struct {
	Products []productJSON
	Texts    []string
	Amounts  []claimJSON
}

func buildFacts(products []branduc.ProductFacts, brief *BriefJSON) factsContext {
	fc := factsContext{Products: []productJSON{}}
	for _, p := range products {
		pj := productJSON{Name: p.Name, Description: p.Description, USP: p.USP, Policies: p.Policies}
		fc.Texts = append(fc.Texts, p.Name, p.Description)
		fc.Texts = append(fc.Texts, p.USP...)
		fc.Texts = append(fc.Texts, p.Policies...)
		if p.Price != nil {
			pj.Price = &priceJSON{Amount: p.Price.Amount.String(), Currency: p.Price.Currency, Note: p.Price.Note}
			v, _ := p.Price.Amount.Float64()
			cur := p.Price.Currency
			if cur == "" {
				cur = "VND"
			}
			fc.Amounts = append(fc.Amounts, claimJSON{Kind: string(domain.ClaimMoney), Value: v, Currency: cur})
			fc.Texts = append(fc.Texts, p.Price.Note)
		}
		for _, o := range p.Offers {
			oj := offerJSON{Text: o.Text}
			if o.ValidTo != nil {
				oj.ValidTo = o.ValidTo.Format("2006-01-02")
			}
			pj.Offers = append(pj.Offers, oj)
			fc.Texts = append(fc.Texts, o.Text)
		}
		for _, pr := range p.Proof {
			pj.Proof = append(pj.Proof, pr.Text)
			fc.Texts = append(fc.Texts, pr.Text)
		}
		fc.Products = append(fc.Products, pj)
	}
	fc.Texts = append(fc.Texts, brief.texts()...)
	return fc
}

func factsFrom(texts []string, amounts []claimJSON) domain.Facts {
	cs := make([]domain.Claim, 0, len(amounts))
	for _, a := range amounts {
		cs = append(cs, domain.Claim{Kind: domain.ClaimKind(a.Kind), Value: a.Value, Currency: a.Currency})
	}
	return domain.NewFacts(texts, cs...)
}

// stepInput is persisted as ai_job_steps.input_json: the fully built
// prompt plus everything the worker needs, so a retry reproduces exactly
// the same request and the job stays auditable.
type stepInput struct {
	System      string          `json:"system"`
	User        string          `json:"user"`
	Schema      json.RawMessage `json:"schema"`
	SchemaName  string          `json:"schema_name"`
	MaxTokens   int             `json:"max_tokens"`
	Temperature float32         `json:"temperature"`

	Format   string `json:"format,omitempty"`
	Platform string `json:"platform,omitempty"`
	Language string `json:"language,omitempty"`
	Variants int    `json:"variants,omitempty"`

	ForbiddenWords []string    `json:"forbidden_words,omitempty"`
	FactTexts      []string    `json:"fact_texts,omitempty"`
	FactAmounts    []claimJSON `json:"fact_amounts,omitempty"`

	TemplateCode    string      `json:"template_code"`
	TemplateVersion int         `json:"template_version"`
	TemplateID      *kernel.ID  `json:"template_id,omitempty"`
	BrandID         kernel.ID   `json:"brand_id"`
	BrandVersionID  kernel.ID   `json:"brand_version_id"`
	BrandVersionNo  int         `json:"brand_version_no"`
	ProductIDs      []kernel.ID `json:"product_ids,omitempty"`
	Brief           *BriefJSON  `json:"brief,omitempty"`

	SourceAssetID *kernel.ID `json:"source_asset_id,omitempty"`
	BaseVersionNo int        `json:"base_version_no,omitempty"`
	ChangeNote    string     `json:"change_note,omitempty"`

	Colors []prompt.Color `json:"colors,omitempty"`
	Tone   []string       `json:"tone,omitempty"`

	// PlanChannels / PlanFormats are the campaign brief's channel and
	// format slugs; content_plan output is snapped onto them.
	PlanChannels []string `json:"plan_channels,omitempty"`
	PlanFormats  []string `json:"plan_formats,omitempty"`
}

// jobPlan is what a request use case hands to start().
type jobPlan struct {
	Type       jobuc.JobType
	StepType   string
	Actor      kernel.Actor
	CampaignID *kernel.ID
	AssetID    *kernel.ID
	Model      provideruc.ModelInfo
	Template   prompt.Template
	Prompt     prompt.Prompt
	Input      stepInput
	Request    any
	Idem       string
	Hash       string
	Override   bool
}

func estimateInputTokens(p prompt.Prompt) int64 {
	return int64(utf8.RuneCountInString(p.System)+utf8.RuneCountInString(p.User))/3 + 1
}

// maxTokens caps a requested budget by the model capability.
func maxTokens(m provideruc.ModelInfo, want int) int {
	if c := m.Capabilities.MaxOutputTokens; c > 0 && want > c {
		return c
	}
	return want
}

// resolveTextModel validates the model and language capability (422).
func (s *Service) resolveTextModel(ctx context.Context, code, language string) (provideruc.ModelInfo, error) {
	m, err := s.Providers.ResolveModel(ctx, ports.AIKindText, code)
	if err != nil {
		return m, err
	}
	if langs := m.Capabilities.Languages; language != "" && len(langs) > 0 && !slices.Contains(langs, language) {
		return m, kernel.ErrCapability.WithDetails(kernel.Detail{"field": "language", "supported": langs})
	}
	return m, nil
}

// start runs SPEC-06 §2.4: estimate → tx{Jobs.Create + Budget.Reserve}.
func (s *Service) start(ctx context.Context, p jobPlan) (JobAccepted, error) {
	if p.Override && !p.Actor.IsLeadOrAdmin() {
		return JobAccepted{}, kernel.ErrForbidden.WithMessage("Chỉ Marketing Lead hoặc Admin được vượt hạn mức mỗi job")
	}
	in := p.Input
	in.System, in.User, in.Schema, in.SchemaName = p.Prompt.System, p.Prompt.User, p.Prompt.Schema, p.Prompt.SchemaName
	in.TemplateCode, in.TemplateVersion, in.TemplateID = p.Template.Code, p.Template.Version, p.Template.ID
	in.MaxTokens = maxTokens(p.Model, in.MaxTokens)
	if in.Temperature == 0 {
		in.Temperature = 0.7
	}
	est, err := s.Budget.Estimate(ctx, budgetuc.EstimateRequest{Category: budgetuc.CategoryText, Items: []budgetuc.EstimateItem{
		{Item: "generation", ProviderCode: p.Model.ProviderCode, ModelCode: p.Model.ModelCode, Unit: "input_token", Units: decimal.NewFromInt(estimateInputTokens(p.Prompt))},
		{Item: "generation", ProviderCode: p.Model.ProviderCode, ModelCode: p.Model.ModelCode, Unit: "output_token", Units: decimal.NewFromInt(int64(in.MaxTokens))},
	}})
	if err != nil {
		return JobAccepted{}, err
	}
	input, err := json.Marshal(in)
	if err != nil {
		return JobAccepted{}, err
	}
	request, err := json.Marshal(map[string]any{
		"params": p.Request, "prompt_template_code": p.Template.Code, "prompt_template_version": p.Template.Version,
	})
	if err != nil {
		return JobAccepted{}, err
	}
	bv := in.BrandVersionID
	var bvp *kernel.ID
	if bv != kernel.NilID {
		bvp = &bv
	}
	spec := jobuc.JobSpec{
		Type: p.Type, CreatedBy: p.Actor.UserID, CampaignID: p.CampaignID, AssetID: p.AssetID, BrandVersionID: bvp,
		PromptTemplateID: p.Template.ID, ProviderCode: p.Model.ProviderCode, ModelCode: p.Model.ModelCode,
		Request: request, IdempotencyKey: p.Idem, RequestHash: p.Hash, CostEstimate: est.Total, PricingVersion: est.PricingVersion,
		Steps: []jobuc.StepSpec{{Key: p.StepType, Type: p.StepType, Input: input}},
	}
	var override *kernel.Actor
	if p.Override {
		a := p.Actor
		override = &a
		spec.BudgetOverrideBy = &a.UserID
	}
	var out JobAccepted
	err = s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		res, err := s.Jobs.Create(ctx, spec)
		if err != nil {
			return err
		}
		out = JobAccepted{JobID: res.JobID, Status: string(res.Status), EstimatedCost: res.CostEstimate}
		if res.Existing {
			return nil
		}
		_, err = s.Budget.Reserve(ctx, budgetuc.ReserveRequest{
			JobID: res.JobID, CampaignID: p.CampaignID, Amount: est.Total, Category: budgetuc.CategoryText, Override: override,
		})
		return err
	})
	if err != nil {
		return JobAccepted{}, err
	}
	if out.EstimatedCost.Currency == "" {
		out.EstimatedCost = est.Total
	}
	return out, nil
}

// tokensPerVariant is the output budget per variant by length.
func tokensPerVariant(length, format string) int {
	n := map[string]int{"short": 500, "medium": 900, "long": 1500}[length]
	if n == 0 {
		n = 900
	}
	if format == string(domain.FormatVideoScript) {
		n += 400
	}
	return n
}

func validationFor(field, issue string) error {
	return kernel.Validation(kernel.FieldIssue(field, issue))
}

func itoa(i int) string { return strconv.Itoa(i) }
