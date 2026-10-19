package domain_test

import (
	"errors"
	"testing"

	"github.com/be/acs/api/internal/modules/campaign/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

func TestTransitions(t *testing.T) {
	for _, tc := range []struct {
		from, to domain.Status
		ok       bool
	}{
		{domain.StatusDraft, domain.StatusInProgress, true},
		{domain.StatusDraft, domain.StatusCompleted, false}, // ACS-025 AC
		{domain.StatusDraft, domain.StatusReview, false},
		{domain.StatusInProgress, domain.StatusReview, true},
		{domain.StatusReview, domain.StatusInProgress, true},
		{domain.StatusReview, domain.StatusCompleted, true},
		{domain.StatusInProgress, domain.StatusCompleted, false},
		{domain.StatusCompleted, domain.StatusArchived, true},
		{domain.StatusCompleted, domain.StatusInProgress, false},
		{domain.StatusDraft, domain.StatusArchived, true},
		{domain.StatusArchived, domain.StatusDraft, false},
	} {
		c := &domain.Campaign{Status: tc.from}
		err := c.TransitionTo(tc.to)
		if tc.ok != (err == nil) {
			t.Errorf("%s → %s: %v", tc.from, tc.to, err)
		}
		if !tc.ok && !errors.Is(err, kernel.ErrInvalidTransition) {
			t.Errorf("%s → %s: want INVALID_STATE_TRANSITION, got %v", tc.from, tc.to, err)
		}
	}
	c := &domain.Campaign{Status: domain.StatusDraft}
	if err := c.TransitionTo("bogus"); !errors.Is(err, kernel.ErrValidation) {
		t.Fatal(err)
	}
}

func fields(err error) map[string]string {
	out := map[string]string{}
	de, _ := kernel.AsDomainError(err)
	if de == nil {
		return out
	}
	for _, d := range de.Details {
		out[d["field"].(string)] = d["issue"].(string)
	}
	return out
}

func TestBriefValidation(t *testing.T) {
	var v kernel.ValidationErrors
	domain.Brief{}.Normalize().Validate(&v, "brief")
	got := fields(v.Err())
	for _, f := range []string{"brief.target_audience", "brief.key_message", "brief.channels", "brief.formats", "brief.deadline"} {
		if got[f] != "required" {
			t.Errorf("%s: %q", f, got[f])
		}
	}
	v = kernel.ValidationErrors{}
	b := domain.Brief{TargetAudience: " SV ", KeyMessage: "x", Channels: []string{"Facebook", "facebook", "tik tok"},
		Formats: []domain.Format{{Type: "poster", Count: 0}, {Type: "poster", Count: 2}}, Deadline: "2026-13-01"}.Normalize()
	if len(b.Channels) != 2 || b.TargetAudience != "SV" {
		t.Fatalf("%+v", b)
	}
	b.Validate(&v, "brief")
	got = fields(v.Err())
	want := map[string]string{"brief.channels[1]": "invalid", "brief.formats[0].count": "out_of_range", "brief.formats[1].type": "duplicate", "brief.deadline": "invalid_date"}
	for f, issue := range want {
		if got[f] != issue {
			t.Errorf("%s: %q want %q", f, got[f], issue)
		}
	}
}

func TestHeaderAndPlan(t *testing.T) {
	var v kernel.ValidationErrors
	domain.ValidateHeader(&v, "", "sales", "2026-12-01", "2026-11-01")
	got := fields(v.Err())
	if got["name"] != "required" || got["goal"] != "invalid" || got["end_date"] != "before_start_date" {
		t.Fatal(got)
	}
	p, err := domain.ParsePlan([]byte(`{"items":[{"topic":" A ","format":"facebook_post"},{"topic":"","format":"x y"}]}`))
	if err != nil || p.Items[0].ID != "item-1" || p.Items[0].Topic != "A" {
		t.Fatal(err, p)
	}
	v = kernel.ValidationErrors{}
	p.Validate(&v, "plan")
	got = fields(v.Err())
	if got["plan.items[1].topic"] != "required" || got["plan.items[1].format"] != "invalid" {
		t.Fatal(got)
	}
	if p, _ := domain.ParsePlan(nil); p != nil {
		t.Fatal("nil plan")
	}
}

func TestCanEdit(t *testing.T) {
	creator := kernel.Actor{UserID: kernel.NewID(), Roles: []string{kernel.RoleContentCreator}, Permissions: kernel.RolePermissions[kernel.RoleContentCreator]}
	other := kernel.Actor{UserID: kernel.NewID(), Roles: []string{kernel.RoleContentCreator}, Permissions: kernel.RolePermissions[kernel.RoleContentCreator]}
	lead := kernel.Actor{UserID: kernel.NewID(), Roles: []string{kernel.RoleMarketingLead}, Permissions: kernel.RolePermissions[kernel.RoleMarketingLead]}
	c := &domain.Campaign{CreatedBy: creator.UserID}
	if !c.CanEdit(creator) || c.CanEdit(other) || !c.CanEdit(lead) {
		t.Fatal("ownership")
	}
	c.MemberIDs = []kernel.ID{other.UserID}
	if !c.CanEdit(other) {
		t.Fatal("member")
	}
}
