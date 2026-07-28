package lot_test

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	lotapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/lot"
	lotdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/lot"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/oracle"
)

// fakeLotRepo records batched merges and replays scripted per-lot outcomes.
type fakeLotRepo struct {
	lotdomain.Repository
	calls    []lotdomain.SourcedLot
	batches  int
	outcomes map[string]lotdomain.UpsertOutcome
	err      error
}

func (r *fakeLotRepo) UpsertSourcedBatch(_ context.Context, src []lotdomain.SourcedLot) (lotdomain.UpsertBatchResult, error) {
	r.batches++
	r.calls = append(r.calls, src...)
	if r.err != nil {
		return lotdomain.UpsertBatchResult{}, r.err
	}
	var res lotdomain.UpsertBatchResult
	for _, s := range src {
		// Unscripted lots default to inserted; OutcomeSkipped is the zero value,
		// so presence has to be checked rather than read straight off the map.
		outcome, scripted := r.outcomes[s.LotNo]
		if !scripted {
			outcome = lotdomain.OutcomeInserted
		}
		switch outcome {
		case lotdomain.OutcomeUpdated:
			res.Updated++
		case lotdomain.OutcomeSkipped:
			res.Skipped++
		case lotdomain.OutcomeInserted:
			res.Inserted++
		}
	}
	return res, nil
}

type fakeLotSource struct {
	rows []oracle.LotRow
	err  error
}

func (s *fakeLotSource) ListLots(_ context.Context) ([]oracle.LotRow, error) {
	return s.rows, s.err
}

func ptrFloat(v float64) *float64 { return &v }
func ptrInt32(v int32) *int32     { return &v }

// sampleRow mirrors MMSMERGE row 1 of the supplied fixture (MERGE_CODE 1015211).
func sampleRow() oracle.LotRow {
	return oracle.LotRow{
		Code:             "1015211",
		ItemCode:         "31111015211",
		QCGrade:          "AX",
		ProdType:         "PTY",
		YarnType:         "PTY",
		Denier:           "55",
		Filament:         ptrInt32(36),
		CrossSection:     "RND",
		Description:      "SD HCSH NI",
		ShadeCode:        "",
		ShadeColor:       "BLACK ORANGE",
		TareBoxWeight:    ptrFloat(1.290),
		TareBobbinWeight: ptrFloat(0.220),
		BobbinsPerBox:    ptrInt32(25),
		BobWeight:        ptrFloat(5.00),
		OrionItemCode:    "",
		MachineNo:        "12",
		Status:           "S",
		PakStatus:        "O",
	}
}

func TestSyncUsecase_MapsMMSMergeRow(t *testing.T) {
	t.Parallel()

	repo := &fakeLotRepo{}
	uc := lotapp.NewSyncUsecase(repo, &fakeLotSource{rows: []oracle.LotRow{sampleRow()}})

	res, err := uc.Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, res.Inserted)
	assert.True(t, res.OracleUsed)

	require.Len(t, repo.calls, 1)
	got := repo.calls[0]
	assert.Equal(t, "1015211", got.LotNo)
	assert.Equal(t, "1015211", got.SourceKey)
	assert.Equal(t, "31111015211", got.ItemCode)
	// SHADE_CODE is empty on the fixture; the sync must not invent one.
	assert.Empty(t, got.ShadeCode)
	require.NotNil(t, got.StdWeightFull)
	assert.InDelta(t, 5.00, *got.StdWeightFull, 0.0001)
	assert.Equal(t, "PTY", got.Spec.ProdType)
	assert.Equal(t, "RND", got.Spec.CrossSection)
	assert.Equal(t, "BLACK ORANGE", got.Spec.ShadeColor)
	require.NotNil(t, got.Spec.BobbinsPerBox)
	assert.Equal(t, int32(25), *got.Spec.BobbinsPerBox)
	assert.Equal(t, "O", got.Spec.SourcePakStatus)
	assert.False(t, got.SyncedAt.IsZero())
}

func TestSyncUsecase_SkipsRowsMissingKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		row  oracle.LotRow
	}{
		{"no lot number", oracle.LotRow{ItemCode: "31111015211"}},
		{"no item code", oracle.LotRow{Code: "1015211"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &fakeLotRepo{}
			uc := lotapp.NewSyncUsecase(repo, &fakeLotSource{rows: []oracle.LotRow{tt.row}})

			res, err := uc.Sync(context.Background())
			require.NoError(t, err)
			assert.Equal(t, 1, res.Skipped)
			assert.Empty(t, repo.calls)
		})
	}
}

func TestSyncUsecase_DegradesWhenOracleUnavailable(t *testing.T) {
	t.Parallel()

	t.Run("nil source", func(t *testing.T) {
		t.Parallel()
		repo := &fakeLotRepo{}
		res, err := lotapp.NewSyncUsecase(repo, nil).Sync(context.Background())
		require.NoError(t, err)
		assert.False(t, res.OracleUsed)
		assert.Empty(t, repo.calls)
	})

	t.Run("source error", func(t *testing.T) {
		t.Parallel()
		repo := &fakeLotRepo{}
		src := &fakeLotSource{err: errors.New("ORA-12541 no listener")}
		res, err := lotapp.NewSyncUsecase(repo, src).Sync(context.Background())
		require.NoError(t, err)
		assert.False(t, res.OracleUsed)
		assert.Empty(t, repo.calls)
	})
}

func TestSyncUsecase_PropagatesRepositoryError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("write failed")
	repo := &fakeLotRepo{err: wantErr}
	uc := lotapp.NewSyncUsecase(repo, &fakeLotSource{rows: []oracle.LotRow{sampleRow()}})

	_, err := uc.Sync(context.Background())
	require.ErrorIs(t, err, wantErr)
}

// The whole master must go out in one batched write: MMSMERGE is ~66k rows and
// a round-trip per row overran the 60s RPC deadline, leaving a partial import.
func TestSyncUsecase_WritesInASingleBatch(t *testing.T) {
	t.Parallel()

	rows := make([]oracle.LotRow, 0, 500)
	for i := range 500 {
		row := sampleRow()
		row.Code = "LOT" + strconv.Itoa(i)
		rows = append(rows, row)
	}

	repo := &fakeLotRepo{}
	res, err := lotapp.NewSyncUsecase(repo, &fakeLotSource{rows: rows}).Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, repo.batches)
	assert.Len(t, repo.calls, 500)
	assert.Equal(t, 500, res.Inserted)
}

// MMSMERGE is not unique on MERGE_CODE, and one ON CONFLICT statement cannot
// touch the same row twice, so a repeat must collapse rather than reach the repo.
func TestSyncUsecase_CollapsesDuplicateLotNumbers(t *testing.T) {
	t.Parallel()

	first := sampleRow()
	second := sampleRow()
	second.ItemCode = "99999999999"

	repo := &fakeLotRepo{}
	uc := lotapp.NewSyncUsecase(repo, &fakeLotSource{rows: []oracle.LotRow{first, second}})

	res, err := uc.Sync(context.Background())
	require.NoError(t, err)
	require.Len(t, repo.calls, 1)
	// Last row wins, matching the previous row-at-a-time behavior.
	assert.Equal(t, "99999999999", repo.calls[0].ItemCode)
	assert.Equal(t, 1, res.Inserted)
	assert.Equal(t, 1, res.Skipped)
}

func TestSyncUsecase_CountsOutcomes(t *testing.T) {
	t.Parallel()

	rowA := sampleRow()
	rowB := sampleRow()
	rowB.Code = "10052C1"
	rowB.ItemCode = "311110052C1"

	repo := &fakeLotRepo{outcomes: map[string]lotdomain.UpsertOutcome{
		"1015211": lotdomain.OutcomeUpdated,
		"10052C1": lotdomain.OutcomeInserted,
	}}
	uc := lotapp.NewSyncUsecase(repo, &fakeLotSource{rows: []oracle.LotRow{rowA, rowB}})

	res, err := uc.Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, res.Updated)
	assert.Equal(t, 1, res.Inserted)
	assert.Equal(t, 0, res.Skipped)
}
