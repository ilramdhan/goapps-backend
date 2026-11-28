package mbhead_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	mbheaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

func importFile(t *testing.T, costing string) []byte {
	t.Helper()
	f := excelize.NewFile()
	sh := f.GetSheetName(0)
	for c, v := range []string{"MB Costing", "Mgt Name", "Dev Code", "Shade Code", "Shade Name", "Cross Section", "Lusture Code", "Denier", "Filament", "Dozing", "Is Bought Out"} {
		cell, _ := excelize.CoordinatesToCellName(c+1, 1)
		require.NoError(t, f.SetCellValue(sh, cell, v))
	}
	require.NoError(t, f.SetCellValue(sh, "A2", costing))
	buf, err := f.WriteToBuffer()
	require.NoError(t, err)
	return buf.Bytes()
}

func TestImport_AmbiguousName_RowFailsAndNothingWritten(t *testing.T) {
	repo := new(MockRepository)
	repo.On("CountByMBCosting", mock.Anything, "DUP").Return(2, nil)

	res, err := mbhead.NewImportHandler(repo).Handle(context.Background(), mbhead.ImportCommand{
		FileContent: importFile(t, "DUP"), FileName: "x.xlsx", DuplicateAction: "update", CreatedBy: "u",
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, res.FailedCount)
	require.Len(t, res.Errors, 1)
	require.Equal(t, "MB costing name matches 2 heads; update it in the UI instead", res.Errors[0].Message)
	repo.AssertNotCalled(t, "GetByMBCosting", mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestImport_UniqueName_StillUpdates(t *testing.T) {
	repo := new(MockRepository)
	repo.On("CountByMBCosting", mock.Anything, "ONE").Return(1, nil)
	e, err := mbheaddomain.New(mbheaddomain.NewParams{MBCosting: "ONE", CreatedBy: "u"})
	require.NoError(t, err)
	repo.On("GetByMBCosting", mock.Anything, "ONE").Return(e, nil)
	repo.On("Update", mock.Anything, mock.Anything).Return(nil)

	res, err := mbhead.NewImportHandler(repo).Handle(context.Background(), mbhead.ImportCommand{
		FileContent: importFile(t, "ONE"), FileName: "x.xlsx", DuplicateAction: "update", CreatedBy: "u",
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, res.SuccessCount, "%+v", res.Errors)
}
