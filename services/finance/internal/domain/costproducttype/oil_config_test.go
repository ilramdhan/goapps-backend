package costproducttype_test

import (
	"errors"
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
)

func TestNormalizeOilConfig(t *testing.T) {
	g := func(code string, def bool) costproducttype.OilGroupEntry {
		return costproducttype.OilGroupEntry{GroupCode: code, IsDefault: def}
	}
	cases := []struct {
		name      string
		class     string
		groups    []costproducttype.OilGroupEntry
		wantClass string
		wantErr   error
	}{
		{"empty", "", nil, "", nil},
		{"lowercase trimmed", " superba ", []costproducttype.OilGroupEntry{g("A", true)}, "SUPERBA", nil},
		{"invalid", "DTY", nil, "", costproducttype.ErrInvalidOilClass},
		{"no groups", "PTY", nil, "", costproducttype.ErrOilConfigNoGroups},
		{"zero defaults", "PTY", []costproducttype.OilGroupEntry{g("A", false)}, "", costproducttype.ErrOilConfigDefaultCount},
		{"two defaults", "POY", []costproducttype.OilGroupEntry{g("A", true), g("B", true)}, "", costproducttype.ErrOilConfigDefaultCount},
		{"groups without class", "", []costproducttype.OilGroupEntry{g("A", true)}, "", costproducttype.ErrOilConfigGroupsWithoutClass},
		{"duplicate after trim", "PTY", []costproducttype.OilGroupEntry{g("A", true), g(" A ", false)}, "", costproducttype.ErrOilConfigDuplicateGroup},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			class, _, err := costproducttype.NormalizeOilConfig(tc.class, tc.groups)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil || class != tc.wantClass {
				t.Fatalf("got (%q, %v), want %q", class, err, tc.wantClass)
			}
		})
	}
}
