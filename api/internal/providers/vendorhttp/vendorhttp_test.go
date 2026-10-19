package vendorhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

func TestByStatusMapping(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "7")
	cases := []struct {
		status int
		kind   ports.ErrKind
	}{
		{429, ports.ErrKindRateLimited}, {401, ports.ErrKindAuth}, {403, ports.ErrKindAuth},
		{500, ports.ErrKindTransient}, {503, ports.ErrKindTransient}, {529, ports.ErrKindTransient},
		{408, ports.ErrKindTransient}, {400, ports.ErrKindInvalidRequest}, {404, ports.ErrKindInvalidRequest},
	}
	for _, c := range cases {
		pe := ByStatus("x", c.status, h, "c", "m")
		if pe.Kind != c.kind {
			t.Errorf("%d: %s", c.status, pe.Kind)
		}
	}
	if pe := ByStatus("x", 429, h, "c", "m"); pe.RetryAfter != 7*time.Second || !pe.Retryable() {
		t.Fatalf("%+v", pe)
	}
	if d := RetryAfter(http.Header{}, 3*time.Second); d != 3*time.Second {
		t.Fatal(d)
	}
	h2 := http.Header{}
	h2.Set("Retry-After-Ms", "1500")
	if d := RetryAfter(h2, 0); d != 1500*time.Millisecond {
		t.Fatal(d)
	}
}

func TestEndpointConfigureAndKey(t *testing.T) {
	e := NewEndpoint("p", "REF_A", "https://a.example", time.Second).WithKeyFunc(func(r string) string {
		if r == "REF_B" {
			return "k-b"
		}
		return ""
	})
	if _, err := e.Key(); err == nil {
		t.Fatal("missing key must fail")
	} else if pe, _ := ports.AsProviderError(err); pe.Kind != ports.ErrKindAuth || strings.Contains(pe.Message, "k-b") {
		t.Fatal(pe)
	}
	e.Configure("REF_B", json.RawMessage(`{"base_url":"http://evil.example","timeout_sec":5,"effort":"low"}`))
	if k, err := e.Key(); err != nil || k != "k-b" {
		t.Fatal(k, err)
	}
	if e.BaseURL() != "https://a.example" {
		t.Fatal("non-https base_url must be ignored")
	}
	if e.Timeout() != 5*time.Second {
		t.Fatal(e.Timeout())
	}
	var eff string
	if !e.Extra("effort", &eff) || eff != "low" {
		t.Fatal(eff)
	}
	e.Configure("", json.RawMessage(`{"base_url":"https://b.example/"}`))
	if e.BaseURL() != "https://b.example" || e.SecretRef() != "REF_B" {
		t.Fatal(e.BaseURL(), e.SecretRef())
	}
}

func TestDoClassifiesAndRedacts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":"slow down"}`))
	}))
	defer srv.Close()
	e := NewEndpoint("p", "", srv.URL, time.Second)
	_, err := e.DoJSON(context.Background(), "POST", srv.URL+"/x?key=SECRET", nil, map[string]string{}, nil, nil)
	pe, ok := ports.AsProviderError(err)
	if !ok || pe.Kind != ports.ErrKindRateLimited || pe.RetryAfter != 2*time.Second {
		t.Fatal(err)
	}
	if got := redact(`Get "https://h/x?key=SECRET&a=1": timeout`); strings.Contains(got, "SECRET") {
		t.Fatal(got)
	}
}

func TestUnsupported(t *testing.T) {
	err := CheckOneOf("aspect_ratio", "5:4", []string{"1:1"})
	if !errors.Is(err, kernel.ErrCapability) {
		t.Fatal(err)
	}
	de, _ := kernel.AsDomainError(err)
	if de.Details[0]["field"] != "aspect_ratio" || de.Details[0]["supported"] == nil {
		t.Fatal(de.Details)
	}
	if CheckOneOf("q", "", []string{"a"}) != nil || CheckOneOf("q", "z", nil) != nil {
		t.Fatal("empty is allowed")
	}
}
