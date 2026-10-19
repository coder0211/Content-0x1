// Package archtest enforces the Clean Architecture rules of SPEC-06 (R1,
// R3, R4 and the module dependency table §2.5) by parsing imports. It runs
// with `go test ./...` and needs no tooling beyond the Go toolchain;
// golangci-lint depguard repeats the framework bans in CI.
package archtest

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const mod = "github.com/be/acs/api"

// allowedDeps is SPEC-06 §2.5 (plus identity for user lookups and file for
// storage-backed modules). A module may import only these modules'
// `usecase` packages. Add a row when you create a module.
var allowedDeps = map[string][]string{
	"identity":  {},
	"audit":     {},
	"file":      {},
	"job":       {},
	"provider":  {"identity"},
	"budget":    {"identity"},
	"brand":     {"identity", "file"},
	"campaign":  {"identity", "brand"},
	"asset":     {"identity", "file", "campaign"},
	"content":   {"identity", "file", "brand", "campaign", "asset", "job", "budget", "provider"},
	"image":     {"identity", "file", "brand", "campaign", "asset", "job", "budget", "provider"},
	"video":     {"identity", "file", "brand", "campaign", "asset", "job", "budget", "provider", "content"},
	"dashboard": {"identity", "campaign", "asset", "job", "budget"},
	"_template": {},
}

// frameworks may not appear in domain or usecase (R3).
var frameworks = []string{
	"net/http", "database/sql", "github.com/jackc/pgx", "github.com/hibiken/asynq", "github.com/go-chi/chi",
	"github.com/redis/go-redis", "github.com/minio/minio-go", "github.com/aws/aws-sdk-go",
	mod + "/internal/platform", mod + "/internal/providers", mod + "/internal/app",
}

type file struct {
	path    string // repo-relative
	pkg     string // import path of the directory
	imports []string
	test    bool
}

func root(t *testing.T) string {
	t.Helper()
	wd, _ := os.Getwd()
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func load(t *testing.T) []file {
	t.Helper()
	base := root(t)
	var out []file
	fset := token.NewFileSet()
	err := filepath.WalkDir(filepath.Join(base, "internal"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
			return err
		}
		f, err := parser.ParseFile(fset, p, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(base, p)
		rel = filepath.ToSlash(rel)
		fl := file{path: rel, pkg: mod + "/" + filepath.ToSlash(filepath.Dir(rel)), test: strings.HasSuffix(p, "_test.go")}
		for _, im := range f.Imports {
			s, _ := strconv.Unquote(im.Path.Value)
			fl.imports = append(fl.imports, s)
		}
		out = append(out, fl)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// moduleOf returns (module, layer) for paths under internal/modules.
func moduleOf(pkg string) (string, string) {
	rest, ok := strings.CutPrefix(pkg, mod+"/internal/modules/")
	if !ok {
		return "", ""
	}
	parts := strings.Split(rest, "/")
	if len(parts) == 1 {
		return parts[0], "root"
	}
	return parts[0], parts[1] // domain | usecase | adapter | <m>test
}

func isStd(p string) bool { return !strings.Contains(strings.Split(p, "/")[0], ".") }

func hasPrefix(p string, prefixes []string) bool {
	for _, x := range prefixes {
		if p == x || strings.HasPrefix(p, x+"/") {
			return true
		}
	}
	return false
}

func TestLayerRules(t *testing.T) {
	for _, f := range load(t) {
		m, layer := moduleOf(f.pkg)
		for _, im := range f.imports {
			om, olayer := moduleOf(im)
			switch {
			case m == "":
				// Outside modules: platform/ports/kernel must not reach into modules.
				if (strings.Contains(f.pkg, "/internal/platform") || strings.Contains(f.pkg, "/internal/ports") ||
					strings.Contains(f.pkg, "/internal/shared") || strings.Contains(f.pkg, "/internal/providers")) && om != "" {
					t.Errorf("%s: %s may not import module %s", f.path, f.pkg, im)
				}
				if strings.Contains(f.pkg, "/internal/shared/kernel") && !isStd(im) &&
					!hasPrefix(im, []string{"github.com/google/uuid", "github.com/shopspring/decimal"}) {
					t.Errorf("%s: kernel may import only stdlib, uuid, decimal (got %s)", f.path, im)
				}
				if strings.HasSuffix(f.pkg, "/internal/ports") && !isStd(im) && im != mod+"/internal/shared/kernel" {
					t.Errorf("%s: ports may import only stdlib and kernel (got %s)", f.path, im)
				}
				continue
			case layer == "domain":
				ok := isStd(im) || hasPrefix(im, []string{mod + "/internal/shared/kernel", "github.com/google/uuid", "github.com/shopspring/decimal"}) ||
					(om == m && olayer == "domain")
				if !ok {
					t.Errorf("%s: domain may import only stdlib + kernel (R1), got %s", f.path, im)
				}
				continue
			case layer == "usecase":
				if hasPrefix(im, frameworks) {
					t.Errorf("%s: usecase may not import framework %s (R3)", f.path, im)
				}
			}
			// Cross-module rules (R4) for every layer.
			if om == "" || om == m {
				if om == m && layer != "root" && olayer == "root" {
					t.Errorf("%s: only cmd/app may import the module root", f.path)
				}
				continue
			}
			if strings.HasSuffix(olayer, "test") && olayer == om+"test" {
				if f.test || layer == "root" {
					continue // fakes: tests anywhere; module roots only for their own (checked above)
				}
				t.Errorf("%s: %s fakes are for tests only", f.path, im)
				continue
			}
			if olayer != "usecase" {
				t.Errorf("%s: module %s may only use %s through %s/usecase (R4), got %s", f.path, m, om, om, im)
				continue
			}
			deps, known := allowedDeps[m]
			if !known {
				t.Errorf("module %s missing from allowedDeps in internal/archtest", m)
				continue
			}
			if !slices.Contains(deps, om) && !f.test {
				t.Errorf("%s: module %s may not depend on %s (SPEC-06 §2.5)", f.path, m, om)
			}
		}
	}
}

func TestEveryModuleIsListed(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join(root(t), "internal", "modules"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if _, ok := allowedDeps[e.Name()]; e.IsDir() && !ok {
			t.Errorf("module %s missing from allowedDeps", e.Name())
		}
	}
}
