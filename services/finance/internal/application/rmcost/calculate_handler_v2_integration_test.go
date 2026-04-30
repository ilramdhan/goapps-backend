//go:build integration

// Package rmcost — V2 end-to-end integration test against a real PostgreSQL DB.
// Run with: INTEGRATION_TEST=true go test -tags integration ./internal/application/rmcost/
package rmcost

import (
	"context"
	"database/sql"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	rmcostdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
	rmgroupdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// TestV2_EndToEnd_ExcelFixture wires the full V2 engine + repos against a real DB,
// mirrors the Excel reference workbook exactly, and asserts every key value.
//
// Setup:
//   - Insert one group head with marketing inputs matching Excel rows 5/20-21.
//   - Insert two details (CGC + PCI) with valuation inputs matching rows 26/27.
//   - Insert two source rows in cst_item_cons_stk_po matching rows 10/11.
//   - Run V2 calc.
//   - Verify cst_rm_cost (group totals + selected costs).
//   - Verify cst_rm_cost_detail (per-row intermediates).
//   - Edit fix_rate via EditFixRateHandler — expect FL update + cost_val cascade.
//   - Edit simulation_rate via EditInputsHandler — expect cost_sim recompute.
func TestV2_EndToEnd_ExcelFixture(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("set INTEGRATION_TEST=true to run")
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://finance:finance123@localhost:5434/finance_db?sslmode=disable"
	}
	rawDB, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	defer rawDB.Close()
	db := postgres.NewDBFromSQL(rawDB)

	ctx := context.Background()

	// --- Test data IDs (deterministic) ---
	headID := uuid.New()
	period := "202604"

	// Cleanup any prior run (in case of crash).
	cleanupTestRun(t, ctx, rawDB, headID, period)
	t.Cleanup(func() { cleanupTestRun(t, context.Background(), rawDB, headID, period) })

	// Insert source feed rows — 2 variants of DYE0000015.
	insertSourceFeed(t, ctx, rawDB, period, "DYE0000015", "CGC",
		1391.41, 100, 2566.1, 184.425, 3000, 150)
	insertSourceFeed(t, ctx, rawDB, period, "DYE0000015", "PCI",
		222, 10, 172.05, 7.75, 200, 9)

	// Build domain head with marketing inputs.
	groupRepo := postgres.NewRMGroupRepository(db)
	costRepo := postgres.NewRMCostRepository(db)
	costDetailRepo := postgres.NewRMCostDetailRepository(db)
	costInputsRepo := postgres.NewRMCostInputsRepository(db)
	syncRepo := postgres.NewSyncDataRepository(db)

	code, err := rmgroupdomain.NewCode("E2E V2 TEST")
	require.NoError(t, err)
	head, err := rmgroupdomain.NewHead(code, "E2E V2 Test Group", "", 5, 0.89, "tester")
	require.NoError(t, err)
	// Marketing inputs: P5=5, Q5=0.89, L5=15. N5/O5 left empty (nil).
	mDef := 15.0
	require.NoError(t, head.AttachMarketingInputs(rmgroupdomain.MarketingInputs{
		DefaultValue:  &mDef,
		ValuationFlag: rmgroupdomain.ValuationFlagSL, // J5='SL' in Excel
		MarketingFlag: rmgroupdomain.MarketingFlagSP, // U5='SP' in Excel
	}))
	// Override head ID for deterministic cleanup.
	overrideHeadID(t, head, headID)
	require.NoError(t, groupRepo.CreateHead(ctx, head))

	// Insert two details with valuation inputs.
	itemCode, err := rmgroupdomain.NewItemCode("DYE0000015")
	require.NoError(t, err)
	d1, err := rmgroupdomain.NewDetail(headID, itemCode, "tester")
	require.NoError(t, err)
	overrideDetailGrade(t, d1, "CGC")
	freight, anti, duty, transport := 0.06, 0.10, 0.04, 0.08125
	require.NoError(t, d1.AttachValuationInputs(rmgroupdomain.ValuationInputs{
		FreightRate:    &freight,
		AntiDumpingPct: &anti,
		DutyPct:        &duty,
		TransportRate:  &transport,
	}))
	require.NoError(t, groupRepo.AddDetail(ctx, d1))

	d2, err := rmgroupdomain.NewDetail(headID, itemCode, "tester")
	require.NoError(t, err)
	overrideDetailGrade(t, d2, "PCI")
	anti2, transport2 := 0.0, 0.0815
	require.NoError(t, d2.AttachValuationInputs(rmgroupdomain.ValuationInputs{
		FreightRate:    &freight,
		AntiDumpingPct: &anti2,
		DutyPct:        &duty,
		TransportRate:  &transport2,
	}))
	require.NoError(t, groupRepo.AddDetail(ctx, d2))

	// --- Run V2 calc ---
	calcV2 := NewCalculateHandlerV2(groupRepo, costRepo, costDetailRepo, syncRepo, syncRepo)
	cost, err := calcV2.HandleOneGroup(ctx, headID, period, "tester")
	require.NoError(t, err)
	require.NotNil(t, cost)

	// --- Assert V2Rates against Excel reference ---
	v2 := cost.V2Rates()
	require.NotNil(t, v2, "V2Rates should be populated")
	const eps = 1e-6
	requireFloatNear(t, "CR", derefP(v2.CR), 14.6673636363636, eps)
	requireFloatNear(t, "SR", derefP(v2.SR), 14.2482112657734, eps)
	requireFloatNear(t, "PR", derefP(v2.PR), 20.125786163522, eps)
	requireFloatNear(t, "CL", derefP(v2.CL), 15.3977309090909, eps)
	requireFloatNear(t, "SL", derefP(v2.SL), 14.9017997983609, eps)
	requireFloatNear(t, "FL (no fix yet)", derefP(v2.FL), 0, eps)
	// SP from SR=14.248211 with duty=5%, transport=0.89, anti/freight/default=0:
	// SP = (14.248211+0)*(1+0.05+0)+0.89 = 14.96062... + 0.89 ≈ 15.85062
	requireFloatNear(t, "SP", derefP(v2.SP), 15.8506218290621, eps)
	requireFloatNear(t, "PP", derefP(v2.PP), 22.0220754716981, eps)
	requireFloatNear(t, "FP", derefP(v2.FP), 16.64, eps)

	// --- Assert flag selection: cost_val from SL flag, cost_mark from SP ---
	requireFloatNear(t, "cost_val from SL", derefP(cost.CostValuation()), 14.9017997983609, eps)
	requireFloatNear(t, "cost_mark from SP", derefP(cost.CostMarketing()), 15.8506218290621, eps)
	// cost_sim with simulation_rate=0 should be 0.
	requireFloatNear(t, "cost_sim (no sim)", derefP(cost.CostSimulation()), 0, eps)

	// --- Assert cost detail rows persisted ---
	details, err := costDetailRepo.ListByCostID(ctx, cost.ID())
	require.NoError(t, err)
	require.Len(t, details, 2, "expect 2 detail rows")

	for _, d := range details {
		snap := d.Snapshot()
		switch d.GradeCode() {
		case "CGC":
			requireFloatNear(t, "CGC cons_landed_cost", derefP(snap.ConsLandedCost), 14.614314, eps)
			requireFloatNear(t, "CGC stock_landed_cost", derefP(snap.StockLandedCost), 14.6142694930188, eps)
			requireFloatNear(t, "CGC fix_landed_cost (no fix)", derefP(snap.FixLandedCost), 0, eps)
		case "PCI":
			requireFloatNear(t, "PCI cons_landed_cost", derefP(snap.ConsLandedCost), 23.2319, eps)
			requireFloatNear(t, "PCI stock_landed_cost", derefP(snap.StockLandedCost), 23.2319, eps)
		default:
			t.Fatalf("unexpected grade %q", d.GradeCode())
		}
	}

	// --- Edit simulation_rate ---
	editInputs := NewEditInputsHandler(costRepo, costInputsRepo)
	simRate := 2.0
	updatedCost, err := editInputs.Handle(ctx, EditInputsCommand{
		RMCostID:       cost.ID(),
		SimulationRate: &simRate,
		UpdatedBy:      "tester",
	})
	require.NoError(t, err)
	// W5 = (2+0)*(1+0.05+0)+0.89 = 2.99
	requireFloatNear(t, "cost_sim after edit", derefP(updatedCost.CostSimulation()), 2.99, eps)

	// --- Edit fix_rate on first detail to 15 → FL chain → 17.24965 → fl_rate updates ---
	editFix := NewEditFixRateHandler(costRepo, costDetailRepo, costInputsRepo)
	fixRate := 15.0
	cgcDetailID := uuid.UUID{}
	for _, d := range details {
		if d.GradeCode() == "CGC" {
			cgcDetailID = d.ID()
		}
	}
	require.NotEqual(t, uuid.UUID{}, cgcDetailID)
	res, err := editFix.Handle(ctx, EditFixRateCommand{
		CostDetailID: cgcDetailID,
		FixRate:      &fixRate,
		UpdatedBy:    "tester",
	})
	require.NoError(t, err)
	requireFloatNear(t, "CGC FL after fix=15", derefP(res.Detail.Snapshot().FixLandedCost), 17.24965, eps)
	require.NotNil(t, res.Cost.V2Rates())
	requireFloatNear(t, "parent fl_rate (MAX)", derefP(res.Cost.V2Rates().FL), 17.24965, eps)
	// valuation_flag still SL → cost_val unchanged.
	requireFloatNear(t, "cost_val still SL", derefP(res.Cost.CostValuation()), 14.9017997983609, eps)
}

// =============================================================================
// Helpers
// =============================================================================

func requireFloatNear(t *testing.T, label string, got, want, eps float64) {
	t.Helper()
	if math.Abs(got-want) > eps {
		t.Errorf("%s: got %.10f, want %.10f (diff %.2e)", label, got, want, math.Abs(got-want))
	}
}

func derefP(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func cleanupTestRun(_ *testing.T, ctx context.Context, db *sql.DB, headID uuid.UUID, period string) {
	// Best-effort cleanup. Match by current run's headID + by group_code (covers
	// previous runs that crashed before cleanup).
	const code = "E2E V2 TEST"
	_, _ = db.ExecContext(ctx, `DELETE FROM cst_rm_cost_detail WHERE group_head_id IN
		(SELECT group_head_id FROM cst_rm_group_head WHERE group_code = $1) OR group_head_id = $2`, code, headID)
	_, _ = db.ExecContext(ctx, `DELETE FROM cst_rm_cost WHERE group_head_id IN
		(SELECT group_head_id FROM cst_rm_group_head WHERE group_code = $1) OR group_head_id = $2`, code, headID)
	_, _ = db.ExecContext(ctx, `DELETE FROM cst_rm_group_detail WHERE group_head_id IN
		(SELECT group_head_id FROM cst_rm_group_head WHERE group_code = $1) OR group_head_id = $2`, code, headID)
	_, _ = db.ExecContext(ctx, `DELETE FROM cst_rm_group_head WHERE group_code = $1 OR group_head_id = $2`, code, headID)
	_, _ = db.ExecContext(ctx, `DELETE FROM cst_item_cons_stk_po WHERE period = $1 AND item_code = 'DYE0000015'`, period)
}

func insertSourceFeed(t *testing.T, ctx context.Context, db *sql.DB, period, itemCode, grade string,
	consVal, consQty, stockVal, stockQty, poVal, poQty float64) {
	t.Helper()
	q := `INSERT INTO cst_item_cons_stk_po
		(period, item_code, grade_code, item_name, uom, cons_val, cons_qty, stores_val, stores_qty, last_po_val1, last_po_qty1)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (period, item_code, grade_code) DO UPDATE SET
			cons_val = EXCLUDED.cons_val, cons_qty = EXCLUDED.cons_qty,
			stores_val = EXCLUDED.stores_val, stores_qty = EXCLUDED.stores_qty,
			last_po_val1 = EXCLUDED.last_po_val1, last_po_qty1 = EXCLUDED.last_po_qty1`
	_, err := db.ExecContext(ctx, q, period, itemCode, grade, "Blue MGTS-5109", "KG",
		consVal, consQty, stockVal, stockQty, poVal, poQty)
	require.NoError(t, err)
}

// Suppress unused import noise.
var _ = strings.TrimSpace

// overrideHeadID resets the head's ID via Reconstruct. Re-applies V2 marketing
// inputs because Reconstruct only carries the V1 fields.
func overrideHeadID(t *testing.T, head *rmgroupdomain.Head, target uuid.UUID) {
	t.Helper()
	mi := head.MarketingInputs() // capture BEFORE Reconstruct discards
	*head = *rmgroupdomain.ReconstructHead(
		target, head.Code(), head.Name(),
		head.Description(), head.Colorant(), head.CIName(),
		head.CostPercentage(), head.CostPerKg(),
		head.FlagValuation(), head.FlagMarketing(), head.FlagSimulation(),
		head.InitValValuation(), head.InitValMarketing(), head.InitValSimulation(),
		head.IsActive(), head.CreatedAt(), head.CreatedBy(),
		head.UpdatedAt(), head.UpdatedBy(), nil, nil,
	)
	require.NoError(t, head.AttachMarketingInputs(mi))
}

// overrideDetailGrade sets the grade_code on a freshly-built Detail.
func overrideDetailGrade(t *testing.T, d *rmgroupdomain.Detail, grade string) {
	t.Helper()
	g := grade
	require.NoError(t, d.Update(rmgroupdomain.DetailUpdateInput{GradeCode: &g}, "tester"))
}

// Suppress unused import noise in non-integration builds.
var _ = rmcostdomain.RMTypeGroup
