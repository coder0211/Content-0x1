// Package domain holds the provider-configuration entities (ai_providers,
// ai_models, model_pricing, prompt_templates) and their invariants.
package domain

import (
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/shared/kernel"
)

// Status of a provider or model.
type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool { return s == StatusActive || s == StatusDisabled }

// Kinds are the AI capability families (ai_models.kind).
var Kinds = []string{"text", "image", "video", "voice"}

// Provider is an ai_providers row. SecretRef is the NAME of the env var /
// secret holding the key — never the key itself.
type Provider struct {
	ID        kernel.ID
	Code      string
	Name      string
	Kinds     []string
	Status    Status
	SecretRef string
	Config    json.RawMessage
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Model is an ai_models row joined with its provider code.
type Model struct {
	ID               kernel.ID
	ProviderID       kernel.ID
	ProviderCode     string
	ProviderStatus   Status
	ModelCode        string
	Kind             string
	Capabilities     json.RawMessage
	ConcurrencyLimit int
	IsDefault        bool
	Status           Status
}

// Usable reports whether the model and its provider are active.
func (m Model) Usable() bool { return m.Status == StatusActive && m.ProviderStatus == StatusActive }

// Price is one model_pricing row.
type Price struct {
	ID             kernel.ID
	ModelID        kernel.ID
	PricingVersion string
	Unit           string
	UnitPrice      decimal.Decimal
	Currency       string
	EffectiveFrom  time.Time
	EffectiveTo    *time.Time
}

var (
	secretRefRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,63}$`)
	modelCodeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9.\-_]{0,79}$`)
	versionRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.\-_]{0,39}$`)
)

// ValidateSecretRef checks a secret reference: an env-var style name or
// "none" (providers without a key). Anything that looks like an actual key
// (lowercase, dashes, long random strings) is rejected so admins cannot
// paste a secret into the database by mistake.
func ValidateSecretRef(v *kernel.ValidationErrors, ref string) {
	if ref == "none" {
		return
	}
	if !secretRefRe.MatchString(ref) {
		v.AddDetail(kernel.Detail{"field": "secret_ref", "issue": "invalid_format",
			"hint": "Tên biến môi trường, ví dụ OPENAI_API_KEY (không nhập giá trị key)"})
	}
}

// secretish lists config_json key fragments that would indicate a secret.
var secretish = []string{"key", "secret", "token", "password", "credential", "auth"}

// ValidateConfig checks config_json: a JSON object (≤ 8 KB) whose keys do
// not look like secrets (secrets live behind secret_ref only).
func ValidateConfig(v *kernel.ValidationErrors, cfg json.RawMessage) {
	if len(cfg) > 8<<10 {
		v.Add("config_json", "too_large")
		return
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(cfg, &m); err != nil || m == nil {
		v.Add("config_json", "must_be_object")
		return
	}
	for k := range m {
		lk := strings.ToLower(k)
		for _, s := range secretish {
			if strings.Contains(lk, s) {
				v.AddDetail(kernel.Detail{"field": "config_json." + k, "issue": "secret_not_allowed"})
			}
		}
	}
}

// ValidateModelCode checks a vendor model identifier.
func ValidateModelCode(v *kernel.ValidationErrors, code string) {
	if !modelCodeRe.MatchString(code) {
		v.Add("model_code", "invalid_format")
	}
}

// ValidateConcurrency checks concurrency_limit.
func ValidateConcurrency(v *kernel.ValidationErrors, n int) {
	if n < 1 || n > 100 {
		v.AddDetail(kernel.Detail{"field": "concurrency_limit", "issue": "out_of_range", "min": 1, "max": 100})
	}
}

// UnitsByKind lists the pricing units the adapters report (ports.Usage.Units).
var UnitsByKind = map[string][]string{
	"text":  {"input_token", "output_token", "cache_read_token", "cache_write_token"},
	"image": {"image_1024_standard", "image_1024_high"},
	"video": {"video_second_720p", "video_second_1080p"},
	"voice": {"char"},
}

// PriceItem is one unit of a new pricing version.
type PriceItem struct {
	Unit      string
	UnitPrice decimal.Decimal
}

// ValidatePricing checks a new pricing version for a model of kind.
func ValidatePricing(kind, version, currency string, items []PriceItem) error {
	var v kernel.ValidationErrors
	if !versionRe.MatchString(version) {
		v.Add("pricing_version", "invalid_format")
	}
	if currency != kernel.CurrencyUSD {
		v.AddDetail(kernel.Detail{"field": "currency", "issue": "unsupported", "supported": []string{kernel.CurrencyUSD}})
	}
	if len(items) == 0 {
		v.Add("items", "required")
	}
	seen := map[string]bool{}
	allowed := UnitsByKind[kind]
	for i, it := range items {
		f := "items[" + strconv.Itoa(i) + "]"
		if !slices.Contains(allowed, it.Unit) {
			v.AddDetail(kernel.Detail{"field": f + ".unit", "issue": "unsupported", "value": it.Unit, "supported": allowed})
		}
		if seen[it.Unit] {
			v.Add(f+".unit", "duplicate")
		}
		seen[it.Unit] = true
		if it.UnitPrice.IsNegative() {
			v.Add(f+".unit_price", "negative")
		}
		if it.UnitPrice.GreaterThan(decimal.NewFromInt(1000)) {
			v.Add(f+".unit_price", "too_large")
		}
	}
	return v.Err()
}
