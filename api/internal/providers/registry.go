// Package providers holds AI vendor adapters (one sub-package per vendor)
// and the Registry that maps ai_providers.code → adapter. Adapters
// implement the ports in internal/ports/ai.go and never leak vendor types.
package providers

import (
	"fmt"
	"sync"

	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Registry implements ports.AIProviders.
type Registry struct {
	mu    sync.RWMutex
	text  map[string]ports.TextGenerator
	image map[string]ports.ImageGenerator
	video map[string]ports.VideoGenerator
	voice map[string]ports.VoiceSynthesizer
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		text: map[string]ports.TextGenerator{}, image: map[string]ports.ImageGenerator{},
		video: map[string]ports.VideoGenerator{}, voice: map[string]ports.VoiceSynthesizer{},
	}
}

var _ ports.AIProviders = (*Registry)(nil)

// Register adds every capability adapter implements (text, image, video,
// voice) under its Code(). Registering the same code twice for a kind panics.
func (r *Registry) Register(adapter any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
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
}

func put[T any](m map[string]T, code string, v T) {
	if _, dup := m[code]; dup {
		panic("providers: duplicate adapter " + code)
	}
	m[code] = v
}

func get[T any](mu *sync.RWMutex, m map[string]T, kind ports.AIKind, code string) (T, error) {
	mu.RLock()
	defer mu.RUnlock()
	v, ok := m[code]
	if !ok {
		var zero T
		return zero, kernel.ErrCapability.WithMessage("Nhà cung cấp AI chưa được cấu hình").
			WithDetails(kernel.Detail{"field": "provider_code", "kind": string(kind), "value": code})
	}
	return v, nil
}

// Text returns the text adapter for code.
func (r *Registry) Text(code string) (ports.TextGenerator, error) {
	return get(&r.mu, r.text, ports.AIKindText, code)
}

// Image returns the image adapter for code.
func (r *Registry) Image(code string) (ports.ImageGenerator, error) {
	return get(&r.mu, r.image, ports.AIKindImage, code)
}

// Video returns the video adapter for code.
func (r *Registry) Video(code string) (ports.VideoGenerator, error) {
	return get(&r.mu, r.video, ports.AIKindVideo, code)
}

// Voice returns the voice adapter for code.
func (r *Registry) Voice(code string) (ports.VoiceSynthesizer, error) {
	return get(&r.mu, r.voice, ports.AIKindVoice, code)
}
