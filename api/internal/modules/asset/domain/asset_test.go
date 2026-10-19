package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/be/acs/api/internal/shared/kernel"
)

// TestTransitionTable covers every (status, event) pair (ACS-080 AC).
func TestTransitionTable(t *testing.T) {
	want := map[Status]map[Event]Status{
		StatusDraft:            {EventSubmit: StatusPendingReview, EventNewVersion: StatusDraft, EventArchive: StatusArchived},
		StatusPendingReview:    {EventApprove: StatusApproved, EventRequestChanges: StatusChangesRequested, EventNewVersion: StatusDraft, EventArchive: StatusArchived},
		StatusChangesRequested: {EventNewVersion: StatusDraft, EventArchive: StatusArchived},
		StatusApproved:         {EventNewVersion: StatusDraft, EventArchive: StatusArchived},
		StatusArchived:         {},
	}
	events := []Event{EventSubmit, EventApprove, EventRequestChanges, EventNewVersion, EventArchive}
	for _, from := range AllStatuses {
		for _, e := range events {
			got, err := Next(from, e)
			exp, ok := want[from][e]
			if ok {
				if err != nil || got != exp {
					t.Errorf("%s --%s--> got %q,%v want %q", from, e, got, err, exp)
				}
				continue
			}
			if !errors.Is(err, kernel.ErrInvalidTransition) {
				t.Errorf("%s --%s--> want INVALID_STATE_TRANSITION, got %q,%v", from, e, got, err)
			}
		}
	}
}

func TestApproveRules(t *testing.T) {
	creator, author, reviewer := kernel.NewID(), kernel.NewID(), kernel.NewID()
	mk := func() *Asset {
		return &Asset{Status: StatusPendingReview, CreatedBy: creator, CurrentVersionBy: author}
	}
	if err := mk().Approve(creator, false); !errors.Is(err, ErrSelfApproval) {
		t.Fatalf("creator self-approval: %v", err)
	}
	if err := mk().Approve(author, false); !errors.Is(err, ErrSelfApproval) {
		t.Fatalf("version author self-approval: %v", err)
	}
	if err := mk().Approve(creator, true); err != nil {
		t.Fatalf("admin may self-approve: %v", err)
	}
	a := mk()
	if err := a.Approve(reviewer, false); err != nil || a.Status != StatusApproved {
		t.Fatalf("approve: %v %s", err, a.Status)
	}
	if err := (&Asset{Status: StatusDraft}).Approve(reviewer, false); !errors.Is(err, kernel.ErrInvalidTransition) {
		t.Fatalf("approve draft: %v", err)
	}
}

func TestSubmitPosterNeedsTypography(t *testing.T) {
	now := time.Now()
	a := &Asset{Status: StatusDraft, Subtype: SubtypePoster}
	if err := a.Submit(Checklist{}, now); !errors.Is(err, kernel.ErrValidation) {
		t.Fatalf("want validation, got %v", err)
	}
	if err := a.Submit(Checklist{TypographyChecked: true}, now); err != nil || a.Status != StatusPendingReview || a.SubmittedAt == nil {
		t.Fatalf("submit: %v %+v", err, a)
	}
	b := &Asset{Status: StatusChangesRequested}
	if err := b.Submit(Checklist{}, now); !errors.Is(err, kernel.ErrInvalidTransition) {
		t.Fatalf("changes_requested must be revised first: %v", err)
	}
}

func TestRequestChangesNeedsComment(t *testing.T) {
	a := &Asset{Status: StatusPendingReview}
	if err := a.RequestChanges("  "); !errors.Is(err, kernel.ErrValidation) {
		t.Fatal(err)
	}
	if err := a.RequestChanges("Sửa CTA"); err != nil || a.Status != StatusChangesRequested {
		t.Fatal(err)
	}
}

func TestNormalizeTags(t *testing.T) {
	got, err := NormalizeTags([]string{" Tết ", "tết", "", "Khuyến  mãi"})
	if err != nil || len(got) != 2 || got[0] != "tết" || got[1] != "khuyến mãi" {
		t.Fatalf("%v %v", got, err)
	}
	many := make([]string, 21)
	for i := range many {
		many[i] = string(rune('a' + i))
	}
	if _, err := NormalizeTags(many); err == nil {
		t.Fatal("want too_many")
	}
}

func TestSearchTextAndPrefixQuery(t *testing.T) {
	s := SearchText([]byte(`{"title":"Ưu đãi hè","body_text":"Giảm 20%","hashtags":["#sale"],"prompt":""}`))
	if s != "Ưu đãi hè\nGiảm 20%\nsale" {
		t.Fatalf("%q", s)
	}
	if SearchText([]byte(`{"body_html":"<p>Xin <b>chào</b></p>"}`)) == "" {
		t.Fatal("html fallback")
	}
	if q := PrefixQuery("Ưu đãi, hè! ưu ' & |"); q != "ưu:* & đãi:* & hè:*" {
		t.Fatalf("%q", q)
	}
	if PrefixQuery("  &|!  ") != "" {
		t.Fatal("empty")
	}
}
