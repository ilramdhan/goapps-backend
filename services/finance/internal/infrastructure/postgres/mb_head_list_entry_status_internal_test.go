package postgres

import (
	"strings"
	"testing"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

func TestBuildMBHeadListWhere_EntryStatusesAlone(t *testing.T) {
	base, args := buildMBHeadListWhere(mbhead.ListFilter{EntryStatuses: []string{"SUBMITTED"}})
	if !strings.HasSuffix(base, " AND mbh_entry_status = ANY($1)") || len(args) != 1 {
		t.Fatalf("unexpected: %q %v", base, args)
	}
	if _, ok := args[0].(*pq.StringArray); !ok {
		t.Fatalf("arg should be pq array, got %T", args[0])
	}
}

func TestBuildMBHeadListWhere_EntryStatusesCombined(t *testing.T) {
	id := int64(5)
	active := true
	base, args := buildMBHeadListWhere(mbhead.ListFilter{
		Search: "x", IsActive: &active, CostProductID: &id, EntryStatuses: []string{"DRAFT", "REJECTED"},
	})
	for _, want := range []string{"mbh_mb_costing ILIKE $1", "mbh_is_active = $2", "mbh_cost_product_id = $3", "mbh_entry_status = ANY($4)"} {
		if !strings.Contains(base, want) {
			t.Fatalf("missing %q in %q", want, base)
		}
	}
	if len(args) != 4 {
		t.Fatalf("want 4 args, got %v", args)
	}
}

func TestBuildMBHeadListWhere_NoEntryStatusUnchanged(t *testing.T) {
	base, _ := buildMBHeadListWhere(mbhead.ListFilter{EntryStatuses: []string{}})
	if base != whereNotDeleted {
		t.Fatalf("got %q", base)
	}
}
