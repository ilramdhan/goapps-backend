package costcalc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// P0-T10b: persistResult writes the USD label for ACTUAL (label only; the
// numbers are unchanged) and keeps IDR for the other calc types.
func TestProcessChunk_CurrencyLabelByCalcType(t *testing.T) {
	t.Parallel()
	for calcType, want := range map[costcalcdom.CalculationType]string{
		costcalcdom.CalcTypeActual:   "USD",
		costcalcdom.CalcTypeForecast: "IDR",
		costcalcdom.CalcTypeSelling:  "IDR",
	} {
		prodRepo := newRecordingProductRepo()
		resRepo := &recordingResultRepo{}
		svc := NewService(nil, &nopChunkRepo{}, prodRepo, resRepo, nil, &mbGuardLoader{}, evaluator.NewCache(), nil, nil)
		in := mbGuardInput([]int64{5, 6})
		in.CalcType = calcType
		_, err := svc.ProcessChunk(context.Background(), in)
		require.NoError(t, err, calcType)
		require.Len(t, resRepo.currencies, 2, calcType)
		for _, c := range resRepo.currencies {
			require.Equal(t, want, c, calcType)
		}
	}
}
