package kernel

import (
	"errors"
	"fmt"
	"testing"
)

func TestDomainErrorIsByCode(t *testing.T) {
	err := fmt.Errorf("wrap: %w", ErrNotFound.WithMessage("Không tìm thấy brand"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatal("errors.Is should match by code")
	}
	if errors.Is(err, ErrConflict) {
		t.Fatal("different code must not match")
	}
	if ErrorCode(err) != CodeNotFound {
		t.Fatal(ErrorCode(err))
	}
	if len(ErrNotFound.Details) != 0 {
		t.Fatal("sentinel mutated")
	}
}

func TestValidationErrors(t *testing.T) {
	var v ValidationErrors
	if v.Err() != nil {
		t.Fatal("empty should be nil")
	}
	v.Add("name", "required")
	v.Add("brief.target_audience", "required")
	de, ok := AsDomainError(v.Err())
	if !ok || de.Code != CodeValidation || len(de.Details) != 2 {
		t.Fatalf("%+v", de)
	}
}

func TestMoney(t *testing.T) {
	a := USD("0.50").Add(USD("0.25"))
	if a.String() != "0.750000 USD" {
		t.Fatal(a.String())
	}
	if !USD("8").GreaterThan(a) || !a.LessThanOrEqual(USD("0.75")) {
		t.Fatal("comparison")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("currency mismatch should panic")
		}
	}()
	_ = a.Add(Money{Amount: a.Amount, Currency: "VND"})
}

func TestCursorRoundTrip(t *testing.T) {
	id := NewID()
	c := EncodeCursor(TimeIDCursor{ID: id})
	var got TimeIDCursor
	ok, err := DecodeCursor(c, &got)
	if !ok || err != nil || got.ID != id {
		t.Fatalf("%v %v %v", ok, err, got)
	}
	if _, err := DecodeCursor("!!!", &got); !errors.Is(err, ErrValidation) {
		t.Fatal("bad cursor should be validation error")
	}
	p := PageFromRows([]int{1, 2, 3}, 2, func(i int) any { return i })
	if len(p.Items) != 2 || p.NextCursor == nil {
		t.Fatalf("%+v", p)
	}
	if (PageRequest{Limit: 500}).Normalized().Limit != MaxPageLimit {
		t.Fatal("limit clamp")
	}
}

// TestRBACMatrix encodes SPEC-01 §2 cell by cell.
func TestRBACMatrix(t *testing.T) {
	// columns: admin, lead, creator, reviewer
	matrix := map[string][4]bool{
		PermUserManage:     {true, false, false, false},
		PermProviderManage: {true, false, false, false},
		PermProviderView:   {true, true, false, false},
		PermBudgetManage:   {true, true, false, false},
		PermBrandManage:    {true, true, false, false},
		PermBrandView:      {true, true, true, true},
		PermCampaignManage: {true, true, true, false},
		PermGenerateRun:    {true, true, true, false},
		PermAssetReview:    {true, true, false, true},
		PermAssetDownload:  {true, true, true, true},
		PermUsageView:      {true, true, false, false},
		PermAuditView:      {true, false, false, false},
	}
	if len(matrix) != len(AllPermissions) {
		t.Fatal("matrix incomplete")
	}
	roles := [4]string{RoleAdmin, RoleMarketingLead, RoleContentCreator, RoleReviewer}
	for perm, row := range matrix {
		for i, role := range roles {
			a := Actor{UserID: NewID(), Roles: []string{role}, Permissions: RolePermissions[role]}
			if a.Can(perm) != row[i] {
				t.Errorf("%s/%s: got %v want %v", role, perm, a.Can(perm), row[i])
			}
			if err := a.Require(perm); (err == nil) != row[i] {
				t.Errorf("Require %s/%s", role, perm)
			}
		}
	}
	if !errors.Is((Actor{}).Require(PermBrandView), ErrUnauthenticated) {
		t.Fatal("anonymous should be unauthenticated")
	}
}
