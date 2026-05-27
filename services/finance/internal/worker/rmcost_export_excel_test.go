package worker_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	workerinternal "github.com/mutugading/goapps-backend/services/finance/internal/worker"
)

func TestBuildRMCostExcel_Empty(t *testing.T) {
	t.Parallel()
	bytes, err := workerinternal.BuildRMCostExcel(nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, bytes)

	f, err := excelize.OpenReader(openReader(bytes))
	require.NoError(t, err)
	defer f.Close()

	sheets := f.GetSheetList()
	assert.ElementsMatch(t, []string{"Header", "Detail"}, sheets)

	// Header row exists with column labels.
	headerRows, err := f.GetRows("Header")
	require.NoError(t, err)
	require.NotEmpty(t, headerRows)
	assert.Equal(t, "rm_cost_id", headerRows[0][0])
	assert.Contains(t, headerRows[0], "cost_valuation")

	detailRows, err := f.GetRows("Detail")
	require.NoError(t, err)
	require.NotEmpty(t, detailRows)
	assert.Equal(t, "cost_detail_id", detailRows[0][0])
	assert.Contains(t, detailRows[0], "stock_qty")
}

func openReader(b []byte) *bytesReader { return &bytesReader{r: bytes.NewReader(b)} }

// bytesReader wraps *bytes.Reader to satisfy excelize.OpenReader signature
// without leaking the buffer dependency to callers.
type bytesReader struct{ r *bytes.Reader }

func (b *bytesReader) Read(p []byte) (int, error) { return b.r.Read(p) }
