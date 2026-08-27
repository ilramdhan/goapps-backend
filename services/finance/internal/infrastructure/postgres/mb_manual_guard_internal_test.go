package postgres

import (
	"reflect"
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

// TestUpsertTypeIDSet pins the input normalization the bulk MB guard depends on: one DB
// round-trip per call instead of one per row, and no query for a non-positive type id (the FK
// and domain validation own that case).
func TestUpsertTypeIDSet(t *testing.T) {
	tests := []struct {
		name string
		in   []int32
		want []int32
	}{
		{name: "empty stays empty", in: nil, want: []int32{}},
		{name: "duplicates collapse", in: []int32{2, 2, 7, 2}, want: []int32{2, 7}},
		{name: "zero and negatives are dropped", in: []int32{0, -1, 3}, want: []int32{3}},
		{name: "only invalid ids yields empty", in: []int32{0, 0}, want: []int32{}},
		{name: "order of first appearance is kept", in: []int32{5, 1, 5, 9}, want: []int32{5, 1, 9}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := upsertTypeIDSet(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
		})
	}
}

// TestUpsertInputTypeIDs checks the ProductUpsertInput adapter feeds the same normalized set.
func TestUpsertInputTypeIDs(t *testing.T) {
	got := upsertInputTypeIDs([]costproductmaster.ProductUpsertInput{
		{ProductTypeID: 4}, {ProductTypeID: 0}, {ProductTypeID: 4}, {ProductTypeID: 7},
	})
	want := []int32{4, 7}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

// TestRejectMBTypeIDs_EmptyShortCircuits guarantees an all-invalid batch never touches the
// database — the nil querier would panic if it did.
func TestRejectMBTypeIDs_EmptyShortCircuits(t *testing.T) {
	if err := rejectMBTypeIDs(t.Context(), nil, nil); err != nil {
		t.Fatalf("empty input must be a no-op, got %v", err)
	}
	if err := rejectMBTypeIDs(t.Context(), nil, upsertTypeIDSet([]int32{0, -3})); err != nil {
		t.Fatalf("all-invalid input must be a no-op, got %v", err)
	}
}
