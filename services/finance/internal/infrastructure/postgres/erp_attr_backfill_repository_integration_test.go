// Integration test for the P3-T6 ERP attribute backfill repository. LOCAL
// PostgreSQL only, via internal/testutil/pgcontainer; migrations
// 000001..latest on a fresh DB.
//
// Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

type attrRow struct {
	fg, chp, ms, itemType, erpItem, g1, g2 sql.NullString
	prd                                    sql.NullString
}

func readAttrRow(ctx context.Context, t *testing.T, raw *sql.DB, id int64) attrRow {
	t.Helper()
	var r attrRow
	require.NoError(t, raw.QueryRowContext(ctx, `
		SELECT cpm_erp_fg_type, cpm_erp_chp_item_code, cpm_erp_ms_batch_item, cpm_erp_item_type,
			cpm_erp_item_code, cpm_erp_grade_code_1, cpm_erp_grade_code_2, cpm_erp_prd_per_day::text
		FROM cost_product_master WHERE cpm_product_sys_id = $1`, id).
		Scan(&r.fg, &r.chp, &r.ms, &r.itemType, &r.erpItem, &r.g1, &r.g2, &r.prd))
	return r
}

func TestErpAttrBackfillRepositoryIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, "erp_attr_backfill_it", "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))

	repo := postgres.NewErpAttrBackfillRepository(postgres.NewDBFromSQL(raw))
	e := erpLinkIT{ctx: ctx, t: t, raw: raw}
	yarn := e.typeID("ITY")

	linked := e.product(yarn, shadeOf("sh01 "), "AX", true, " fgx0001")
	withValue := e.product(yarn, shadeOf("S2"), "", true, "FGX0002")
	_ = e.product(yarn, shadeOf("S3"), "AX", true, "")         // unlinked
	_ = e.product(yarn, shadeOf("S4"), "B", true, "FGX0004")   // non-AX
	_ = e.product(yarn, shadeOf("S5"), "AX", false, "FGX0005") // inactive
	_, err = raw.ExecContext(ctx, `UPDATE cost_product_master SET cpm_erp_fg_type = 'KEEP', cpm_erp_item_type = '  '
		WHERE cpm_product_sys_id = $1`, withValue)
	require.NoError(t, err)

	t.Run("ListLinkedProducts lists active AX linked products only", func(t *testing.T) {
		ps, err := repo.ListLinkedProducts(ctx)
		require.NoError(t, err)
		require.Len(t, ps, 2)
		assert.Equal(t, linked, ps[0].ProductSysID)
		assert.Equal(t, "fgx0001", ps[0].ErpItemCode)
		assert.Equal(t, "sh01", ps[0].ShadeCode)
		assert.True(t, ps[0].Attrs.IsEmpty())
		assert.Equal(t, withValue, ps[1].ProductSysID)
		assert.Equal(t, erpintegration.AttrValues{FgType: "KEEP"}, ps[1].Attrs)
	})

	t.Run("FillNullAttributes fills NULL/blank only and never overwrites", func(t *testing.T) {
		res, err := repo.FillNullAttributes(ctx, erpintegration.AttrFillWrite{
			ProductSysID: withValue, ErpItemCode: "FGX0002", ShadeKey: "S2", Actor: "it-user",
			Values: erpintegration.AttrValues{FgType: "DTY", ItemType: "FGX", PrdPerDay: "1250.5"},
		})
		require.NoError(t, err)
		assert.False(t, res.Stale)
		assert.Equal(t, []erpintegration.AttrField{erpintegration.AttrItemType, erpintegration.AttrPrdPerDay}, res.Filled)
		assert.Equal(t, "KEEP", res.After.FgType)
		assert.Equal(t, "1250.50000", res.After.PrdPerDay)
		r := readAttrRow(ctx, t, raw, withValue)
		assert.Equal(t, "KEEP", r.fg.String)
		assert.Equal(t, "FGX", r.itemType.String)
		assert.False(t, r.chp.Valid)
		assert.Equal(t, "FGX0002", r.erpItem.String, "link untouched")
		assert.Equal(t, "KEEP-G1", r.g1.String)
		assert.Equal(t, "KEEP-G2", r.g2.String)
		var by string
		require.NoError(t, raw.QueryRowContext(ctx, `SELECT cpm_updated_by FROM cost_product_master WHERE cpm_product_sys_id=$1`, withValue).Scan(&by))
		assert.Equal(t, "it-user", by)

		again, err := repo.FillNullAttributes(ctx, erpintegration.AttrFillWrite{
			ProductSysID: withValue, ErpItemCode: "FGX0002", ShadeKey: "S2",
			Values: erpintegration.AttrValues{ItemType: "CHANGED", PrdPerDay: "9"},
		})
		require.NoError(t, err)
		assert.Empty(t, again.Filled, "second run is a no-op")
		r = readAttrRow(ctx, t, raw, withValue)
		assert.Equal(t, "FGX", r.itemType.String)
		assert.Equal(t, "1250.50000", r.prd.String)
	})

	t.Run("FillNullAttributes is stale when the link key changed", func(t *testing.T) {
		for name, w := range map[string]erpintegration.AttrFillWrite{
			"other item":  {ProductSysID: linked, ErpItemCode: "FGX9999", ShadeKey: "SH01"},
			"other shade": {ProductSysID: linked, ErpItemCode: "FGX0001", ShadeKey: "SH02"},
			"missing":     {ProductSysID: 999999, ErpItemCode: "FGX0001", ShadeKey: "SH01"},
		} {
			w.Values = erpintegration.AttrValues{FgType: "DTY"}
			res, err := repo.FillNullAttributes(ctx, w)
			require.NoError(t, err, name)
			assert.True(t, res.Stale, name)
			assert.Empty(t, res.Filled, name)
		}
		assert.False(t, readAttrRow(ctx, t, raw, linked).fg.Valid, "stale write left NULL")

		res, err := repo.FillNullAttributes(ctx, erpintegration.AttrFillWrite{
			ProductSysID: linked, ErpItemCode: "FGX0001", ShadeKey: "SH01",
			Values: erpintegration.AttrValues{FgType: "DTY", ChpItemCode: "CHPX001", MsBatchItem: "MB0001"},
		})
		require.NoError(t, err)
		assert.False(t, res.Stale)
		assert.Len(t, res.Filled, 3)
		r := readAttrRow(ctx, t, raw, linked)
		assert.Equal(t, "DTY", r.fg.String)
		assert.Equal(t, "MB0001", r.ms.String)
	})
}
