// Integration test for LookupSyncApplyRepository + lookupsync.Applier. LOCAL
// PostgreSQL only via internal/testutil/pgcontainer. Skipped unless
// INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/lookupsync"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/machine"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

// staticResolver resolves MACHINE|M1|LSA_MC only.
type staticResolver struct{}

func (staticResolver) Resolve(_ context.Context, master, key, trigger string) (lookupsync.FillResult, error) {
	if master == lookupsync.MasterMachine && key == "M1" && trigger == "LSA_MC" {
		return lookupsync.FillResult{
			Numeric: map[string]float64{"LSA_SPEED": 1200.5},
			Text:    map[string]string{"LSA_LABEL": "Fast"},
		}, nil
	}
	return lookupsync.FillResult{}, machine.ErrNotFound
}

type cppRow struct {
	num, text, filledBy, createdBy sql.NullString
}

func TestLookupSyncApplyIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, "lookup_sync_apply_it", "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))

	exec := func(q string, args ...any) {
		t.Helper()
		_, err := raw.ExecContext(ctx, q, args...)
		require.NoError(t, err)
	}
	param := func(code, dataType, master, group string) uuid.UUID {
		var id uuid.UUID
		require.NoError(t, raw.QueryRowContext(ctx, `
			INSERT INTO mst_parameter (param_code, param_name, data_type, param_category, lookup_master_code, lookup_fill_group_code, created_by)
			VALUES ($1::text, $1::text, $2::text, 'INPUT', NULLIF($3::text,''), NULLIF($4::text,''), 'it') RETURNING id`, code, dataType, master, group).Scan(&id))
		return id
	}
	var typeID int
	require.NoError(t, raw.QueryRowContext(ctx, `SELECT min(cpt_type_id) FROM cost_product_type`).Scan(&typeID))
	product := func(code string, locked bool) int64 {
		var id int64
		require.NoError(t, raw.QueryRowContext(ctx, `
			INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_is_active, cpm_is_locked, cpm_created_by, cpm_updated_by)
			VALUES ($1::text, $2, $1::text, TRUE, $3, 'it', 'it') RETURNING cpm_product_sys_id`, code, typeID, locked).Scan(&id))
		return id
	}
	capp := func(pid int64, ids ...uuid.UUID) {
		for _, p := range ids {
			exec(`INSERT INTO cost_product_applicable_param (capp_product_sys_id, capp_param_id, capp_created_by) VALUES ($1,$2,'it')`, pid, p)
		}
	}
	setText := func(pid int64, p uuid.UUID, v string) {
		exec(`INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_text, cpp_filled_by, cpp_created_by) VALUES ($1,$2,$3,'it','it')`, pid, p, v)
	}
	setNum := func(pid int64, p uuid.UUID, v string) {
		exec(`INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_filled_by, cpp_created_by) VALUES ($1,$2,$3::numeric,'it','it')`, pid, p, v)
	}
	get := func(pid int64, p uuid.UUID) (cppRow, bool) {
		var r cppRow
		err := raw.QueryRowContext(ctx, `SELECT cpp_value_numeric::text, cpp_value_text, cpp_filled_by, cpp_created_by
			FROM cost_product_parameter WHERE cpp_product_sys_id=$1 AND cpp_param_id=$2`, pid, p).
			Scan(&r.num, &r.text, &r.filledBy, &r.createdBy)
		if errors.Is(err, sql.ErrNoRows) {
			return r, false
		}
		require.NoError(t, err)
		return r, true
	}
	cappExists := func(pid int64, p uuid.UUID) bool {
		var n int
		require.NoError(t, raw.QueryRowContext(ctx, `SELECT count(*) FROM cost_product_applicable_param WHERE capp_product_sys_id=$1 AND capp_param_id=$2`, pid, p).Scan(&n))
		return n == 1
	}

	trig := param("LSA_MC", "TEXT", "MACHINE", "")
	speed := param("LSA_SPEED", "NUMBER", "", "LSA_MC")
	label := param("LSA_LABEL", "TEXT", "", "LSA_MC")
	rpm := param("LSA_RPM", "NUMBER", "", "LSA_MC") // master returns nothing → CLEAR

	p1 := product("LSA-1", false) // all children applicable
	p2 := product("LSA-2", true)  // locked
	p3 := product("LSA-3", false) // label not applicable, has a stray label row
	capp(p1, trig, speed, label, rpm)
	capp(p2, trig, speed)
	capp(p3, trig, speed)
	for _, p := range []int64{p1, p2, p3} {
		setText(p, trig, "M1")
	}
	setNum(p1, speed, "1000")
	setNum(p1, rpm, "30")
	setNum(p2, speed, "1000")
	setText(p3, label, "Manual")

	db := postgres.NewDBFromSQL(raw)
	planner := lookupsync.NewPlanner(staticResolver{}, postgres.NewParameterRepository(db), postgres.NewLookupSyncRepository(db))
	applier := lookupsync.NewApplier(planner, postgres.NewLookupSyncApplyRepository(db))

	t.Run("SET writes value with filled_by lookup_sync, CLEAR deletes row, CAPP kept", func(t *testing.T) {
		res, err := applier.ApplyProduct(ctx, p1, lookupsync.Scope{}, "alice")
		require.NoError(t, err)
		assert.Equal(t, lookupsync.ProductApplyResult{ProductSysID: p1, ProductCode: "LSA-1", ValuesWritten: 2, ValuesCleared: 1}, res)

		r, ok := get(p1, speed)
		require.True(t, ok)
		assert.Equal(t, "1200.500000", r.num.String)
		assert.Equal(t, lookupsync.FilledByLookupSync, r.filledBy.String)
		assert.Equal(t, "it", r.createdBy.String) // existing row keeps its creator

		r, ok = get(p1, label)
		require.True(t, ok)
		assert.Equal(t, "Fast", r.text.String)
		assert.Equal(t, lookupsync.FilledByLookupSync, r.filledBy.String)
		assert.Equal(t, "alice", r.createdBy.String)

		_, ok = get(p1, rpm)
		assert.False(t, ok, "CLEAR must delete the cpp row")
		assert.True(t, cappExists(p1, rpm), "CLEAR must keep CAPP")

		r, ok = get(p1, trig)
		require.True(t, ok)
		assert.Equal(t, "M1", r.text.String)
		assert.Equal(t, "it", r.filledBy.String, "trigger row untouched")

		// re-apply is a no-op
		res, err = applier.ApplyProduct(ctx, p1, lookupsync.Scope{}, "alice")
		require.NoError(t, err)
		assert.Zero(t, res.ValuesWritten+res.ValuesCleared)
	})

	t.Run("locked product untouched", func(t *testing.T) {
		res, err := applier.ApplyProduct(ctx, p2, lookupsync.Scope{}, "alice")
		require.NoError(t, err)
		assert.True(t, res.Skipped)
		assert.Equal(t, lookupsync.SkipLocked, res.SkipReason)
		r, ok := get(p2, speed)
		require.True(t, ok)
		assert.Equal(t, "1000.000000", r.num.String)
		assert.Equal(t, "it", r.filledBy.String)
	})

	t.Run("non-applicable child untouched, never adds CAPP", func(t *testing.T) {
		res, err := applier.ApplyProduct(ctx, p3, lookupsync.Scope{}, "alice")
		require.NoError(t, err)
		assert.Equal(t, 1, res.ValuesWritten)
		assert.Zero(t, res.ValuesCleared)
		r, ok := get(p3, label)
		require.True(t, ok)
		assert.Equal(t, "Manual", r.text.String)
		assert.False(t, cappExists(p3, label))
		assert.False(t, cappExists(p3, rpm))
		_, ok = get(p3, rpm)
		assert.False(t, ok)
	})

	t.Run("missing product skipped", func(t *testing.T) {
		res, err := applier.ApplyProduct(ctx, 999999999, lookupsync.Scope{}, "alice")
		require.NoError(t, err)
		assert.Equal(t, lookupsync.SkipProductNotFound, res.SkipReason)
	})
}
