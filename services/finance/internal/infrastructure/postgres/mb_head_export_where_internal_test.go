package postgres

import (
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// TestBuildMBHeadExportWhere_DefaultExcludesRejected pins §11 item 140's fix: with the
// filter's zero value (IncludeRejected=false), ListAll's WHERE clause must add a
// parameterized "!= " predicate on mbh_entry_status carrying mbhead.StatusRejected — never
// a string-interpolated "REJECTED" literal. This is a structural test (no SQL mock / test
// database in this package — same approach as buildMBHeadListWhere's tests).
func TestBuildMBHeadExportWhere_DefaultExcludesRejected(t *testing.T) {
	base, args := buildMBHeadExportWhere(mbhead.ExportFilter{})

	if !strings.Contains(base, "mbh_entry_status != $1") {
		t.Fatalf("expected mbh_entry_status exclusion predicate at placeholder $1, got %q", base)
	}
	if strings.Contains(base, "'REJECTED'") || strings.Contains(base, "\"REJECTED\"") {
		t.Fatalf("status value must travel as a bound parameter, not interpolated into SQL: %q", base)
	}
	if len(args) != 1 {
		t.Fatalf("expected exactly one arg (the excluded status), got %v", args)
	}
	if args[0] != mbhead.StatusRejected {
		t.Fatalf("expected arg[0] = %q (mbhead.StatusRejected), got %v", mbhead.StatusRejected, args[0])
	}
}

// TestBuildMBHeadExportWhere_IncludeRejectedOmitsPredicate pins the explicit opt-in: when
// IncludeRejected is true, no status predicate is added at all, so REJECTED heads (and
// every other status) are returned — this is the "audit" path.
func TestBuildMBHeadExportWhere_IncludeRejectedOmitsPredicate(t *testing.T) {
	base, args := buildMBHeadExportWhere(mbhead.ExportFilter{IncludeRejected: true})

	if base != whereNotDeleted {
		t.Fatalf("expected base clause unchanged (%q) when IncludeRejected=true, got %q", whereNotDeleted, base)
	}
	if len(args) != 0 {
		t.Fatalf("expected no args when IncludeRejected=true and IsActive is nil, got %v", args)
	}
}

// TestBuildMBHeadExportWhere_CombinesWithIsActive pins placeholder ordering when both
// IsActive and the (default) rejected-exclusion predicate are present — a regression here
// would silently misalign the ORDER BY/positional args appended afterward by ListAll.
func TestBuildMBHeadExportWhere_CombinesWithIsActive(t *testing.T) {
	active := true
	base, args := buildMBHeadExportWhere(mbhead.ExportFilter{IsActive: &active})

	if !strings.Contains(base, "mbh_is_active = $1") {
		t.Fatalf("expected mbh_is_active predicate at placeholder $1, got %q", base)
	}
	if !strings.Contains(base, "mbh_entry_status != $2") {
		t.Fatalf("expected mbh_entry_status exclusion predicate at placeholder $2, got %q", base)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args (active, excluded status), got %v", args)
	}
	if args[1] != mbhead.StatusRejected {
		t.Fatalf("expected arg[1] = %q, got %v", mbhead.StatusRejected, args[1])
	}
}

// TestBuildMBHeadExportWhere_IncludeRejectedWithIsActive pins that IncludeRejected=true
// still lets IsActive filter through on its own, without the status predicate riding
// along on the same placeholder.
func TestBuildMBHeadExportWhere_IncludeRejectedWithIsActive(t *testing.T) {
	active := false
	base, args := buildMBHeadExportWhere(mbhead.ExportFilter{IsActive: &active, IncludeRejected: true})

	if !strings.Contains(base, "mbh_is_active = $1") {
		t.Fatalf("expected mbh_is_active predicate at placeholder $1, got %q", base)
	}
	if strings.Contains(base, "mbh_entry_status") {
		t.Fatalf("IncludeRejected=true must omit the status predicate entirely: %q", base)
	}
	if len(args) != 1 {
		t.Fatalf("expected exactly one arg (active), got %v", args)
	}
}
