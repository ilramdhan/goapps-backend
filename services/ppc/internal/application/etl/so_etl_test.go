package etl

import (
	"context"
	"testing"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/postgres"
)

type stubSoSource struct {
	rows []oracle.SoPendingRow
}

func (s *stubSoSource) ListSoPending(_ context.Context) ([]oracle.SoPendingRow, error) {
	return s.rows, nil
}

type stubSoRepo struct {
	got []postgres.SoStagingRow
}

func (r *stubSoRepo) ReplaceSalesOrderStaging(_ context.Context, rows []postgres.SoStagingRow) error {
	r.got = rows
	return nil
}

func TestSoETLNilSourceNoOp(t *testing.T) {
	repo := &stubSoRepo{}
	uc := NewSoStagingETL(nil, repo)
	res, err := uc.Run(context.Background())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.OracleUp || res.Replaced != 0 {
		t.Errorf("nil source should no-op, got OracleUp=%v Replaced=%d", res.OracleUp, res.Replaced)
	}
	if repo.got != nil {
		t.Errorf("repo should not be called on nil source")
	}
}

func TestSoETLMapsAndReplaces(t *testing.T) {
	deadline := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	src := &stubSoSource{rows: []oracle.SoPendingRow{
		{
			CustomerCode: "C001", CustomerName: "Acme", ContractNo: "K-1",
			ContractSysID: 777, ItemCode: "IT1", GradeCode: "AX", ShadeCode: "SH1",
			QtyOrdered: 100, QtyDelivered: 40, QtyRemaining: 60,
			Deadline: deadline, Rate: 1.5, Currency: "USD", OutstandingAR: 250.75,
		},
	}}
	repo := &stubSoRepo{}
	uc := NewSoStagingETL(src, repo)

	res, err := uc.Run(context.Background())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !res.OracleUp || res.Replaced != 1 {
		t.Fatalf("res = OracleUp %v Replaced %d, want true/1", res.OracleUp, res.Replaced)
	}
	if len(repo.got) != 1 {
		t.Fatalf("expected 1 staging row, got %d", len(repo.got))
	}
	row := repo.got[0]
	if row.ContractSysID != 777 || row.ItemCode != "IT1" || row.GradeCode != "AX" || row.ShadeCode != "SH1" {
		t.Errorf("natural key mapping wrong: %+v", row)
	}
	if row.QtyRemaining != 60 || row.OutstandingAR != 250.75 || !row.Deadline.Equal(deadline) {
		t.Errorf("numeric/date mapping wrong: %+v", row)
	}
}
