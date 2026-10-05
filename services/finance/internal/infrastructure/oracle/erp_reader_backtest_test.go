package oracle_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/fakeoracle"
)

func TestLegacyAdjRateReader_Read(t *testing.T) {
	q := fakeoracle.New()
	q.Register(adjHeadTable, fakeoracle.Dataset{
		Columns: []string{"ITEM", "GRADE", "SHADE", "ITEMS", "VARIANTS", "MAX_RATE"},
		Rows: [][]string{
			{"POY001 ", "A", "S1", "3", "2", "1.2"},
			{"POY009", "B", "S2", "1", "0", ""},
		},
	})
	rec := &recorder{Querier: q}
	got, err := oracle.NewLegacyAdjRateReader(rec).ReadLegacyAdjRates(context.Background(), "202604")
	if err != nil {
		t.Fatal(err)
	}
	assertReadOnly(t, q)
	if len(rec.args) != 1 || len(rec.args[0]) != 2 || rec.args[0][0] != "202604" || rec.args[0][1] != "202604" {
		t.Fatalf("binds = %v", rec.args)
	}
	if len(got) != 2 || got[0].Key.ItemCode != "POY001" || got[0].Items != 3 || got[0].RateVariants != 2 ||
		got[0].MaxRate.Decimal.String() != "1.2" || got[1].MaxRate.Valid {
		t.Fatalf("rows = %+v", got)
	}
}

func TestLegacyAdjRateReader_Errors(t *testing.T) {
	ctx := context.Background()
	if _, err := oracle.NewLegacyAdjRateReader(nil).ReadLegacyAdjRates(ctx, "202604"); !errors.Is(err, oracle.ErrNoReadConnection) {
		t.Fatalf("nil querier: %v", err)
	}
	if _, err := oracle.NewLegacyAdjRateReader(fakeoracle.New()).ReadLegacyAdjRates(ctx, "2026-04"); err == nil {
		t.Fatal("bad period accepted")
	}
}
