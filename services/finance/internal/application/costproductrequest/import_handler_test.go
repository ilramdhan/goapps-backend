package costproductrequest_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductrequest"
	pmDomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductrequest"
	rtDomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costrequesttype"
)

func timeNow() time.Time { return time.Now().UTC() }

// fakeImportRequestRepo is a configurable fake for domain.Repository, used
// only by ImportHandler tests (via the real CreateHandler). Records every
// Created request so tests can assert create-only (no dedup) behavior.
type fakeImportRequestRepo struct {
	created []*domain.Request
	// createErrForTitle, when non-empty, makes Create fail for a request
	// whose Title matches — used to test the "create" row-level ImportError path.
	createErrForTitle string
}

func (r *fakeImportRequestRepo) Create(_ context.Context, req *domain.Request) error {
	if r.createErrForTitle != "" && req.Title() == r.createErrForTitle {
		return errors.New("simulated create failure")
	}
	r.created = append(r.created, req)
	return nil
}
func (r *fakeImportRequestRepo) GetByID(_ context.Context, _ int64) (*domain.Request, error) {
	return nil, nil
}
func (r *fakeImportRequestRepo) GetByNo(_ context.Context, _ string) (*domain.Request, error) {
	return nil, nil
}
func (r *fakeImportRequestRepo) Save(_ context.Context, _ *domain.Request) error { return nil }
func (r *fakeImportRequestRepo) List(_ context.Context, _ domain.Filter) ([]*domain.Request, int64, error) {
	return nil, 0, nil
}
func (r *fakeImportRequestRepo) ListAll(_ context.Context, _ domain.Filter) ([]*domain.Request, error) {
	return nil, nil
}

// fakeImportRequestTypeRepo is a configurable fake for rtDomain.Repository,
// used only by ImportHandler tests (only GetIDByCode is exercised).
type fakeImportRequestTypeRepo struct {
	idsByCode map[string]int32
}

func (r *fakeImportRequestTypeRepo) List(_ context.Context, _ rtDomain.Filter) ([]*rtDomain.CostRequestType, int64, error) {
	return nil, 0, nil
}
func (r *fakeImportRequestTypeRepo) GetByID(_ context.Context, _ int32) (*rtDomain.CostRequestType, error) {
	return nil, rtDomain.ErrNotFound
}
func (r *fakeImportRequestTypeRepo) GetIDByCode(_ context.Context, code string) (int32, error) {
	if id, ok := r.idsByCode[code]; ok {
		return id, nil
	}
	return 0, rtDomain.ErrNotFound
}

// fakeImportProductMasterRepo is a configurable fake for pmDomain.Repository,
// used only by ImportHandler tests (only GetByCode is exercised).
type fakeImportProductMasterRepo struct {
	byCode map[string]*pmDomain.CostProductMaster
}

func (r *fakeImportProductMasterRepo) Create(_ context.Context, _ *pmDomain.CostProductMaster) error {
	return nil
}
func (r *fakeImportProductMasterRepo) GetBySysID(_ context.Context, _ int64) (*pmDomain.CostProductMaster, error) {
	return nil, pmDomain.ErrNotFound
}
func (r *fakeImportProductMasterRepo) GetByCode(_ context.Context, code string) (*pmDomain.CostProductMaster, error) {
	if p, ok := r.byCode[code]; ok {
		return p, nil
	}
	return nil, pmDomain.ErrNotFound
}
func (r *fakeImportProductMasterRepo) Update(_ context.Context, _ *pmDomain.CostProductMaster) error {
	return nil
}
func (r *fakeImportProductMasterRepo) List(_ context.Context, _ pmDomain.Filter) ([]*pmDomain.CostProductMaster, int64, error) {
	return nil, 0, nil
}
func (r *fakeImportProductMasterRepo) BulkCreate(_ context.Context, _ []*pmDomain.CostProductMaster, _ string) (map[string]int64, error) {
	return nil, nil
}
func (r *fakeImportProductMasterRepo) ListAll(_ context.Context, _ pmDomain.Filter) ([]*pmDomain.CostProductMaster, error) {
	return nil, nil
}
func (r *fakeImportProductMasterRepo) BulkUpsertByLegacyID(_ context.Context, _ []pmDomain.ProductUpsertInput, _ string) ([]pmDomain.ProductUpsertResult, error) {
	return nil, nil
}
func (r *fakeImportProductMasterRepo) ListAllLegacyIDs(_ context.Context) (map[string]int64, error) {
	return nil, nil
}
func (r *fakeImportProductMasterRepo) RollbackImport(_ context.Context, _ []int64) error { return nil }

func (r *fakeImportProductMasterRepo) UnlockWithLog(_ context.Context, _ pmDomain.LockLogInput) error {
	return nil
}

func (r *fakeImportProductMasterRepo) DuplicateProduct(_ context.Context, _ pmDomain.DuplicateInput) (pmDomain.DuplicateOutput, error) {
	return pmDomain.DuplicateOutput{}, nil
}

// buildXlsxRows builds an .xlsx file's bytes from a header row + data rows,
// in exportHeaders column order. Returns file content ready to feed into
// ImportCommand.FileContent.
func buildXlsxRows(t *testing.T, rows [][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sheet := f.GetSheetList()[0]
	for r, row := range rows {
		for c, v := range row {
			cell, err := excelize.CoordinatesToCellName(c+1, r+1)
			require.NoError(t, err)
			require.NoError(t, f.SetCellValue(sheet, cell, v))
		}
	}
	buf, err := f.WriteToBuffer()
	require.NoError(t, err)
	return buf.Bytes()
}

var importHeader = []string{
	"Request type", "Title", "Description", "Customer name", "Customer code",
	"Urgency", "Needed by (YYYY-MM-DD)", "Product description", "Shade code",
	"Shade name", "Tube (Paper/Plastic)", "Reference product", "Target volume",
	"Target price range",
}

func newImportHandler(reqTypeRepo *fakeImportRequestTypeRepo, pmRepo *fakeImportProductMasterRepo, reqRepo *fakeImportRequestRepo) *app.ImportHandler {
	createHandler := app.NewCreateHandler(reqRepo)
	return app.NewImportHandler(reqTypeRepo, pmRepo, createHandler)
}

func TestImportHandler_Handle(t *testing.T) {
	t.Run("resolves valid request type and reference product codes and creates a draft", func(t *testing.T) {
		reqTypeRepo := &fakeImportRequestTypeRepo{idsByCode: map[string]int32{"STANDARD": 1}}
		pmRepo := &fakeImportProductMasterRepo{byCode: map[string]*pmDomain.CostProductMaster{
			"FG-001": pmDomain.Reconstruct(101, "FG-001", 1, "Product One", "SH-001", "AX", "", "", "", "", nil, "", true, timeNow(), "seed", timeNow(), "seed", "", "", "", "", "", false),
		}}
		reqRepo := &fakeImportRequestRepo{}
		h := newImportHandler(reqTypeRepo, pmRepo, reqRepo)

		content := buildXlsxRows(t, [][]string{
			importHeader,
			{"STANDARD", "Row One", "desc", "Acme Corp", "ACME001", "medium", "2026-08-01", "50mm PET film", "SH-001", "Sky Blue", "PAPER", "FG-001", "10000", "1.20-1.35"},
		})

		result, err := h.Handle(context.Background(), app.ImportCommand{FileContent: content, FileName: "import.xlsx", CreatedBy: "user-1"})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, int32(1), result.SuccessCount)
		assert.Equal(t, int32(0), result.FailedCount)
		assert.Empty(t, result.Errors)
		require.Len(t, reqRepo.created, 1)
		assert.Equal(t, int32(1), reqRepo.created[0].RequestTypeID())
		assert.Equal(t, int64(101), reqRepo.created[0].ReferenceProductSysID())
		assert.Equal(t, domain.StatusDraft, reqRepo.created[0].Status())
	})

	t.Run("rejects a row with an unresolvable request type code but keeps processing other rows", func(t *testing.T) {
		reqTypeRepo := &fakeImportRequestTypeRepo{idsByCode: map[string]int32{"STANDARD": 1}}
		pmRepo := &fakeImportProductMasterRepo{}
		reqRepo := &fakeImportRequestRepo{}
		h := newImportHandler(reqTypeRepo, pmRepo, reqRepo)

		content := buildXlsxRows(t, [][]string{
			importHeader,
			{"BOGUS", "Bad Row", "desc", "Acme Corp", "ACME001", "medium", "", "50mm PET film", "SH-001", "", "", "", "", ""},
			{"STANDARD", "Good Row", "desc", "Acme Corp", "ACME001", "medium", "", "50mm PET film", "SH-002", "", "", "", "", ""},
		})

		result, err := h.Handle(context.Background(), app.ImportCommand{FileContent: content, FileName: "import.xlsx", CreatedBy: "user-1"})
		require.NoError(t, err)
		assert.Equal(t, int32(1), result.SuccessCount)
		assert.Equal(t, int32(1), result.FailedCount)
		require.Len(t, result.Errors, 1)
		assert.Equal(t, int32(2), result.Errors[0].RowNumber) // header is row 1
		assert.Equal(t, "request_type", result.Errors[0].Field)
		require.Len(t, reqRepo.created, 1)
		assert.Equal(t, "Good Row", reqRepo.created[0].Title())
	})

	t.Run("rejects a row with an unresolvable reference product code but keeps processing other rows", func(t *testing.T) {
		reqTypeRepo := &fakeImportRequestTypeRepo{idsByCode: map[string]int32{"STANDARD": 1}}
		pmRepo := &fakeImportProductMasterRepo{}
		reqRepo := &fakeImportRequestRepo{}
		h := newImportHandler(reqTypeRepo, pmRepo, reqRepo)

		content := buildXlsxRows(t, [][]string{
			importHeader,
			{"STANDARD", "Bad Reference Row", "desc", "Acme Corp", "ACME001", "medium", "", "50mm PET film", "SH-001", "", "", "NOPE-999", "", ""},
			{"STANDARD", "Good Row", "desc", "Acme Corp", "ACME001", "medium", "", "50mm PET film", "SH-002", "", "", "", "", ""},
		})

		result, err := h.Handle(context.Background(), app.ImportCommand{FileContent: content, FileName: "import.xlsx", CreatedBy: "user-1"})
		require.NoError(t, err)
		assert.Equal(t, int32(1), result.SuccessCount)
		assert.Equal(t, int32(1), result.FailedCount)
		require.Len(t, result.Errors, 1)
		assert.Equal(t, "reference_product", result.Errors[0].Field)
		require.Len(t, reqRepo.created, 1)
		assert.Equal(t, "Good Row", reqRepo.created[0].Title())
	})

	t.Run("rejects a row with a bad tube type but keeps processing other rows", func(t *testing.T) {
		reqTypeRepo := &fakeImportRequestTypeRepo{idsByCode: map[string]int32{"STANDARD": 1}}
		pmRepo := &fakeImportProductMasterRepo{}
		reqRepo := &fakeImportRequestRepo{}
		h := newImportHandler(reqTypeRepo, pmRepo, reqRepo)

		content := buildXlsxRows(t, [][]string{
			importHeader,
			{"STANDARD", "Bad Tube Row", "desc", "Acme Corp", "ACME001", "medium", "", "50mm PET film", "SH-001", "", "METAL", "", "", ""},
			{"STANDARD", "Good Row", "desc", "Acme Corp", "ACME001", "medium", "", "50mm PET film", "SH-002", "", "PAPER", "", "", ""},
		})

		result, err := h.Handle(context.Background(), app.ImportCommand{FileContent: content, FileName: "import.xlsx", CreatedBy: "user-1"})
		require.NoError(t, err)
		assert.Equal(t, int32(1), result.SuccessCount)
		assert.Equal(t, int32(1), result.FailedCount)
		require.Len(t, result.Errors, 1)
		assert.Equal(t, "spec", result.Errors[0].Field)
		require.Len(t, reqRepo.created, 1)
		assert.Equal(t, "Good Row", reqRepo.created[0].Title())
	})

	t.Run("never silently drops a row: every row is accounted for in success+failed counts", func(t *testing.T) {
		reqTypeRepo := &fakeImportRequestTypeRepo{idsByCode: map[string]int32{"STANDARD": 1}}
		pmRepo := &fakeImportProductMasterRepo{}
		reqRepo := &fakeImportRequestRepo{}
		h := newImportHandler(reqTypeRepo, pmRepo, reqRepo)

		content := buildXlsxRows(t, [][]string{
			importHeader,
			{"STANDARD", "Row A", "desc", "Acme Corp", "ACME001", "medium", "", "50mm PET film", "SH-001", "", "", "", "", ""},
			{"BOGUS", "Row B", "desc", "Acme Corp", "ACME001", "medium", "", "50mm PET film", "SH-002", "", "", "", "", ""},
			{"STANDARD", "Row C", "desc", "Acme Corp", "ACME001", "medium", "", "50mm PET film", "SH-003", "", "", "", "", ""},
		})

		result, err := h.Handle(context.Background(), app.ImportCommand{FileContent: content, FileName: "import.xlsx", CreatedBy: "user-1"})
		require.NoError(t, err)
		assert.EqualValues(t, 3, result.SuccessCount+result.FailedCount)
		assert.Equal(t, int32(2), result.SuccessCount)
		assert.Equal(t, int32(1), result.FailedCount)
	})

	t.Run("every row creates a new draft even if it duplicates an existing request (create-only policy)", func(t *testing.T) {
		reqTypeRepo := &fakeImportRequestTypeRepo{idsByCode: map[string]int32{"STANDARD": 1}}
		pmRepo := &fakeImportProductMasterRepo{}
		reqRepo := &fakeImportRequestRepo{}
		h := newImportHandler(reqTypeRepo, pmRepo, reqRepo)

		row := []string{"STANDARD", "Duplicate Row", "desc", "Acme Corp", "ACME001", "medium", "", "50mm PET film", "SH-001", "", "", "", "", ""}
		content := buildXlsxRows(t, [][]string{importHeader, row, row})

		result, err := h.Handle(context.Background(), app.ImportCommand{FileContent: content, FileName: "import.xlsx", CreatedBy: "user-1"})
		require.NoError(t, err)
		assert.Equal(t, int32(2), result.SuccessCount)
		assert.Equal(t, int32(0), result.SkippedCount)
		assert.Equal(t, int32(0), result.UpdatedCount)
		assert.Len(t, reqRepo.created, 2)
	})

	t.Run("returns an empty result for a header-only file", func(t *testing.T) {
		reqTypeRepo := &fakeImportRequestTypeRepo{}
		pmRepo := &fakeImportProductMasterRepo{}
		reqRepo := &fakeImportRequestRepo{}
		h := newImportHandler(reqTypeRepo, pmRepo, reqRepo)

		content := buildXlsxRows(t, [][]string{importHeader})
		result, err := h.Handle(context.Background(), app.ImportCommand{FileContent: content, FileName: "import.xlsx"})
		require.NoError(t, err)
		assert.Equal(t, int32(0), result.SuccessCount)
		assert.Equal(t, int32(0), result.FailedCount)
		assert.Empty(t, result.Errors)
	})

	t.Run("rejects a row that fails domain-level create validation (e.g. blank title) as a create error", func(t *testing.T) {
		reqTypeRepo := &fakeImportRequestTypeRepo{idsByCode: map[string]int32{"STANDARD": 1}}
		pmRepo := &fakeImportProductMasterRepo{}
		reqRepo := &fakeImportRequestRepo{}
		h := newImportHandler(reqTypeRepo, pmRepo, reqRepo)

		content := buildXlsxRows(t, [][]string{
			importHeader,
			{"STANDARD", "", "desc", "Acme Corp", "ACME001", "medium", "", "50mm PET film", "SH-001", "", "", "", "", ""},
			{"STANDARD", "Good Row", "desc", "Acme Corp", "ACME001", "medium", "", "50mm PET film", "SH-002", "", "", "", "", ""},
		})

		result, err := h.Handle(context.Background(), app.ImportCommand{FileContent: content, FileName: "import.xlsx", CreatedBy: "user-1"})
		require.NoError(t, err)
		assert.Equal(t, int32(1), result.SuccessCount)
		assert.Equal(t, int32(1), result.FailedCount)
		require.Len(t, result.Errors, 1)
		assert.Equal(t, "create", result.Errors[0].Field)
	})

	t.Run("returns an error for an unsupported file extension", func(t *testing.T) {
		reqTypeRepo := &fakeImportRequestTypeRepo{}
		pmRepo := &fakeImportProductMasterRepo{}
		reqRepo := &fakeImportRequestRepo{}
		h := newImportHandler(reqTypeRepo, pmRepo, reqRepo)

		_, err := h.Handle(context.Background(), app.ImportCommand{FileContent: []byte("not excel"), FileName: "import.csv"})
		require.Error(t, err)
	})
}
