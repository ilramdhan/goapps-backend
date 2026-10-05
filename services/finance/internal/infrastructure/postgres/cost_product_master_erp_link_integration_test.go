// Integration test for the P3-T5 ERP link workflow (D-LINK) on
// CostProductMasterRepository. LOCAL PostgreSQL only, via
// internal/testutil/pgcontainer; migrations 000001..latest on a fresh DB.
//
// Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auditapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costauditlog"
	cpmapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	cpm "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

type erpLinkIT struct {
	ctx context.Context
	t   *testing.T
	raw *sql.DB
}

func (e erpLinkIT) typeID(code string) int32 {
	var id int32
	require.NoError(e.t, e.raw.QueryRowContext(e.ctx, `
		INSERT INTO cost_product_type (cpt_type_code, cpt_type_name) VALUES ($1, $1)
		ON CONFLICT (cpt_type_code) DO UPDATE SET cpt_type_name = cost_product_type.cpt_type_name
		RETURNING cpt_type_id`, code).Scan(&id))
	return id
}

// product inserts a product row directly (bypasses the MB create guard).
func (e erpLinkIT) product(typeID int32, shade sql.NullString, grade string, active bool, erpItem string) int64 {
	var id int64
	var item any
	if erpItem != "" {
		item = erpItem
	}
	require.NoError(e.t, e.raw.QueryRowContext(e.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name,
			cpm_shade_code, cpm_grade_code, cpm_is_active, cpm_erp_item_code,
			cpm_erp_grade_code_1, cpm_erp_grade_code_2, cpm_created_by, cpm_updated_by)
		VALUES (generate_cost_product_code($1, NOW()), $1, 'IT product', $2, $3, $4, $5,
			'KEEP-G1', 'KEEP-G2', 'it', 'it')
		RETURNING cpm_product_sys_id`, typeID, shade, grade, active, item).Scan(&id))
	return id
}

func shadeOf(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

func TestCostProductMasterErpLinkIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, "cpm_erp_link_it", "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))

	db := postgres.NewDBFromSQL(raw)
	repo := postgres.NewCostProductMasterRepository(db)
	typeRepo := postgres.NewCostProductTypeRepository(db)
	emitter := auditapp.NewEmitter(postgres.NewCostAuditLogRepository(db))
	e := erpLinkIT{ctx: ctx, t: t, raw: raw}
	yarn := e.typeID("ITY")
	mb := e.typeID("MB")
	now := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	link := cpmapp.NewLinkErpItemHandler(repo, repo, typeRepo, emitter).WithClock(func() time.Time { return now })

	holder := e.product(yarn, shadeOf("x419t "), "AX", true, " poy0000275")
	target := e.product(yarn, shadeOf("X419T"), "AX", true, "")
	_ = e.product(yarn, shadeOf("X419T"), "B", true, "POY0000275")   // non-AX: not a key holder
	_ = e.product(yarn, shadeOf("X419T"), "AX", false, "POY0000275") // inactive: not a key holder
	emptyGrade := e.product(yarn, shadeOf("Z1"), "", true, "POY0000999")
	noShade := e.product(yarn, sql.NullString{}, "AX", true, "")
	mbProd := e.product(mb, shadeOf("RED01"), "AX", true, "")

	t.Run("ListActiveAxByErpKey matches trimmed/case-insensitive, AX, active, excludes self", func(t *testing.T) {
		ids, err := repo.ListActiveAxByErpKey(ctx, "POY0000275", "X419T", target)
		require.NoError(t, err)
		assert.Equal(t, []int64{holder}, ids)
		ids, err = repo.ListActiveAxByErpKey(ctx, "POY0000275", "X419T", holder)
		require.NoError(t, err)
		assert.Empty(t, ids)
		ids, err = repo.ListActiveAxByErpKey(ctx, "POY0000999", "z1", 0)
		require.NoError(t, err)
		assert.Equal(t, []int64{emptyGrade}, ids, "empty grade counts as AX")
	})

	t.Run("duplicate refused by handler (V-04)", func(t *testing.T) {
		_, err := link.Handle(ctx, cpmapp.LinkErpItemCommand{ProductSysID: target, ErpItemCode: "POY0000275", ErpShadeCode: "X419T", ActorUserID: "alice"})
		require.ErrorIs(t, err, cpm.ErrLinkDuplicate)
	})

	t.Run("duplicate re-checked inside SaveErpLink", func(t *testing.T) {
		err := repo.SaveErpLink(ctx, cpm.ErpLinkWrite{ProductSysID: target, ShadeKey: "X419T", NewItemCode: "POY0000275", UpdatedAt: now, UpdatedBy: "x"})
		require.ErrorIs(t, err, cpm.ErrLinkDuplicate)
	})

	t.Run("stale and not found", func(t *testing.T) {
		err := repo.SaveErpLink(ctx, cpm.ErpLinkWrite{ProductSysID: target, PrevItemCode: "OTHER", ShadeKey: "X419T", NewItemCode: "POY1", UpdatedAt: now, UpdatedBy: "x"})
		require.ErrorIs(t, err, cpm.ErrLinkStale)
		err = repo.SaveErpLink(ctx, cpm.ErpLinkWrite{ProductSysID: target, ShadeKey: "OTHER", NewItemCode: "POY1", UpdatedAt: now, UpdatedBy: "x"})
		require.ErrorIs(t, err, cpm.ErrLinkStale)
		err = repo.SaveErpLink(ctx, cpm.ErpLinkWrite{ProductSysID: 999999, NewItemCode: "POY1", UpdatedAt: now, UpdatedBy: "x"})
		require.ErrorIs(t, err, cpm.ErrNotFound)
	})

	t.Run("pure validations via handler", func(t *testing.T) {
		_, err := link.Handle(ctx, cpmapp.LinkErpItemCommand{ProductSysID: noShade, ErpItemCode: "POY0000500", ErpShadeCode: "X419T"})
		require.ErrorIs(t, err, cpm.ErrLinkProductHasNoShade)
		_, err = link.Handle(ctx, cpmapp.LinkErpItemCommand{ProductSysID: target, ErpItemCode: "CMB0000001", ErpShadeCode: "X419T"})
		require.ErrorIs(t, err, cpm.ErrLinkCmbRequiresMB)
		_, err = link.Handle(ctx, cpmapp.LinkErpItemCommand{ProductSysID: mbProd, ErpItemCode: "POY0000001", ErpShadeCode: "RED01"})
		require.ErrorIs(t, err, cpm.ErrLinkCmbRequiresMB)
	})

	t.Run("link writes item only, keeps grade codes, audits", func(t *testing.T) {
		res, err := link.Handle(ctx, cpmapp.LinkErpItemCommand{ProductSysID: mbProd, ErpItemCode: "CMB0000001", ErpShadeCode: "red01", ActorUserID: "alice"})
		require.NoError(t, err)
		assert.True(t, res.Changed)

		var item, g1, g2, by string
		var at time.Time
		require.NoError(t, raw.QueryRowContext(ctx, `
			SELECT cpm_erp_item_code, cpm_erp_grade_code_1, cpm_erp_grade_code_2, cpm_erp_linked_by, cpm_erp_linked_at
			FROM cost_product_master WHERE cpm_product_sys_id=$1`, mbProd).Scan(&item, &g1, &g2, &by, &at))
		assert.Equal(t, "CMB0000001", item)
		assert.Equal(t, "KEEP-G1", g1)
		assert.Equal(t, "KEEP-G2", g2)
		assert.Equal(t, "alice", by)
		assert.True(t, at.Equal(now))

		var n int
		require.NoError(t, raw.QueryRowContext(ctx, `SELECT count(*) FROM cost_audit_log
			WHERE cal_entity_type='cost_product_master' AND cal_entity_id=$1 AND cal_operation='ERP_LINK'`, mbProd).Scan(&n))
		assert.Equal(t, 1, n)

		// Unlink clears item and linked_at/by, audits again.
		res, err = link.Handle(ctx, cpmapp.LinkErpItemCommand{ProductSysID: mbProd, ActorUserID: "bob"})
		require.NoError(t, err)
		assert.True(t, res.Changed)
		var nItem, nBy sql.NullString
		var nAt sql.NullTime
		require.NoError(t, raw.QueryRowContext(ctx, `SELECT cpm_erp_item_code, cpm_erp_linked_by, cpm_erp_linked_at
			FROM cost_product_master WHERE cpm_product_sys_id=$1`, mbProd).Scan(&nItem, &nBy, &nAt))
		assert.False(t, nItem.Valid)
		assert.False(t, nBy.Valid)
		assert.False(t, nAt.Valid)
		require.NoError(t, raw.QueryRowContext(ctx, `SELECT count(*) FROM cost_audit_log
			WHERE cal_entity_type='cost_product_master' AND cal_entity_id=$1 AND cal_operation='ERP_LINK'`, mbProd).Scan(&n))
		assert.Equal(t, 2, n)
	})

	t.Run("ERP attributes round-trip incl NULL and decimal; existing Update leaves them", func(t *testing.T) {
		p, err := repo.GetBySysID(ctx, target)
		require.NoError(t, err)
		assert.Equal(t, cpm.ErpAttributes{}, p.ErpAttributes(), "000551 columns start NULL")

		s := func(v string) *string { return &v }
		upd := cpmapp.NewUpdateErpAttributesHandler(repo, repo, emitter)
		_, err = upd.Handle(ctx, cpmapp.UpdateErpAttributesCommand{ProductSysID: target, ActorUserID: "alice",
			Patch: cpm.ErpAttributesPatch{FgType: s("Type 1"), ChpItemCode: s("CHP01"), MsBatchItem: s("MSB000000001"), ItemType: s("FG"), PrdPerDay: s("999999999999999.99999")}})
		require.NoError(t, err)

		p, err = repo.GetBySysID(ctx, target)
		require.NoError(t, err)
		a := p.ErpAttributes()
		assert.Equal(t, "Type 1", a.FgType)
		assert.Equal(t, "CHP01", a.ChpItemCode)
		assert.Equal(t, "MSB000000001", a.MsBatchItem)
		assert.Equal(t, "FG", a.ItemType)
		require.True(t, a.PrdPerDay.Valid)
		assert.True(t, a.PrdPerDay.Decimal.Equal(decimal.RequireFromString("999999999999999.99999")))

		// Existing Update path must not touch the attribute columns.
		require.NoError(t, repo.Update(ctx, p))
		p, err = repo.GetBySysID(ctx, target)
		require.NoError(t, err)
		assert.Equal(t, "Type 1", p.ErpAttributes().FgType)

		_, err = upd.Handle(ctx, cpmapp.UpdateErpAttributesCommand{ProductSysID: target, ActorUserID: "bob",
			Patch: cpm.ErpAttributesPatch{FgType: s(""), PrdPerDay: s("")}})
		require.NoError(t, err)
		var fg sql.NullString
		var prd sql.NullString
		require.NoError(t, raw.QueryRowContext(ctx, `SELECT cpm_erp_fg_type, cpm_erp_prd_per_day::text
			FROM cost_product_master WHERE cpm_product_sys_id=$1`, target).Scan(&fg, &prd))
		assert.False(t, fg.Valid, "cleared to NULL")
		assert.False(t, prd.Valid, "cleared to NULL")

		var n int
		require.NoError(t, raw.QueryRowContext(ctx, `SELECT count(*) FROM cost_audit_log
			WHERE cal_entity_type='cost_product_master' AND cal_entity_id=$1 AND cal_operation='UPDATE'`, target).Scan(&n))
		assert.Equal(t, 2, n)
	})
}
