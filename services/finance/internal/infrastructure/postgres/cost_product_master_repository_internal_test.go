package postgres

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

func TestCpmSortColumn(t *testing.T) {
	tests := []struct {
		name   string
		sortBy string
		want   string
	}{
		{"empty defaults to product_code", "", "cpm_product_code"},
		{"unknown key defaults to product_code", "bogus", "cpm_product_code"},
		{"product_code", "product_code", "cpm_product_code"},
		{"product_name", "product_name", "cpm_product_name"},
		{"created_at", "created_at", "cpm_created_at"},
		{"updated_at", "updated_at", "cpm_updated_at"},
		{"product_type_code uses scalar subquery", "product_type_code", "(SELECT cpt_type_code FROM cost_product_type WHERE cpt_type_id = cpm_product_type_id)"},
		{"shade_code", "shade_code", "cpm_shade_code"},
		{"grade_code", "grade_code", "cpm_grade_code"},
		{"oracle_sys_id maps to flex_02", "oracle_sys_id", "cpm_flex_02"},
		{"erp_compound_key maps to flex_01", "erp_compound_key", "cpm_flex_01"},
		{"type_label maps to flex_03", "type_label", "cpm_flex_03"},
		{"status maps to is_active", "status", "cpm_is_active"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, cpmSortColumn(tt.sortBy))
		})
	}
}

func TestCpmOrderBy(t *testing.T) {
	tests := []struct {
		name      string
		sortBy    string
		sortOrder string
		want      string
	}{
		{"default is product_code asc without secondary", "", "", "cpm_product_code ASC"},
		{"product_code desc without secondary", "product_code", "desc", "cpm_product_code DESC"},
		{"desc is case-insensitive", "product_code", "DESC", "cpm_product_code DESC"},
		{"unknown direction falls back to asc", "product_code", "sideways", "cpm_product_code ASC"},
		{"non-code column gets stable secondary ordering", "product_name", "desc", "cpm_product_name DESC, cpm_product_code ASC"},
		{"updated_at asc gets secondary ordering", "updated_at", "asc", "cpm_updated_at ASC, cpm_product_code ASC"},
		{"status desc gets secondary ordering", "status", "desc", "cpm_is_active DESC, cpm_product_code ASC"},
		{
			"type subquery gets secondary ordering",
			"product_type_code",
			"asc",
			"(SELECT cpt_type_code FROM cost_product_type WHERE cpt_type_id = cpm_product_type_id) ASC, cpm_product_code ASC",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, cpmOrderBy(tt.sortBy, tt.sortOrder))
		})
	}
}

func TestCpmEffectiveTypeIDs(t *testing.T) {
	tests := []struct {
		name   string
		filter costproductmaster.Filter
		want   []int64
	}{
		{"no filter yields empty set", costproductmaster.Filter{}, []int64{}},
		{"legacy single id only", costproductmaster.Filter{ProductTypeID: 3}, []int64{3}},
		{"slice only", costproductmaster.Filter{ProductTypeIDs: []int32{5, 7}}, []int64{5, 7}},
		{"union of legacy and slice", costproductmaster.Filter{ProductTypeID: 2, ProductTypeIDs: []int32{5, 7}}, []int64{2, 5, 7}},
		{"legacy duplicated in slice is deduplicated", costproductmaster.Filter{ProductTypeID: 5, ProductTypeIDs: []int32{5, 7}}, []int64{5, 7}},
		{"duplicates within slice are deduplicated", costproductmaster.Filter{ProductTypeIDs: []int32{7, 7, 5}}, []int64{7, 5}},
		{"non-positive entries are ignored", costproductmaster.Filter{ProductTypeID: 0, ProductTypeIDs: []int32{0, -1, 4}}, []int64{4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, cpmEffectiveTypeIDs(tt.filter))
		})
	}
}

// TestCpmFromRow_GradeFallback proves the AX fallback in cpmFromRow is now
// conditional on cpm_source: MB Recipe rows (mbCostProductSource, written NULL by
// mbInsertCostProductMaster in mb_autogen_repository.go) must read back with an empty
// grade so the UI can render "—", while every other source — manual create/update,
// legacy import, duplicate route, or simply unset — must keep falling back to "AX"
// exactly as before this change.
func TestCpmFromRow_GradeFallback(t *testing.T) {
	baseRow := func(source string, grade sql.NullString) cpmRow {
		return cpmRow{
			sysID:     1,
			code:      "CPM-0001",
			typeID:    10,
			name:      "Test Product",
			grade:     grade,
			active:    true,
			createdAt: time.Now(),
			createdBy: "tester",
			updatedAt: time.Now(),
			updatedBy: "tester",
			source:    source,
		}
	}

	tests := []struct {
		name      string
		source    string
		grade     sql.NullString
		wantGrade string
	}{
		{
			name:      "MB Recipe source with NULL grade stays empty",
			source:    mbCostProductSource,
			grade:     sql.NullString{},
			wantGrade: "",
		},
		{
			name:      "MB Recipe source with empty-string grade stays empty",
			source:    mbCostProductSource,
			grade:     sql.NullString{String: "", Valid: true},
			wantGrade: "",
		},
		{
			name:      "MB Recipe source with an explicit grade is preserved verbatim",
			source:    mbCostProductSource,
			grade:     sql.NullString{String: "B2", Valid: true},
			wantGrade: "B2",
		},
		{
			name:      "non-MB source (manual/import/duplicate) with NULL grade falls back to AX",
			source:    "",
			grade:     sql.NullString{},
			wantGrade: "AX",
		},
		{
			name:      "non-MB source with empty-string grade falls back to AX",
			source:    "MANUAL",
			grade:     sql.NullString{String: "", Valid: true},
			wantGrade: "AX",
		},
		{
			name:      "non-MB source with an explicit grade is preserved verbatim",
			source:    "IMPORT",
			grade:     sql.NullString{String: "A1", Valid: true},
			wantGrade: "A1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cpmFromRow(baseRow(tt.source, tt.grade))
			assert.Equal(t, tt.wantGrade, got.GradeCode())
		})
	}
}

// TestCpmFromRow_MBFullRow_AllColumns exercises cpmFromRow with a cpmRow shaped exactly like what
// mbInsertCostProductMaster (mb_autogen_repository.go) writes for a complete MB Head: product_code
// generated, product_name set from entity.MBCosting(), source MB_RECIPE, is_locked TRUE, shade
// code/name copied from the head, grade NULL, and the legacy flex_01/02/03 columns never touched by
// that INSERT's column list (so they arrive back as SQL NULL). This is the no-DB counterpart to
// mb_autogen_cpm_fullrow_integration_test.go's real round trip — it runs under plain `go test`
// (no INTEGRATION_TEST needed) and pins down every column cpmFromRow is responsible for mapping,
// not just the grade fallback TestCpmFromRow_GradeFallback above already covers.
func TestCpmFromRow_MBFullRow_AllColumns(t *testing.T) {
	createdAt := time.Now()
	row := cpmRow{
		sysID:     42,
		code:      "CSTMB260800000123",
		typeID:    10,
		name:      "MB COSTING FULL ROW",
		shade:     sql.NullString{String: "SH-777", Valid: true},
		shadeName: "MIDNIGHT BLUE",
		grade:     sql.NullString{}, // written NULL by mbInsertCostProductMaster
		active:    true,
		createdAt: createdAt,
		createdBy: "itest",
		updatedAt: createdAt,
		updatedBy: "itest",
		// flex01/02/03 intentionally left at zero value (sql equivalent of NULL — never in the
		// mbInsertCostProductMaster column list): confirmed by reading mb_autogen_repository.go's
		// insertQ column list, which lists only cpm_product_code, cpm_product_type_id,
		// cpm_product_name, cpm_source, cpm_is_locked, cpm_shade_code, cpm_shade_name,
		// cpm_grade_code, cpm_created_by, cpm_updated_by — no cpm_flex_01/02/03 anywhere.
		source: mbCostProductSource,
		locked: true,
	}

	got := cpmFromRow(row)

	assert.Equal(t, "CSTMB260800000123", got.ProductCode())
	assert.Equal(t, "MB COSTING FULL ROW", got.ProductName())
	assert.Equal(t, mbCostProductSource, got.Source())
	assert.True(t, got.IsLocked())
	assert.Equal(t, "SH-777", got.ShadeCode())
	assert.Equal(t, "MIDNIGHT BLUE", got.ShadeName())
	assert.Empty(t, got.GradeCode(), "MB_RECIPE source must never fall back to AX")
	assert.Empty(t, got.Flex01())
	assert.Empty(t, got.Flex02())
	assert.Empty(t, got.Flex03())
}
