// Gated by INTEGRATION_TEST=true; requires a reachable, migrated ppc_db (defaults
// match the local docker container goapps-ppc-postgres on port 5436). Exercises
// the Layer 1→3 happy path: demand → confirm → plan item → WO → submit → approve.
package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	demandapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/demand"
	planitemapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/planitem"
	workorderapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/workorder"
	demanddomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/demand"
	planitemdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/planitem"
	workorderdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/postgres"
)

func openPPCDB(t *testing.T) *postgres.DB {
	t.Helper()
	host := getEnvOrDefault("TEST_DB_HOST", "localhost")
	port := getEnvOrDefault("TEST_DB_PORT", "5436")
	user := getEnvOrDefault("TEST_DB_USER", "ppc")
	password := getEnvOrDefault("TEST_DB_PASSWORD", "ppc123")
	dbname := getEnvOrDefault("TEST_DB_NAME", "ppc_db")
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", host, port, user, password, dbname)

	sqlDB, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, sqlDB.PingContext(ctx))
	return postgres.NewDBFromSQL(sqlDB)
}

func TestPlanningFlow_DemandToApprovedWO_Integration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	db := openPPCDB(t)
	ctx := context.Background()
	suffix := time.Now().UnixNano()

	// Seed a machine (TXT) + lot for WO validation.
	machineID := seedMachine(ctx, t, db, suffix)
	lotNo := seedLot(ctx, t, db, suffix)
	groupID := machineGroupID(ctx, t, db, machineID)

	// Deadlines are relative to now, so Month MUST be derived from each deadline
	// rather than hardcoded: without MonthOverride the domain rejects any month
	// that disagrees with its deadline (ErrMonthMismatch). A frozen "2026-09"
	// here silently expires once now+30d rolls out of that month. The mismatch
	// rule itself is covered by domain/planitem/timeline_test.go, not here.
	demandDeadline := time.Now().Add(30 * 24 * time.Hour)
	planDeadline := time.Now().Add(25 * 24 * time.Hour)

	// Layer 1 — demand → confirm.
	demandSvc := demandapp.NewService(postgres.NewDemandRepository(db), nil, nil)
	dmd, err := demandSvc.Create(ctx, demandapp.CreateCommand{
		Type:            demanddomain.TypeContract,
		SubType:         demanddomain.SubTypeNewExport,
		Source:          demanddomain.SourceManual,
		CpmProductSysID: 999999,
		QtyOriginal:     1000,
		Deadline:        demandDeadline,
		GradeReq:        demanddomain.GradeReqNone,
		Month:           demandDeadline.Format("2006-01"),
		CreatedBy:       1,
	})
	require.NoError(t, err)
	_, err = demandSvc.Confirm(ctx, dmd.ID(), 1)
	require.NoError(t, err)

	// Layer 2 — plan item. No route provider is wired here, so the FG is created
	// alone with a warning rather than cascading; the cascade itself is covered
	// by the application-layer walker tests.
	planSvc := planitemapp.NewService(postgres.NewPlanItemRepository(db), nil, nil)
	demandID := dmd.ID()
	planRes, err := planSvc.Create(ctx, planitemapp.CreateCommand{
		CpmProductSysID: 999999,
		Type:            planitemdomain.TypeFGDelivery,
		DemandID:        &demandID,
		QtyTarget:       1000,
		Deadline:        planDeadline,
		MachineGroupID:  groupID,
		Month:           planDeadline.Format("2006-01"),
		CreatedBy:       1,
	})
	require.NoError(t, err)
	require.Empty(t, planRes.Children, "no route provider wired, so no cascade")
	require.NotEmpty(t, planRes.Warning, "a cascade-less FG create must say why")

	// Layer 3 — WO → submit → sequential PC then PM approve (v1.2).
	woSvc := workorderapp.NewService(
		postgres.NewWorkOrderRepository(db),
		workorderapp.Deps{
			Machines:  postgres.NewMachineAreaLookup(postgres.NewMachineRepository(db)),
			Lots:      postgres.NewLotExistsLookup(postgres.NewLotRepository(db)),
			PlanItems: postgres.NewPlanItemProductLookup(db),
		},
	)
	wo, err := woSvc.Create(ctx, workorderapp.CreateCommand{
		AreaCode:   "TXT",
		PlanItemID: planRes.Item.ID(),
		MachineID:  machineID,
		CrhHeadID:  1,
		CrhVersion: 1,
		LotNo:      lotNo,
		QtyTarget:  1000,
		Deadline:   time.Now().Add(20 * 24 * time.Hour),
		CreatedBy:  1,
	})
	require.NoError(t, err)

	_, err = woSvc.Submit(ctx, wo.ID(), nil)
	require.NoError(t, err)
	_, err = woSvc.ApproveWO(ctx, wo.ID(), workorderdomain.ApprovalSidePC, 2)
	require.NoError(t, err)
	approved, err := woSvc.ApproveWO(ctx, wo.ID(), workorderdomain.ApprovalSidePM, 3)
	require.NoError(t, err)
	assert.Equal(t, workorderdomain.StatusApproved, approved.Status())

	t.Cleanup(func() {
		sqlExec(ctx, t, db, `DELETE FROM wo_parameter WHERE wop_wo_id = $1`, wo.ID())
		sqlExec(ctx, t, db, `DELETE FROM wo_rm_allocation WHERE wra_wo_id = $1`, wo.ID())
		sqlExec(ctx, t, db, `DELETE FROM work_order WHERE wo_id = $1`, wo.ID())
		sqlExec(ctx, t, db, `DELETE FROM production_plan_item WHERE ppi_demand_id = $1 OR ppi_parent_item_id = $2`, demandID, planRes.Item.ID())
		sqlExec(ctx, t, db, `DELETE FROM production_plan_item WHERE ppi_id = $1`, planRes.Item.ID())
		sqlExec(ctx, t, db, `DELETE FROM production_demand WHERE pd_id = $1`, demandID)
		sqlExec(ctx, t, db, `DELETE FROM lot_master WHERE lm_lot_no = $1`, lotNo)
		sqlExec(ctx, t, db, `DELETE FROM machine WHERE machine_id = $1`, machineID)
	})
}

func seedMachine(ctx context.Context, t *testing.T, db *postgres.DB, suffix int64) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(ctx,
		`INSERT INTO machine (machine_no, machine_area, machine_is_active, created_by)
		 VALUES ($1, 'TXT', TRUE, 'itest') RETURNING machine_id`,
		fmt.Sprintf("IT%d", suffix%100000)).Scan(&id)
	require.NoError(t, err)
	return id
}

func seedLot(ctx context.Context, t *testing.T, db *postgres.DB, suffix int64) string {
	t.Helper()
	lotNo := fmt.Sprintf("ITLOT-%d", suffix)
	_, err := db.ExecContext(ctx,
		`INSERT INTO lot_master (lm_lot_no, lm_item_code, lm_shade_code, lm_std_weight_full, lm_std_weight_unfull, lm_created_by)
		 VALUES ($1, 'ITEM', 'SHADE', 10.0, 8.0, 'itest')`, lotNo)
	require.NoError(t, err)
	return lotNo
}

// machineGroupID returns a usable machine-group id, seeding one if none exist.
func machineGroupID(ctx context.Context, t *testing.T, db *postgres.DB, _ int64) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(ctx, `SELECT group_id FROM machine_group ORDER BY group_id LIMIT 1`).Scan(&id)
	if err == nil {
		return id
	}
	err = db.QueryRowContext(ctx,
		`INSERT INTO machine_group (group_name, group_area, created_by)
		 VALUES ('ITEST-GROUP', 'TXT', 'itest') RETURNING group_id`).Scan(&id)
	require.NoError(t, err)
	return id
}

func sqlExec(ctx context.Context, t *testing.T, db *postgres.DB, q string, args ...interface{}) {
	t.Helper()
	if _, err := db.ExecContext(ctx, q, args...); err != nil {
		t.Logf("cleanup exec failed (non-fatal): %v", err)
	}
}
