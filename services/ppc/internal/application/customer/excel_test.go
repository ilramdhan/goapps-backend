package customer_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	customerapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/customer"
	customerdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/customer"
)

// workbook builds an in-memory .xlsx with a header row plus the given data rows,
// matching the customer import template layout.
func workbook(t *testing.T, rows [][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	header := []string{"Code", "Name", "Short Name", "Tax No", "Parent Code"}
	all := append([][]string{header}, rows...)
	for r, row := range all {
		for c, val := range row {
			cell, err := excelize.CoordinatesToCellName(c+1, r+1)
			require.NoError(t, err)
			require.NoError(t, f.SetCellValue(sheet, cell, val))
		}
	}
	var buf bytes.Buffer
	require.NoError(t, f.Write(&buf))
	require.NoError(t, f.Close())
	return buf.Bytes()
}

func TestExport_AlwaysProducesAWorkbook(t *testing.T) {
	repo := newFakeRepo()
	seed(t, repo, "DC1", "ACME")
	svc := customerapp.NewService(repo)

	res, err := svc.Export(context.Background(), customerapp.ListQuery{})
	require.NoError(t, err)
	assert.Equal(t, "customer_export.xlsx", res.FileName)
	assert.NotEmpty(t, res.FileContent)
}

func TestExport_EmptyMaster_StillCarriesHeader(t *testing.T) {
	svc := customerapp.NewService(newFakeRepo())
	res, err := svc.Export(context.Background(), customerapp.ListQuery{})
	require.NoError(t, err)
	assert.NotEmpty(t, res.FileContent)
}

func TestTemplate(t *testing.T) {
	svc := customerapp.NewService(newFakeRepo())
	res, err := svc.Template()
	require.NoError(t, err)
	assert.Equal(t, "customer_import_template.xlsx", res.FileName)
	assert.NotEmpty(t, res.FileContent)
}

func TestImport_CreatesNewRowsAsManual(t *testing.T) {
	repo := newFakeRepo()
	svc := customerapp.NewService(repo)
	content := workbook(t, [][]string{
		{"dc00594", "PT. BESTOW", "BESTOW", "01.234", ""},
		{"DC00100", "PT. BEHAESTEX", "", "", "DC00099"},
	})

	res, err := svc.Import(context.Background(), customerapp.ImportCommand{
		FileContent: content, FileName: "customers.xlsx",
		DuplicateAction: customerapp.DuplicateSkip, CreatedBy: "planner",
	})
	require.NoError(t, err)
	assert.Equal(t, int32(2), res.SuccessCount)
	assert.Zero(t, res.FailedCount)
	require.Contains(t, repo.byCode, "DC00594")
	assert.Equal(t, customerdomain.SourceManual, repo.byCode["DC00594"].Source())
	assert.Nil(t, repo.byCode["DC00100"].ShortName(), "an empty cell must store NULL")
}

func TestImport_DuplicateActions(t *testing.T) {
	content := workbook(t, [][]string{{"DC1", "NEW NAME", "NN", "", ""}})

	tests := []struct {
		name         string
		action       string
		wantSkipped  int32
		wantUpdated  int32
		wantFailed   int32
		wantFinalNam string
	}{
		{"skip", customerapp.DuplicateSkip, 1, 0, 0, "OLD NAME"},
		{"update", customerapp.DuplicateUpdate, 0, 1, 0, "NEW NAME"},
		{"error", customerapp.DuplicateError, 0, 0, 1, "OLD NAME"},
		{"unrecognized treated as skip", "wat", 1, 0, 0, "OLD NAME"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			seed(t, repo, "DC1", "OLD NAME")
			svc := customerapp.NewService(repo)

			res, err := svc.Import(context.Background(), customerapp.ImportCommand{
				FileContent: content, FileName: "c.xlsx",
				DuplicateAction: tt.action, CreatedBy: "planner",
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantSkipped, res.SkippedCount)
			assert.Equal(t, tt.wantUpdated, res.UpdatedCount)
			assert.Equal(t, tt.wantFailed, res.FailedCount)
			assert.Zero(t, res.SuccessCount)
			assert.Equal(t, tt.wantFinalNam, repo.byCode["DC1"].Name())
		})
	}
}

func TestImport_BadRowDoesNotAbortTheRun(t *testing.T) {
	repo := newFakeRepo()
	svc := customerapp.NewService(repo)
	content := workbook(t, [][]string{
		{"DC1", "GOOD", "", "", ""},
		{"DC2", "", "", "", ""}, // missing name
		{"DC3", "ALSO GOOD", "", "", ""},
	})

	res, err := svc.Import(context.Background(), customerapp.ImportCommand{
		FileContent: content, FileName: "c.xlsx",
		DuplicateAction: customerapp.DuplicateSkip, CreatedBy: "planner",
	})
	require.NoError(t, err)
	assert.Equal(t, int32(2), res.SuccessCount)
	assert.Equal(t, int32(1), res.FailedCount)
	require.Len(t, res.Errors, 1)
	assert.Equal(t, int32(3), res.Errors[0].RowNumber)
	assert.Equal(t, "name", res.Errors[0].Field)
	assert.Contains(t, repo.byCode, "DC3")
}

func TestImport_BlankRowIsNotAFailure(t *testing.T) {
	repo := newFakeRepo()
	svc := customerapp.NewService(repo)
	content := workbook(t, [][]string{
		{"DC1", "GOOD", "", "", ""},
		{"", "", "", "", ""},
	})

	res, err := svc.Import(context.Background(), customerapp.ImportCommand{
		FileContent: content, FileName: "c.xlsx",
		DuplicateAction: customerapp.DuplicateSkip, CreatedBy: "planner",
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), res.SuccessCount)
	assert.Zero(t, res.FailedCount)
	assert.Zero(t, res.SkippedCount)
}

func TestImport_RejectsNonExcelFile(t *testing.T) {
	svc := customerapp.NewService(newFakeRepo())
	_, err := svc.Import(context.Background(), customerapp.ImportCommand{
		FileContent: []byte("nope"), FileName: "customers.csv",
		DuplicateAction: customerapp.DuplicateSkip, CreatedBy: "planner",
	})
	require.Error(t, err)
}
