// Package mbhead_test provides unit tests for the MB Head import handler (spec section 5).
package mbhead_test

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	mbheaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// mbHeadCanonicalHeaders mirrors the header order documented in spec section 5.2. Deliberately
// hardcoded here (rather than imported) so the test independently pins the contract instead of
// trivially agreeing with whatever import_columns.go says.
var mbHeadCanonicalHeaders = []string{
	"MB Costing", "MB Name", "Development No", "VS Number", "No of Process",
	"Shade Code", "Shade Name", "POY Denier", "POY Filament", "Cross Section",
	"LDR %", "Final Product", "Dozing", "Lusture Code", "Is Bought Out",
	"Shade Code 2", "Shade Name 2", "Shade Code 3", "Shade Name 3",
}

// validMBHeadRowValues returns a header-name-keyed row that satisfies every required field.
func validMBHeadRowValues(mbCosting string) map[string]string {
	return map[string]string{
		"MB Costing":     mbCosting,
		"MB Name":        "MGT NAME",
		"Development No": "DEV-" + mbCosting,
		"VS Number":      "VS-" + mbCosting,
		"No of Process":  "S",
		"Shade Code":     "SH1",
		"Shade Name":     "SHADE ONE",
		"POY Denier":     "150",
		"POY Filament":   "48",
		"Cross Section":  "ROUND",
		"LDR %":          "10",
		"Final Product":  "FP",
		"Dozing":         "",
		"Lusture Code":   "BR",
		"Is Bought Out":  "",
		"Shade Code 2":   "",
		"Shade Name 2":   "",
		"Shade Code 3":   "",
		"Shade Name 3":   "",
	}
}

// buildMBHeadXLSX writes headers as row 1 and rows (keyed by header) starting at row 2, so tests
// exercise the real Excel parse -> header-resolve -> row-parse pipeline end to end, not a
// hand-built []mbHeadRowData shortcut.
func buildMBHeadXLSX(t *testing.T, headers []string, rows []map[string]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { require.NoError(t, f.Close()) }()

	sheet := f.GetSheetName(0)
	for col, h := range headers {
		cell, err := excelize.CoordinatesToCellName(col+1, 1)
		require.NoError(t, err)
		require.NoError(t, f.SetCellValue(sheet, cell, h))
	}
	for r, row := range rows {
		for col, h := range headers {
			cell, err := excelize.CoordinatesToCellName(col+1, r+2)
			require.NoError(t, err)
			require.NoError(t, f.SetCellValue(sheet, cell, row[h]))
		}
	}

	buf, err := f.WriteToBuffer()
	require.NoError(t, err)
	return buf.Bytes()
}

func newImportHandler(repo *MockRepository, params *MockParamRepository) *mbhead.ImportHandler {
	return mbhead.NewImportHandler(repo, params)
}

// TestImportHandler_HeaderMapping_Reordered proves columns are matched by header name, not
// position: the sheet lists every canonical header in reverse order and the row still imports.
func TestImportHandler_HeaderMapping_Reordered(t *testing.T) {
	reordered := make([]string, len(mbHeadCanonicalHeaders))
	for i, h := range mbHeadCanonicalHeaders {
		reordered[len(mbHeadCanonicalHeaders)-1-i] = h
	}

	content := buildMBHeadXLSX(t, reordered, []map[string]string{validMBHeadRowValues("MBC-R1")})

	repo := new(MockRepository)
	repo.On("ExistsByMBCosting", mock.Anything, "MBC-R1").Return(false, nil)
	repo.On("ExistsByDevCode", mock.Anything, "DEV-MBC-R1", (*uuid.UUID)(nil)).Return(false, nil)
	repo.On("ExistsByVsNumber", mock.Anything, "VS-MBC-R1", (*uuid.UUID)(nil)).Return(false, nil)
	repo.On("Create", mock.Anything, mock.MatchedBy(func(e *mbheaddomain.Entity) bool {
		return e.MBCosting() == "MBC-R1" && e.DevCode() == "DEV-MBC-R1" && e.VsNumber() == "VS-MBC-R1" &&
			e.ShadeCode() == "SH1" && e.NoOfProcess() == "S"
	})).Return(nil)

	h := newImportHandler(repo, newNoOfProcessParamRepo(t))
	result, err := h.Handle(t.Context(), mbhead.ImportCommand{
		FileContent: content, FileName: "import.xlsx", DuplicateAction: "skip", CreatedBy: "admin",
	})

	require.NoError(t, err)
	assert.Equal(t, int32(1), result.SuccessCount)
	assert.Equal(t, int32(0), result.FailedCount)
	repo.AssertExpectations(t)
}

// TestImportHandler_MissingRequiredHeader_AbortsWholeFile proves a missing required header fails
// the whole file (spec section 5.1), not just the rows that would have used it.
func TestImportHandler_MissingRequiredHeader_AbortsWholeFile(t *testing.T) {
	headers := make([]string, 0, len(mbHeadCanonicalHeaders)-1)
	for _, h := range mbHeadCanonicalHeaders {
		if h == "Development No" {
			continue
		}
		headers = append(headers, h)
	}

	content := buildMBHeadXLSX(t, headers, []map[string]string{validMBHeadRowValues("MBC-M1")})

	repo := new(MockRepository)
	h := newImportHandler(repo, newNoOfProcessParamRepo(t))
	result, err := h.Handle(t.Context(), mbhead.ImportCommand{
		FileContent: content, FileName: "import.xlsx", DuplicateAction: "skip", CreatedBy: "admin",
	})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "Development No")
	repo.AssertNotCalled(t, "ExistsByMBCosting", mock.Anything, mock.Anything)
}

// TestImportHandler_RequiredFieldBlank_RejectsRow covers all 11 required fields of spec
// section 2.1: blanking any one of them fails that row with the domain's own sentinel message.
func TestImportHandler_RequiredFieldBlank_RejectsRow(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		wantField string
		wantMsg   string
	}{
		{"mb name", "MB Name", "mbh_mgt_name", mbheaddomain.ErrEmptyMgtName.Error()},
		{"development no", "Development No", "mbh_dev_code", mbheaddomain.ErrEmptyDevCode.Error()},
		{"vs number", "VS Number", "mbh_vs_number", mbheaddomain.ErrEmptyVsNumber.Error()},
		{"no of process", "No of Process", "mbh_no_of_process", mbheaddomain.ErrEmptyNoOfProcess.Error()},
		{"shade code", "Shade Code", "mbh_shade_code", mbheaddomain.ErrEmptyShadeCode.Error()},
		{"shade name", "Shade Name", "mbh_shade_name", mbheaddomain.ErrEmptyShadeName.Error()},
		{"poy denier", "POY Denier", "mbh_denier", mbheaddomain.ErrInvalidDenier.Error()},
		{"poy filament", "POY Filament", "mbh_filament", mbheaddomain.ErrInvalidFilament.Error()},
		{"cross section", "Cross Section", "mbh_cross_section", mbheaddomain.ErrEmptyCrossSection.Error()},
		{"ldr %", "LDR %", "mbh_ldr_prsn", mbheaddomain.ErrInvalidLdrPercent.Error()},
		{"final product", "Final Product", "mbh_final_product", mbheaddomain.ErrEmptyFinalProduct.Error()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := validMBHeadRowValues("MBC-BLANK")
			row[tt.header] = ""
			content := buildMBHeadXLSX(t, mbHeadCanonicalHeaders, []map[string]string{row})

			repo := new(MockRepository)
			h := newImportHandler(repo, newNoOfProcessParamRepo(t))
			result, err := h.Handle(t.Context(), mbhead.ImportCommand{
				FileContent: content, FileName: "import.xlsx", DuplicateAction: "skip", CreatedBy: "admin",
			})

			require.NoError(t, err)
			require.Len(t, result.Errors, 1)
			assert.Equal(t, int32(0), result.SuccessCount)
			assert.Equal(t, int32(1), result.FailedCount)
			assert.Equal(t, tt.wantField, result.Errors[0].Field)
			assert.Equal(t, tt.wantMsg, result.Errors[0].Message)
			repo.AssertNotCalled(t, "ExistsByMBCosting", mock.Anything, mock.Anything)
		})
	}
}

// TestImportHandler_InFileDuplicateDetection_DevCode proves a second row claiming a dev code
// already used earlier in the same file is rejected before it ever reaches the database.
func TestImportHandler_InFileDuplicateDetection_DevCode(t *testing.T) {
	row1 := validMBHeadRowValues("MBC-D1")
	row2 := validMBHeadRowValues("MBC-D2")
	row2["Development No"] = row1["Development No"] // same dev code, distinct vs number/mb_costing

	content := buildMBHeadXLSX(t, mbHeadCanonicalHeaders, []map[string]string{row1, row2})

	repo := new(MockRepository)
	repo.On("ExistsByMBCosting", mock.Anything, "MBC-D1").Return(false, nil)
	repo.On("ExistsByDevCode", mock.Anything, "DEV-MBC-D1", (*uuid.UUID)(nil)).Return(false, nil)
	repo.On("ExistsByVsNumber", mock.Anything, "VS-MBC-D1", (*uuid.UUID)(nil)).Return(false, nil)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)

	h := newImportHandler(repo, newNoOfProcessParamRepo(t))
	result, err := h.Handle(t.Context(), mbhead.ImportCommand{
		FileContent: content, FileName: "import.xlsx", DuplicateAction: "skip", CreatedBy: "admin",
	})

	require.NoError(t, err)
	assert.Equal(t, int32(1), result.SuccessCount)
	assert.Equal(t, int32(1), result.FailedCount)
	require.Len(t, result.Errors, 1)
	assert.Equal(t, "mbh_dev_code", result.Errors[0].Field)
	assert.Contains(t, result.Errors[0].Message, "duplicates row 2")
	repo.AssertExpectations(t)
	repo.AssertNotCalled(t, "ExistsByMBCosting", mock.Anything, "MBC-D2")
}

// TestImportHandler_InFileDuplicateDetection_VsNumber mirrors the dev-code case for vs_number.
func TestImportHandler_InFileDuplicateDetection_VsNumber(t *testing.T) {
	row1 := validMBHeadRowValues("MBC-V1")
	row2 := validMBHeadRowValues("MBC-V2")
	row2["VS Number"] = row1["VS Number"]

	content := buildMBHeadXLSX(t, mbHeadCanonicalHeaders, []map[string]string{row1, row2})

	repo := new(MockRepository)
	repo.On("ExistsByMBCosting", mock.Anything, "MBC-V1").Return(false, nil)
	repo.On("ExistsByDevCode", mock.Anything, "DEV-MBC-V1", (*uuid.UUID)(nil)).Return(false, nil)
	repo.On("ExistsByVsNumber", mock.Anything, "VS-MBC-V1", (*uuid.UUID)(nil)).Return(false, nil)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)

	h := newImportHandler(repo, newNoOfProcessParamRepo(t))
	result, err := h.Handle(t.Context(), mbhead.ImportCommand{
		FileContent: content, FileName: "import.xlsx", DuplicateAction: "skip", CreatedBy: "admin",
	})

	require.NoError(t, err)
	assert.Equal(t, int32(1), result.SuccessCount)
	assert.Equal(t, int32(1), result.FailedCount)
	require.Len(t, result.Errors, 1)
	assert.Equal(t, "mbh_vs_number", result.Errors[0].Field)
	assert.Contains(t, result.Errors[0].Message, "duplicates row 2")
}

// TestImportHandler_DryRun_WritesNothing proves dry_run leaves the mock repository's write
// methods entirely uncalled on both the create and the update path, while still validating and
// counting rows as it would for real (spec section 5.4).
func TestImportHandler_DryRun_WritesNothing(t *testing.T) {
	createRow := validMBHeadRowValues("MBC-NEW")
	updateRow := validMBHeadRowValues("MBC-EXIST")

	content := buildMBHeadXLSX(t, mbHeadCanonicalHeaders, []map[string]string{createRow, updateRow})

	existing := newTestEntity(t, "MBC-EXIST")

	repo := new(MockRepository)
	repo.On("ExistsByMBCosting", mock.Anything, "MBC-NEW").Return(false, nil)
	repo.On("ExistsByDevCode", mock.Anything, "DEV-MBC-NEW", (*uuid.UUID)(nil)).Return(false, nil)
	repo.On("ExistsByVsNumber", mock.Anything, "VS-MBC-NEW", (*uuid.UUID)(nil)).Return(false, nil)

	repo.On("ExistsByMBCosting", mock.Anything, "MBC-EXIST").Return(true, nil)
	repo.On("GetByMBCosting", mock.Anything, "MBC-EXIST").Return(existing, nil)
	repo.On("ExistsByDevCode", mock.Anything, "DEV-MBC-EXIST", mock.AnythingOfType("*uuid.UUID")).Return(false, nil)
	repo.On("ExistsByVsNumber", mock.Anything, "VS-MBC-EXIST", mock.AnythingOfType("*uuid.UUID")).Return(false, nil)

	h := newImportHandler(repo, newNoOfProcessParamRepo(t))
	result, err := h.Handle(t.Context(), mbhead.ImportCommand{
		FileContent: content, FileName: "import.xlsx", DuplicateAction: "update", CreatedBy: "admin", DryRun: true,
	})

	require.NoError(t, err)
	assert.Equal(t, int32(1), result.SuccessCount)
	assert.Equal(t, int32(1), result.UpdatedCount)
	assert.Equal(t, int32(0), result.FailedCount)

	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "ReplaceShades", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestImportHandler_RoundTrip_ExportThenImport feeds an ExportHandler's own output straight back
// into ImportHandler and checks the parsed values match the original entity (spec section 5.2:
// export and import must agree on the round-trippable column set).
func TestImportHandler_RoundTrip_ExportThenImport(t *testing.T) {
	original := newTestEntity(t, "MBC-RT1")
	shade2, err := mbheaddomain.NewShade(original.ID(), mbheaddomain.MinShadeSeqNo, "SH2", "SHADE TWO", "admin")
	require.NoError(t, err)

	exportRepo := new(MockRepository)
	exportRepo.On("ListAll", mock.Anything, mock.Anything).Return([]*mbheaddomain.Entity{original}, nil)
	exportRepo.On("ListShadesByHeads", mock.Anything, []uuid.UUID{original.ID()}).
		Return(map[uuid.UUID][]*mbheaddomain.Shade{original.ID(): {shade2}}, nil)

	exportHandler := mbhead.NewExportHandler(exportRepo)
	exportResult, err := exportHandler.Handle(t.Context(), mbhead.ExportQuery{})
	require.NoError(t, err)
	require.NotEmpty(t, exportResult.FileContent)

	// Sanity-check the exported file actually round-trips through excelize before importing it.
	_, err = excelize.OpenReader(bytes.NewReader(exportResult.FileContent))
	require.NoError(t, err)

	importRepo := new(MockRepository)
	importRepo.On("ExistsByMBCosting", mock.Anything, "MBC-RT1").Return(false, nil)
	importRepo.On("ExistsByDevCode", mock.Anything, "DEV-MBC-RT1", (*uuid.UUID)(nil)).Return(false, nil)
	importRepo.On("ExistsByVsNumber", mock.Anything, "VS-MBC-RT1", (*uuid.UUID)(nil)).Return(false, nil)
	importRepo.On("Create", mock.Anything, mock.MatchedBy(func(e *mbheaddomain.Entity) bool {
		return e.MBCosting() == "MBC-RT1" &&
			e.DevCode() == original.DevCode() &&
			e.VsNumber() == original.VsNumber() &&
			e.NoOfProcess() == original.NoOfProcess() &&
			e.ShadeCode() == original.ShadeCode() &&
			e.ShadeName() == original.ShadeName() &&
			e.CrossSection() == original.CrossSection() &&
			*e.MBHFinalProduct() == *original.MBHFinalProduct() &&
			*e.Denier() == *original.Denier() &&
			*e.Filament() == *original.Filament() &&
			*e.MBHLdrPrsn() == *original.MBHLdrPrsn() &&
			e.LustureCode() == original.LustureCode()
	})).Return(nil)
	importRepo.On("ReplaceShades", mock.Anything, mock.Anything, mock.MatchedBy(func(shades []*mbheaddomain.Shade) bool {
		return len(shades) == 1 && shades[0].ShadeCode() == "SH2" && shades[0].ShadeName() == "SHADE TWO"
	}), "admin").Return(nil)

	importHandler := newImportHandler(importRepo, newNoOfProcessParamRepo(t))
	importResult, err := importHandler.Handle(t.Context(), mbhead.ImportCommand{
		FileContent: exportResult.FileContent, FileName: "roundtrip.xlsx",
		DuplicateAction: "skip", CreatedBy: "admin",
	})

	require.NoError(t, err)
	assert.Equal(t, int32(1), importResult.SuccessCount)
	assert.Equal(t, int32(0), importResult.FailedCount)
	importRepo.AssertExpectations(t)
}

// TestImportHandler_UpdateBoughtOut_Regression is the I4 regression test: updating an MB Head
// through import with "Is Bought Out" set to TRUE must land on the persisted entity, not be
// silently dropped the way the pre-fix update path used to drop it.
func TestImportHandler_UpdateBoughtOut_Regression(t *testing.T) {
	existing := newTestEntity(t, "MBC-BOUGHT")
	require.False(t, existing.IsBoughtout())

	row := validMBHeadRowValues("MBC-BOUGHT")
	row["Is Bought Out"] = "TRUE"
	content := buildMBHeadXLSX(t, mbHeadCanonicalHeaders, []map[string]string{row})

	repo := new(MockRepository)
	repo.On("ExistsByMBCosting", mock.Anything, "MBC-BOUGHT").Return(true, nil)
	repo.On("GetByMBCosting", mock.Anything, "MBC-BOUGHT").Return(existing, nil)
	repo.On("ExistsByDevCode", mock.Anything, "DEV-MBC-BOUGHT", mock.AnythingOfType("*uuid.UUID")).Return(false, nil)
	repo.On("ExistsByVsNumber", mock.Anything, "VS-MBC-BOUGHT", mock.AnythingOfType("*uuid.UUID")).Return(false, nil)
	repo.On("Update", mock.Anything, mock.MatchedBy(func(e *mbheaddomain.Entity) bool {
		return e.IsBoughtout()
	})).Return(nil)
	repo.On("ReplaceShades", mock.Anything, existing.ID(), mock.Anything, "admin").Return(nil)

	h := newImportHandler(repo, newNoOfProcessParamRepo(t))
	result, err := h.Handle(t.Context(), mbhead.ImportCommand{
		FileContent: content, FileName: "import.xlsx", DuplicateAction: "update", CreatedBy: "admin",
	})

	require.NoError(t, err)
	assert.Equal(t, int32(1), result.UpdatedCount)
	assert.Equal(t, int32(0), result.FailedCount)
	assert.True(t, existing.IsBoughtout(), "Is Bought Out must survive the import update (I4 regression)")
	repo.AssertExpectations(t)
}
