package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	c, err := load(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if c.Session.IdleTimeout != 12*time.Hour || c.Session.Absolute != 7*24*time.Hour {
		t.Fatalf("unexpected session timeouts: %+v", c.Session)
	}
	if c.SignedURLTTL != 15*time.Minute {
		t.Fatalf("SignedURLTTL = %v", c.SignedURLTTL)
	}
	if !c.EnableFakeProviders {
		t.Fatal("fake providers should be on locally")
	}
	if len(c.Worker.Queues) != 5 {
		t.Fatalf("queues = %v", c.Worker.Queues)
	}
}

func TestLoadRejectsInsecureProduction(t *testing.T) {
	vals := map[string]string{"APP_ENV": "production"}
	_, err := load(func(k string) string { return vals[k] })
	if err == nil {
		t.Fatal("expected production validation error")
	}
}

func TestLoadParseErrors(t *testing.T) {
	vals := map[string]string{"SESSION_IDLE_TIMEOUT": "soon", "SIGNED_URL_TTL": "1h"}
	if _, err := load(func(k string) string { return vals[k] }); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadVideoFontsDir(t *testing.T) {
	c, err := load(func(string) string { return "" })
	if err != nil || c.FFmpeg.FontsDir != "" {
		t.Fatalf("default FontsDir = %q, err %v", c.FFmpeg.FontsDir, err)
	}
	vals := map[string]string{"VIDEO_FONTS_DIR": "/usr/share/fonts/noto"}
	c, err = load(func(k string) string { return vals[k] })
	if err != nil || c.FFmpeg.FontsDir != "/usr/share/fonts/noto" {
		t.Fatalf("FontsDir = %q, err %v", c.FFmpeg.FontsDir, err)
	}
}

func TestSecretOnlyResolvesProviderCredentials(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("DATABASE_URL", "postgres://secret")
	t.Setenv("S3_SECRET_KEY", "minio-secret")
	if Secret("OPENAI_API_KEY") != "sk-test" {
		t.Fatal("provider key must resolve")
	}
	for _, ref := range []string{"DATABASE_URL", "S3_SECRET_KEY", "REDIS_URL", "HOME", "none", "", "openai_api_key"} {
		if Secret(ref) != "" || IsProviderSecretRef(ref) {
			t.Errorf("%q must not resolve", ref)
		}
	}
	for _, ref := range []string{"ANTHROPIC_API_KEY", "GEMINI_API_KEY", "GOOGLE_TTS_CREDENTIALS", "VENDOR_TOKEN"} {
		if !IsProviderSecretRef(ref) {
			t.Errorf("%q must be accepted", ref)
		}
	}
}
