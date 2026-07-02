package postgres

import (
	"testing"

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
