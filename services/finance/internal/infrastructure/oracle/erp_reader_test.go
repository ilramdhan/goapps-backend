package oracle_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/fakeoracle"
)

const (
	fixtureItems  = "../../testutil/fakeoracle/testdata/erp/om_item.csv"
	fixtureGrades = "../../testutil/fakeoracle/testdata/erp/om_grade_code_1.csv"
)

func newFakeMaster(t *testing.T) *fakeoracle.Querier {
	t.Helper()
	q := fakeoracle.New()
	if err := q.LoadCSV("MGTDAT.OM_ITEM", fixtureItems); err != nil {
		t.Fatal(err)
	}
	if err := q.LoadCSV("MGTDAT.OM_GRADE_CODE_1", fixtureGrades); err != nil {
		t.Fatal(err)
	}
	return q
}

func TestErpMasterReader_ListItems(t *testing.T) {
	q := newFakeMaster(t)
	items, err := oracle.NewErpMasterReader(q).ListItems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items (blank code skipped), got %d", len(items))
	}
	if items[1].Name != "SYNTHETIC YARN TWO" || !items[1].Active {
		t.Errorf("trim/NULL flag handling wrong: %+v", items[1])
	}
	if items[2].Active {
		t.Errorf("frozen item must be inactive: %+v", items[2])
	}
	for _, s := range q.Queries() {
		if err := oracle.CheckReadOnly(s); err != nil {
			t.Errorf("reader sent non-read-only SQL %q: %v", s, err)
		}
	}
}

func TestErpMasterReader_ListGrades(t *testing.T) {
	q := newFakeMaster(t)
	grades, err := oracle.NewErpMasterReader(q).ListGrades(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(grades) != 3 || grades[0].Code != "ZA" || grades[1].Active || !grades[2].Active {
		t.Fatalf("unexpected grades: %+v", grades)
	}
}

func TestErpMasterReader_Errors(t *testing.T) {
	boom := errors.New("ORA-03113: end-of-file on communication channel")
	q := newFakeMaster(t)
	q.FailWith(boom)
	if _, err := oracle.NewErpMasterReader(q).ListItems(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("expected wrapped error, got %v", err)
	}
	if _, err := oracle.NewErpMasterReader(nil).ListGrades(context.Background()); !errors.Is(err, oracle.ErrNoReadConnection) {
		t.Fatalf("expected ErrNoReadConnection, got %v", err)
	}
	if _, err := fakeoracle.New().QueryRO(context.Background(), "DELETE FROM MGTDAT.OM_ITEM"); !errors.Is(err, oracle.ErrNonSelectRejected) {
		t.Fatalf("fake must apply the guard, got %v", err)
	}
	if _, err := fakeoracle.New().QueryRO(context.Background(), "SELECT 1 FROM DUAL"); !errors.Is(err, fakeoracle.ErrNoDataset) {
		t.Fatalf("expected ErrNoDataset, got %v", err)
	}
}
