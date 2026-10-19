package providers

import (
	"errors"
	"testing"

	"github.com/be/acs/api/internal/platform/config"
	"github.com/be/acs/api/internal/shared/kernel"
)

func TestBuildRegistersFake(t *testing.T) {
	r := Build(config.Config{EnableFakeProviders: true})
	if _, err := r.Text("fake"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Voice("fake"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Video("veo"); !errors.Is(err, kernel.ErrCapability) {
		t.Fatalf("unknown provider: %v", err)
	}
	if _, err := Build(config.Config{}).Text("fake"); err == nil {
		t.Fatal("fake must be disabled when not enabled")
	}
}
