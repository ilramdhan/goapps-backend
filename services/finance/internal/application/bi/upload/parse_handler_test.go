package upload

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	uploaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/upload"
)

// fakeRepo is an in-memory upload.Repository for parse-handler tests.
type fakeRepo struct {
	session    *uploaddomain.Upload
	staging    []uploaddomain.StagingRow
	overwrites int
}

func (f *fakeRepo) CreateSession(_ context.Context, u *uploaddomain.Upload) error {
	f.session = u
	return nil
}

func (f *fakeRepo) InsertStaging(_ context.Context, _ uuid.UUID, rows []uploaddomain.StagingRow) error {
	f.staging = rows
	return nil
}

func (f *fakeRepo) GetSession(_ context.Context, _ uuid.UUID) (*uploaddomain.Upload, error) {
	return f.session, nil
}

func (f *fakeRepo) UpdateSession(_ context.Context, u *uploaddomain.Upload) error {
	f.session = u
	return nil
}

func (f *fakeRepo) ListSessions(_ context.Context, _, _ int) ([]*uploaddomain.Upload, int, error) {
	return nil, 0, nil
}

func (f *fakeRepo) MarkOverwrites(_ context.Context, _ uuid.UUID) (int, error) {
	return f.overwrites, nil
}

func (f *fakeRepo) CommitToFact(_ context.Context, _ uuid.UUID) (int, error) {
	return len(f.staging), nil
}

func (f *fakeRepo) RefreshViews(_ context.Context) error { return nil }

func buildXLSX(t *testing.T, headers []string, rows [][]any) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	_, err := f.NewSheet(uploadSheetName)
	require.NoError(t, err)
	require.NoError(t, f.DeleteSheet("Sheet1"))
	for i, h := range headers {
		cell, cErr := excelize.CoordinatesToCellName(i+1, 1)
		require.NoError(t, cErr)
		require.NoError(t, f.SetCellValue(uploadSheetName, cell, h))
	}
	for r, row := range rows {
		for c, v := range row {
			cell, cErr := excelize.CoordinatesToCellName(c+1, r+2)
			require.NoError(t, cErr)
			require.NoError(t, f.SetCellValue(uploadSheetName, cell, v))
		}
	}
	buf, err := f.WriteToBuffer()
	require.NoError(t, err)
	return buf.Bytes()
}

func TestParseHandler_Handle_MixedValidity(t *testing.T) {
	content := buildXLSX(t, uploadHeaders, [][]any{
		{"MIS", "EBITDA", "INCOME", "", 1, 1, 1, "MONTHLY", "202604", 1000, "IDR", "ACTUAL"},
		{"MIS", "EBITDA", "MANPOWER", "", 1, 1, 1, "MONTHLY", "202604", 200, "IDR", "ACTUAL"},
		{"MIS", "", "", "", 1, 1, 1, "MONTHLY", "202604", 5, "IDR", "ACTUAL"}, // invalid: no group_1
	})

	repo := &fakeRepo{overwrites: 0}
	src := func(context.Context) (uuid.UUID, error) { return uuid.New(), nil }
	h := NewParseHandler(repo, src)

	result, err := h.Handle(context.Background(), ParseCommand{
		TargetType:  "MIS",
		FileName:    "upload.xlsx",
		FileContent: content,
		UploadedBy:  uuid.New(),
	})
	require.NoError(t, err)
	require.NotNil(t, result.Upload)
	assert.Equal(t, 3, result.Upload.TotalRows())
	assert.Equal(t, 2, result.Upload.ValidRows())
	assert.Equal(t, 1, result.Upload.InvalidRows())
	assert.Equal(t, uploaddomain.StatusValidated, result.Upload.Status())
	assert.NotEmpty(t, result.Errors)
	// Income row flips negative; manpower (cost) flips negative.
	assert.InDelta(t, -1000.0, repo.staging[0].DisplayValue, 1e-9)
	assert.InDelta(t, -200.0, repo.staging[1].DisplayValue, 1e-9)
}

func TestParseHandler_Handle_DuplicateKey(t *testing.T) {
	content := buildXLSX(t, uploadHeaders, [][]any{
		{"MIS", "EBITDA", "INCOME", "", 1, 1, 1, "MONTHLY", "202604", 1000, "IDR", "ACTUAL"},
		{"MIS", "EBITDA", "INCOME", "", 1, 1, 1, "MONTHLY", "202604", 2000, "IDR", "ACTUAL"},
	})
	repo := &fakeRepo{}
	h := NewParseHandler(repo, func(context.Context) (uuid.UUID, error) { return uuid.New(), nil })
	result, err := h.Handle(context.Background(), ParseCommand{
		TargetType: "MIS", FileName: "dup.xlsx", FileContent: content, UploadedBy: uuid.New(),
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Upload.ValidRows())
	assert.Equal(t, 1, result.Upload.InvalidRows())
}

func TestParseHandler_Handle_EmptyTargetType(t *testing.T) {
	h := NewParseHandler(&fakeRepo{}, func(context.Context) (uuid.UUID, error) { return uuid.New(), nil })
	_, err := h.Handle(context.Background(), ParseCommand{FileContent: []byte("x"), FileName: "x.xlsx"})
	require.ErrorIs(t, err, uploaddomain.ErrInvalidTargetType)
}

func TestParseHandler_Handle_ReorderedColumns(t *testing.T) {
	// Headers in a different order; parser maps by name.
	headers := []string{"PERIODE", "VALUE", "TYPE", "GROUP_1", "PERIODE_GRAIN"}
	content := buildXLSX(t, headers, [][]any{
		{"202604", 1000, "MIS", "EBITDA", "MONTHLY"},
	})
	repo := &fakeRepo{}
	h := NewParseHandler(repo, func(context.Context) (uuid.UUID, error) { return uuid.New(), nil })
	result, err := h.Handle(context.Background(), ParseCommand{
		TargetType: "MIS", FileName: "ro.xlsx", FileContent: content, UploadedBy: uuid.New(),
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Upload.ValidRows())
}
