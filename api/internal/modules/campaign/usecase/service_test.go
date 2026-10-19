package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/be/acs/api/internal/modules/brand/brandtest"
	branduc "github.com/be/acs/api/internal/modules/brand/usecase"
	"github.com/be/acs/api/internal/modules/campaign/campaigntest"
	"github.com/be/acs/api/internal/modules/campaign/domain"
	"github.com/be/acs/api/internal/modules/campaign/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

var ctx = context.Background()

type env struct {
	svc     *usecase.Service
	audit   *testutil.Audit
	users   *campaigntest.Users
	brandID kernel.ID
	product kernel.ID
	lead    kernel.Actor
	creator kernel.Actor
}

func actorFor(users *campaigntest.Users, name, role string) kernel.Actor {
	a := testutil.Actor(role)
	a.UserID = users.Add(name, role)
	return a
}

func newEnv() *env {
	brands := brandtest.NewAPI()
	bc := brands.AddSample()
	pid := kernel.NewID()
	bc.Products = []branduc.ProductFacts{{ProductID: pid, Name: "Giao tiếp 12 tuần"}}
	brands.Brands[bc.BrandID] = bc
	users := campaigntest.NewUsers()
	e := &env{audit: &testutil.Audit{}, users: users, brandID: bc.BrandID, product: pid}
	e.lead = actorFor(users, "Lan Lead", kernel.RoleMarketingLead)
	e.creator = actorFor(users, "Cuong Creator", kernel.RoleContentCreator)
	e.svc = &usecase.Service{Tx: &testutil.Tx{}, Repo: campaigntest.NewRepo(), Clock: testutil.NewClock(), Audit: e.audit, Brands: brands, Users: users}
	return e
}

func (e *env) input() usecase.CreateInput {
	return usecase.CreateInput{BrandID: e.brandID, Name: "Khóa giao tiếp T11", Goal: "lead",
		Brief: domain.Brief{ProductIDs: []kernel.ID{e.product}, TargetAudience: "Sinh viên", KeyMessage: "Tự tin giao tiếp",
			Channels: []string{"facebook"}, Formats: []domain.Format{{Type: "facebook_post", Count: 5}}, Deadline: "2026-11-15"},
		StartDate: "2026-11-01", EndDate: "2026-12-31"}
}

func (e *env) create(t *testing.T, a kernel.Actor) *usecase.View {
	t.Helper()
	v, err := e.svc.Create(ctx, a, e.input())
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func ptr[T any](v T) *T { return &v }

func TestCreateRBACAndValidation(t *testing.T) {
	e := newEnv()
	if _, err := e.svc.Create(ctx, testutil.Actor(kernel.RoleReviewer), e.input()); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatal(err)
	}
	in := e.input()
	in.Brief.TargetAudience = ""
	if _, err := e.svc.Create(ctx, e.creator, in); !errors.Is(err, kernel.ErrValidation) {
		t.Fatal(err)
	}
	in = e.input()
	in.BrandID = kernel.NewID()
	if _, err := e.svc.Create(ctx, e.creator, in); !errors.Is(err, kernel.ErrValidation) {
		t.Fatalf("unknown brand: %v", err)
	}
	in = e.input()
	in.Brief.ProductIDs = []kernel.ID{kernel.NewID()}
	if _, err := e.svc.Create(ctx, e.creator, in); !errors.Is(err, kernel.ErrValidation) {
		t.Fatalf("foreign product: %v", err)
	}
	in = e.input()
	in.MemberIDs = []kernel.ID{kernel.NewID()}
	if _, err := e.svc.Create(ctx, e.creator, in); !errors.Is(err, kernel.ErrValidation) {
		t.Fatalf("unknown member: %v", err)
	}
	v := e.create(t, e.creator)
	if v.Status != domain.StatusDraft || v.CreatedBy != e.creator.UserID {
		t.Fatalf("%+v", v)
	}
	if e.audit.Actions()[0] != "campaign.created" {
		t.Fatal(e.audit.Actions())
	}
}

func TestStateMachineAndOwnership(t *testing.T) {
	e := newEnv()
	c := e.create(t, e.creator)
	// ACS-025 AC: draft → completed is 409.
	if _, err := e.svc.Update(ctx, e.creator, c.ID, usecase.UpdateInput{Status: ptr("completed")}); !errors.Is(err, kernel.ErrInvalidTransition) {
		t.Fatal(err)
	}
	other := actorFor(e.users, "Other Creator", kernel.RoleContentCreator)
	if _, err := e.svc.Update(ctx, other, c.ID, usecase.UpdateInput{Name: ptr("x")}); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatalf("non member: %v", err)
	}
	if _, err := e.svc.Update(ctx, e.creator, kernel.NewID(), usecase.UpdateInput{Name: ptr("x")}); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatal(err)
	}
	v, err := e.svc.Update(ctx, e.creator, c.ID, usecase.UpdateInput{MemberIDs: &[]kernel.ID{other.UserID}})
	if err != nil || len(v.Members) != 1 || v.Members[0].Name != "Other Creator" {
		t.Fatal(err, v)
	}
	// a member may edit but not change members
	if _, err := e.svc.Update(ctx, other, c.ID, usecase.UpdateInput{Status: ptr("in_progress")}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Update(ctx, other, c.ID, usecase.UpdateInput{MemberIDs: &[]kernel.ID{}}); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatal(err)
	}
	for _, s := range []string{"review", "completed", "archived"} {
		if _, err := e.svc.Update(ctx, e.lead, c.ID, usecase.UpdateInput{Status: ptr(s)}); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	if _, err := e.svc.Update(ctx, e.lead, c.ID, usecase.UpdateInput{Name: ptr("y")}); !errors.Is(err, kernel.ErrInvalidTransition) {
		t.Fatalf("archived edit: %v", err)
	}
	if _, err := e.svc.GetCampaignForGeneration(ctx, e.creator, c.ID); !errors.Is(err, kernel.ErrInvalidTransition) {
		t.Fatalf("archived generation: %v", err)
	}
}

func TestContentPlanFlow(t *testing.T) {
	e := newEnv()
	c := e.create(t, e.creator)
	if err := e.svc.SaveContentPlanProposal(ctx, c.ID, json.RawMessage(`{"themes":["Tự tin"],"items":[{"topic":"Mở đầu","format":"facebook_post","publish_date":"2026-11-02"}]}`)); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SaveContentPlanProposal(ctx, c.ID, json.RawMessage(`"oops"`)); !errors.Is(err, kernel.ErrValidation) {
		t.Fatal(err)
	}
	v, _ := e.svc.Get(ctx, e.creator, c.ID)
	if v.PlanProposal == nil || v.ContentPlan != nil {
		t.Fatalf("%+v", v)
	}
	if _, err := e.svc.SavePlan(ctx, e.creator, c.ID, domain.Plan{}); !errors.Is(err, kernel.ErrValidation) {
		t.Fatal(err)
	}
	if _, err := e.svc.SavePlan(ctx, testutil.Actor(kernel.RoleReviewer), c.ID, *v.PlanProposal); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatal(err)
	}
	// ACS-037 AC: the approved plan is stored in campaigns.content_plan.
	v, err := e.svc.SavePlan(ctx, e.creator, c.ID, *v.PlanProposal)
	if err != nil || v.ContentPlan == nil || v.PlanProposal != nil || *v.ContentPlanApprovedBy != e.creator.UserID {
		t.Fatal(err, v)
	}
	// a later proposal does not overwrite the approved plan
	_ = e.svc.SaveContentPlanProposal(ctx, c.ID, json.RawMessage(`{"items":[{"topic":"Khác","format":"poster"}]}`))
	v, _ = e.svc.Get(ctx, e.creator, c.ID)
	if v.ContentPlan.Items[0].Topic != "Mở đầu" || v.PlanProposal.Items[0].Topic != "Khác" {
		t.Fatalf("%+v", v)
	}
	if err := e.svc.DiscardProposal(ctx, e.creator, c.ID); err != nil {
		t.Fatal(err)
	}
	g, err := e.svc.GetCampaignForGeneration(ctx, e.creator, c.ID)
	if err != nil || len(g.ContentPlan) == 0 || g.Brief.Formats[0].Count != 5 {
		t.Fatal(err, g)
	}
	if _, err := e.svc.GetCampaignForGeneration(ctx, testutil.Actor(kernel.RoleReviewer), c.ID); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatal(err)
	}
	ok, _ := e.svc.IsMember(ctx, c.ID, e.creator.UserID)
	if !ok {
		t.Fatal("creator is member")
	}
}

func TestListAndCandidates(t *testing.T) {
	e := newEnv()
	e.create(t, e.creator)
	e.create(t, e.lead)
	page, err := e.svc.List(ctx, testutil.Actor(kernel.RoleReviewer), usecase.Filter{}, false, kernel.PageRequest{})
	if err != nil || len(page.Items) != 2 {
		t.Fatal(err, page)
	}
	page, _ = e.svc.List(ctx, e.creator, usecase.Filter{}, true, kernel.PageRequest{})
	if len(page.Items) != 1 {
		t.Fatal(len(page.Items))
	}
	if _, err := e.svc.List(ctx, e.creator, usecase.Filter{Status: "x"}, false, kernel.PageRequest{}); !errors.Is(err, kernel.ErrValidation) {
		t.Fatal(err)
	}
	ms, err := e.svc.MemberCandidates(ctx, e.lead)
	if err != nil || len(ms) != 2 {
		t.Fatal(err, ms)
	}
	if _, err := e.svc.MemberCandidates(ctx, testutil.Actor(kernel.RoleReviewer)); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatal(err)
	}
}
