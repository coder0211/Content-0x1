package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/be/acs/api/internal/modules/asset/assettest"
	"github.com/be/acs/api/internal/modules/asset/domain"
	"github.com/be/acs/api/internal/modules/asset/usecase"
	identityuc "github.com/be/acs/api/internal/modules/identity/usecase"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

type env struct {
	svc      *usecase.Service
	repo     *assettest.Repo
	files    *assettest.Files
	audit    *testutil.Audit
	notifier *testutil.Notifier
	clock    *testutil.Clock
	creator  kernel.Actor
	other    kernel.Actor // another creator
	reviewer kernel.Actor
	lead     kernel.Actor
	admin    kernel.Actor
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{
		repo: assettest.NewRepo(), files: assettest.NewFiles(), audit: &testutil.Audit{}, notifier: &testutil.Notifier{},
		clock:   testutil.NewClock(),
		creator: testutil.Actor(kernel.RoleContentCreator), other: testutil.Actor(kernel.RoleContentCreator),
		reviewer: testutil.Actor(kernel.RoleReviewer), lead: testutil.Actor(kernel.RoleMarketingLead),
		admin: testutil.Actor(kernel.RoleAdmin),
	}
	idn := &assettest.Identity{}
	for _, a := range []kernel.Actor{e.creator, e.other, e.reviewer, e.lead, e.admin} {
		idn.Users = append(idn.Users, identityuc.UserSummary{ID: a.UserID, Name: "U-" + a.Roles[0], Status: "active", Roles: a.Roles})
	}
	e.svc = &usecase.Service{Tx: &testutil.Tx{}, Repo: e.repo, Clock: e.clock, Audit: e.audit, Notifier: e.notifier,
		Files: e.files, Identity: idn}
	return e
}

func content(body string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"title": "Bài đăng", "body_text": body, "hashtags": []string{"#tienganh"}})
	return b
}

func (e *env) create(t *testing.T, owner kernel.Actor, typ usecase.AssetType, subtype, title string, files ...usecase.VersionFile) kernel.ID {
	t.Helper()
	e.clock.Advance(time.Second)
	ref, err := e.svc.CreateAsset(context.Background(), usecase.CreateAssetInput{
		BrandID: kernel.NewID(), Type: typ, Subtype: subtype, Title: title, CreatedBy: owner.UserID,
		Version: usecase.NewVersion{ContentJSON: content("Nội dung " + title), Files: files},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ref.AssetID
}

func ctxFor(a kernel.Actor) context.Context { return kernel.WithActor(context.Background(), a) }

func mustStatus(t *testing.T, e *env, id kernel.ID, want domain.Status) {
	t.Helper()
	a, err := e.repo.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != want {
		t.Fatalf("status = %s, want %s", a.Status, want)
	}
}

// ACS-034 AC: restoring v1 while on v3 creates v4 with v1's content.
func TestRestoreCreatesNewVersion(t *testing.T) {
	e := newEnv(t)
	ctx := ctxFor(e.creator)
	id := e.create(t, e.creator, usecase.TypeContent, "facebook_post", "Ưu đãi hè")
	for i := 2; i <= 3; i++ {
		if _, err := e.svc.CreateVersion(ctx, e.creator, id, usecase.ManualVersionInput{
			ContentJSON: content(fmt.Sprintf("bản %d", i)), BaseVersionNo: i - 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	v, err := e.svc.Restore(ctx, e.creator, id, 1)
	if err != nil {
		t.Fatal(err)
	}
	v1, _ := e.svc.GetVersion(ctx, id, 1)
	if v.VersionNo != 4 || string(v.ContentJSON) != string(v1.ContentJSON) {
		t.Fatalf("restore: got v%d %s", v.VersionNo, v.ContentJSON)
	}
	if v.ChangeNote == nil || *v.ChangeNote != "Khôi phục từ phiên bản 1" {
		t.Fatalf("change note %v", v.ChangeNote)
	}
	got, _ := e.svc.Get(ctx, id)
	if got.CurrentVersion.VersionNo != 4 {
		t.Fatalf("current = %d", got.CurrentVersion.VersionNo)
	}
	vs, err := e.svc.ListVersions(ctx, e.reviewer, id)
	if err != nil || len(vs) != 4 || vs[0].VersionNo != 4 {
		t.Fatalf("list versions: %v %d", err, len(vs))
	}
	if !slices.Contains(e.audit.Actions(), "asset.version_restore") {
		t.Fatalf("audit: %v", e.audit.Actions())
	}
	if _, err := e.svc.Restore(ctx, e.creator, id, 9); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatalf("restore missing: %v", err)
	}
}

func TestCreateVersionConflictAndSearchBody(t *testing.T) {
	e := newEnv(t)
	ctx := ctxFor(e.creator)
	id := e.create(t, e.creator, usecase.TypeContent, "", "Khóa học")
	_, err := e.svc.CreateVersion(ctx, e.creator, id, usecase.ManualVersionInput{ContentJSON: content("x"), BaseVersionNo: 2})
	if !errors.Is(err, kernel.ErrVersionConflict) {
		t.Fatalf("want VERSION_CONFLICT, got %v", err)
	}
	if _, err := e.svc.CreateVersion(ctx, e.creator, id, usecase.ManualVersionInput{ContentJSON: content("giảm học phí"), BaseVersionNo: 1}); err != nil {
		t.Fatal(err)
	}
	if b := e.repo.SearchBody(id); !containsAll(b, "giảm học phí", "tienganh") {
		t.Fatalf("search body not updated: %q", b)
	}
	_, err = e.svc.CreateVersion(ctx, e.creator, id, usecase.ManualVersionInput{ContentJSON: json.RawMessage(`[1]`), BaseVersionNo: 2})
	if !errors.Is(err, kernel.ErrValidation) {
		t.Fatalf("want validation, got %v", err)
	}
	// IDOR: another creator may see but not edit.
	_, err = e.svc.CreateVersion(ctxFor(e.other), e.other, id, usecase.ManualVersionInput{ContentJSON: content("y"), BaseVersionNo: 2})
	if !errors.Is(err, kernel.ErrForbidden) {
		t.Fatalf("other creator: %v", err)
	}
	// Lead may edit any asset.
	if _, err := e.svc.CreateVersion(ctxFor(e.lead), e.lead, id, usecase.ManualVersionInput{ContentJSON: content("z"), BaseVersionNo: 2}); err != nil {
		t.Fatal(err)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, x := range subs {
		if !strings.Contains(s, x) {
			return false
		}
	}
	return true
}

// ACS-034: an approved asset that receives a new version (API or manual)
// goes back to draft.
func TestApprovedNewVersionBackToDraft(t *testing.T) {
	e := newEnv(t)
	id := e.create(t, e.creator, usecase.TypeContent, "", "A")
	e.approve(t, id)
	mustStatus(t, e, id, domain.StatusApproved)
	if _, err := e.svc.AddVersion(context.Background(), id, usecase.NewVersion{ContentJSON: content("mới"), CreatedBy: e.creator.UserID}); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, e, id, domain.StatusDraft)
}

func (e *env) approve(t *testing.T, id kernel.ID) {
	t.Helper()
	if _, err := e.svc.SubmitReview(ctxFor(e.creator), e.creator, id, usecase.SubmitInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Approve(ctxFor(e.reviewer), e.reviewer, id, "ok"); err != nil {
		t.Fatal(err)
	}
}

// ACS-080 AC: full status transition table through the use cases.
func TestReviewTransitionsThroughUseCases(t *testing.T) {
	type op func(e *env, id kernel.ID) error
	submit := func(e *env, id kernel.ID) error {
		_, err := e.svc.SubmitReview(ctxFor(e.creator), e.creator, id, usecase.SubmitInput{})
		return err
	}
	approve := func(e *env, id kernel.ID) error {
		_, err := e.svc.Approve(ctxFor(e.reviewer), e.reviewer, id, "")
		return err
	}
	reqChanges := func(e *env, id kernel.ID) error {
		_, err := e.svc.RequestChanges(ctxFor(e.reviewer), e.reviewer, id, "Sửa CTA")
		return err
	}
	newVersion := func(e *env, id kernel.ID) error {
		_, err := e.svc.AddVersion(context.Background(), id, usecase.NewVersion{ContentJSON: content("v"), CreatedBy: e.creator.UserID})
		return err
	}
	ops := map[string]op{"submit": submit, "approve": approve, "request_changes": reqChanges, "new_version": newVersion}
	// setup brings a fresh asset into each status.
	setup := map[domain.Status][]op{
		domain.StatusDraft:            nil,
		domain.StatusPendingReview:    {submit},
		domain.StatusChangesRequested: {submit, reqChanges},
		domain.StatusApproved:         {submit, approve},
	}
	want := map[domain.Status]map[string]domain.Status{
		domain.StatusDraft:            {"submit": domain.StatusPendingReview, "new_version": domain.StatusDraft},
		domain.StatusPendingReview:    {"approve": domain.StatusApproved, "request_changes": domain.StatusChangesRequested, "new_version": domain.StatusDraft},
		domain.StatusChangesRequested: {"new_version": domain.StatusDraft},
		domain.StatusApproved:         {"new_version": domain.StatusDraft},
	}
	for from, steps := range setup {
		for name, o := range ops {
			t.Run(string(from)+"/"+name, func(t *testing.T) {
				e := newEnv(t)
				id := e.create(t, e.creator, usecase.TypeContent, "", "T")
				for _, s := range steps {
					if err := s(e, id); err != nil {
						t.Fatal(err)
					}
				}
				err := o(e, id)
				if to, ok := want[from][name]; ok {
					if err != nil {
						t.Fatalf("unexpected %v", err)
					}
					mustStatus(t, e, id, to)
					return
				}
				if !errors.Is(err, kernel.ErrInvalidTransition) {
					t.Fatalf("want INVALID_STATE_TRANSITION, got %v", err)
				}
				mustStatus(t, e, id, from)
			})
		}
	}
}

func TestSelfApprovalAndAdmin(t *testing.T) {
	e := newEnv(t)
	// Lead creates and submits its own asset: lead cannot approve it.
	id := e.create(t, e.lead, usecase.TypeContent, "", "L")
	if _, err := e.svc.SubmitReview(ctxFor(e.lead), e.lead, id, usecase.SubmitInput{}); err != nil {
		t.Fatal(err)
	}
	_, err := e.svc.Approve(ctxFor(e.lead), e.lead, id, "")
	if kernel.ErrorCode(err) != kernel.CodeSelfApproval {
		t.Fatalf("want SELF_APPROVAL_NOT_ALLOWED, got %v", err)
	}
	// Admin may approve their own asset.
	id2 := e.create(t, e.admin, usecase.TypeContent, "", "A")
	if _, err := e.svc.SubmitReview(ctxFor(e.admin), e.admin, id2, usecase.SubmitInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Approve(ctxFor(e.admin), e.admin, id2, ""); err != nil {
		t.Fatalf("admin self-approve: %v", err)
	}
	// Creator lacks asset.review.
	if _, err := e.svc.Approve(ctxFor(e.creator), e.creator, id, ""); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatalf("creator approve: %v", err)
	}
}

func TestPosterTypographyChecklist(t *testing.T) {
	e := newEnv(t)
	id := e.create(t, e.creator, usecase.TypeImage, "poster", "Poster")
	_, err := e.svc.SubmitReview(ctxFor(e.creator), e.creator, id, usecase.SubmitInput{})
	if !errors.Is(err, kernel.ErrValidation) {
		t.Fatalf("want validation, got %v", err)
	}
	mustStatus(t, e, id, domain.StatusDraft)
	d, err := e.svc.SubmitReview(ctxFor(e.creator), e.creator, id, usecase.SubmitInput{Checklist: domain.Checklist{TypographyChecked: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Approvals) != 1 || string(d.Approvals[0].Checklist) != `{"typography_checked":true}` {
		t.Fatalf("approvals: %+v", d.Approvals)
	}
}

func TestReviewNotificationsAndAudit(t *testing.T) {
	e := newEnv(t)
	id := e.create(t, e.creator, usecase.TypeContent, "", "N")
	if _, err := e.svc.SubmitReview(ctxFor(e.creator), e.creator, id, usecase.SubmitInput{Comment: "Nhờ duyệt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.RequestChanges(ctxFor(e.reviewer), e.reviewer, id, "  "); !errors.Is(err, kernel.ErrValidation) {
		t.Fatalf("empty comment: %v", err)
	}
	if _, err := e.svc.RequestChanges(ctxFor(e.reviewer), e.reviewer, id, "Đổi hook"); err != nil {
		t.Fatal(err)
	}
	if len(e.notifier.Sent) != 2 {
		t.Fatalf("notifications: %+v", e.notifier.Sent)
	}
	rr := e.notifier.Sent[0]
	if rr.Type != ports.NotifyReviewRequested || slices.Contains(rr.UserIDs, e.creator.UserID) ||
		!slices.Contains(rr.UserIDs, e.reviewer.UserID) || !slices.Contains(rr.UserIDs, e.lead.UserID) {
		t.Fatalf("review_requested to %v", rr.UserIDs)
	}
	cr := e.notifier.Sent[1]
	if cr.Type != ports.NotifyChangesRequested || !slices.Equal(cr.UserIDs, []kernel.ID{e.creator.UserID}) {
		t.Fatalf("changes_requested: %+v", cr)
	}
	if got := e.audit.Actions(); !slices.Contains(got, "asset.submit_review") || !slices.Contains(got, "asset.request_changes") {
		t.Fatalf("audit %v", got)
	}
	d, _ := e.svc.Detail(ctxFor(e.reviewer), e.reviewer, id)
	if len(d.Approvals) != 2 || d.Approvals[0].Decision != usecase.DecisionChangesRequested || d.Approvals[0].ReviewerName == "" {
		t.Fatalf("history: %+v", d.Approvals)
	}
}

// ACS-084 AC: a Creator downloading a draft asset gets 403; every download
// is audited; Lead/Admin may download any status.
func TestDownloadRules(t *testing.T) {
	e := newEnv(t)
	img := e.files.Put("image/png", "image")
	id := e.create(t, e.creator, usecase.TypeImage, "poster", "Ưu đãi Hè 2026", usecase.VersionFile{FileID: img, Role: usecase.RolePrimary})
	if _, err := e.svc.Download(ctxFor(e.creator), e.creator, id, nil); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatalf("creator draft: %v", err)
	}
	if _, err := e.svc.Download(ctxFor(e.reviewer), e.reviewer, id, nil); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatalf("reviewer draft: %v", err)
	}
	u, err := e.svc.Download(ctxFor(e.lead), e.lead, id, nil)
	if err != nil || u.FileID != img || !strings.Contains(u.URL, "name=Uu-dai-He-2026.png") {
		t.Fatalf("lead draft: %+v %v", u, err)
	}
	if _, err := e.svc.SubmitReview(ctxFor(e.creator), e.creator, id, usecase.SubmitInput{Checklist: domain.Checklist{TypographyChecked: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Approve(ctxFor(e.reviewer), e.reviewer, id, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Download(ctxFor(e.creator), e.creator, id, nil); err != nil {
		t.Fatalf("creator approved: %v", err)
	}
	other := kernel.NewID()
	if _, err := e.svc.Download(ctxFor(e.creator), e.creator, id, &other); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatalf("unknown file: %v", err)
	}
	n := 0
	for _, a := range e.audit.Actions() {
		if a == "asset.download" {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("download audits = %d", n)
	}
}

func TestSearchFiltersAndCursor(t *testing.T) {
	e := newEnv(t)
	for i := range 5 {
		e.create(t, e.creator, usecase.TypeContent, "", fmt.Sprintf("Ưu đãi %d", i))
	}
	img := e.create(t, e.other, usecase.TypeImage, "", "Poster hè")
	ctx := ctxFor(e.reviewer)
	page, err := e.svc.Search(ctx, e.reviewer, usecase.SearchInput{PageCursor: kernel.PageRequest{Limit: 2}})
	if err != nil || len(page.Items) != 2 || page.NextCursor == nil || page.Items[0].Asset.ID != img {
		t.Fatalf("page1 %v %+v", err, page)
	}
	seen := 2
	for c := page.NextCursor; c != nil; {
		p, err := e.svc.Search(ctx, e.reviewer, usecase.SearchInput{PageCursor: kernel.PageRequest{Limit: 2, Cursor: *c}})
		if err != nil {
			t.Fatal(err)
		}
		seen += len(p.Items)
		c = p.NextCursor
	}
	if seen != 6 {
		t.Fatalf("paged %d", seen)
	}
	typ := domain.TypeImage
	p, _ := e.svc.Search(ctx, e.reviewer, usecase.SearchInput{Filter: usecase.SearchFilter{Type: &typ}})
	if len(p.Items) != 1 {
		t.Fatalf("type filter %d", len(p.Items))
	}
	p, _ = e.svc.Search(ctx, e.reviewer, usecase.SearchInput{Q: "đãi 3"})
	if len(p.Items) != 1 {
		t.Fatalf("q filter %d", len(p.Items))
	}
	p, _ = e.svc.Search(ctx, e.reviewer, usecase.SearchInput{Filter: usecase.SearchFilter{CreatedBy: &e.other.UserID, Statuses: []domain.Status{domain.StatusDraft}}})
	if len(p.Items) != 1 || p.Items[0].CreatedByName == "" {
		t.Fatalf("created_by filter %+v", p.Items)
	}
	if _, err := e.svc.Queue(ctx, e.creator, usecase.SearchFilter{}, kernel.PageRequest{}); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatalf("creator queue: %v", err)
	}
}

func TestUpdateTagsUsageRightsAndDelete(t *testing.T) {
	e := newEnv(t)
	id := e.create(t, e.creator, usecase.TypeVideo, "", "Video")
	tags := []string{" Hè ", "hè", "Sale"}
	rights := "Nhạc: thư viện nội bộ #12"
	d, err := e.svc.Update(ctxFor(e.creator), e.creator, id, usecase.UpdateInput{Tags: &tags, UsageRights: &rights, UsageRightsSet: true})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(d.Asset.Tags, []string{"hè", "sale"}) || d.Asset.UsageRights == nil || *d.Asset.UsageRights != rights {
		t.Fatalf("update: %+v", d.Asset)
	}
	if _, err := e.svc.Update(ctxFor(e.other), e.other, id, usecase.UpdateInput{Tags: &tags}); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatalf("other update: %v", err)
	}
	if err := e.svc.Delete(ctxFor(e.creator), e.creator, id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Detail(ctxFor(e.creator), e.creator, id); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatalf("deleted detail: %v", err)
	}
	if got := e.audit.Actions(); !slices.Contains(got, "asset.update") || !slices.Contains(got, "asset.delete") {
		t.Fatalf("audit %v", got)
	}
}

func TestCountByStatus(t *testing.T) {
	e := newEnv(t)
	a := e.create(t, e.creator, usecase.TypeContent, "", "1")
	e.create(t, e.creator, usecase.TypeContent, "", "2")
	e.create(t, e.other, usecase.TypeContent, "", "3")
	if _, err := e.svc.SubmitReview(ctxFor(e.creator), e.creator, a, usecase.SubmitInput{}); err != nil {
		t.Fatal(err)
	}
	all, _ := e.svc.CountByStatus(context.Background(), nil)
	mine, _ := e.svc.CountByStatus(context.Background(), &e.creator.UserID)
	if all[usecase.StatusDraft] != 2 || all[usecase.StatusPendingReview] != 1 || mine[usecase.StatusDraft] != 1 {
		t.Fatalf("counts all=%v mine=%v", all, mine)
	}
}
