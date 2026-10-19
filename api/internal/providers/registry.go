// Package providers holds AI vendor adapters (one sub-package per vendor)
// and the Registry that maps ai_providers.code → adapter. Adapters
// implement the ports in internal/ports/ai.go and never leak vendor types.
package providers

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Registry implements ports.AIProviders. Adapters are registered once at
// start-up (Build); the provider module then pushes each ai_providers row
// (status, secret_ref, config_json) with ApplyProviderConfig, so disabling a
// provider in the admin UI takes effect without a restart.
type Registry struct {
	mu       sync.RWMutex
	text     map[string]ports.TextGenerator
	image    map[string]ports.ImageGenerator
	video    map[string]ports.VideoGenerator
	voice    map[string]ports.VoiceSynthesizer
	adapters map[string][]any
	disabled map[string]bool
}

// Configurable is implemented by adapters that accept an ai_providers row.
type Configurable interface {
	Configure(secretRef string, cfg json.RawMessage)
}

// StorageUser is implemented by adapters that park outputs in object
// storage (kenburns).
type StorageUser interface {
	UseStorage(s ports.ObjectStorage)
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		text: map[string]ports.TextGenerator{}, image: map[string]ports.ImageGenerator{},
		video: map[string]ports.VideoGenerator{}, voice: map[string]ports.VoiceSynthesizer{},
		adapters: map[string][]any{}, disabled: map[string]bool{},
	}
}

// ApplyProviderConfig applies an ai_providers row: active=false makes every
// capability of code unavailable (CAPABILITY_NOT_SUPPORTED); secretRef and
// cfg are handed to the adapters. Unknown codes are ignored (a row may exist
// before its adapter is deployed).
func (r *Registry) ApplyProviderConfig(code string, active bool, secretRef string, cfg json.RawMessage) {
	r.mu.Lock()
	r.disabled[code] = !active
	list := append([]any(nil), r.adapters[code]...)
	r.mu.Unlock()
	for _, a := range list {
		if c, ok := a.(Configurable); ok {
			c.Configure(secretRef, cfg)
		}
	}
}

// UseStorage hands object storage to adapters that need it.
func (r *Registry) UseStorage(s ports.ObjectStorage) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, list := range r.adapters {
		for _, a := range list {
			if u, ok := a.(StorageUser); ok {
				u.UseStorage(s)
			}
		}
	}
}

// Codes returns the registered provider codes per kind (sorted), for
// diagnostics and the admin API ("adapter_available").
func (r *Registry) Codes() map[ports.AIKind][]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[ports.AIKind][]string{}
	add := func(k ports.AIKind, code string) { out[k] = append(out[k], code) }
	for c := range r.text {
		add(ports.AIKindText, c)
	}
	for c := range r.image {
		add(ports.AIKindImage, c)
	}
	for c := range r.video {
		add(ports.AIKindVideo, c)
	}
	for c := range r.voice {
		add(ports.AIKindVoice, c)
	}
	for k := range out {
		sort.Strings(out[k])
	}
	return out
}

var _ ports.AIProviders = (*Registry)(nil)

// Register adds every capability adapter implements (text, image, video,
// voice) under its Code(). Registering the same code twice for a kind panics.
func (r *Registry) Register(adapter any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	code := ""
	if a, ok := adapter.(ports.TextGenerator); ok {
		put(r.text, a.Code(), a)
		n++
	}
	if a, ok := adapter.(ports.ImageGenerator); ok {
		put(r.image, a.Code(), a)
		n++
	}
	if a, ok := adapter.(ports.VideoGenerator); ok {
		put(r.video, a.Code(), a)
		n++
	}
	if a, ok := adapter.(ports.VoiceSynthesizer); ok {
		put(r.voice, a.Code(), a)
		n++
	}
	if n == 0 {
		panic(fmt.Sprintf("providers: %T implements no AI port", adapter))
	}
	if c, ok := adapter.(interface{ Code() string }); ok {
		code = c.Code()
	}
	r.adapters[code] = append(r.adapters[code], adapter)
}

func put[T any](m map[string]T, code string, v T) {
	if _, dup := m[code]; dup {
		panic("providers: duplicate adapter " + code)
	}
	m[code] = v
}

func get[T any](r *Registry, m map[string]T, kind ports.AIKind, code string) (T, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var zero T
	v, ok := m[code]
	if !ok {
		return zero, kernel.ErrCapability.WithMessage("Nhà cung cấp AI chưa được cấu hình").
			WithDetails(kernel.Detail{"field": "provider_code", "issue": "unsupported", "kind": string(kind), "value": code})
	}
	if r.disabled[code] {
		return zero, kernel.ErrCapability.WithMessage("Nhà cung cấp AI đang bị tắt").
			WithDetails(kernel.Detail{"field": "provider_code", "issue": "disabled", "kind": string(kind), "value": code})
	}
	return v, nil
}

// Text returns the text adapter for code.
func (r *Registry) Text(code string) (ports.TextGenerator, error) {
	return get(r, r.text, ports.AIKindText, code)
}

// Image returns the image adapter for code.
func (r *Registry) Image(code string) (ports.ImageGenerator, error) {
	return get(r, r.image, ports.AIKindImage, code)
}

// Video returns the video adapter for code.
func (r *Registry) Video(code string) (ports.VideoGenerator, error) {
	return get(r, r.video, ports.AIKindVideo, code)
}

// Voice returns the voice adapter for code.
func (r *Registry) Voice(code string) (ports.VoiceSynthesizer, error) {
	return get(r, r.voice, ports.AIKindVoice, code)
}
