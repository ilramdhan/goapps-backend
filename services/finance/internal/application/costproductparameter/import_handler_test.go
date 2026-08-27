package costproductparameter_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costimportjob"
	cpp "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
)

// =============================================================================
// fakeJobRepo — minimal test double for costimportjob.Repository. AsyncImportHandler.Handle
// only calls GetByID once (to load the job) and Update repeatedly (progress /
// completion); neither Create nor List is exercised by the import path.
// =============================================================================
type fakeJobRepo struct {
	job *costimportjob.CostImportJob
}

func (f *fakeJobRepo) Create(_ context.Context, _ *costimportjob.CostImportJob) error { return nil }

func (f *fakeJobRepo) GetByID(_ context.Context, _ int64) (*costimportjob.CostImportJob, error) {
	return f.job, nil
}

func (f *fakeJobRepo) Update(_ context.Context, _ *costimportjob.CostImportJob) error { return nil }

func (f *fakeJobRepo) List(_ context.Context, _, _ string, _, _ int) ([]*costimportjob.CostImportJob, int64, error) {
	return nil, 0, nil
}

// buildCPPImportXLSX assembles a minimal CPP import workbook: a header row
// (ignored by AsyncImportHandler.Handle, which always skips rows[0]) followed
// by the given data rows in column order product_code, param_code,
// value_numeric, value_text, value_flag.
func buildCPPImportXLSX(t *testing.T, rows [][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			t.Fatalf("close workbook: %v", err)
		}
	}()
	sheet := f.GetSheetName(0)
	header := []interface{}{"product_code", "param_code", "value_numeric", "value_text", "value_flag"}
	if err := f.SetSheetRow(sheet, "A1", &header); err != nil {
		t.Fatalf("write header: %v", err)
	}
	for i, row := range rows {
		cells := make([]interface{}, len(row))
		for j, v := range row {
			cells[j] = v
		}
		cellRef := "A" + intToA1Row(i+2)
		if err := f.SetSheetRow(sheet, cellRef, &cells); err != nil {
			t.Fatalf("write row %d: %v", i, err)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("serialize workbook: %v", err)
	}
	return buf.Bytes()
}

// intToA1Row renders a positive row number as its decimal string (small
// helper so buildCPPImportXLSX reads as plain string concatenation).
func intToA1Row(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

// =============================================================================
// AsyncImportHandler — MB_SPIN companion column resolution
// (cpp_value_mb_spin_id) on the Excel bulk import path.
//
// Mirrors TestUpsert_MBSpinResolution in handlers_test.go: ValueText is ALWAYS
// written unchanged; ValueMBSpinID is populated only for an unambiguous
// (exactly-one) match, and stays nil — without failing the row — for 0 or
// >1 matches.
// =============================================================================
func TestAsyncImportHandler_MBSpinResolution(t *testing.T) {
	t.Parallel()

	mbSpinParamID := uuid.Nil // fakeRepo.GetParamIDByCode always returns uuid.Nil
	spinID := uuid.New()

	t.Run("unique ORION code resolves; ambiguous/unmatched code leaves nil; value_text always unchanged", func(t *testing.T) {
		t.Parallel()

		repo := &fakeRepo{
			productExists: true,
			getMeta: cpp.ParamMeta{
				ParamID:  mbSpinParamID,
				DataType: "TEXT",
				// LookupMasterCode set below via handlers.go's unexported
				// mbSpinLookupMasterCode constant is not reachable from this
				// external test package, so the literal "MB_SPIN" is used —
				// it MUST stay in sync with mbSpinLookupMasterCode in
				// handlers.go.
				LookupMasterCode: "MB_SPIN",
			},
		}
		mbRepo := &fakeMBSpinRepo{
			uniqueByOrionCode: map[string]uuid.UUID{
				"ORION-1": spinID,
				// "ORION-DUP" and "ORION-NONE" deliberately absent: they
				// simulate ">1 matches" and "0 matches" respectively — both
				// collapse to ok=false in ResolveUniqueByOrionItemCode and
				// therefore MUST leave ValueMBSpinID nil.
			},
		}
		jobRepo := &fakeJobRepo{job: costimportjob.NewJob(costimportjob.EntityCPP, "cpp.xlsx", "tester", "")}

		h := app.NewAsyncImportHandler(repo, jobRepo, mbRepo)

		content := buildCPPImportXLSX(t, [][]string{
			{"PROD-1", "MB_SP_CODE", "", "ORION-1", ""},
			{"PROD-1", "MB_SP_CODE", "", "ORION-DUP", ""},
			{"PROD-1", "MB_SP_CODE", "", "ORION-NONE", ""},
		})

		if err := h.Handle(context.Background(), 1, content, "cpp.xlsx"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(repo.upsertedValues) != 3 {
			t.Fatalf("want 3 upserted rows, got %d", len(repo.upsertedValues))
		}

		wantText := []string{"ORION-1", "ORION-DUP", "ORION-NONE"}
		wantSpin := []*uuid.UUID{&spinID, nil, nil}
		for i, v := range repo.upsertedValues {
			if v.ValueText == nil || *v.ValueText != wantText[i] {
				t.Fatalf("row %d: value_text must stay unchanged, want %q got %+v", i, wantText[i], v.ValueText)
			}
			if wantSpin[i] == nil {
				if v.ValueMBSpinID != nil {
					t.Fatalf("row %d (%s): want nil ValueMBSpinID (ambiguous/unmatched), got %s", i, wantText[i], *v.ValueMBSpinID)
				}
				continue
			}
			if v.ValueMBSpinID == nil || *v.ValueMBSpinID != *wantSpin[i] {
				t.Fatalf("row %d (%s): want ValueMBSpinID=%s, got %+v", i, wantText[i], *wantSpin[i], v.ValueMBSpinID)
			}
		}
	})

	t.Run("non MB_SPIN param never resolves ValueMBSpinID even with a configured resolver", func(t *testing.T) {
		t.Parallel()

		repo := &fakeRepo{
			productExists: true,
			getMeta: cpp.ParamMeta{
				DataType:         "TEXT",
				LookupMasterCode: "", // not an MB_SPIN lookup param
			},
		}
		// Configured so that IF the import wrongly attempted resolution, it
		// would succeed — proving the nil result below is a deliberate skip,
		// not an incidental non-match.
		mbRepo := &fakeMBSpinRepo{uniqueByOrionCode: map[string]uuid.UUID{"ORION-1": uuid.New()}}
		jobRepo := &fakeJobRepo{job: costimportjob.NewJob(costimportjob.EntityCPP, "cpp.xlsx", "tester", "")}

		h := app.NewAsyncImportHandler(repo, jobRepo, mbRepo)

		content := buildCPPImportXLSX(t, [][]string{
			{"PROD-1", "SOME_TEXT_PARAM", "", "ORION-1", ""},
		})

		if err := h.Handle(context.Background(), 2, content, "cpp.xlsx"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(repo.upsertedValues) != 1 {
			t.Fatalf("want 1 upserted row, got %d", len(repo.upsertedValues))
		}
		v := repo.upsertedValues[0]
		if v.ValueText == nil || *v.ValueText != "ORION-1" {
			t.Fatalf("value_text must stay unchanged, got %+v", v.ValueText)
		}
		if v.ValueMBSpinID != nil {
			t.Fatalf("want nil ValueMBSpinID for a non-MB_SPIN param, got %s", *v.ValueMBSpinID)
		}
	})

	t.Run("nil mbSpinRepo (not configured) never errors, ValueMBSpinID stays nil, value_text unaffected", func(t *testing.T) {
		t.Parallel()

		repo := &fakeRepo{
			productExists: true,
			getMeta:       cpp.ParamMeta{DataType: "TEXT", LookupMasterCode: "MB_SPIN"},
		}
		jobRepo := &fakeJobRepo{job: costimportjob.NewJob(costimportjob.EntityCPP, "cpp.xlsx", "tester", "")}

		h := app.NewAsyncImportHandler(repo, jobRepo, nil)

		content := buildCPPImportXLSX(t, [][]string{
			{"PROD-1", "MB_SP_CODE", "", "ORION-1", ""},
		})

		if err := h.Handle(context.Background(), 3, content, "cpp.xlsx"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(repo.upsertedValues) != 1 {
			t.Fatalf("want 1 upserted row, got %d", len(repo.upsertedValues))
		}
		v := repo.upsertedValues[0]
		if v.ValueText == nil || *v.ValueText != "ORION-1" {
			t.Fatalf("value_text must stay unchanged, got %+v", v.ValueText)
		}
		if v.ValueMBSpinID != nil {
			t.Fatalf("want nil ValueMBSpinID when no resolver is configured, got %s", *v.ValueMBSpinID)
		}
	})
}
