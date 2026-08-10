package postgres

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// proto sort_by allow-list from finance/v1/cost_calc.proto ListCostResultsRequest.
// Keys here MUST stay character-identical to the proto in-list: a mismatch makes
// cpcSortColumn fall through to the default and silently drops the user's sort.
var cpcProtoSortKeys = []string{
	"productCode", "productName", "period", "calculationType",
	"costPerUnit", "totalCost", "status", "calculatedAt",
}

func TestCPCSortColumn_CoversProtoAllowList(t *testing.T) {
	for _, key := range cpcProtoSortKeys {
		t.Run(key, func(t *testing.T) {
			assert.NotEmpty(t, cpcSortColumn(key), "proto sort key %q has no column mapping", key)
		})
	}
}

func TestCPCSortColumn_UnknownIsEmpty(t *testing.T) {
	assert.Empty(t, cpcSortColumn(""))
	assert.Empty(t, cpcSortColumn("bogus"))
	assert.Empty(t, cpcSortColumn("ProductCode"), "mapping must be case-sensitive")
}

func TestCPCOrderBy(t *testing.T) {
	tests := []struct {
		name      string
		sortBy    string
		sortOrder string
		want      string
	}{
		{
			"empty falls back to newest first",
			"", "",
			"cpc.cpc_calculated_at DESC, cpc.cpc_cost_id DESC",
		},
		{
			"unknown key falls back to newest first",
			"bogus", "asc",
			"cpc.cpc_calculated_at DESC, cpc.cpc_cost_id DESC",
		},
		{
			"known key defaults to ASC",
			"productCode", "",
			"cpm.cpm_product_code ASC, cpc.cpc_cost_id DESC",
		},
		{
			"desc is case-insensitive",
			"totalCost", "DESC",
			"COALESCE(cpc.cpc_total_cost, 0) DESC, cpc.cpc_cost_id DESC",
		},
		{
			"calculatedAt ascending",
			"calculatedAt", "asc",
			"cpc.cpc_calculated_at ASC, cpc.cpc_cost_id DESC",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, cpcOrderBy(tt.sortBy, tt.sortOrder))
		})
	}
}

// TestCPCOrderBy_AlwaysTieBreaks guards paging stability: every allow-listed key
// must append cpc_cost_id DESC so equal values keep a deterministic page order.
func TestCPCOrderBy_AlwaysTieBreaks(t *testing.T) {
	for _, key := range cpcProtoSortKeys {
		assert.True(t, strings.HasSuffix(cpcOrderBy(key, "asc"), ", cpc.cpc_cost_id DESC"), key)
	}
}

func TestCPCListWhere_ProductTypeFilter(t *testing.T) {
	t.Run("empty type list adds no clause", func(t *testing.T) {
		sql, args := cpcListWhere(costcalc.ResultListFilter{}, "", "2026")
		assert.NotContains(t, sql, "cpm_product_type_id")
		assert.Len(t, args, 1) // year only
	})

	t.Run("non-empty type list adds ANY clause", func(t *testing.T) {
		sql, args := cpcListWhere(
			costcalc.ResultListFilter{ProductTypeIDs: []int32{1, 2, 3}}, "202606", "")
		assert.Contains(t, sql, "cpm.cpm_product_type_id = ANY($2)")
		assert.Len(t, args, 2) // period + type array
	})

	t.Run("placeholders stay sequential with every filter set", func(t *testing.T) {
		sql, args := cpcListWhere(costcalc.ResultListFilter{
			Status:         "VERIFIED",
			CalcType:       costcalc.CalcTypeActual,
			Search:         "abc",
			ProductTypeIDs: []int32{9},
		}, "202606", "")
		assert.Len(t, args, 5)
		for i := 1; i <= 5; i++ {
			assert.Contains(t, sql, "$"+strconv.Itoa(i))
		}
		assert.NotContains(t, sql, "$6")
	})

	t.Run("unset status excludes superseded", func(t *testing.T) {
		sql, _ := cpcListWhere(costcalc.ResultListFilter{}, "202606", "")
		assert.Contains(t, sql, "cpc.cpc_status != 'SUPERSEDED'")
	})
}

// countTopLevelColumns counts comma-separated select items, ignoring commas
// nested inside function calls such as COALESCE(x, 0).
func countTopLevelColumns(sel string) int {
	depth, n := 0, 1
	for _, r := range sel {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				n++
			}
		}
	}
	return n
}

// TestResultColumns_MatchesScanArity guards the write-only-column regression:
// resultColumns and scanResult must agree on count, or every read path breaks.
// The 7 captive/delivery cost columns were selected nowhere before T2.3, so
// HydrateResult returned 0 for them on every read.
func TestResultColumns_MatchesScanArity(t *testing.T) {
	assert.Equal(t, 30, countTopLevelColumns(resultColumns),
		"resultColumns must select 30 columns (23 base + 7 captive/delivery cost cols)")
	for _, col := range []string{
		"cpc_captive_cost", "cpc_delivery_cost", "cpc_vb1_del_cost",
		"cpc_vb2_del_cost", "cpc_vb3_del_cost", "cpc_vb4_del_cost", "cpc_vb5_del_cost",
	} {
		assert.Contains(t, resultColumns, col)
	}
}
