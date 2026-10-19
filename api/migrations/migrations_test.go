package migrations

import (
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/be/acs/api/internal/shared/kernel"
)

// The seed's RBAC tuples must equal kernel.RolePermissions (SPEC-01 §2).
func TestSeedRBACMatchesKernel(t *testing.T) {
	b, err := fs.ReadFile(FS, "0011_seed.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.Split(string(b), "-- +goose Down")[0]
	re := regexp.MustCompile(`\('(admin|marketing_lead|content_creator|reviewer)','([a-z]+\.[a-z]+)'\)`)
	got := map[string][]string{}
	for _, m := range re.FindAllStringSubmatch(up, -1) {
		got[m[1]] = append(got[m[1]], m[2])
	}
	for role, want := range kernel.RolePermissions {
		g, w := slices.Clone(got[role]), slices.Clone(want)
		slices.Sort(g)
		slices.Sort(w)
		if !slices.Equal(g, w) {
			t.Errorf("%s: seed %v, kernel %v", role, g, w)
		}
	}
}

// Every migration has Up and Down sections and a unique 4-digit prefix.
func TestMigrationFiles(t *testing.T) {
	entries, err := fs.Glob(FS, "*.sql")
	if err != nil || len(entries) < 11 {
		t.Fatalf("%v %v", err, entries)
	}
	seen := map[string]bool{}
	for _, name := range entries {
		prefix := name[:4]
		if seen[prefix] {
			t.Errorf("duplicate migration number %s", prefix)
		}
		seen[prefix] = true
		b, _ := fs.ReadFile(FS, name)
		if !strings.Contains(string(b), "-- +goose Up") || !strings.Contains(string(b), "-- +goose Down") {
			t.Errorf("%s: missing goose Up/Down", name)
		}
	}
}
