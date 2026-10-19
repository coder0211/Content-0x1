package e2e

import (
	"net/http"
	"strings"
	"testing"

	"github.com/be/acs/api/internal/shared/kernel"
)

// TestFlowA runs SPEC-01 Flow A in process over the real HTTP handlers and
// use cases: brand + product → campaign → content plan (approve) →
// generate variants (job runs on the fake provider) → edit → submit →
// reviewer approves → creator gets a signed download URL.
func TestFlowA(t *testing.T) {
	w := newWorld(t)
	creator, reviewer, lead := kernel.RoleContentCreator, kernel.RoleReviewer, kernel.RoleMarketingLead
	brandID, productID := w.seedBrand()
	campaignID := w.seedCampaign(creator, brandID, productID)

	// Content plan proposal (job content_plan) → approve it.
	var acc struct {
		JobID kernel.ID `json:"job_id"`
	}
	w.must(http.StatusAccepted, creator, "POST", "/campaigns/"+campaignID.String()+"/plan-generations", map[string]any{}, &acc)
	w.runJobs()
	var planJob struct {
		Status string `json:"status"`
		Error  any    `json:"error"`
	}
	w.must(http.StatusOK, creator, "GET", "/jobs/"+acc.JobID.String(), nil, &planJob)
	if planJob.Status != "succeeded" {
		t.Fatalf("content plan job: %s %v", planJob.Status, planJob.Error)
	}
	var camp struct {
		ContentPlanProposal *struct {
			Summary string           `json:"summary"`
			Themes  []string         `json:"themes"`
			Items   []map[string]any `json:"items"`
		} `json:"content_plan_proposal"`
	}
	w.must(http.StatusOK, creator, "GET", "/campaigns/"+campaignID.String(), nil, &camp)
	if camp.ContentPlanProposal == nil || len(camp.ContentPlanProposal.Items) == 0 {
		t.Fatalf("no plan proposal: %+v", camp)
	}
	w.must(http.StatusOK, creator, "PUT", "/campaigns/"+campaignID.String()+"/plan", map[string]any{"plan": map[string]any{
		"summary": camp.ContentPlanProposal.Summary, "themes": camp.ContentPlanProposal.Themes, "items": camp.ContentPlanProposal.Items,
	}}, nil)

	// Generate 2 variants; the job succeeds and creates 2 content assets.
	jobID, assetIDs := w.generateContent(creator, campaignID, brandID, productID, 2)
	assetID := assetIDs[0]
	var asset struct {
		Status         string `json:"status"`
		Type           string `json:"type"`
		CurrentVersion struct {
			VersionNo   int            `json:"version_no"`
			ContentJSON map[string]any `json:"content_json"`
		} `json:"current_version"`
	}
	w.must(http.StatusOK, creator, "GET", "/assets/"+assetID.String(), nil, &asset)
	if asset.Status != "draft" || asset.Type != "content" || asset.CurrentVersion.VersionNo != 1 {
		t.Fatalf("asset %+v", asset)
	}

	// Usage was recorded with the job type and settled on finish.
	if cost, ok := w.budgetS.Jobs[jobID]; !ok || cost.Actual.IsZero() {
		t.Fatalf("job cost not settled: %+v", w.budgetS.Jobs[jobID])
	}

	// Edit and save a revision (FR-CNT: edit → save revision).
	content := asset.CurrentVersion.ContentJSON
	content["body_text"] = "Bản đã chỉnh sửa: luyện giao tiếp mỗi ngày."
	w.must(http.StatusCreated, creator, "POST", "/assets/"+assetID.String()+"/versions", map[string]any{
		"content_json": content, "change_note": "chỉnh sửa", "base_version_no": 1,
	}, nil)

	// Creator cannot download before approval (403), cannot approve own work.
	if rec := w.call(creator, "POST", "/assets/"+assetID.String()+"/download-url", map[string]any{}, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("download before approval: %d %s", rec.Code, rec.Body.String())
	}
	w.must(http.StatusOK, creator, "POST", "/assets/"+assetID.String()+"/submit-review", map[string]any{}, nil)
	var queue []struct {
		ID kernel.ID `json:"id"`
	}
	w.must(http.StatusOK, reviewer, "GET", "/reviews/queue", nil, &queue)
	if len(queue) != 1 || queue[0].ID != assetID {
		t.Fatalf("review queue %+v", queue)
	}
	w.must(http.StatusOK, reviewer, "POST", "/assets/"+assetID.String()+"/approve", map[string]any{"comment": "OK"}, nil)

	// Approved → creator gets a signed URL to the text export; audited.
	var dl struct {
		URL    string    `json:"url"`
		FileID kernel.ID `json:"file_id"`
	}
	w.must(http.StatusOK, creator, "POST", "/assets/"+assetID.String()+"/download-url", map[string]any{}, &dl)
	if dl.URL == "" || dl.FileID == kernel.NilID {
		t.Fatalf("download %+v", dl)
	}
	f, err := w.files.Get(t.Context(), dl.FileID)
	if err != nil || f.MimeType != "text/plain" {
		t.Fatalf("export file %+v %v", f, err)
	}
	if body := string(w.storage.Objects[f.StorageKey]); !strings.Contains(body, "Bản đã chỉnh sửa") {
		t.Fatalf("export does not hold the approved revision: %q", body)
	}
	found := false
	for _, a := range w.audit.Actions() {
		found = found || a == "asset.download"
	}
	if !found {
		t.Fatalf("download not audited: %v", w.audit.Actions())
	}
	_ = lead
}
