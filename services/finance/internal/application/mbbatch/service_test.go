package mbbatch

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// --- fake ports ------------------------------------------------------------

type fakeHeadReader struct{ heads []MBHeadCandidate }

func (f *fakeHeadReader) ListValidated(context.Context) ([]MBHeadCandidate, error) {
	return f.heads, nil
}

type fakeEdgeReader struct{ edges []MBEdge }

func (f *fakeEdgeReader) ListMBEdgesBulk(context.Context, []string, []int32) ([]MBEdge, error) {
	return f.edges, nil
}

type fakeResultWriter struct{ got []*costcalcdom.Result }

func (f *fakeResultWriter) UpsertWithSupersedeTx(_ context.Context, _ *sql.Tx, r *costcalcdom.Result) (int64, int, float64, int64, error) {
	f.got = append(f.got, r)
	return int64(len(f.got)), 0, 0, 0, nil
}

// fakeLoader serves one parent MB whose route consumes one nested child MB.
// upstream is the child's per-unit cost keyed by product sys id — exactly the map
// LoadUpstreamCosts produces after the ENG-MB-02 fix.
type fakeLoader struct {
	parentID, childID int64
	ratio             float64
	upstream          map[int64]float64
}

func (f *fakeLoader) LoadProducts(context.Context, []int64) (map[int64]*costproductmaster.CostProductMaster, error) {
	return map[int64]*costproductmaster.CostProductMaster{}, nil
}

func (f *fakeLoader) LoadRoutesByProducts(_ context.Context, ids []int64) (map[int64]*costroute.Graph, error) {
	out := map[int64]*costroute.Graph{}
	for _, id := range ids {
		rm := &costroute.Rm{
			RmID: 1, SeqID: 1,
			RmType:         costroute.RmTypeProduct,
			RmProductSysID: f.childID,
			RouteRmRatio:   f.ratio,
		}
		out[id] = &costroute.Graph{
			Head: &costroute.Head{HeadID: 900, ProductSysID: id, RoutingStatus: costroute.StatusComplete},
			Seqs: []*costroute.Seq{
				{SeqID: 1, HeadID: 900, ProductSysID: id, RouteLevel: 1, RouteSeq: 1, Rms: []*costroute.Rm{rm}},
			},
		}
	}
	return out, nil
}

func (f *fakeLoader) LoadCAPP(_ context.Context, ids []int64) (map[int64]map[string]float64, error) {
	out := map[int64]map[string]float64{}
	for _, id := range ids {
		out[id] = map[string]float64{}
	}
	return out, nil
}

func (f *fakeLoader) LoadCAPPText(context.Context, []int64) (map[int64]map[string]string, error) {
	return map[int64]map[string]string{}, nil
}

func (f *fakeLoader) LoadFormulas(_ context.Context, ids []int64) (map[int64][]costcalc.Formula, error) {
	out := map[int64][]costcalc.Formula{}
	for _, id := range ids {
		out[id] = []costcalc.Formula{{
			FormulaCode:     "F_MB_FINAL",
			FormulaName:     "MB final cost",
			Expression:      "COST_RM_TOTAL",
			ResultParamCode: costcalc.ScopeKeyFinalCost,
			InputParamCodes: []string{costcalc.ScopeKeyCostRMTotal},
		}}
	}
	return out, nil
}

func (f *fakeLoader) LoadRMCosts(context.Context, []string, string, string) (map[string]float64, error) {
	return map[string]float64{}, nil
}

func (f *fakeLoader) LoadUpstreamCosts(context.Context, []int64, string, string) (map[int64]float64, error) {
	return f.upstream, nil
}

func (f *fakeLoader) LoadSellingSnapshots(context.Context, []int64, string) (map[int64]map[string]float64, error) {
	return map[int64]map[string]float64{}, nil
}

func (f *fakeLoader) LoadMBCosts(context.Context, []string) (map[string]map[string]float64, error) {
	return map[string]map[string]float64{}, nil
}

func (f *fakeLoader) LoadSpinFixedCost(context.Context, string) (costcalc.SpinPool, error) {
	return costcalc.SpinPool{}, nil
}

// --- suite -----------------------------------------------------------------

func testDB(t *testing.T) *postgres.DB {
	t.Helper()
	envOr := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		envOr("TEST_DB_HOST", "localhost"), envOr("TEST_DB_PORT", "5434"),
		envOr("TEST_DB_USER", "finance"), envOr("TEST_DB_PASSWORD", "finance123"),
		envOr("TEST_DB_NAME", "finance_db"))
	raw, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if pingErr := raw.Ping(); pingErr == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	require.NoError(t, raw.Ping())
	t.Cleanup(func() { _ = raw.Close() })
	return &postgres.DB{DB: raw}
}

// newTestService wires a Service whose every port is a fake. Only the transaction,
// savepoint, and advisory lock touch the database; nothing is written or seeded.
func newTestService(t *testing.T, loader *fakeLoader) (*Service, *fakeResultWriter) {
	t.Helper()
	writer := &fakeResultWriter{}
	heads := &fakeHeadReader{heads: []MBHeadCandidate{{
		MBHID:          "11111111-1111-1111-1111-111111111111",
		Code:           "MB-PARENT",
		Name:           "parent mb",
		CostProductID:  loader.parentID,
		CurrentVersion: 1,
	}}}
	svc := NewService(testDB(t), heads, &fakeEdgeReader{}, writer, loader, evaluator.NewCache(), nil, nil)
	return svc, writer
}

// T5 — the full ENG-MB-02 scenario at the batch level. The parent MB's RM cost must be
// the nested child's per-unit cost times the composition ratio. Before the loader fix
// the upstream map carried 0 for every MB and this came out as 0 with no error.
func TestRunMBBatch_NestedMBContributesToParentRMCost(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	loader := &fakeLoader{
		parentID: 40895, childID: 40418, ratio: 0.25,
		upstream: map[int64]float64{40418: 12.451513},
	}
	svc, writer := newTestService(t, loader)

	result, err := svc.RunMBBatch(context.Background(), "999999", 4242)
	require.NoError(t, err)
	require.Empty(t, result.Errors)
	require.Len(t, writer.got, 3, "one row per calc type: ACTUAL, SELLING, FORECAST")

	for _, r := range writer.got {
		require.InDelta(t, 12.451513*0.25, r.TotalRMCost(), 1e-9,
			"parent RM cost must be the nested MB's per-unit cost times the ratio")
	}
}

// T6 — audit trail. persistResult used to hardcode 0 for the job id, so every MB row
// landed with cpc_job_id = 0 and could not be traced back to its cal_job.
func TestRunMBBatch_PersistsRealJobID(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	loader := &fakeLoader{
		parentID: 40895, childID: 40418, ratio: 1.0,
		upstream: map[int64]float64{40418: 5.0},
	}
	svc, writer := newTestService(t, loader)

	_, err := svc.RunMBBatch(context.Background(), "999999", 4242)
	require.NoError(t, err)
	require.NotEmpty(t, writer.got)
	for _, r := range writer.got {
		require.Equal(t, int64(4242), r.JobID(), "the real cal_job id must reach the persisted row")
	}
}

// The captive/delivery/vb1..vb5 columns stay zero for MB on purpose: they are yarn
// cost-sheet concepts, and after ENG-MB-02 nothing reads them for MB.
func TestNewMBResult_LeavesYarnOnlyCostSheetFieldsZero(t *testing.T) {
	r := newMBResult(1, "202607", costcalcdom.CalcTypeActual, 900, 77, &costcalc.ComputeOutput{
		CostPerUnit: 10, TotalRMCost: 8, TotalConversion: 2, TotalCost: 10,
	})
	require.Equal(t, int64(77), r.JobID())
	require.Zero(t, r.CaptiveCost())
	require.Zero(t, r.DeliveryCost())
	require.Zero(t, r.VB1DelCost())
}
