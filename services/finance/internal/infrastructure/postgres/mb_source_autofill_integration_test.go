package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbsourceautofill"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

const mbsrcPrefix = "ITEST-MBSRC-"

// TestMBSourceAutoFill_Integration exercises the spin pick order (Q2), the superba fallback,
// spin-wins, and the empty-only writes against the local DB. All fixtures use mbsrcPrefix and
// are hard-deleted before/after.
func TestMBSourceAutoFill_Integration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnvOrDefault("TEST_DB_HOST", "localhost"), getEnvOrDefault("TEST_DB_PORT", "5434"),
		getEnvOrDefault("TEST_DB_USER", "finance"), getEnvOrDefault("TEST_DB_PASSWORD", "finance123"),
		getEnvOrDefault("TEST_DB_NAME", "finance_db"))
	raw, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	require.NoError(t, waitForDB(raw, 10*time.Second))
	db := postgres.NewDBFromSQL(raw)
	defer func() { _ = db.Close() }()

	cleanup := func() {
		for _, q := range []string{
			`DELETE FROM cost_product_applicable_param WHERE capp_product_sys_id IN (SELECT cpm_product_sys_id FROM cost_product_master WHERE cpm_product_code LIKE 'ITEST-MBSRC-%')`,
			`DELETE FROM cost_product_parameter WHERE cpp_product_sys_id IN (SELECT cpm_product_sys_id FROM cost_product_master WHERE cpm_product_code LIKE 'ITEST-MBSRC-%')`,
			`DELETE FROM cost_product_master WHERE cpm_product_code LIKE 'ITEST-MBSRC-%'`,
			`DELETE FROM mst_mb_spin WHERE mbs_mgt_name LIKE 'ITEST-MBSRC-%'`,
			`DELETE FROM cost_superba_cost_sp WHERE shade_code LIKE 'ITEST-MBSRC-%'`,
		} {
			_, e := db.ExecContext(ctx, q)
			require.NoError(t, e)
		}
	}
	cleanup()
	defer cleanup()

	var headID uuid.UUID
	require.NoError(t, db.QueryRowContext(ctx, `SELECT mbh_id FROM mst_mb_head WHERE deleted_at IS NULL LIMIT 1`).Scan(&headID))
	var typeID int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code='FG'`).Scan(&typeID))

	spin := func(name, shade, status, orion string, ageDays int) uuid.UUID {
		id := uuid.New()
		_, e := db.ExecContext(ctx, `INSERT INTO mst_mb_spin (mbs_id, mbs_mbh_id, mbs_mgt_name, mbs_shade_code, mbs_status, mbs_orion_item_code, mbs_cost_rate_mkt, created_at, created_by)
			VALUES ($1,$2,$3,$4,$5,$6, 12.5, NOW() - ($7 || ' days')::interval, 'itest')`, id, headID, name, shade, status, orion, fmt.Sprint(ageDays))
		require.NoError(t, e)
		return id
	}
	// Same shade: R and D newest, Boughtout, Spinning old, Spinning older -> Spinning (newer one) wins.
	spin(mbsrcPrefix+"rd", " "+mbsrcPrefix+"s1", "R and D", mbsrcPrefix+"O-RD", 0)
	spin(mbsrcPrefix+"bo", mbsrcPrefix+"S1", "Boughtout", mbsrcPrefix+"O-BO", 1)
	wantSpin := spin(mbsrcPrefix+"sp-new", mbsrcPrefix+"s1", "Spinning", mbsrcPrefix+"O-SP-NEW", 5)
	spin(mbsrcPrefix+"sp-old", mbsrcPrefix+"S1", "Spinning", mbsrcPrefix+"O-SP-OLD", 9)
	// Shade present in both masters -> spin wins.
	spin(mbsrcPrefix+"both", mbsrcPrefix+"BOTH", "Spinning", mbsrcPrefix+"O-BOTH", 1)
	for _, sh := range []string{mbsrcPrefix + "BOTH", mbsrcPrefix + "SUP"} {
		_, e := db.ExecContext(ctx, `INSERT INTO cost_superba_cost_sp (legacy_sys_id, shade_code, colour_name, old_value, source) VALUES ($1,$2,'SKY BLUE',1,'MANUAL')`,
			time.Now().UnixNano()%1_000_000_000+int64(len(sh)), sh)
		require.NoError(t, e)
	}

	mkProduct := func(n, shade string, locked bool) int64 {
		var id int64
		require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_shade_code, cpm_is_locked, cpm_created_by, cpm_updated_by)
			VALUES ($1,$2,$5,$3,$4,'itest','itest') RETURNING cpm_product_sys_id`, mbsrcPrefix+n, typeID, shade, locked, "p "+n).Scan(&id))
		return id
	}
	pickP := mkProduct("PICK", mbsrcPrefix+"s1", false)
	bothP := mkProduct("BOTH", mbsrcPrefix+"both", false)
	supP := mkProduct("SUP", mbsrcPrefix+"sup", false)
	noP := mkProduct("NONE", mbsrcPrefix+"nomatch", false)
	lockP := mkProduct("LOCK", mbsrcPrefix+"s1", true)
	manualP := mkProduct("MANUAL", mbsrcPrefix+"s1", false)

	paramID := func(code string) uuid.UUID {
		var id uuid.UUID
		require.NoError(t, db.QueryRowContext(ctx, `SELECT id FROM mst_parameter WHERE param_code=$1 AND deleted_at IS NULL`, code).Scan(&id))
		return id
	}
	// manual pick must be untouched.
	_, err = db.ExecContext(ctx, `INSERT INTO cost_product_parameter (cpp_product_sys_id,cpp_param_id,cpp_value_text,cpp_filled_by,cpp_created_by) VALUES ($1,$2,'MANUAL-PICK','user','user')`, manualP, paramID("MB_SP_CODE"))
	require.NoError(t, err)

	paramRepo := postgres.NewParameterRepository(db)
	spinRepo := postgres.NewMBSpinRepository(db)
	svc := mbsourceautofill.New(postgres.NewMBSourceAutoFillStore(db),
		mbsource.NewResolver(mbsource.NewMBSpinProvider(spinRepo, paramRepo), mbsource.NewSuperbaProvider(postgres.NewSuperbaCostSpRepository(db))),
		paramRepo)

	for run := 0; run < 2; run++ { // second run must be a no-op (idempotent)
		stats, runErr := svc.AutoFillMBSource(ctx, []int64{pickP, bothP, supP, noP, lockP, manualP}, "itest")
		require.NoError(t, runErr)
		if run == 0 {
			require.Equal(t, 3, stats.Filled)
			require.Equal(t, 1, stats.NoMatch)
			require.Equal(t, 2, stats.BySource[mbsource.SourceMBSpin])
			require.Equal(t, 1, stats.BySource[mbsource.SourceSuperbaCostSP])
		} else {
			require.Equal(t, 0, stats.Filled, "second run is a no-op")
			require.Equal(t, 1, stats.NoMatch)
		}
	}

	val := func(pid int64, code string) (txt sql.NullString, num sql.NullFloat64, spinID uuid.NullUUID, by sql.NullString) {
		_ = db.QueryRowContext(ctx, `SELECT cpp_value_text, cpp_value_numeric, cpp_value_mb_spin_id, cpp_filled_by FROM cost_product_parameter WHERE cpp_product_sys_id=$1 AND cpp_param_id=$2`, pid, paramID(code)).Scan(&txt, &num, &spinID, &by)
		return
	}
	txt, _, sid, by := val(pickP, "MB_SP_CODE")
	require.Equal(t, mbsrcPrefix+"O-SP-NEW", txt.String, "Spinning > Boughtout > R and D, then newest")
	require.True(t, sid.Valid)
	require.Equal(t, wantSpin, sid.UUID)
	require.Equal(t, mbsource.FilledBy, by.String)
	dye, _, _, _ := val(pickP, "MB_SP_DYE")
	require.Equal(t, mbsrcPrefix+"sp-new", dye.String)
	_, rate, _, _ := val(pickP, "MB_RATE_MKT")
	require.InDelta(t, 12.5, rate.Float64, 1e-9)

	txt, _, _, _ = val(bothP, "MB_SP_CODE")
	require.Equal(t, mbsrcPrefix+"O-BOTH", txt.String, "spin wins when shade is in both masters")

	txt, _, sid, _ = val(supP, "MB_SP_CODE")
	require.Equal(t, mbsrcPrefix+"SUP", txt.String, "superba: shade code, normalized")
	require.False(t, sid.Valid)
	dye, _, _, _ = val(supP, "MB_SP_DYE")
	require.Equal(t, "SKY BLUE", dye.String)
	_, rate, _, _ = val(supP, "MB_RATE_MKT")
	require.False(t, rate.Valid, "superba leaves rate empty")

	txt, _, _, _ = val(manualP, "MB_SP_CODE")
	require.Equal(t, "MANUAL-PICK", txt.String, "existing value never overwritten")
	txt, _, _, _ = val(lockP, "MB_SP_CODE")
	require.False(t, txt.Valid, "locked product untouched")
	txt, _, _, _ = val(noP, "MB_SP_CODE")
	require.False(t, txt.Valid)

	var attached int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cost_product_applicable_param WHERE capp_product_sys_id=$1`, pickP).Scan(&attached))
	require.Equal(t, 7, attached, "MB_SP_CODE + 6 children attached")
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cost_product_applicable_param WHERE capp_product_sys_id=$1`, noP).Scan(&attached))
	require.Equal(t, 0, attached, "no match -> nothing attached")
}
