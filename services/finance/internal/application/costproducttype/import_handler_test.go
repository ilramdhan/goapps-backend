// Package costproducttype contains application use cases for CostProductType.
package costproducttype

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/xuri/excelize/v2"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
)

// mockRepository is a testify mock for domain.Repository.
type mockRepository struct {
	mock.Mock
}

func (m *mockRepository) Create(ctx context.Context, t *domain.CostProductType) error {
	args := m.Called(ctx, t)
	return args.Error(0)
}

func (m *mockRepository) GetByID(ctx context.Context, id int32) (*domain.CostProductType, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.CostProductType), args.Error(1)
}

func (m *mockRepository) GetByCode(ctx context.Context, code string) (*domain.CostProductType, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.CostProductType), args.Error(1)
}

func (m *mockRepository) Update(ctx context.Context, t *domain.CostProductType) error {
	args := m.Called(ctx, t)
	return args.Error(0)
}

func (m *mockRepository) List(ctx context.Context, f domain.Filter) ([]*domain.CostProductType, int64, error) {
	args := m.Called(ctx, f)
	if args.Get(0) == nil {
		return nil, int64(args.Int(1)), args.Error(2)
	}
	return args.Get(0).([]*domain.CostProductType), int64(args.Int(1)), args.Error(2)
}

func (m *mockRepository) ListAllActive(_ context.Context) ([]*domain.CostProductType, error) {
	return nil, nil
}

// buildTestXLSX creates an in-memory xlsx file with the given rows (first row = header).
func buildTestXLSX(t *testing.T, rows [][]string) []byte {
	t.Helper()

	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			t.Logf("close xlsx: %v", err)
		}
	}()

	sheetName := f.GetSheetName(f.GetActiveSheetIndex())
	for rowIdx, row := range rows {
		for colIdx, val := range row {
			cell, err := excelize.CoordinatesToCellName(colIdx+1, rowIdx+1)
			if err != nil {
				t.Fatalf("cell coordinates (%d,%d): %v", colIdx+1, rowIdx+1, err)
			}
			if err := f.SetCellValue(sheetName, cell, val); err != nil {
				t.Fatalf("set cell %s: %v", cell, err)
			}
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("write xlsx to buffer: %v", err)
	}
	return buf.Bytes()
}

func TestImportHandler_Handle(t *testing.T) {
	t.Parallel()

	headerRow := []string{"cpt_type_code", "cpt_type_name", "cpt_is_active"}

	tests := []struct {
		name            string
		rows            [][]string
		duplicateAction string
		setupMock       func(repo *mockRepository)
		wantSuccess     int32
		wantSkipped     int32
		wantUpdated     int32
		wantFailed      int32
		wantErrors      int
	}{
		{
			name: "new type imported successfully",
			rows: [][]string{
				headerRow,
				{"POY", "Partially Oriented Yarn", "true"},
			},
			duplicateAction: "skip",
			setupMock: func(repo *mockRepository) {
				repo.On("GetByCode", mock.Anything, "POY").Return(nil, domain.ErrNotFound)
				repo.On("Create", mock.Anything, mock.AnythingOfType("*costproducttype.CostProductType")).Return(nil)
			},
			wantSuccess: 1,
		},
		{
			name: "duplicate code with skip action",
			rows: [][]string{
				headerRow,
				{"POY", "Partially Oriented Yarn", "true"},
			},
			duplicateAction: "skip",
			setupMock: func(repo *mockRepository) {
				existing := domain.Reconstruct(1, "POY", "Old Name", true, testTime(), testTime())
				repo.On("GetByCode", mock.Anything, "POY").Return(existing, nil)
			},
			wantSkipped: 1,
		},
		{
			name: "duplicate code with update action",
			rows: [][]string{
				headerRow,
				{"POY", "Updated Yarn Name", "true"},
			},
			duplicateAction: "update",
			setupMock: func(repo *mockRepository) {
				existing := domain.Reconstruct(1, "POY", "Old Name", true, testTime(), testTime())
				repo.On("GetByCode", mock.Anything, "POY").Return(existing, nil)
				repo.On("Update", mock.Anything, mock.AnythingOfType("*costproducttype.CostProductType")).Return(nil)
			},
			wantUpdated: 1,
		},
		{
			name: "duplicate code with error action",
			rows: [][]string{
				headerRow,
				{"POY", "Partially Oriented Yarn", ""},
			},
			duplicateAction: "error",
			setupMock: func(repo *mockRepository) {
				existing := domain.Reconstruct(1, "POY", "Old Name", true, testTime(), testTime())
				repo.On("GetByCode", mock.Anything, "POY").Return(existing, nil)
			},
			wantFailed: 1,
			wantErrors: 1,
		},
		{
			name: "invalid type_code fails validation",
			rows: [][]string{
				headerRow,
				{"toolongcode123", "Some Name", "true"},
			},
			duplicateAction: "skip",
			setupMock: func(repo *mockRepository) {
				repo.On("GetByCode", mock.Anything, "toolongcode123").Return(nil, domain.ErrNotFound)
			},
			wantFailed: 1,
			wantErrors: 1,
		},
		{
			name: "empty type_code row fails",
			rows: [][]string{
				headerRow,
				{"", "Some Name", ""},
			},
			duplicateAction: "skip",
			setupMock:       func(_ *mockRepository) {},
			wantFailed:      1,
			wantErrors:      1,
		},
		{
			name: "empty type_name row fails",
			rows: [][]string{
				headerRow,
				{"POY", "", ""},
			},
			duplicateAction: "skip",
			setupMock:       func(_ *mockRepository) {},
			wantFailed:      1,
			wantErrors:      1,
		},
		{
			name: "no data rows returns empty result",
			rows: [][]string{
				headerRow,
			},
			duplicateAction: "skip",
			setupMock:       func(_ *mockRepository) {},
		},
		{
			name: "multiple rows mixed outcomes",
			rows: [][]string{
				headerRow,
				{"POY", "Partially Oriented Yarn", "true"},
				{"PTY", "Polyester Textured Yarn", "true"},
			},
			duplicateAction: "skip",
			setupMock: func(repo *mockRepository) {
				repo.On("GetByCode", mock.Anything, "POY").Return(nil, domain.ErrNotFound)
				repo.On("GetByCode", mock.Anything, "PTY").Return(nil, domain.ErrNotFound)
				repo.On("Create", mock.Anything, mock.AnythingOfType("*costproducttype.CostProductType")).Return(nil)
			},
			wantSuccess: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := &mockRepository{}
			tc.setupMock(repo)

			handler := NewImportHandler(repo)

			fileContent := buildTestXLSX(t, tc.rows)
			result, err := handler.Handle(context.Background(), ImportCommand{
				FileContent:     fileContent,
				FileName:        "test.xlsx",
				DuplicateAction: tc.duplicateAction,
			})

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tc.wantSuccess, result.SuccessCount)
			assert.Equal(t, tc.wantSkipped, result.SkippedCount)
			assert.Equal(t, tc.wantUpdated, result.UpdatedCount)
			assert.Equal(t, tc.wantFailed, result.FailedCount)
			assert.Len(t, result.Errors, tc.wantErrors)

			repo.AssertExpectations(t)
		})
	}
}

func TestImportHandler_UnsupportedFormat(t *testing.T) {
	t.Parallel()

	repo := &mockRepository{}
	handler := NewImportHandler(repo)

	_, err := handler.Handle(context.Background(), ImportCommand{
		FileContent:     []byte("not excel"),
		FileName:        "test.csv",
		DuplicateAction: "skip",
	})
	assert.ErrorContains(t, err, "unsupported file format")
}

func TestImportHandler_InvalidExcelContent(t *testing.T) {
	t.Parallel()

	repo := &mockRepository{}
	handler := NewImportHandler(repo)

	_, err := handler.Handle(context.Background(), ImportCommand{
		FileContent:     bytes.Repeat([]byte{0xFF}, 100),
		FileName:        "test.xlsx",
		DuplicateAction: "skip",
	})
	assert.Error(t, err)
}

func TestParseIsActive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw      string
		def      bool
		expected bool
	}{
		{"true", false, true},
		{"True", false, true},
		{"TRUE", false, true},
		{"yes", false, true},
		{"1", false, true},
		{"active", false, true},
		{"false", true, false},
		{"no", true, false},
		{"0", true, false},
		{"inactive", true, false},
		{"", true, true},
		{"", false, false},
		{"unknown", true, true},
	}

	for _, tc := range tests {
		t.Run(tc.raw+"_default_"+boolStr(tc.def), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, parseIsActive(tc.raw, tc.def))
		})
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func testTime() time.Time {
	return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
}
