package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/be/acs/api/internal/shared/kernel"
)

// access is the coarse requirement of a route (SPEC-01 §2, SPEC-04 §2).
type access struct {
	public bool   // no session needed
	perm   string // "" = any authenticated user (ownership checked in the use case)
	note   string // spec reference / ownership rule, for the report
}

var (
	public   = access{public: true}
	authOnly = func(note string) access { return access{note: note} }
	perm     = func(p string) access { return access{perm: p} }
)

// expectedAccess is the RBAC contract, written from SPEC-04 §2 and the
// recorded decisions (docs/DECISIONS.md), NOT copied from the handlers.
// Every route on the router must be listed: a new route without an entry
// fails TestRBACMatrixCoversEveryRoute.
var expectedAccess = map[string]access{
	// 2.1 Auth & users
	"POST /auth/login":  public,
	"POST /auth/logout": authOnly("own session"),
	"GET /me":           authOnly("own profile"),
	"GET /users":        perm(kernel.PermUserManage),
	"POST /users":       perm(kernel.PermUserManage),
	"GET /users/{id}":   perm(kernel.PermUserManage),
	"PATCH /users/{id}": perm(kernel.PermUserManage),
	// 2.2 Brand & product
	"GET /brands":                               perm(kernel.PermBrandView),
	"POST /brands":                              perm(kernel.PermBrandManage),
	"GET /brands/{id}":                          perm(kernel.PermBrandView),
	"PATCH /brands/{id}":                        perm(kernel.PermBrandManage),
	"DELETE /brands/{id}":                       perm(kernel.PermBrandManage),
	"GET /brands/{id}/versions":                 perm(kernel.PermBrandView),
	"GET /brands/{id}/assets":                   perm(kernel.PermBrandView),
	"POST /brands/{id}/assets":                  perm(kernel.PermBrandManage),
	"DELETE /brands/{id}/assets/{brandAssetId}": perm(kernel.PermBrandManage),
	"GET /brands/{id}/products":                 perm(kernel.PermBrandView),
	"POST /brands/{id}/products":                perm(kernel.PermBrandManage),
	"GET /products/{id}":                        perm(kernel.PermBrandView),
	"PATCH /products/{id}":                      perm(kernel.PermBrandManage),
	// 2.3 Campaign
	"GET /campaigns":                        perm(kernel.PermBrandView),
	"POST /campaigns":                       perm(kernel.PermCampaignManage),
	"GET /campaigns/member-candidates":      perm(kernel.PermCampaignManage),
	"GET /campaigns/{id}":                   perm(kernel.PermBrandView),
	"PATCH /campaigns/{id}":                 perm(kernel.PermCampaignManage),
	"POST /campaigns/{id}/plan-generations": perm(kernel.PermGenerateRun),
	"PUT /campaigns/{id}/plan":              perm(kernel.PermCampaignManage),
	"DELETE /campaigns/{id}/plan-proposal":  perm(kernel.PermCampaignManage),
	"GET /campaigns/{id}/budget":            perm(kernel.PermUsageView),
	"PUT /campaigns/{id}/budget":            perm(kernel.PermBudgetManage),
	// 2.4 Content
	"POST /content/generations":          perm(kernel.PermGenerateRun),
	"POST /content/{assetId}/rewrites":   perm(kernel.PermGenerateRun),
	"POST /content/{assetId}/transforms": perm(kernel.PermGenerateRun),
	// 2.5 Image
	"POST /images/prompt-suggestions":   perm(kernel.PermGenerateRun),
	"GET /images/generations":           perm(kernel.PermGenerateRun),
	"POST /images/generations":          perm(kernel.PermGenerateRun),
	"GET /images/references":            perm(kernel.PermGenerateRun),
	"POST /images/estimate":             perm(kernel.PermGenerateRun),
	"POST /images/{assetId}/edits":      perm(kernel.PermGenerateRun),
	"POST /images/{assetId}/variations": perm(kernel.PermGenerateRun),
	// 2.6 Video
	"GET /videos":                                      perm(kernel.PermBrandView),
	"POST /videos":                                     perm(kernel.PermGenerateRun),
	"GET /videos/{id}":                                 perm(kernel.PermBrandView),
	"PUT /videos/{id}/scenes":                          perm(kernel.PermGenerateRun),
	"POST /videos/{id}/estimate":                       perm(kernel.PermGenerateRun),
	"POST /videos/{id}/productions":                    perm(kernel.PermGenerateRun),
	"POST /videos/{id}/scenes/{sceneId}/regenerations": perm(kernel.PermGenerateRun),
	"POST /videos/{id}/renders":                        perm(kernel.PermGenerateRun),
	// 2.7 Jobs: owner or provider.view (reads) / owner or admin (writes), in the use case
	"GET /jobs":              authOnly("own jobs unless provider.view"),
	"GET /jobs/{id}":         authOnly("owner or provider.view, else 404"),
	"GET /jobs/{id}/events":  authOnly("owner or provider.view, else 404"),
	"GET /jobs/{id}/history": authOnly("owner or provider.view, else 404"),
	"POST /jobs/{id}/cancel": authOnly("owner or admin"),
	"POST /jobs/{id}/retry":  authOnly("owner or admin"),
	// 2.8 Assets, review, files
	"GET /assets":                                    perm(kernel.PermBrandView),
	"GET /assets/{id}":                               perm(kernel.PermBrandView),
	"PATCH /assets/{id}":                             perm(kernel.PermBrandView), // + owner/lead in use case
	"DELETE /assets/{id}":                            perm(kernel.PermBrandView), // + owner/lead in use case
	"GET /assets/{id}/versions":                      perm(kernel.PermBrandView),
	"POST /assets/{id}/versions":                     perm(kernel.PermBrandView), // + owner/lead
	"POST /assets/{id}/versions/{versionNo}/restore": perm(kernel.PermBrandView), // + owner/lead
	"POST /assets/{id}/submit-review":                perm(kernel.PermBrandView), // + owner/lead
	"POST /assets/{id}/approve":                      perm(kernel.PermAssetReview),
	"POST /assets/{id}/request-changes":              perm(kernel.PermAssetReview),
	"GET /reviews/queue":                             perm(kernel.PermAssetReview),
	"POST /assets/{id}/download-url":                 perm(kernel.PermAssetDownload),
	"POST /files/upload-url":                         authOnly("uploader becomes owner"),
	"POST /files/{id}/complete":                      authOnly("owner only, else 404"),
	"GET /files/{id}":                                authOnly("owner only, else 404"),
	// 2.9 Ops
	"GET /settings/providers":                       perm(kernel.PermProviderView),
	"PATCH /settings/providers/{id}":                perm(kernel.PermProviderManage),
	"GET /settings/models":                          perm(kernel.PermProviderView),
	"POST /settings/models":                         perm(kernel.PermProviderManage),
	"GET /settings/models/{id}":                     perm(kernel.PermProviderView),
	"PATCH /settings/models/{id}":                   perm(kernel.PermProviderManage),
	"GET /settings/models/{id}/pricing":             perm(kernel.PermProviderView),
	"POST /settings/models/{id}/pricing":            perm(kernel.PermProviderManage),
	"GET /settings/prompt-templates":                perm(kernel.PermProviderView),
	"GET /settings/prompt-templates/{id}":           perm(kernel.PermProviderView),
	"POST /settings/prompt-templates":               perm(kernel.PermProviderView), // DECISIONS Agent 4: Lead/Admin (= provider.view holders)
	"POST /settings/prompt-templates/{id}/activate": perm(kernel.PermProviderView),
	"POST /settings/prompt-templates/{id}/retire":   perm(kernel.PermProviderView),
	"POST /cost-estimates":                          perm(kernel.PermGenerateRun),
	"GET /usage/summary":                            perm(kernel.PermUsageView),
	"GET /settings/budgets":                         perm(kernel.PermUsageView),
	"PATCH /settings/budgets":                       perm(kernel.PermBudgetManage),
	"GET /audit-logs":                               perm(kernel.PermAuditView),
	"GET /notifications":                            authOnly("own notifications"),
	"POST /notifications/{id}/read":                 authOnly("own notification"),
	"GET /dashboard/summary":                        authOnly("widgets filtered by permission"),
}

func roleActors() map[string]kernel.Actor {
	out := map[string]kernel.Actor{}
	for _, r := range kernel.AllRoles {
		out[r] = kernel.Actor{UserID: kernel.NewID(), Roles: []string{r}, Permissions: kernel.RolePermissions[r], Email: r + "@acs.local", Name: r}
	}
	return out
}

func TestRBACMatrixCoversEveryRoute(t *testing.T) {
	root, _ := realRouter(t, tokenAuth(roleActors()))
	routes := apiRoutes(t, root)
	if len(routes) < 80 {
		t.Fatalf("only %d routes registered; composition is missing modules", len(routes))
	}
	seen := map[string]bool{}
	for _, r := range routes {
		seen[r.String()] = true
		if _, ok := expectedAccess[r.String()]; !ok {
			t.Errorf("route %s has no RBAC expectation: add it to expectedAccess (SPEC-04)", r)
		}
	}
	for k := range expectedAccess {
		if !seen[k] {
			t.Errorf("expected route %s is not registered", k)
		}
	}
}

type rbacResult struct {
	Status int
	Code   string
}

func doRequest(t *testing.T, h http.Handler, method, path, token string, body any) (*httptest.ResponseRecorder, rbacResult) {
	t.Helper()
	var rd *strings.Reader
	if body == nil {
		rd = strings.NewReader("")
	} else {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req := httptest.NewRequestWithContext(ctx, method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.Header.Set("Idempotency-Key", kernel.NewID().String())
	if token != "" {
		req.AddCookie(&http.Cookie{Name: testCookies.SessionName, Value: token})
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var env struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	res := rbacResult{Status: w.Code}
	if env.Error != nil {
		res.Code = env.Error.Code
	}
	return w, res
}

// TestRBACMatrix sends every route × {anonymous, admin, lead, creator,
// reviewer} through the real router and checks the coarse gate:
//   - anonymous → 401 UNAUTHENTICATED on every non-public route;
//   - a role without the required permission → 403 FORBIDDEN;
//   - a role with it → anything but 401/403 (the request reaches the use
//     case; with no database it then fails with 5xx or a validation error).
func TestRBACMatrix(t *testing.T) {
	actors := roleActors()
	root, _ := realRouter(t, tokenAuth(actors))
	id := kernel.NewID()
	for _, r := range apiRoutes(t, root) {
		exp, ok := expectedAccess[r.String()]
		if !ok {
			continue // reported by TestRBACMatrixCoversEveryRoute
		}
		path := concretePath(r.Pattern, id)
		t.Run(r.String(), func(t *testing.T) {
			if _, res := doRequest(t, root, r.Method, path, "", map[string]any{}); !exp.public && (res.Status != http.StatusUnauthorized || res.Code != kernel.CodeUnauthenticated) {
				t.Errorf("anonymous: got %d %s, want 401 UNAUTHENTICATED", res.Status, res.Code)
			}
			for _, role := range kernel.AllRoles {
				if exp.public {
					continue
				}
				allowed := exp.perm == "" || slices.Contains(kernel.RolePermissions[role], exp.perm)
				_, res := doRequest(t, root, r.Method, path, role, map[string]any{})
				switch {
				case !allowed && (res.Status != http.StatusForbidden || res.Code != kernel.CodeForbidden):
					t.Errorf("%s lacks %s: got %d %s, want 403 FORBIDDEN", role, exp.perm, res.Status, res.Code)
				case allowed && (res.Status == http.StatusUnauthorized || res.Status == http.StatusForbidden):
					t.Errorf("%s holds %q: got %d %s, want the request to reach the use case", role, exp.perm, res.Status, res.Code)
				}
			}
		})
	}
}

// TestCSRFRequiredOnEveryUnsafeRoute: an authenticated unsafe request
// without X-CSRF-Token is rejected on every route except login.
func TestCSRFRequiredOnEveryUnsafeRoute(t *testing.T) {
	actors := roleActors()
	root, _ := realRouter(t, tokenAuth(actors))
	id := kernel.NewID()
	for _, r := range apiRoutes(t, root) {
		if r.Method == http.MethodGet || r.String() == "POST /auth/login" {
			continue
		}
		req := httptest.NewRequest(r.Method, concretePath(r.Pattern, id), strings.NewReader("{}"))
		req.AddCookie(&http.Cookie{Name: testCookies.SessionName, Value: kernel.RoleAdmin})
		w := httptest.NewRecorder()
		root.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), kernel.CodeCSRFInvalid) {
			t.Errorf("%s without CSRF token: got %d %s", r, w.Code, w.Body.String())
		}
	}
}
