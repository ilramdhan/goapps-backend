package lot_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/lot"
)

func TestNewMaster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		lotNo     string
		itemCode  string
		shadeCode string
		stdFull   float64
		stdUnfull float64
		createdBy string
		wantErr   error
	}{
		{"valid", "LOT-1", "ITEM-1", "SHADE-1", 10.5, 8.25, "tester", nil},
		{"empty lot no", "", "ITEM-1", "SHADE-1", 10, 8, "tester", lot.ErrEmptyLotNo},
		{"lot no too long", strings.Repeat("x", 31), "ITEM-1", "SHADE-1", 10, 8, "tester", lot.ErrLotNoTooLong},
		{"empty item code", "LOT-1", "", "SHADE-1", 10, 8, "tester", lot.ErrEmptyItemCode},
		{"empty shade code", "LOT-1", "ITEM-1", "", 10, 8, "tester", lot.ErrEmptyShadeCode},
		{"zero full weight", "LOT-1", "ITEM-1", "SHADE-1", 0, 8, "tester", lot.ErrInvalidWeight},
		{"negative unfull weight", "LOT-1", "ITEM-1", "SHADE-1", 10, -1, "tester", lot.ErrInvalidWeight},
		{"empty created_by", "LOT-1", "ITEM-1", "SHADE-1", 10, 8, "", lot.ErrEmptyCreatedBy},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entity, err := lot.NewMaster(tt.lotNo, tt.itemCode, tt.shadeCode, tt.stdFull, tt.stdUnfull, "", tt.createdBy)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, entity)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, entity)
			assert.Equal(t, tt.lotNo, entity.LotNo())
			assert.InDelta(t, tt.stdFull, entity.StdWeightFull(), 0.0001)
		})
	}
}

func TestMaster_Update_RejectsZeroWeight(t *testing.T) {
	t.Parallel()
	entity, err := lot.NewMaster("LOT-1", "ITEM-1", "SHADE-1", 10, 8, "", "tester")
	require.NoError(t, err)

	zero := 0.0
	assert.ErrorIs(t, entity.Update(nil, nil, &zero, nil, nil, "editor"), lot.ErrInvalidWeight)
}

func TestNewMaster_DefaultsToPPCSource(t *testing.T) {
	t.Parallel()

	entity, err := lot.NewMaster("SPG0042-26", "ITEM-1", "SHADE-1", 10, 5, "", "tester")
	require.NoError(t, err)
	assert.Equal(t, lot.SourcePPC, entity.Source())
	assert.False(t, entity.IsSourced())
	assert.Empty(t, entity.SourceKey())
	assert.Nil(t, entity.SyncedAt())
}

func TestMaster_WithProvenance(t *testing.T) {
	t.Parallel()

	syncedAt := time.Date(2026, 7, 28, 3, 0, 0, 0, time.UTC)
	filament := int32(36)
	entity := lot.Reconstruct(
		"1015211", "31111015211", "", 0, 0, "",
		time.Now(), "sync", nil, nil,
	).WithProvenance(lot.SourceMMSMERGE, "1015211", &syncedAt, lot.Spec{
		ProdType: "PTY",
		Denier:   "55",
		Filament: &filament,
	})

	assert.Equal(t, lot.SourceMMSMERGE, entity.Source())
	assert.True(t, entity.IsSourced())
	assert.Equal(t, "1015211", entity.SourceKey())
	require.NotNil(t, entity.SyncedAt())
	assert.Equal(t, syncedAt, *entity.SyncedAt())
	assert.Equal(t, "PTY", entity.Spec().ProdType)
	require.NotNil(t, entity.Spec().Filament)
	assert.Equal(t, int32(36), *entity.Spec().Filament)
}

// An empty source must fall back to PPC so the chk_lot_master_source constraint
// cannot be violated by a row written before 000037 backfilled the default.
func TestMaster_WithProvenance_EmptySourceDefaultsToPPC(t *testing.T) {
	t.Parallel()

	entity := lot.Reconstruct(
		"SPG0042-26", "ITEM-1", "SHADE-1", 10, 5, "",
		time.Now(), "tester", nil, nil,
	).WithProvenance("", "", nil, lot.Spec{})

	assert.Equal(t, lot.SourcePPC, entity.Source())
	assert.False(t, entity.IsSourced())
}

func TestMaster_UpdateSpec_StampsAudit(t *testing.T) {
	t.Parallel()

	entity, err := lot.NewMaster("SPG0042-26", "ITEM-1", "SHADE-1", 10, 5, "", "tester")
	require.NoError(t, err)
	require.Nil(t, entity.UpdatedAt())

	entity.UpdateSpec(lot.Spec{ProdType: "POY", QCGrade: "AX"}, "operator")

	assert.Equal(t, "POY", entity.Spec().ProdType)
	assert.Equal(t, "AX", entity.Spec().QCGrade)
	require.NotNil(t, entity.UpdatedAt())
	require.NotNil(t, entity.UpdatedBy())
	assert.Equal(t, "operator", *entity.UpdatedBy())
}
