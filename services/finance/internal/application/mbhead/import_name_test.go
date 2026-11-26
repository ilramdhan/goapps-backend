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

// The import template still has a "Mgt Name" column, but its value must be ignored: name = costing.
func TestImport_MgtNameColumnIgnored_NameEqualsCosting(t *testing.T) {
	f := excelize.NewFile()
	sh := f.GetSheetName(0)
	hdr := []string{"MB Costing", "Mgt Name", "Dev Code", "Shade Code", "Shade Name", "Cross Section", "Lusture Code", "Denier", "Filament", "Dozing", "Is Bought Out"}
	for c, v := range hdr {
		cell, _ := excelize.CoordinatesToCellName(c+1, 1)
		require.NoError(t, f.SetCellValue(sh, cell, v))
	}
	require.NoError(t, f.SetCellValue(sh, "A2", "COST-IMP-1"))
	require.NoError(t, f.SetCellValue(sh, "B2", "SOME OTHER NAME"))
	buf, err := f.WriteToBuffer()
	require.NoError(t, err)

	repo := new(MockRepository)
	repo.On("ExistsByMBCosting", mock.Anything, "COST-IMP-1").Return(false, nil)
	var created *mbheaddomain.Entity
	repo.On("Create", mock.Anything, mock.Anything).Run(func(a mock.Arguments) {
		created = a.Get(1).(*mbheaddomain.Entity)
	}).Return(nil)

	res, err := mbhead.NewImportHandler(repo).Handle(context.Background(), mbhead.ImportCommand{
		FileContent: buf.Bytes(), FileName: "x.xlsx", DuplicateAction: "skip", CreatedBy: "u",
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, res.SuccessCount, "%+v", res.Errors)
	require.NotNil(t, created.MgtName())
	require.Equal(t, "COST-IMP-1", *created.MgtName())
}
