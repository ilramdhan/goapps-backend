package worker

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAllDataCellValue_OilNameShowsGroupName locks D18: column "59.Oil Name"
// shows the RM group NAME behind the stored OIL_NAME code, falls back to the
// code when no name was resolved, and stays blank when there is no code (or
// the stage has no cost row, like every other snapshot column).
func TestAllDataCellValue_OilNameShowsGroupName(t *testing.T) {
	col := allDataColumnByHeader(t, "59.Oil Name")
	require.Equal(t, paramOilName, col.ParamCode)

	tests := []struct {
		name  string
		stage Stage
		want  string
	}{
		{
			name: "resolved name wins over the code",
			stage: Stage{HasCost: true, OilGroupName: "CONING OIL",
				ParamSnapshot: map[string]string{"OIL_NAME": "202006101"}},
			want: "CONING OIL",
		},
		{
			name:  "unresolved code falls back to the code",
			stage: Stage{HasCost: true, ParamSnapshot: map[string]string{"OIL_NAME": "202006101"}},
			want:  "202006101",
		},
		{
			name:  "no OIL_NAME stays blank even with a stray name",
			stage: Stage{HasCost: true, OilGroupName: "CONING OIL", ParamSnapshot: map[string]string{}},
			want:  "",
		},
		{
			name: "no cost row stays blank",
			stage: Stage{HasCost: false, OilGroupName: "CONING OIL",
				ParamSnapshot: map[string]string{"OIL_NAME": "202006101"}},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, allDataCellValue(col, 1, tt.stage))
		})
	}
}

// allDataColumnByHeader returns the manifest column with the given header.
func allDataColumnByHeader(t *testing.T, header string) allDataColumn {
	t.Helper()
	for _, c := range allDataColumns {
		if c.Header == header {
			return c
		}
	}
	t.Fatalf("all data column %q not found", header)
	return allDataColumn{}
}
