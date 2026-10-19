package usecase

import (
	"context"
	"encoding/json"
	"slices"
	"sort"

	"github.com/be/acs/api/internal/modules/provider/domain"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Service implements API plus the admin use cases (ACS-090, ACS-038).
type Service struct {
	Tx       ports.TxManager
	Repo     Repository
	Clock    ports.Clock
	Audit    ports.AuditRecorder
	Registry AdapterRegistry // optional
	Secrets  SecretProbe     // optional
}

var _ API = (*Service)(nil)

func (s *Service) adapterAvailable(kind, code string) bool {
	if s.Registry == nil {
		return true
	}
	return slices.Contains(s.Registry.Codes()[ports.AIKind(kind)], code)
}

// DecodeCapabilities parses ai_models.capabilities (unknown keys ignored).
func DecodeCapabilities(raw json.RawMessage) ports.Capabilities {
	var c ports.Capabilities
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &c)
	}
	return c
}

func toInfo(m domain.Model) ModelInfo {
	return ModelInfo{ModelID: m.ID, ProviderCode: m.ProviderCode, ModelCode: m.ModelCode, Kind: ports.AIKind(m.Kind),
		Capabilities: DecodeCapabilities(m.Capabilities), ConcurrencyLimit: m.ConcurrencyLimit}
}

// ResolveModel implements API. With modelCode == "" it returns the active
// default of kind; if that default cannot run in this process (e.g. the
// fake provider in production) it falls back to the first usable model,
// real providers before "fake", ordered by provider and model code.
func (s *Service) ResolveModel(ctx context.Context, kind ports.AIKind, modelCode string) (ModelInfo, error) {
	all, err := s.Repo.ListModels(ctx)
	if err != nil {
		return ModelInfo{}, err
	}
	var usable []domain.Model
	for _, m := range all {
		if m.Kind == string(kind) && m.Usable() && s.adapterAvailable(m.Kind, m.ProviderCode) {
			usable = append(usable, m)
		}
	}
	sort.SliceStable(usable, func(i, j int) bool {
		a, b := usable[i], usable[j]
		if (a.ProviderCode == "fake") != (b.ProviderCode == "fake") {
			return b.ProviderCode == "fake"
		}
		if a.ProviderCode != b.ProviderCode {
			return a.ProviderCode < b.ProviderCode
		}
		return a.ModelCode < b.ModelCode
	})
	codes := make([]string, 0, len(usable))
	for _, m := range usable {
		codes = append(codes, m.ModelCode)
	}
	if modelCode != "" {
		for _, m := range usable {
			if m.ModelCode == modelCode {
				return toInfo(m), nil
			}
		}
		return ModelInfo{}, kernel.ErrCapability.WithMessage("Model không tồn tại hoặc đang bị tắt").
			WithDetails(kernel.Detail{"field": "model_code", "issue": "unsupported", "value": modelCode, "supported": codes})
	}
	for _, m := range usable {
		if m.IsDefault {
			return toInfo(m), nil
		}
	}
	if len(usable) > 0 {
		return toInfo(usable[0]), nil
	}
	return ModelInfo{}, kernel.ErrCapability.WithMessage("Chưa có model khả dụng cho loại tác vụ này").
		WithDetails(kernel.Detail{"field": "model_code", "issue": "no_model_available", "kind": string(kind), "supported": codes})
}

func toTemplate(t *domain.PromptTemplate) PromptTemplate {
	return PromptTemplate{ID: t.ID, Code: t.Code, Version: t.Version, SystemPrompt: t.SystemPrompt,
		UserTemplate: t.UserTemplate, OutputSchema: t.OutputSchema}
}

// ActivePromptTemplate implements API.
func (s *Service) ActivePromptTemplate(ctx context.Context, code string) (PromptTemplate, error) {
	t, err := s.Repo.ActiveTemplate(ctx, code)
	if err != nil {
		return PromptTemplate{}, err
	}
	return toTemplate(t), nil
}

// GetPromptTemplate implements API (any status: old jobs keep resolving
// the version they used).
func (s *Service) GetPromptTemplate(ctx context.Context, id kernel.ID) (PromptTemplate, error) {
	t, err := s.Repo.GetTemplate(ctx, id)
	if err != nil {
		return PromptTemplate{}, err
	}
	return toTemplate(t), nil
}

// ListProviders implements API (internal: includes secret_ref names).
func (s *Service) ListProviders(ctx context.Context) ([]ProviderConfig, error) {
	ps, err := s.Repo.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ProviderConfig, 0, len(ps))
	for _, p := range ps {
		kinds := make([]ports.AIKind, 0, len(p.Kinds))
		for _, k := range p.Kinds {
			kinds = append(kinds, ports.AIKind(k))
		}
		out = append(out, ProviderConfig{Code: p.Code, Kinds: kinds, Active: p.Status == domain.StatusActive, SecretRef: p.SecretRef, Config: p.Config})
	}
	return out, nil
}

// SyncRegistry pushes every ai_providers row into the adapter registry
// (start-up and periodic refresh, so workers see admin changes).
func (s *Service) SyncRegistry(ctx context.Context) error {
	if s.Registry == nil {
		return nil
	}
	ps, err := s.Repo.ListProviders(ctx)
	if err != nil {
		return err
	}
	for _, p := range ps {
		s.Registry.ApplyProviderConfig(p.Code, p.Status == domain.StatusActive, p.SecretRef, p.Config)
	}
	return nil
}
