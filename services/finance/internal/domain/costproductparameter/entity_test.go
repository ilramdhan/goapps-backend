package costproductparameter_test

import (
	"errors"
	"testing"

	cpp "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
)

func TestEnsureValueShape(t *testing.T) {
	t.Parallel()

	strPtr := func(s string) *string { return &s }
	boolPtr := func(b bool) *bool { return &b }

	tests := []struct {
		name     string
		dataType string
		num      *string
		text     *string
		flag     *bool
		wantErr  error
	}{
		{
			name:     "NUMBER with numeric value is valid",
			dataType: "NUMBER",
			num:      strPtr("12.5"),
		},
		{
			name:     "TEXT with text value is valid",
			dataType: "TEXT",
			text:     strPtr("hello"),
		},
		{
			name:     "BOOLEAN with flag value is valid",
			dataType: "BOOLEAN",
			flag:     boolPtr(true),
		},
		{
			name:     "no value populated rejects",
			dataType: "NUMBER",
			wantErr:  cpp.ErrInvalidValueShape,
		},
		{
			name:     "two values populated rejects",
			dataType: "NUMBER",
			num:      strPtr("1"),
			text:     strPtr("two"),
			wantErr:  cpp.ErrInvalidValueShape,
		},
		{
			name:     "value type does not match data_type rejects",
			dataType: "NUMBER",
			text:     strPtr("not a number"),
			wantErr:  cpp.ErrInvalidDataType,
		},
		{
			name:     "BOOLEAN with text rejects",
			dataType: "BOOLEAN",
			text:     strPtr("yes"),
			wantErr:  cpp.ErrInvalidDataType,
		},
		{
			name:     "unknown data_type rejects",
			dataType: "JSON",
			text:     strPtr("{}"),
			wantErr:  cpp.ErrInvalidDataType,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := cpp.EnsureValueShape(tc.dataType, tc.num, tc.text, tc.flag)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}
