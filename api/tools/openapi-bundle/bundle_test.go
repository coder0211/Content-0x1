package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The committed api/openapi.yaml must equal the bundle of the fragments.
func TestBundleIsFresh(t *testing.T) {
	b, err := Bundle(filepath.Join("..", "..", "openapi"))
	if err != nil {
		t.Fatal(err)
	}
	cur, err := os.ReadFile(filepath.Join("..", "..", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, cur) {
		t.Fatal("api/openapi.yaml is stale: run `make openapi`")
	}
}

func TestDuplicatePathRejected(t *testing.T) {
	dir := t.TempDir()
	write := func(name, s string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("_base.yaml", "openapi: 3.1.0\npaths: {}\ncomponents: {schemas: {}}\n")
	write("a.yaml", "paths:\n  /x: {get: {responses: {}}}\n")
	write("b.yaml", "paths:\n  /x: {get: {responses: {}}}\n")
	if _, err := Bundle(dir); err == nil {
		t.Fatal("duplicate path accepted")
	}
	write("b.yaml", "paths:\n  /y: {get: {responses: {'200': {$ref: '#/components/responses/Nope'}}}}\n")
	if _, err := Bundle(dir); err == nil {
		t.Fatal("dangling $ref accepted")
	}
}

func TestUndeclaredTagRejected(t *testing.T) {
	dir := t.TempDir()
	write := func(name, s string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("_base.yaml", "openapi: 3.1.0\ntags: [{name: a}]\npaths: {}\ncomponents: {schemas: {}}\n")
	write("a.yaml", "paths:\n  /x: {get: {tags: [a], responses: {}}}\n")
	if _, err := Bundle(dir); err != nil {
		t.Fatalf("declared tag rejected: %v", err)
	}
	write("b.yaml", "paths:\n  /y: {get: {tags: [b], responses: {}}}\n")
	if _, err := Bundle(dir); err == nil {
		t.Fatal("undeclared tag accepted")
	}
}
