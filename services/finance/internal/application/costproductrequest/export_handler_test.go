package costproductrequest_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductrequest"
	pmDomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductrequest"
)

// fakeExportRequestRepo is a configurable fake for domain.Repository, used
// only by ExportHandler tests (only ListAll is exercised).
type fakeExportRequestRepo struct {
	requests []*domain.Request
	err      error
}

func (r *fakeExportRequestRepo) Create(_ context.Context, _ *domain.Request) error { return nil }
func (r *fakeExportRequestRepo) GetByID(_ context.Context, _ int64) (*domain.Request, error) {
	return nil, nil
}
func (r *fakeExportRequestRepo) GetByNo(_ context.Context, _ string) (*domain.Request, error) {
	return nil, nil
}
func (r *fakeExportRequestRepo) Save(_ context.Context, _ *domain.Request) error { return nil }
func (r *fakeExportRequestRepo) List(_ context.Context, _ domain.Filter) ([]*domain.Request, int64, error) {
	return nil, 0, nil
}
func (r *fakeExportRequestRepo) ListAll(_ context.Context, _ domain.Filter) ([]*domain.Request, error) {
	return r.requests, r.err
}

// fakeExportProductMasterRepo is a configurable fake for pmDomain.Repository,
// used only by ExportHandler tests (only GetBySysID is exercised).
type fakeExportProductMasterRepo struct {
	bySysID map[int64]*pmDomain.CostProductMaster
}

func (r *fakeExportProductMasterRepo) Create(_ context.Context, _ *pmDomain.CostProductMaster) error {
	return nil
}
func (r *fakeExportProductMasterRepo) GetBySysID(_ context.Context, sysID int64) (*pmDomain.CostProductMaster, error) {
	if p, ok := r.bySysID[sysID]; ok {
		return p, nil
	}
	return nil, pmDomain.ErrNotFound
}
func (r *fakeExportProductMasterRepo) GetByCode(_ context.Context, _ string) (*pmDomain.CostProductMaster, error) {
	return nil, pmDomain.ErrNotFound
}
func (r *fakeExportProductMasterRepo) Update(_ context.Context, _ *pmDomain.CostProductMaster) error {
	return nil
}
func (r *fakeExportProductMasterRepo) List(_ context.Context, _ pmDomain.Filter) ([]*pmDomain.CostProductMaster, int64, error) {
	return nil, 0, nil
}
func (r *fakeExportProductMasterRepo) BulkCreate(_ context.Context, _ []*pmDomain.CostProductMaster, _ string) (map[string]int64, error) {
	return nil, nil
}
func (r *fakeExportProductMasterRepo) ListAll(_ context.Context, _ pmDomain.Filter) ([]*pmDomain.CostProductMaster, error) {
	return nil, nil
}
func (r *fakeExportProductMasterRepo) BulkUpsertByLegacyID(_ context.Context, _ []pmDomain.ProductUpsertInput, _ string) ([]pmDomain.ProductUpsertResult, error) {
	return nil, nil
}
func (r *fakeExportProductMasterRepo) ListAllLegacyIDs(_ context.Context) (map[string]int64, error) {
	return nil, nil
}
func (r *fakeExportProductMasterRepo) RollbackImport(_ context.Context, _ []int64) error { return nil }

func (r *fakeExportProductMasterRepo) UnlockWithLog(_ context.Context, _ pmDomain.LockLogInput) error {
	return nil
}

func (r *fakeExportProductMasterRepo) DuplicateProduct(_ context.Context, _ pmDomain.DuplicateInput) (pmDomain.DuplicateOutput, error) {
	return pmDomain.DuplicateOutput{}, nil
}

func newTestRequest(t *testing.T, referenceProductSysID int64) *domain.Request {
	t.Helper()
	req, err := domain.New(domain.NewInput{
		RequestTypeID:         1,
		Title:                 "Test Request",
		CustomerName:          "Acme Corp",
		CustomerCode:          "ACME001",
		ProductClassification: domain.ClassNew,
		UrgencyLevel:          domain.UrgencyMedium,
		RequesterUserID:       "user-1",
		Spec: &domain.SpecInput{
			ProductDescription: "50mm x 100m PET film",
			ShadeCode:          "SH-001",
			ShadeName:          "Sky Blue",
			TubeType:           domain.TubeTypePaper,
		},
		ReferenceProductSysID: referenceProductSysID,
	})
	require.NoError(t, err)
	return req
}

func readExcelHeaderRow(t *testing.T, content []byte) []string {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(content))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	sheets := f.GetSheetList()
	require.NotEmpty(t, sheets)
	rows, err := f.GetRows(sheets[0])
	require.NoError(t, err)
	require.NotEmpty(t, rows)
	return rows[0]
}

func TestExportHandler_Handle(t *testing.T) {
	t.Run("produces valid parseable xlsx bytes with the correct D6 headers", func(t *testing.T) {
		repo := &fakeExportRequestRepo{requests: []*domain.Request{newTestRequest(t, 0)}}
		pmRepo := &fakeExportProductMasterRepo{}
		h := app.NewExportHandler(repo, pmRepo)

		result, err := h.Handle(context.Background(), app.ExportQuery{})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.NotEmpty(t, result.FileContent)
		assert.Equal(t, "cost_product_requests_export.xlsx", result.FileName)

		header := readExcelHeaderRow(t, result.FileContent)
		assert.Equal(t, []string{
			"Request type", "Title", "Description", "Customer name", "Customer code",
			"Urgency", "Needed by (YYYY-MM-DD)", "Product description", "Shade code",
			"Shade name", "Tube (Paper/Plastic)", "Reference product", "Target volume",
			"Target price range",
		}, header)
	})

	t.Run("resolves reference product sys ID to its product code", func(t *testing.T) {
		req := newTestRequest(t, 42)
		repo := &fakeExportRequestRepo{requests: []*domain.Request{req}}
		pmRepo := &fakeExportProductMasterRepo{
			bySysID: map[int64]*pmDomain.CostProductMaster{
				42: pmDomain.Reconstruct(42, "FG-042", 1, "Product Forty Two", "SH-001", "AX", "", "", "", "", nil, "", true, req.CreatedAt(), "user-1", req.CreatedAt(), "user-1", "", "", "", "", "", false),
			},
		}
		h := app.NewExportHandler(repo, pmRepo)

		result, err := h.Handle(context.Background(), app.ExportQuery{})
		require.NoError(t, err)

		f, err := excelize.OpenReader(bytes.NewReader(result.FileContent))
		require.NoError(t, err)
		defer func() { _ = f.Close() }()
		rows, err := f.GetRows(f.GetSheetList()[0])
		require.NoError(t, err)
		require.Len(t, rows, 2)
		assert.Equal(t, "FG-042", rows[1][11]) // "Reference product" column (0-indexed 11)
	})

	t.Run("leaves reference product column blank when unset", func(t *testing.T) {
		repo := &fakeExportRequestRepo{requests: []*domain.Request{newTestRequest(t, 0)}}
		pmRepo := &fakeExportProductMasterRepo{}
		h := app.NewExportHandler(repo, pmRepo)

		result, err := h.Handle(context.Background(), app.ExportQuery{})
		require.NoError(t, err)

		f, err := excelize.OpenReader(bytes.NewReader(result.FileContent))
		require.NoError(t, err)
		defer func() { _ = f.Close() }()
		rows, err := f.GetRows(f.GetSheetList()[0])
		require.NoError(t, err)
		require.Len(t, rows, 2)
		// excelize.GetRows trims trailing empty cells, so "Reference product"
		// (col 11) being blank means the row may be shorter than 12 cells.
		if len(rows[1]) > 11 {
			assert.Empty(t, rows[1][11])
		}
	})

	t.Run("returns error when repo fails", func(t *testing.T) {
		repo := &fakeExportRequestRepo{err: errors.New("db unavailable")}
		pmRepo := &fakeExportProductMasterRepo{}
		h := app.NewExportHandler(repo, pmRepo)

		result, err := h.Handle(context.Background(), app.ExportQuery{})
		require.Error(t, err)
		assert.Nil(t, result)
	})
}
