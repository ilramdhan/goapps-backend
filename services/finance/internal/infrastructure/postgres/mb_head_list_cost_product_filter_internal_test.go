package postgres

import (
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// TestBuildMBHeadListWhere_EmptyFilterUnchanged pins R16's core safety requirement: adding
// the CostProductID predicate must NOT alter List's SQL/args when the filter is omitted.
// This is a structural test (no SQL mock / test database in this package — same approach
// as mb_head_relock_internal_test.go).
func TestBuildMBHeadListWhere_EmptyFilterUnchanged(t *testing.T) {
	base, args := buildMBHeadListWhere(mbhead.ListFilter{})

	if base != whereNotDeleted {
		t.Fatalf("expected base clause unchanged (%q), got %q", whereNotDeleted, base)
	}
	if len(args) != 0 {
		t.Fatalf("expected no args when filter is empty, got %v", args)
	}
	if strings.Contains(base, "mbh_cost_product_id") {
		t.Fatalf("empty filter must never add the cost-product predicate: %q", base)
	}
}

// TestBuildMBHeadListWhere_CostProductIDAddsPredicate pins that a non-nil CostProductID
// adds exactly one AND predicate on mbh_cost_product_id, with the value carried as the
// last positional arg.
func TestBuildMBHeadListWhere_CostProductIDAddsPredicate(t *testing.T) {
	id := int64(42)
	base, args := buildMBHeadListWhere(mbhead.ListFilter{CostProductID: &id})

	if !strings.Contains(base, "mbh_cost_product_id = $1") {
		t.Fatalf("expected mbh_cost_product_id predicate at placeholder $1, got %q", base)
	}
	if len(args) != 1 {
		t.Fatalf("expected exactly one arg, got %v", args)
	}
	if args[0] != id {
		t.Fatalf("expected arg[0] = %d, got %v", id, args[0])
	}
}

// TestBuildMBHeadListWhere_CostProductIDCombinesWithOtherFilters pins placeholder ordering
// when CostProductID is combined with Search and IsActive — a regression here would
// silently misalign LIMIT/OFFSET placeholders appended afterward by List().
func TestBuildMBHeadListWhere_CostProductIDCombinesWithOtherFilters(t *testing.T) {
	id := int64(7)
	active := true
	base, args := buildMBHeadListWhere(mbhead.ListFilter{
		Search:        "MB001",
		IsActive:      &active,
		CostProductID: &id,
	})

	if !strings.Contains(base, "mbh_is_active = $2") {
		t.Fatalf("expected mbh_is_active predicate at placeholder $2, got %q", base)
	}
	if !strings.Contains(base, "mbh_cost_product_id = $3") {
		t.Fatalf("expected mbh_cost_product_id predicate at placeholder $3, got %q", base)
	}
	if len(args) != 3 {
		t.Fatalf("expected 3 args (search, active, cost product id), got %v", args)
	}
	if args[2] != id {
		t.Fatalf("expected last arg = %d, got %v", id, args[2])
	}
}
