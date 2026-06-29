package costimportetl

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

// buildZip assembles an in-memory .zip from name→content entries.
func buildZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func collectEntry(t *testing.T, c *container, token string) ([][]string, error) {
	t.Helper()
	var got [][]string
	err := c.streamCSVEntry(token, func(row []string) error {
		cp := make([]string, len(row))
		copy(cp, row)
		got = append(got, cp)
		return nil
	})
	return got, err
}

// TestStreamCSVEntry_MergesSplitParts proves a logical layer split across several
// CSV parts (product_parameters_1.csv + _2.csv) is fully merged in name order,
// each part's header skipped — the multi-part contract the params bundle relies on.
func TestStreamCSVEntry_MergesSplitParts(t *testing.T) {
	zipBytes := buildZip(t, map[string]string{
		"product_parameters_1.csv":        "legacy_oracle_sys_id,param_code,data_type,value_numeric,value_text,value_flag\n1,A,NUMERIC,1,,\n2,B,NUMERIC,2,,\n",
		"product_parameters_2.csv":        "legacy_oracle_sys_id,param_code,data_type,value_numeric,value_text,value_flag\n3,C,NUMERIC,3,,\n",
		"product_applicable_params_1.csv": "legacy_oracle_sys_id,param_code,is_required,display_order\n1,A,TRUE,1\n",
	})

	c, err := openContainer(io.NopCloser(bytes.NewReader(zipBytes)), "params.zip")
	require.NoError(t, err)
	defer func() { require.NoError(t, c.Close()) }()

	// Both product_parameters parts merge → 3 data rows, in ascending file order.
	rows, err := collectEntry(t, c, tokenProductParameter)
	require.NoError(t, err)
	require.Equal(t, [][]string{
		{"1", "A", "NUMERIC", "1", "", ""},
		{"2", "B", "NUMERIC", "2", "", ""},
		{"3", "C", "NUMERIC", "3", "", ""},
	}, rows)

	// applicable_param matches only the applicable file (NOT the product_parameters
	// ones — "product_applicable_params" does not contain "product_parameter").
	appRows, err := collectEntry(t, c, tokenApplicableParam)
	require.NoError(t, err)
	require.Equal(t, [][]string{{"1", "A", "TRUE", "1"}}, appRows)

	// A token with no matching entry returns errEmptyContainer (caller → 0 rows).
	_, err = collectEntry(t, c, tokenRouteHead)
	require.ErrorIs(t, err, errEmptyContainer)
}
