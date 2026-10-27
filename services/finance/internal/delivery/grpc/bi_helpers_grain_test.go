package grpc

import (
	"testing"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
)

func TestPeriodeGrainRoundTrip(t *testing.T) {
	for _, g := range []financev1.PeriodeGrain{
		financev1.PeriodeGrain_PERIODE_GRAIN_DAILY,
		financev1.PeriodeGrain_PERIODE_GRAIN_WEEKLY,
		financev1.PeriodeGrain_PERIODE_GRAIN_MONTHLY,
		financev1.PeriodeGrain_PERIODE_GRAIN_QUARTERLY,
		financev1.PeriodeGrain_PERIODE_GRAIN_YEARLY,
	} {
		s := periodeGrainToString(g)
		if s == "" || stringToPeriodeGrain(s) != g {
			t.Fatalf("grain %v round trip failed via %q", g, s)
		}
	}
	if periodeGrainToString(financev1.PeriodeGrain_PERIODE_GRAIN_WEEKLY) != "WEEKLY" {
		t.Fatal("WEEKLY string mismatch")
	}
}
