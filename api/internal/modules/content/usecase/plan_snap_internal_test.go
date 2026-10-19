package usecase

import "testing"

func TestToCampaignPlanSnapsChannelsAndFormats(t *testing.T) {
	var ai aiPlan
	ai.Schedule = append(ai.Schedule,
		struct {
			Date    string `json:"date"`
			Channel string `json:"channel"`
			Format  string `json:"format"`
			Theme   string `json:"theme"`
			Idea    string `json:"idea"`
		}{Channel: "Facebook Page", Format: "facebook_post", Idea: "A"},
		struct {
			Date    string `json:"date"`
			Channel string `json:"channel"`
			Format  string `json:"format"`
			Theme   string `json:"theme"`
			Idea    string `json:"idea"`
		}{Channel: "Nội dung mẫu", Format: "Nội dung mẫu", Idea: "B"},
	)
	got := toCampaignPlan(ai, []string{"facebook", "tiktok"}, []string{"facebook_post", "short_video"})
	if len(got.Items) != 2 {
		t.Fatalf("items = %d", len(got.Items))
	}
	if it := got.Items[0]; it.Channel != "facebook" || it.Format != "facebook_post" {
		t.Errorf("item 0 = %+v", it)
	}
	// Unknown values: channel dropped, format falls back to the brief's format at that position.
	if it := got.Items[1]; it.Channel != "" || it.Format != "short_video" {
		t.Errorf("item 1 = %+v", it)
	}
	// Without a brief list the plain slug is kept (back-compat).
	if got := snapSlug("Facebook Post", nil); got != "facebook_post" {
		t.Errorf("snapSlug = %q", got)
	}
}
