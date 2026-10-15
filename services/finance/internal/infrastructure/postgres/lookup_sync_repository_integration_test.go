// Integration test for LookupSyncRepository (lookupsync.SyncReader). LOCAL
// PostgreSQL only via internal/testutil/pgcontainer. Skipped unless
// INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/lookupsync"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

type lsFixture struct {
	db                     *sql.DB
	typeID                 int
	trigMC, trigSP, trigPG uuid.UUID
	childNum, childTxt     uuid.UUID
	p                      []int64 // products LSY-1..LSY-5
	spinA, spinB, spinC    uuid.UUID
	ctx                    context.Context
	t                      *testing.T
}

func (f *lsFixture) exec(q string, args ...any) {
	f.t.Helper()
	_, err := f.db.ExecContext(f.ctx, q, args...)
	require.NoError(f.t, err)
}

func (f *lsFixture) param(code, master string) uuid.UUID {
	var id uuid.UUID
	require.NoError(f.t, f.db.QueryRowContext(f.ctx, `
		INSERT INTO mst_parameter (param_code, param_name, data_type, param_category, lookup_master_code, created_by)
		VALUES ($1, $1, 'TEXT', 'INPUT', NULLIF($2,''), 'it') RETURNING id`, code, master).Scan(&id))
	return id
}

func (f *lsFixture) product(code, shade string, active bool, locked bool) int64 {
	var id int64
	require.NoError(f.t, f.db.QueryRowContext(f.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name,
			cpm_shade_code, cpm_is_active, cpm_is_locked, cpm_created_by, cpm_updated_by)
		VALUES ($1, $2, $3, NULLIF($4,''), $5, $6, 'it', 'it') RETURNING cpm_product_sys_id`,
		code, f.typeID, "name "+code, shade, active, locked).Scan(&id))
	return id
}

func (f *lsFixture) capp(pid int64, params ...uuid.UUID) {
	for _, p := range params {
		f.exec(`INSERT INTO cost_product_applicable_param (capp_product_sys_id, capp_param_id, capp_created_by) VALUES ($1,$2,'it')`, pid, p)
	}
}

func (f *lsFixture) text(pid int64, param uuid.UUID, v string, spin *uuid.UUID) {
	f.exec(`INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_text, cpp_value_mb_spin_id, cpp_filled_by, cpp_created_by)
		VALUES ($1,$2,$3,$4,'it','it')`, pid, param, v, spin)
}

func (f *lsFixture) spin(orion string) uuid.UUID {
	var head uuid.UUID
	require.NoError(f.t, f.db.QueryRowContext(f.ctx,
		`INSERT INTO mst_mb_head (mbh_mb_costing, created_by) VALUES ($1,'it') RETURNING mbh_id`, "LSY-HEAD-"+uuid.NewString()).Scan(&head))
	var id uuid.UUID
	require.NoError(f.t, f.db.QueryRowContext(f.ctx, `
		INSERT INTO mst_mb_spin (mbs_mbh_id, mbs_mgt_name, mbs_orion_item_code, created_by)
		VALUES ($1,'x',NULLIF($2,''),'it') RETURNING mbs_id`, head, orion).Scan(&id))
	return id
}

func newLSFixture(t *testing.T) *lsFixture {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, "lookup_sync_it", "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))

	f := &lsFixture{db: raw, ctx: ctx, t: t}
	require.NoError(t, raw.QueryRowContext(ctx, `SELECT min(cpt_type_id) FROM cost_product_type`).Scan(&f.typeID))
	f.trigMC = f.param("LSY_MC", "MACHINE")
	f.trigSP = f.param("LSY_SP", "MB_SPIN")
	f.trigPG = f.param("LSY_PG", "PRODUCT_GRADE")
	f.childNum = f.param("LSY_CHILD_N", "")
	f.childTxt = f.param("LSY_CHILD_T", "")

	f.spinA = f.spin("LSY-ORION-DUP")
	f.spinB = f.spin("LSY-ORION-DUP")
	f.spinC = f.spin("LSY-ORION-ONE")
	f.exec(`INSERT INTO mst_product_grade (pg_code, pg_name, created_by) VALUES
		('LSY_G1','Lsy Unique Name','it'),('LSY_G2','Lsy Dup Name','it'),('LSY_G3','Lsy Dup Name','it')`)

	// p1: MC=M1 (+children N,T; N has value, T has no row), shade RED, active
	// p2: MC=M1, locked, shade BLK
	// p3: MC=M1 stored but trigger NOT applicable -> excluded; has SP spinC by id
	// p4: inactive, MC=M2, PG=LSY_G1
	// p5: MC=M2 + SP text orion LSY-ORION-ONE (no spin id) + PG legacy name
	mk := func(i int, shade string, active, locked bool) int64 {
		return f.product("LSY-"+string(rune('0'+i)), shade, active, locked)
	}
	p1, p2, p3, p4, p5 := mk(1, "RED", true, false), mk(2, "BLK", true, true), mk(3, "RED", true, false), mk(4, "RED", false, false), mk(5, "", true, false)
	f.p = []int64{p1, p2, p3, p4, p5}

	f.capp(p1, f.trigMC, f.childNum, f.childTxt)
	f.capp(p2, f.trigMC)
	f.capp(p3, f.trigSP) // MC trigger not applicable
	f.capp(p4, f.trigMC, f.trigPG)
	f.capp(p5, f.trigMC, f.trigSP, f.trigPG)

	f.text(p1, f.trigMC, "M1", nil)
	f.text(p2, f.trigMC, "M1", nil)
	f.text(p3, f.trigMC, "M1", nil)
	f.text(p3, f.trigSP, "LSY-ORION-ONE", &f.spinC)
	f.text(p4, f.trigMC, "M2", nil)
	f.text(p4, f.trigPG, "LSY_G1", nil)
	f.text(p5, f.trigMC, "M2", nil)
	f.text(p5, f.trigSP, "LSY-ORION-ONE", nil)
	f.text(p5, f.trigPG, "Lsy Unique Name", nil)
	f.exec(`INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_filled_by, cpp_created_by)
		VALUES ($1,$2,1.5,'it','it')`, p1, f.childNum)
	return f
}

func pidsOf(rows []lookupsync.TriggerRow) []int64 {
	var out []int64
	seen := map[int64]bool{}
	for _, r := range rows {
		if !seen[r.ProductSysID] {
			seen[r.ProductSysID] = true
			out = append(out, r.ProductSysID)
		}
	}
	return out
}

func TestLookupSyncRepositoryIntegration(t *testing.T) {
	f := newLSFixture(t)
	repo := postgres.NewLookupSyncRepository(postgres.NewDBFromSQL(f.db))
	ctx := f.ctx
	filter := &lookupsync.ProductFilter{Search: "LSY-"}

	t.Run("paging never splits a product and covers all", func(t *testing.T) {
		var all []int64
		after := int64(0)
		for i := 0; i < 10; i++ {
			rows, err := repo.ListTriggerRows(ctx, lookupsync.Scope{Filter: filter}, after, 2)
			require.NoError(t, err)
			if len(rows) == 0 {
				break
			}
			ids := pidsOf(rows)
			assert.LessOrEqual(t, len(ids), 2)
			all = append(all, ids...)
			after = ids[len(ids)-1]
			if len(ids) < 2 {
				break
			}
		}
		assert.Equal(t, f.p[:1], all[:1])
		assert.Equal(t, f.p, all)
		// p5 has 3 triggers in a single page
		rows, err := repo.ListTriggerRows(ctx, lookupsync.Scope{Filter: filter}, f.p[3], 1)
		require.NoError(t, err)
		assert.Len(t, rows, 3)
		assert.Equal(t, []string{"LSY_MC", "LSY_PG", "LSY_SP"}, []string{rows[0].TriggerParamCode, rows[1].TriggerParamCode, rows[2].TriggerParamCode})
	})

	t.Run("applicable only and locked flag", func(t *testing.T) {
		rows, err := repo.ListTriggerRows(ctx, lookupsync.Scope{ProductSysIDs: []int64{f.p[1], f.p[2]}}, 0, 10)
		require.NoError(t, err)
		require.Len(t, rows, 2) // p2: MC (locked); p3: SP only
		assert.Equal(t, f.p[1], rows[0].ProductSysID)
		assert.True(t, rows[0].IsLocked)
		assert.Equal(t, "M1", rows[0].StoredKey)
		assert.Equal(t, "LSY_SP", rows[1].TriggerParamCode)
		require.NotNil(t, rows[1].MBSpinID)
		assert.Equal(t, f.spinC, *rows[1].MBSpinID)
		assert.False(t, rows[1].IsLocked)
	})

	t.Run("filter kinds", func(t *testing.T) {
		cases := map[string]struct {
			flt  lookupsync.ProductFilter
			want []int64
		}{
			"search case-insensitive": {lookupsync.ProductFilter{Search: "lsy-3"}, []int64{f.p[2]}},
			"shade":                   {lookupsync.ProductFilter{Search: "LSY-", ShadeCode: "BLK"}, []int64{f.p[1]}},
			"type":                    {lookupsync.ProductFilter{Search: "LSY-", ProductTypeIDs: []int32{int32(f.typeID)}}, f.p},
			"type none":               {lookupsync.ProductFilter{Search: "LSY-", ProductTypeIDs: []int32{-1}}, nil},
			"active":                  {lookupsync.ProductFilter{Search: "LSY-", ActiveFilter: "active"}, []int64{f.p[0], f.p[1], f.p[2], f.p[4]}},
			"inactive":                {lookupsync.ProductFilter{Search: "LSY-", ActiveFilter: "inactive"}, []int64{f.p[3]}},
			"all":                     {lookupsync.ProductFilter{Search: "LSY-", ActiveFilter: "all"}, f.p},
		}
		for name, c := range cases {
			t.Run(name, func(t *testing.T) {
				flt := c.flt
				rows, err := repo.ListTriggerRows(ctx, lookupsync.Scope{Filter: &flt}, 0, 50)
				require.NoError(t, err)
				assert.Equal(t, c.want, pidsOf(rows))
			})
		}
		// filter AND product ids = intersection
		rows, err := repo.ListTriggerRows(ctx, lookupsync.Scope{Filter: filter, ProductSysIDs: []int64{f.p[0], f.p[3]}}, 0, 50)
		require.NoError(t, err)
		assert.Equal(t, []int64{f.p[0], f.p[3]}, pidsOf(rows))
		rows, err = repo.ListTriggerRows(ctx, lookupsync.Scope{Filter: &lookupsync.ProductFilter{Search: "LSY-", ShadeCode: "BLK"}, ProductSysIDs: []int64{f.p[0]}}, 0, 50)
		require.NoError(t, err)
		assert.Empty(t, rows)
	})

	t.Run("master_ref", func(t *testing.T) {
		rows, err := repo.ListTriggerRows(ctx, lookupsync.Scope{MasterRef: &lookupsync.MasterRef{LookupMasterCode: "MACHINE", MasterKey: "M1"}}, 0, 50)
		require.NoError(t, err)
		// p3's MC trigger is not applicable, so only p1, p2 (and no unrelated product)
		assert.Equal(t, []int64{f.p[0], f.p[1]}, filterIDs(pidsOf(rows), f.p))

		// grade: code and legacy unique name both match
		rows, err = repo.ListTriggerRows(ctx, lookupsync.Scope{MasterRef: &lookupsync.MasterRef{LookupMasterCode: "PRODUCT_GRADE", MasterKey: "LSY_G1"}}, 0, 50)
		require.NoError(t, err)
		assert.Equal(t, []int64{f.p[3], f.p[4]}, filterIDs(pidsOf(rows), f.p))
		// ambiguous legacy name does not attach to either duplicate grade
		rows, err = repo.ListTriggerRows(ctx, lookupsync.Scope{MasterRef: &lookupsync.MasterRef{LookupMasterCode: "PRODUCT_GRADE", MasterKey: "LSY_G2"}}, 0, 50)
		require.NoError(t, err)
		assert.Empty(t, filterIDs(pidsOf(rows), f.p))
	})

	t.Run("master_ref MB_SPIN orion match", func(t *testing.T) {
		rows, err := repo.ListTriggerRows(ctx, lookupsync.Scope{MasterRef: &lookupsync.MasterRef{LookupMasterCode: "MB_SPIN", MasterKey: f.spinC.String()}}, 0, 50)
		require.NoError(t, err)
		// p3 by spin id, p5 by orion text
		assert.Equal(t, []int64{f.p[2], f.p[4]}, filterIDs(pidsOf(rows), f.p))
		// non-UUID key matches by text only
		rows, err = repo.ListTriggerRows(ctx, lookupsync.Scope{MasterRef: &lookupsync.MasterRef{LookupMasterCode: "MB_SPIN", MasterKey: "LSY-ORION-ONE"}}, 0, 50)
		require.NoError(t, err)
		assert.Equal(t, []int64{f.p[2], f.p[4]}, filterIDs(pidsOf(rows), f.p))
	})

	t.Run("child states applicable only, nil stored when no row", func(t *testing.T) {
		got, err := repo.LoadChildStates(ctx, []int64{f.p[0], f.p[1]}, []uuid.UUID{f.childNum, f.childTxt})
		require.NoError(t, err)
		require.Len(t, got, 2) // p1 only; p2 has neither child applicable
		n := got[lookupsync.ProductParamKey{ProductSysID: f.p[0], ParamID: f.childNum}]
		require.NotNil(t, n.Stored)
		require.NotNil(t, n.Stored.Numeric)
		assert.Equal(t, "1.5", n.Stored.Numeric.String())
		assert.Nil(t, n.Stored.Text)
		tt, ok := got[lookupsync.ProductParamKey{ProductSysID: f.p[0], ParamID: f.childTxt}]
		assert.True(t, ok)
		assert.Nil(t, tt.Stored)
		_, ok = got[lookupsync.ProductParamKey{ProductSysID: f.p[1], ParamID: f.childNum}]
		assert.False(t, ok)
		empty, err := repo.LoadChildStates(ctx, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, empty)
	})

	t.Run("orion count and grade by name", func(t *testing.T) {
		n, err := repo.CountMBSpinsByOrionCode(ctx, "LSY-ORION-DUP")
		require.NoError(t, err)
		assert.Equal(t, 2, n)
		n, err = repo.CountMBSpinsByOrionCode(ctx, "LSY-ORION-ONE")
		require.NoError(t, err)
		assert.Equal(t, 1, n)
		n, err = repo.CountMBSpinsByOrionCode(ctx, "LSY-NONE")
		require.NoError(t, err)
		assert.Zero(t, n)

		code, ok, err := repo.ProductGradeCodeByName(ctx, "Lsy Unique Name")
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, "LSY_G1", code)
		_, ok, err = repo.ProductGradeCodeByName(ctx, "Lsy Dup Name")
		require.NoError(t, err)
		assert.False(t, ok)
		_, ok, err = repo.ProductGradeCodeByName(ctx, "nope")
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("ForTx reads inside the transaction", func(t *testing.T) {
		tx, err := f.db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		_, err = tx.ExecContext(ctx, `DELETE FROM cost_product_parameter WHERE cpp_product_sys_id=$1 AND cpp_param_id=$2`, f.p[0], f.trigMC)
		require.NoError(t, err)
		rows, err := repo.ForTx(tx).ListTriggerRows(ctx, lookupsync.Scope{ProductSysIDs: []int64{f.p[0]}}, 0, 5)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, "", rows[0].StoredKey)
		outside, err := repo.ListTriggerRows(ctx, lookupsync.Scope{ProductSysIDs: []int64{f.p[0]}}, 0, 5)
		require.NoError(t, err)
		assert.Equal(t, "M1", outside[0].StoredKey)
	})
}

// filterIDs keeps only ids belonging to the fixture (seed migrations may hold
// unrelated products that reference the same keys).
func filterIDs(ids, mine []int64) []int64 {
	set := map[int64]bool{}
	for _, m := range mine {
		set[m] = true
	}
	var out []int64
	for _, id := range ids {
		if set[id] {
			out = append(out, id)
		}
	}
	return out
}
