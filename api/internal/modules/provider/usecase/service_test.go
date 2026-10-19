package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/modules/provider/domain"
	"github.com/be/acs/api/internal/modules/provider/usecase"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

type fakeRegistry struct {
	codes   map[ports.AIKind][]string
	applied map[string]bool
}

func (f *fakeRegistry) ApplyProviderConfig(code string, active bool, _ string, _ json.RawMessage) {
	f.applied[code] = active
}
func (f *fakeRegistry) Codes() map[ports.AIKind][]string { return f.codes }

type fixture struct {
	svc   *usecase.Service
	repo  *usecase.MemoryRepo
	audit *testutil.Audit
	reg   *fakeRegistry
	clock *testutil.Clock
	ids   map[string]kernel.ID
}

func newFixture() *fixture {
	repo := &usecase.MemoryRepo{}
	ids := map[string]kernel.ID{}
	id := func(k string) kernel.ID { ids[k] = kernel.NewID(); return ids[k] }
	fake := repo.Seed("fake", []string{"text", "video"},
		domain.Model{ID: id("fake-text"), ModelCode: "fake-text", Kind: "text", IsDefault: true},
		domain.Model{ID: id("fake-video"), ModelCode: "fake-video", Kind: "video", IsDefault: true})
	ids["fake"] = fake.ID
	ids["anthropic"] = repo.Seed("anthropic", []string{"text"},
		domain.Model{ID: id("claude"), ModelCode: "claude-sonnet-5-5", Kind: "text", Capabilities: []byte(`{"max_output_tokens":64000}`)}).ID
	ids["veo"] = repo.Seed("veo", []string{"video"},
		domain.Model{ID: id("veo"), ModelCode: "veo-3.1-generate-preview", Kind: "video"}).ID
	ids["kenburns"] = repo.Seed("kenburns", []string{"video"},
		domain.Model{ID: id("kenburns"), ModelCode: "kenburns-zoompan", Kind: "video"}).ID
	reg := &fakeRegistry{applied: map[string]bool{}, codes: map[ports.AIKind][]string{
		ports.AIKindText: {"anthropic", "fake"}, ports.AIKindVideo: {"fake", "kenburns", "veo"}}}
	au := &testutil.Audit{}
	clock := testutil.NewClock()
	svc := &usecase.Service{Tx: &testutil.Tx{}, Repo: repo, Clock: clock, Audit: au, Registry: reg,
		Secrets: func(ref string) bool { return ref == "ANTHROPIC_API_KEY" }}
	return &fixture{svc: svc, repo: repo, audit: au, reg: reg, clock: clock, ids: ids}
}

var ctx = context.Background()

func TestResolveModel(t *testing.T) {
	f := newFixture()
	m, err := f.svc.ResolveModel(ctx, ports.AIKindText, "")
	if err != nil || m.ModelCode != "fake-text" {
		t.Fatalf("default: %v %+v", err, m)
	}
	m, err = f.svc.ResolveModel(ctx, ports.AIKindText, "claude-sonnet-5-5")
	if err != nil || m.ProviderCode != "anthropic" || m.Capabilities.MaxOutputTokens != 64000 {
		t.Fatalf("explicit: %v %+v", err, m)
	}
	// Fake adapter not registered (production): fall back to a real model.
	f.reg.codes[ports.AIKindText] = []string{"anthropic"}
	f.reg.codes[ports.AIKindVideo] = []string{"kenburns", "veo"}
	if m, _ := f.svc.ResolveModel(ctx, ports.AIKindText, ""); m.ModelCode != "claude-sonnet-5-5" {
		t.Fatalf("fallback: %+v", m)
	}
	if m, _ := f.svc.ResolveModel(ctx, ports.AIKindVideo, ""); m.ProviderCode != "kenburns" {
		t.Fatalf("fallback order: %+v", m)
	}
	// Disabled provider → 422 with the supported list.
	disabled := "disabled"
	if _, err := f.svc.UpdateProvider(ctx, testutil.Actor(kernel.RoleAdmin), f.ids["anthropic"], usecase.UpdateProviderInput{Status: &disabled}); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.ResolveModel(ctx, ports.AIKindText, "claude-sonnet-5-5")
	de, ok := kernel.AsDomainError(err)
	if !ok || !errors.Is(err, kernel.ErrCapability) || de.Details[0]["field"] != "model_code" {
		t.Fatal(err)
	}
	if _, err := f.svc.ResolveModel(ctx, ports.AIKindVoice, ""); !errors.Is(err, kernel.ErrCapability) {
		t.Fatal(err)
	}
}

func TestAdminRBAC(t *testing.T) {
	f := newFixture()
	for _, tc := range []struct {
		role         string
		view, manage bool
	}{
		{kernel.RoleAdmin, true, true}, {kernel.RoleMarketingLead, true, false},
		{kernel.RoleContentCreator, false, false}, {kernel.RoleReviewer, false, false},
	} {
		a := testutil.Actor(tc.role)
		_, err := f.svc.AdminListProviders(ctx, a)
		if (err == nil) != tc.view {
			t.Errorf("%s list providers: %v", tc.role, err)
		}
		_, err = f.svc.AdminListModels(ctx, a, "")
		if (err == nil) != tc.view {
			t.Errorf("%s list models: %v", tc.role, err)
		}
		_, err = f.svc.ListPricing(ctx, a, f.ids["claude"])
		if (err == nil) != tc.view {
			t.Errorf("%s pricing: %v", tc.role, err)
		}
		n := 3
		_, err = f.svc.UpdateModel(ctx, a, f.ids["claude"], usecase.UpdateModelInput{ConcurrencyLimit: &n})
		if (err == nil) != tc.manage || (err != nil && !errors.Is(err, kernel.ErrForbidden)) {
			t.Errorf("%s update model: %v", tc.role, err)
		}
		_, err = f.svc.UpdateProvider(ctx, a, f.ids["veo"], usecase.UpdateProviderInput{})
		if (err == nil) != tc.manage {
			t.Errorf("%s update provider: %v", tc.role, err)
		}
		_, err = f.svc.AddPricingVersion(ctx, a, f.ids["claude"], usecase.AddPricingInput{Version: "v-" + tc.role,
			Items: []domain.PriceItem{{Unit: "input_token", UnitPrice: decimal.RequireFromString("0.000002")}}})
		if (err == nil) != tc.manage {
			t.Errorf("%s add pricing: %v", tc.role, err)
		}
		f.clock.Advance(time.Minute)
	}
}

func TestUpdateProviderValidatesAndAppliesWithoutSecrets(t *testing.T) {
	f := newFixture()
	admin := testutil.Actor(kernel.RoleAdmin)
	bad := "sk-live-abc"
	if _, err := f.svc.UpdateProvider(ctx, admin, f.ids["anthropic"], usecase.UpdateProviderInput{SecretRef: &bad}); !errors.Is(err, kernel.ErrValidation) {
		t.Fatal("a pasted key must be rejected", err)
	}
	if _, err := f.svc.UpdateProvider(ctx, admin, f.ids["anthropic"], usecase.UpdateProviderInput{Config: json.RawMessage(`{"api_key":"x"}`)}); !errors.Is(err, kernel.ErrValidation) {
		t.Fatal("secrets in config_json must be rejected", err)
	}
	ref, st := "ANTHROPIC_API_KEY", "active"
	v, err := f.svc.UpdateProvider(ctx, admin, f.ids["anthropic"], usecase.UpdateProviderInput{SecretRef: &ref, Status: &st,
		Config: json.RawMessage(`{"effort":"low"}`)})
	if err != nil || !v.SecretConfigured || !v.AdapterAvailable || v.SecretRef != ref {
		t.Fatalf("%v %+v", err, v)
	}
	if !f.reg.applied["anthropic"] || len(f.audit.Entries) != 1 || f.audit.Entries[0].Action != "provider.update" {
		t.Fatalf("registry/audit: %+v %+v", f.reg.applied, f.audit.Entries)
	}
	if _, err := f.svc.UpdateProvider(ctx, admin, kernel.NewID(), usecase.UpdateProviderInput{}); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatal(err)
	}
	if err := f.svc.SyncRegistry(ctx); err != nil || len(f.reg.applied) != 4 {
		t.Fatal(err, f.reg.applied)
	}
}

func TestModelsDefaultAndCreate(t *testing.T) {
	f := newFixture()
	admin := testutil.Actor(kernel.RoleAdmin)
	yes := true
	if _, err := f.svc.UpdateModel(ctx, admin, f.ids["claude"], usecase.UpdateModelInput{IsDefault: &yes}); err != nil {
		t.Fatal(err)
	}
	if m, _ := f.svc.ResolveModel(ctx, ports.AIKindText, ""); m.ModelCode != "claude-sonnet-5-5" {
		t.Fatalf("new default: %+v", m)
	}
	ms, _ := f.svc.AdminListModels(ctx, admin, "text")
	defaults := 0
	for _, m := range ms {
		if m.IsDefault {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("exactly one default per kind, got %d", defaults)
	}
	off := "disabled"
	m, err := f.svc.UpdateModel(ctx, admin, f.ids["claude"], usecase.UpdateModelInput{Status: &off})
	if err != nil || m.IsDefault {
		t.Fatal("disabling clears default", err)
	}
	if _, err := f.svc.UpdateModel(ctx, admin, f.ids["claude"], usecase.UpdateModelInput{Capabilities: json.RawMessage(`{"max_output_token":5}`)}); !errors.Is(err, kernel.ErrValidation) {
		t.Fatal("unknown capability key", err)
	}
	in := usecase.CreateModelInput{ProviderID: f.ids["veo"], ModelCode: "veo-3.1-fast-generate-preview", Kind: "video",
		Capabilities: json.RawMessage(`{"aspect_ratios":["16:9"],"max_duration_sec":8}`), ConcurrencyLimit: 2}
	if _, err := f.svc.CreateModel(ctx, admin, in); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateModel(ctx, admin, in); !errors.Is(err, kernel.ErrConflict) {
		t.Fatal(err)
	}
	in.ModelCode, in.Kind = "veo-text", "text"
	if _, err := f.svc.CreateModel(ctx, admin, in); !errors.Is(err, kernel.ErrValidation) {
		t.Fatal("kind not offered by provider", err)
	}
}

func TestPricingVersionsAreAppendOnly(t *testing.T) {
	f := newFixture()
	admin := testutil.Actor(kernel.RoleAdmin)
	items := []domain.PriceItem{{Unit: "input_token", UnitPrice: decimal.RequireFromString("0.000002")},
		{Unit: "output_token", UnitPrice: decimal.RequireFromString("0.00001")}}
	v1, err := f.svc.AddPricingVersion(ctx, admin, f.ids["claude"], usecase.AddPricingInput{Version: "2026-10", Items: items})
	if err != nil || len(v1.Items) != 2 || !v1.Current {
		t.Fatalf("%v %+v", err, v1)
	}
	if _, err := f.svc.AddPricingVersion(ctx, admin, f.ids["claude"], usecase.AddPricingInput{Version: "2026-10", Items: items}); !errors.Is(err, kernel.ErrConflict) {
		t.Fatal("same version twice", err)
	}
	past := f.clock.Now().Add(-time.Hour)
	if _, err := f.svc.AddPricingVersion(ctx, admin, f.ids["claude"], usecase.AddPricingInput{Version: "old", EffectiveFrom: &past, Items: items}); !errors.Is(err, kernel.ErrValidation) {
		t.Fatal("backdating", err)
	}
	if _, err := f.svc.AddPricingVersion(ctx, admin, f.ids["claude"], usecase.AddPricingInput{Version: "img", Items: []domain.PriceItem{{Unit: "char", UnitPrice: decimal.NewFromInt(1)}}}); !errors.Is(err, kernel.ErrValidation) {
		t.Fatal("unit of another kind", err)
	}
	f.clock.Advance(time.Hour)
	if _, err := f.svc.AddPricingVersion(ctx, admin, f.ids["claude"], usecase.AddPricingInput{Version: "2026-11", Items: items[:1]}); err != nil {
		t.Fatal(err)
	}
	list, _ := f.svc.ListPricing(ctx, admin, f.ids["claude"])
	if len(list) != 2 || list[0].Version != "2026-11" || !list[0].Current || list[1].Current || list[1].EffectiveTo == nil {
		t.Fatalf("%+v", list)
	}
	// Old prices are untouched.
	if !list[1].Items[0].UnitPrice.Equal(decimal.RequireFromString("0.000002")) {
		t.Fatal("old version modified")
	}
	if f.audit.Entries[len(f.audit.Entries)-1].Action != "pricing.add" {
		t.Fatal("audit")
	}
}

func TestPromptTemplates(t *testing.T) {
	f := newFixture()
	lead := testutil.Actor(kernel.RoleMarketingLead)
	in := usecase.CreateTemplateInput{Code: "content.facebook_post", SystemPrompt: "Bạn là copywriter", UserTemplate: "{{.Brief}}",
		OutputSchema: json.RawMessage(`{"type":"object"}`)}
	v1, err := f.svc.CreateTemplateVersion(ctx, lead, usecase.CreateTemplateInput{Code: in.Code, SystemPrompt: in.SystemPrompt, UserTemplate: in.UserTemplate, Activate: true})
	if err != nil || v1.Version != 1 || v1.Status != domain.TemplateActive {
		t.Fatalf("%v %+v", err, v1)
	}
	v2, err := f.svc.CreateTemplateVersion(ctx, lead, in)
	if err != nil || v2.Version != 2 || v2.Status != domain.TemplateDraft {
		t.Fatalf("%v %+v", err, v2)
	}
	if a, _ := f.svc.ActivePromptTemplate(ctx, in.Code); a.ID != v1.ID {
		t.Fatal("draft must not be used by new jobs")
	}
	if _, err := f.svc.ActivateTemplate(ctx, testutil.Actor(kernel.RoleAdmin), v2.ID); err != nil {
		t.Fatal(err)
	}
	a, _ := f.svc.ActivePromptTemplate(ctx, in.Code)
	if a.ID != v2.ID || !strings.Contains(string(a.OutputSchema), "object") {
		t.Fatalf("%+v", a)
	}
	// Old job still resolves the retired version it used.
	old, err := f.svc.GetPromptTemplate(ctx, v1.ID)
	if err != nil || old.Version != 1 {
		t.Fatal(err)
	}
	list, _ := f.svc.ListTemplates(ctx, lead, in.Code)
	if len(list) != 2 || list[0].Version != 2 || list[1].Status != domain.TemplateRetired {
		t.Fatalf("%+v", list)
	}
	if _, err := f.svc.CreateTemplateVersion(ctx, testutil.Actor(kernel.RoleContentCreator), in); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := f.svc.ListTemplates(ctx, testutil.Actor(kernel.RoleReviewer), ""); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := f.svc.RetireTemplate(ctx, lead, v2.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.ActivePromptTemplate(ctx, in.Code); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := f.svc.RetireTemplate(ctx, lead, v2.ID); !errors.Is(err, kernel.ErrInvalidTransition) {
		t.Fatal(err)
	}
	actions := map[string]int{}
	for _, e := range f.audit.Entries {
		actions[e.Action]++
	}
	if actions["prompt_template.create"] != 2 || actions["prompt_template.activate"] != 2 || actions["prompt_template.retire"] != 2 {
		t.Fatalf("%v", actions)
	}
}
