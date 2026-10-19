package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/be/acs/api/internal/modules/_template/domain"
	"github.com/be/acs/api/internal/modules/_template/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

func newNotes() (*usecase.Notes, *testutil.Audit) {
	au := &testutil.Audit{}
	return &usecase.Notes{Tx: &testutil.Tx{}, Repo: usecase.NewMemoryRepo(), Clock: testutil.NewClock(), Audit: au}, au
}

func TestCreateRequiresPermission(t *testing.T) {
	uc, _ := newNotes()
	if _, err := uc.Create(context.Background(), testutil.Actor(kernel.RoleReviewer), "x"); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatal(err)
	}
}

func TestCreateAndPublish(t *testing.T) {
	uc, au := newNotes()
	owner := testutil.Actor(kernel.RoleContentCreator)
	n, err := uc.Create(context.Background(), owner, "Ý tưởng")
	if err != nil || len(au.Entries) != 1 {
		t.Fatal(err)
	}
	if _, err := uc.Publish(context.Background(), testutil.Actor(kernel.RoleContentCreator), n.ID); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatalf("other creator must not see it: %v", err)
	}
	p, err := uc.Publish(context.Background(), owner, n.ID)
	if err != nil || p.Status != domain.StatusPublished {
		t.Fatal(err)
	}
	if _, err := uc.Publish(context.Background(), owner, n.ID); !errors.Is(err, kernel.ErrInvalidTransition) {
		t.Fatal(err)
	}
}
