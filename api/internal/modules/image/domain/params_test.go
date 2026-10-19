package domain

import (
	"errors"
	"strings"
	"testing"

	"github.com/be/acs/api/internal/shared/kernel"
)

func TestOptionsValidate(t *testing.T) {
	ok := Options{Prompt: "Poster khóa học", Preset: PresetPoster}.Normalize()
	if err := ok.Validate(true); err != nil {
		t.Fatal(err)
	}
	if ok.AspectRatio != "1:1" || ok.Quality != "standard" || ok.Count != 1 {
		t.Fatalf("defaults: %+v", ok)
	}
	cases := map[string]Options{
		"prompt":       {Preset: PresetPoster},
		"preset":       {Prompt: "x", Preset: "banner"},
		"aspect_ratio": {Prompt: "x", Preset: PresetPoster, AspectRatio: "3:2"},
		"quality":      {Prompt: "x", Preset: PresetPoster, Quality: "ultra"},
		"count":        {Prompt: "x", Preset: PresetPoster, Count: 5},
		"too_long":     {Prompt: strings.Repeat("a", MaxPromptRunes+1), Preset: PresetPoster},
	}
	for name, o := range cases {
		err := o.Normalize().Validate(true)
		de, isDE := kernel.AsDomainError(err)
		if !isDE || de.Code != kernel.CodeValidation {
			t.Errorf("%s: want VALIDATION_FAILED, got %v", name, err)
		}
	}
	if err := (Options{Preset: PresetPoster}).Normalize().Validate(false); err != nil {
		t.Fatalf("prompt optional for variations: %v", err)
	}
}

func TestCheckCapabilities(t *testing.T) {
	caps := Caps{AspectRatios: []string{"1:1", "16:9", "9:16"}, Qualities: []string{"standard"}, MaxVariants: 2}
	o := Options{Prompt: "x", Preset: PresetPoster, AspectRatio: "4:5", Quality: "high", Count: 4}
	err := CheckCapabilities(caps, o, OpGenerate, 1)
	de, ok := kernel.AsDomainError(err)
	if !ok || de.Code != kernel.CodeCapabilityNotSupported {
		t.Fatalf("want 422, got %v", err)
	}
	fields := map[string]any{}
	for _, d := range de.Details {
		fields[d["field"].(string)] = d["supported"]
	}
	for _, f := range []string{"aspect_ratio", "quality", "count", "reference_file_ids"} {
		if _, ok := fields[f]; !ok {
			t.Errorf("missing detail for %s: %+v", f, de.Details)
		}
	}
	if s, _ := fields["aspect_ratio"].([]string); len(s) != 3 {
		t.Errorf("supported aspect ratios: %v", fields["aspect_ratio"])
	}
	full := Caps{AspectRatios: AspectRatios, Qualities: Qualities, MaxVariants: 4, SupportsRefImage: true, SupportsEdit: true}
	if err := CheckCapabilities(full, Options{AspectRatio: "4:5", Quality: "high", Count: 4}, OpEdit, 2); err != nil {
		t.Fatal(err)
	}
	if err := CheckCapabilities(Caps{SupportsRefImage: true}, Options{Count: 1}, OpEdit, 0); !errors.Is(err, kernel.ErrCapability) {
		t.Fatalf("edit unsupported: %v", err)
	}
	if err := CheckCapabilities(Caps{}, Options{Count: 1}, OpVariation, 0); !errors.Is(err, kernel.ErrCapability) {
		t.Fatalf("variation needs ref image support: %v", err)
	}
}

func TestReferenceMIME(t *testing.T) {
	for _, m := range []string{"image/png", "image/jpeg", "image/webp"} {
		if err := CheckReferenceMIME("reference_file_ids", m); err != nil {
			t.Fatal(m, err)
		}
	}
	for _, m := range []string{"image/svg+xml", "application/pdf", "video/mp4"} {
		if err := CheckReferenceMIME("reference_file_ids", m); !errors.Is(err, kernel.ErrUnsupportedMedia) {
			t.Fatal(m, err)
		}
	}
}

func TestPricingUnit(t *testing.T) {
	cases := map[[2]string]string{
		{"1:1", "standard"}:  "image_1024_standard",
		{"", "high"}:         "image_1024_high",
		{"4:5", "high"}:      "image_1536_high",
		{"16:9", "standard"}: "image_1536_standard",
	}
	for in, want := range cases {
		if got := PricingUnit(in[0], in[1]); got != want {
			t.Errorf("%v: %s want %s", in, got, want)
		}
	}
	if FallbackPricingUnit("") != "image_1024_standard" {
		t.Fatal("fallback")
	}
}

func TestTitleAndTypography(t *testing.T) {
	if got := Title("  Poster mùa hè\nchi tiết", PresetPoster); got != "Poster mùa hè" {
		t.Fatal(got)
	}
	if got := Title(strings.Repeat("ả", 100), PresetPoster); len([]rune(got)) != MaxTitleRunes {
		t.Fatal(len([]rune(got)))
	}
	if Title("", PresetThumbnail) == "" {
		t.Fatal("empty title")
	}
	if !RequiresTypographyCheck(PresetPoster) || RequiresTypographyCheck(PresetSocialPost) {
		t.Fatal("typography rule")
	}
}

func TestThumbSize(t *testing.T) {
	cases := []struct{ w, h, ew, eh int }{
		{1024, 1024, 384, 384}, {1024, 576, 384, 216}, {576, 1024, 216, 384}, {200, 100, 200, 100}, {0, 5, 0, 0},
	}
	for _, c := range cases {
		if w, h := ThumbSize(c.w, c.h, ThumbnailMaxDim); w != c.ew || h != c.eh {
			t.Errorf("%dx%d → %dx%d", c.w, c.h, w, h)
		}
	}
}
