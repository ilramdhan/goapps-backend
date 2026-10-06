// Migration round-trip tests for the ERP cost integration chain 000547–000557
// (plan-01 P0-T6; P0-T2..T5 and P0-T10b acceptance). Local PostgreSQL only,
// via internal/testutil/pgcontainer (testcontainers, or a LOCAL
// TEST_DATABASE_URL whose role may CREATE DATABASE).
//
// Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

const (
	preERP      uint64 = 527 // last migration before the ERP chain
	seedVersion uint64 = 548
	relabelVer  uint64 = 557
	templateDB         = "tpl_erp_527"
	migDir             = "."
	seedFile           = "000548_seed_erp_rule_master.up.sql"
)

// erpTables are every table created by 000547–000559.
var erpTables = []string{
	"cst_erp_grade_group_seed", "cst_erp_sell_price", "cst_erp_valloss_rule",
	"cst_erp_int_batch", "cst_erp_adj_demand", "cst_erp_coverage", "cst_erp_std_cost",
	"cst_period_lock", "cst_erp_valuation_preview", "cst_erp_adj_snapshot",
	"cst_erp_oracle_call_log", "cst_currency_relabel_log",
	"cst_erp_integration_setting", // 000558 (P5-T11)
	"cst_erp_backtest_line",       // 000559 (P5-T10b)
}

func TestERPMigrations(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)

	// Build a template at 000527 once; every subtest clones it.
	tpl := srv.CreateDatabase(t, templateDB, "")
	tplMig, err := pgcontainer.NewMigrator(ctx, tpl, migDir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, tplMig.Latest(), relabelVer, "ERP migrations missing on disk")
	require.NoError(t, tplMig.UpTo(ctx, preERP), "base chain 000001-000527 must apply")
	baseFingerprint := schemaFingerprint(ctx, t, tpl)
	require.NoError(t, tpl.Close())

	clone := func(t *testing.T, name string) (*sql.DB, *pgcontainer.Migrator) {
		t.Helper()
		db := srv.CreateDatabase(t, name, templateDB)
		m, err := pgcontainer.NewMigrator(ctx, db, migDir)
		require.NoError(t, err)
		v, err := m.Version(ctx)
		require.NoError(t, err)
		require.Equal(t, preERP, v)
		return db, m
	}

	t.Run("RoundTripUpDownUp", func(t *testing.T) {
		db, m := clone(t, "erp_roundtrip")

		require.NoError(t, m.Up(ctx), "up (empty tables: 000557 must be a no-op)")
		assertSeedCounts(ctx, t, db)
		for _, tbl := range erpTables {
			assert.True(t, tableExists(ctx, t, db, tbl), "table %s after up", tbl)
		}
		assert.Equal(t, 0, count(ctx, t, db, "SELECT count(*) FROM cst_currency_relabel_log"))
		assert.Equal(t, 1, count(ctx, t, db, "SELECT count(*) FROM cst_erp_integration_setting WHERE ceis_setting_id = 1 AND NOT ceis_schedule_enabled"), "000558 seeds one disabled row")

		require.NoError(t, m.DownTo(ctx, preERP), "down to 000527")
		for _, tbl := range erpTables {
			assert.False(t, tableExists(ctx, t, db, tbl), "table %s after down", tbl)
		}
		assert.Equal(t, baseFingerprint, schemaFingerprint(ctx, t, db), "down must restore the 000527 schema exactly")

		require.NoError(t, m.Up(ctx), "second up")
		assertSeedCounts(ctx, t, db)
	})

	t.Run("SeedAssertFiresWhenRowRemoved", func(t *testing.T) {
		raw, err := os.ReadFile(seedFile)
		require.NoError(t, err)
		seed := string(raw)

		cases := []struct {
			name, line, wantErr string
		}{
			{"rule", "    ('Type 1', 'POY', 'JLT', 'SPPTY', 0.5, 'migration:000548'),\n", "expected 115 cst_erp_valloss_rule"},
			{"price", "    ('SPITY', 1.5, 'migration:000548'),\n", "expected 3 cst_erp_sell_price"},
			{"group", "    ('A9', 'NS', 'migration:000548'),\n", "expected 22 cst_erp_grade_group_seed"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				require.Equal(t, 1, strings.Count(seed, tc.line), "fixture line must occur once in %s", seedFile)
				db, m := clone(t, "erp_seed_"+tc.name)
				require.NoError(t, m.UpTo(ctx, seedVersion-1))

				err := pgcontainer.ExecSQL(ctx, db, strings.Replace(seed, tc.line, "", 1))
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				// Whole file is one transaction: nothing was seeded.
				assert.Equal(t, 0, count(ctx, t, db, "SELECT count(*) FROM cst_erp_valloss_rule"))
				assert.Equal(t, 0, count(ctx, t, db, "SELECT count(*) FROM cst_erp_sell_price"))
				assert.Equal(t, 0, count(ctx, t, db, "SELECT count(*) FROM cst_erp_grade_group_seed"))
			})
		}

		t.Run("conflicting_preexisting_rule", func(t *testing.T) {
			// A pre-existing active rule on a seed key makes ON CONFLICT skip
			// one marker row; the count assert must catch it.
			db, m := clone(t, "erp_seed_conflict")
			require.NoError(t, m.UpTo(ctx, seedVersion-1))
			mustExec(ctx, t, db, `INSERT INTO cst_erp_valloss_rule
				(cevr_fg_type, cevr_prod_type, cevr_grade_group, cevr_basis, cevr_val_loss, created_by)
				VALUES ('Type 1', 'POY', 'BC', 'SPPTY', 0.5, 'someone')`)
			err := m.UpTo(ctx, seedVersion)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "expected 115 cst_erp_valloss_rule")
		})
	})

	t.Run("ConstraintsAndSafetyTables", func(t *testing.T) {
		db, m := clone(t, "erp_constraints")
		require.NoError(t, m.Up(ctx))

		// Grade-group apply (000548) only fills NULLs on existing grades.
		// Covered by seed; here: cost_erp_grade column exists and is nullable.
		mustExec(ctx, t, db, `INSERT INTO cost_erp_grade (ceg_grade_code) VALUES ('ZZTEST')`)

		// uix_ceib_inflight: one in-flight LIVE batch per period.
		mustExec(ctx, t, db, `INSERT INTO cst_erp_int_batch (ceib_period, ceib_seq, created_by) VALUES ('202609', 1, 'test')`)
		assertExecFails(ctx, t, db, `INSERT INTO cst_erp_int_batch (ceib_period, ceib_seq, created_by) VALUES ('202609', 2, 'test')`, "uix_ceib_inflight")

		// uix_ceib_active: one VALUATED/RECONCILED/LOCKED LIVE batch per period.
		mustExec(ctx, t, db, `UPDATE cst_erp_int_batch SET ceib_status = 'VALUATED' WHERE ceib_period = '202609' AND ceib_seq = 1`)
		mustExec(ctx, t, db, `INSERT INTO cst_erp_int_batch (ceib_period, ceib_seq, created_by) VALUES ('202609', 2, 'test')`)
		assertExecFails(ctx, t, db, `UPDATE cst_erp_int_batch SET ceib_status = 'LOCKED' WHERE ceib_period = '202609' AND ceib_seq = 2`, "uix_ceib_active")

		// SHADOW batches never reach PUSHED or beyond, and do not block LIVE.
		mustExec(ctx, t, db, `INSERT INTO cst_erp_int_batch (ceib_period, ceib_seq, ceib_mode, created_by) VALUES ('202609', 3, 'SHADOW', 'test')`)
		assertExecFails(ctx, t, db, `UPDATE cst_erp_int_batch SET ceib_status = 'PUSHED' WHERE ceib_seq = 3 AND ceib_period = '202609'`, "chk_ceib_shadow_status")
		assertExecFails(ctx, t, db, `INSERT INTO cst_erp_int_batch (ceib_period, ceib_seq, ceib_status, created_by) VALUES ('202609', 9, 'BOGUS', 't')`, "chk_ceib_status")

		// Period lock: ACTUAL only, one row per (period, calc_type).
		mustExec(ctx, t, db, `INSERT INTO cst_period_lock (cpl_period, cpl_calc_type, cpl_locked_by, cpl_reason) VALUES ('202609', 'ACTUAL', 'test', 'test')`)
		assertExecFails(ctx, t, db, `INSERT INTO cst_period_lock (cpl_period, cpl_calc_type, cpl_locked_by, cpl_reason) VALUES ('202609', 'ACTUAL', 'test', 'test')`, "pk_cst_period_lock")
		assertExecFails(ctx, t, db, `INSERT INTO cst_period_lock (cpl_period, cpl_calc_type, cpl_locked_by, cpl_reason) VALUES ('202608', 'FORECAST', 'test', 'test')`, "chk_cpl_calc_type")

		// Preview: one live preview per (batch, operation).
		var batchID int64
		require.NoError(t, db.QueryRowContext(ctx, `SELECT ceib_batch_id FROM cst_erp_int_batch WHERE ceib_period='202609' AND ceib_seq=1`).Scan(&batchID))
		var previewID string
		require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO cst_erp_valuation_preview
			(cevp_batch_id, cevp_operation, cevp_status, cevp_created_by, cevp_expires_at)
			VALUES ($1, 'VALUATE', 'OPEN', 'test', NOW() + interval '30 minutes') RETURNING cevp_preview_id`, batchID).Scan(&previewID))
		assertExecFails(ctx, t, db, `INSERT INTO cst_erp_valuation_preview
			(cevp_batch_id, cevp_operation, cevp_created_by, cevp_expires_at)
			VALUES (`+itoa(batchID)+`, 'VALUATE', 'test', NOW() + interval '30 minutes')`, "uix_cevp_live")

		// Snapshot immutability: insert ok, UPDATE and DELETE rejected.
		mustExec(ctx, t, db, `INSERT INTO cst_erp_adj_snapshot
			(ceas_preview_id, ceas_batch_id, ceas_period, ceas_adjh_sys_id, ceas_adji_sys_id, ceas_txn_code, ceas_flex, ceas_row_hash)
			VALUES ('`+previewID+`', `+itoa(batchID)+`, '202609', 10, 11, 'ADJ', '{}'::jsonb, repeat('a', 64))`)
		assertExecFails(ctx, t, db, `UPDATE cst_erp_adj_snapshot SET ceas_rate = 1`, "immutable")
		assertExecFails(ctx, t, db, `DELETE FROM cst_erp_adj_snapshot`, "immutable")
		assert.Equal(t, 1, count(ctx, t, db, "SELECT count(*) FROM cst_erp_adj_snapshot"))

		// Oracle call log: only the 6 allowlisted statement keys.
		mustExec(ctx, t, db, `INSERT INTO cst_erp_oracle_call_log (ceocl_call_id, ceocl_statement_key, ceocl_status, ceocl_actor)
			VALUES (gen_random_uuid(), 'W2_VALUATE_ADJ', 'STARTED', 'test')`)
		assertExecFails(ctx, t, db, `INSERT INTO cst_erp_oracle_call_log (ceocl_call_id, ceocl_statement_key, ceocl_status, ceocl_actor)
			VALUES (gen_random_uuid(), 'W3_DELETE_ADJ', 'STARTED', 'test')`, "chk_ceocl_statement_key")

		// 000555 / 000553 widenings.
		mustExec(ctx, t, db, `INSERT INTO job_execution (job_code, job_type, created_by) VALUES ('JOB-ERP-T', 'erp_integration', 'test')`)
		mustExec(ctx, t, db, `INSERT INTO job_execution (job_code, job_type, created_by) VALUES ('JOB-ERP-M', 'erp_master_sync', 'test')`)
		mustExec(ctx, t, db, `INSERT INTO cost_audit_log (cal_entity_type, cal_entity_id, cal_operation, cal_user_id) VALUES ('ERP_BATCH', '1', 'ERP_PUSH', 'test')`)
		assertExecFails(ctx, t, db, `INSERT INTO cost_audit_log (cal_entity_type, cal_entity_id, cal_operation, cal_user_id) VALUES ('ERP_BATCH', '1', 'ERP_DROP', 'test')`, "chk_cal_operation")

		// Seed rules: grade group AX is forbidden; one active rule per key.
		assertExecFails(ctx, t, db, `INSERT INTO cst_erp_valloss_rule (cevr_fg_type, cevr_prod_type, cevr_grade_group, cevr_basis, cevr_val_loss, created_by)
			VALUES ('Type 1', 'POY', 'AX', 'COST', 0, 't')`, "chk_cevr_grade_group")
		assertExecFails(ctx, t, db, `INSERT INTO cst_erp_valloss_rule (cevr_fg_type, cevr_prod_type, cevr_grade_group, cevr_basis, cevr_val_loss, created_by)
			VALUES ('Type 1', 'POY', 'BC', 'SPPTY', 0.5, 't')`, "uk_cevr_key")
	})

	t.Run("ApprovedBackfillAndRelabel", func(t *testing.T) {
		db, m := clone(t, "erp_relabel")
		f := seedCostFixtures(ctx, t, db, false)
		before := costSnapshot(ctx, t, db)

		require.NoError(t, m.Up(ctx))

		// 000552: APPROVED rows backfilled from verified_*; others untouched.
		var at, by sql.NullString
		require.NoError(t, db.QueryRowContext(ctx, `SELECT cpc_approved_at::text, cpc_approved_by FROM cst_product_cost WHERE cpc_cost_id=$1`, f.approved).Scan(&at, &by))
		assert.True(t, at.Valid)
		assert.Equal(t, "verifier", by.String)
		assert.Equal(t, 1, count(ctx, t, db, `SELECT count(*) FROM cst_product_cost WHERE cpc_approved_at IS NOT NULL`))
		assert.Equal(t, 1, count(ctx, t, db, `SELECT count(*) FROM cst_product_cost WHERE cpc_approved_at = cpc_verified_at AND cpc_approved_by = cpc_verified_by`))

		// 000557: in-scope ACTUAL IDR rows -> USD; 202605, FORECAST untouched.
		after := costSnapshot(ctx, t, db)
		require.Len(t, after, len(before))
		for id, b := range before {
			a := after[id]
			assert.Equal(t, b.value, a.value, "cost values must not change (row %d)", id)
			want := b.label
			if b.calcType == "ACTUAL" && b.label == "IDR" && b.period != "202605" && inScope(b.period) {
				want = "USD"
			}
			assert.Equal(t, want, a.label, "label of row %d (%s %s)", id, b.period, b.calcType)
		}
		assert.Equal(t, "IDR", after[f.may].label, "202605 is excluded")
		assert.Equal(t, "IDR", after[f.forecast].label, "FORECAST untouched")
		assert.Equal(t, "USD", after[f.alreadyUSD].label)
		assert.Equal(t, 3, count(ctx, t, db, `SELECT count(*) FROM cst_currency_relabel_log`))

		// Down 000557 restores exactly the touched rows.
		require.NoError(t, m.DownTo(ctx, relabelVer-1))
		assert.Equal(t, before, costSnapshot(ctx, t, db), "000557 down must restore labels exactly")
		assert.False(t, tableExists(ctx, t, db, "cst_currency_relabel_log"))

		// Down to 000527 then up again with data present.
		require.NoError(t, m.DownTo(ctx, preERP))
		assert.Equal(t, before, costSnapshot(ctx, t, db))
		require.NoError(t, m.Up(ctx))
		assert.Equal(t, 3, count(ctx, t, db, `SELECT count(*) FROM cst_currency_relabel_log`))
	})

	t.Run("RelabelGuardFires", func(t *testing.T) {
		db, m := clone(t, "erp_guard")
		seedCostFixtures(ctx, t, db, true)
		before := costSnapshot(ctx, t, db)

		err := m.Up(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "000557 guard: period 202607")
		v, verr := m.Version(ctx)
		require.NoError(t, verr)
		assert.Equal(t, uint64(555), v, "everything before 000557 applied, 000557 rolled back")
		assert.False(t, tableExists(ctx, t, db, "cst_currency_relabel_log"))
		after := costSnapshot(ctx, t, db)
		assert.Equal(t, before, after, "guard must change nothing")
	})
}

type costFixtures struct {
	approved, may, forecast, alreadyUSD int64
}

// seedCostFixtures inserts cost_product_master / cost_route_head /
// cst_product_cost rows at 000527. With outlier=true it adds an active
// ACTUAL row above the 000557 threshold in 202607.
func seedCostFixtures(ctx context.Context, t *testing.T, db *sql.DB, outlier bool) costFixtures {
	t.Helper()
	product := func(code string) (int64, int64) {
		var pid, hid int64
		require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO cost_product_master
			(cpm_product_type_id, cpm_product_code, cpm_product_name, cpm_created_by, cpm_updated_by)
			VALUES ((SELECT min(cpt_type_id) FROM cost_product_type), $1::varchar, $1::text, 'test', 'test')
			RETURNING cpm_product_sys_id`, code).Scan(&pid))
		require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO cost_route_head (crh_product_sys_id, crh_created_by)
			VALUES ($1, 'test') RETURNING crh_head_id`, pid).Scan(&hid))
		return pid, hid
	}
	cost := func(pid, hid int64, period, calc, status, cur string, val float64) int64 {
		var id int64
		require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO cst_product_cost
			(cpc_product_sys_id, cpc_period, cpc_calculation_type, cpc_route_head_id, cpc_cost_per_unit,
			 cpc_calculated_by, cpc_currency_code, cpc_status, cpc_verified_at, cpc_verified_by)
			VALUES ($1, $2, $3, $4, $5, 'test', $6, $7::varchar,
			        CASE WHEN $7::varchar IN ('VERIFIED','APPROVED') THEN NOW() END,
			        CASE WHEN $7::varchar IN ('VERIFIED','APPROVED') THEN 'verifier' END)
			RETURNING cpc_cost_id`, pid, period, calc, hid, val, cur, status).Scan(&id))
		return id
	}
	p1, h1 := product("ERPT-P1")
	p2, h2 := product("ERPT-P2")
	var f costFixtures
	f.approved = cost(p1, h1, "202604", "ACTUAL", "APPROVED", "IDR", 42.123456)
	cost(p1, h1, "202606", "ACTUAL", "VERIFIED", "IDR", 91.5)
	f.may = cost(p1, h1, "202605", "ACTUAL", "CALCULATED", "IDR", 19271.5)
	f.forecast = cost(p1, h1, "202604", "FORECAST", "CALCULATED", "IDR", 150)
	f.alreadyUSD = cost(p2, h2, "202604", "ACTUAL", "CALCULATED", "USD", 10)
	// SUPERSEDED rows are outside the guard but still carry the wrong label.
	cost(p2, h2, "202606", "ACTUAL", "SUPERSEDED", "IDR", 500)
	cost(p2, h2, "202610", "ACTUAL", "CALCULATED", "IDR", 12) // not in the period list
	if outlier {
		cost(p2, h2, "202607", "ACTUAL", "CALCULATED", "IDR", 150)
	}
	return f
}

var relabelPeriods = map[string]bool{"202604": true, "202606": true, "202607": true, "202608": true, "202609": true}

func inScope(p string) bool { return relabelPeriods[p] }

type costRow struct {
	period, calcType, label, value string
}

func costSnapshot(ctx context.Context, t *testing.T, db *sql.DB) map[int64]costRow {
	t.Helper()
	rows, err := db.QueryContext(ctx, `SELECT cpc_cost_id, cpc_period, cpc_calculation_type, cpc_currency_code, cpc_cost_per_unit::text FROM cst_product_cost`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	out := map[int64]costRow{}
	for rows.Next() {
		var id int64
		var r costRow
		require.NoError(t, rows.Scan(&id, &r.period, &r.calcType, &r.label, &r.value))
		out[id] = r
	}
	require.NoError(t, rows.Err())
	return out
}

func assertSeedCounts(ctx context.Context, t *testing.T, db *sql.DB) {
	t.Helper()
	assert.Equal(t, 115, count(ctx, t, db, "SELECT count(*) FROM cst_erp_valloss_rule WHERE created_by='migration:000548'"))
	assert.Equal(t, 3, count(ctx, t, db, "SELECT count(*) FROM cst_erp_sell_price WHERE created_by='migration:000548'"))
	assert.Equal(t, 22, count(ctx, t, db, "SELECT count(*) FROM cst_erp_grade_group_seed WHERE created_by='migration:000548'"))
	assert.Equal(t, 0, count(ctx, t, db, `SELECT count(*) FROM cst_erp_valloss_rule r
		WHERE r.cevr_basis <> 'COST' AND NOT EXISTS (SELECT 1 FROM cst_erp_sell_price p WHERE p.cesp_basis = r.cevr_basis)`))
}

// schemaFingerprint captures columns and constraints of the public schema
// (excluding the harness version table) so down can be compared exactly.
func schemaFingerprint(ctx context.Context, t *testing.T, db *sql.DB) string {
	t.Helper()
	var fp string
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT md5(coalesce(string_agg(x, '|' ORDER BY x), '')) FROM (
			SELECT table_name || '.' || column_name || ':' || data_type || ':' || is_nullable || ':' || coalesce(column_default, '') AS x
			  FROM information_schema.columns
			 WHERE table_schema = 'public' AND table_name <> 'pgcontainer_schema_version'
			UNION ALL
			SELECT conrelid::regclass::text || '.' || conname || ':' || pg_get_constraintdef(oid)
			  FROM pg_constraint WHERE connamespace = 'public'::regnamespace
			UNION ALL
			SELECT indexname || ':' || indexdef FROM pg_indexes WHERE schemaname = 'public'
			UNION ALL
			SELECT 'trg:' || tgname FROM pg_trigger WHERE NOT tgisinternal
			UNION ALL
			SELECT 'fn:' || proname FROM pg_proc WHERE pronamespace = 'public'::regnamespace
		) s`).Scan(&fp))
	return fp
}

func tableExists(ctx context.Context, t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var ok bool
	require.NoError(t, db.QueryRowContext(ctx, `SELECT to_regclass('public.' || $1) IS NOT NULL`, name).Scan(&ok))
	return ok
}

func count(ctx context.Context, t *testing.T, db *sql.DB, q string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRowContext(ctx, q).Scan(&n), q)
	return n
}

func mustExec(ctx context.Context, t *testing.T, db *sql.DB, q string) {
	t.Helper()
	_, err := db.ExecContext(ctx, q)
	require.NoError(t, err, q)
}

func assertExecFails(ctx context.Context, t *testing.T, db *sql.DB, q, want string) {
	t.Helper()
	_, err := db.ExecContext(ctx, q)
	if assert.Error(t, err, q) {
		assert.Contains(t, err.Error(), want, q)
	}
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
