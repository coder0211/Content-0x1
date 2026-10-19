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
