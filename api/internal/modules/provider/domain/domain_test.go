package domain

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/shared/kernel"
)

func TestSecretRefAndConfig(t *testing.T) {
	for ref, ok := range map[string]bool{"OPENAI_API_KEY": true, "GOOGLE_TTS_CREDENTIALS": true, "none": true, "sk-abc123": false, "": false, "openai_key": false, "DATABASE_URL": false, "S3_SECRET_KEY": false, "HOME": false} {
		var v kernel.ValidationErrors
		ValidateSecretRef(&v, ref)
		if (v.Err() == nil) != ok {
			t.Errorf("%q: %v", ref, v.Err())
		}
	}
	for cfg, ok := range map[string]bool{`{"base_url":"https://x"}`: true, `{}`: true, `{"api_key":"x"}`: false,
		`{"Access_Token":"x"}`: false, `[]`: false, `null`: false} {
		var v kernel.ValidationErrors
		ValidateConfig(&v, json.RawMessage(cfg))
		if (v.Err() == nil) != ok {
			t.Errorf("%s: %v", cfg, v.Err())
		}
	}
}

func TestValidatePricing(t *testing.T) {
	ok := []PriceItem{{Unit: "input_token", UnitPrice: decimal.RequireFromString("0.000002")}}
	if err := ValidatePricing("text", "2026-11", "USD", ok); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		kind, version, cur string
		items              []PriceItem
	}{
		{"text", "", "USD", ok},
		{"text", "v2", "VND", ok},
		{"text", "v2", "USD", nil},
		{"image", "v2", "USD", ok},
		{"text", "v2", "USD", []PriceItem{{Unit: "input_token", UnitPrice: decimal.NewFromInt(-1)}}},
		{"text", "v2", "USD", append(ok, ok...)},
	}
	for i, c := range cases {
		if err := ValidatePricing(c.kind, c.version, c.cur, c.items); !errors.Is(err, kernel.ErrValidation) {
			t.Errorf("case %d: %v", i, err)
		}
	}
}

func TestTemplateLifecycle(t *testing.T) {
	tp, err := NewTemplateVersion("content.facebook_post", 1, "sys", "user {{.Brief}}", json.RawMessage(`{"type":"object"}`), kernel.NewID(), time.Now())
	if err != nil || tp.Status != TemplateDraft {
		t.Fatal(err)
	}
	if err := tp.Activate(); err != nil {
		t.Fatal(err)
	}
	if err := tp.Activate(); !errors.Is(err, kernel.ErrInvalidTransition) {
		t.Fatal(err)
	}
	if err := tp.Retire(); err != nil {
		t.Fatal(err)
	}
	if err := tp.Retire(); !errors.Is(err, kernel.ErrInvalidTransition) {
		t.Fatal(err)
	}
	if err := tp.Activate(); err != nil {
		t.Fatal("rollback to a retired version is allowed")
	}
	if _, err := NewTemplateVersion("Bad Code", 1, "", "", json.RawMessage(`[1]`), kernel.NewID(), time.Now()); !errors.Is(err, kernel.ErrValidation) {
		t.Fatal(err)
	}
	de, _ := kernel.AsDomainError(func() error {
		_, err := NewTemplateVersion("Bad Code", 1, "", "", json.RawMessage(`[1]`), kernel.NewID(), time.Now())
		return err
	}())
	if len(de.Details) != 4 {
		t.Fatalf("all issues reported: %v", de.Details)
	}
}
