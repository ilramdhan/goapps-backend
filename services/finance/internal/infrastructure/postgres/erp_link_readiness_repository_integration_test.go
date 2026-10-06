// Integration test for the P3-T9 link-readiness source. LOCAL PostgreSQL
// only, via internal/testutil/pgcontainer; migrations 000001..latest on a
// fresh DB.
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

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

func TestErpLinkReadinessSourceIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, "erp_link_readiness_it", "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))

	src := postgres.NewErpLinkReadinessRepository(postgres.NewDBFromSQL(raw))
	e := erpLinkIT{ctx: ctx, t: t, raw: raw}
	yarn := e.typeID("ITY")
	mb := e.typeID("MB")
	const period = "202607"

	n, err := src.CountReplicaItems(ctx)
	require.NoError(t, err)
	baseReplica := n

	linked := e.product(yarn, shadeOf(" x419t "), "AX", true, " POY0000275 ")
	_ = e.product(yarn, shadeOf("X419T"), "B", true, "POY0000275")   // non-AX excluded
	_ = e.product(yarn, shadeOf("X419T"), "AX", false, "POY0000275") // inactive excluded
	noShade := e.product(mb, sql.NullString{}, "", true, "ACY0000068")
	unlinked := e.product(yarn, shadeOf("s2"), "AX", true, "")
	_ = e.product(yarn, shadeOf("S2"), "B", true, "")              // non-AX excluded
	_ = e.product(yarn, shadeOf("OTHER"), "AX", true, "")          // shade not asked
	blankItem := e.product(yarn, shadeOf("S2"), "AX", true, "   ") // blank item counts as unlinked

	_, err = raw.ExecContext(ctx, `UPDATE cost_product_master SET cpm_product_name = 'DTY 150 WHITE',
		cpm_erp_fg_type = 'DTY', cpm_erp_prd_per_day = 12.5 WHERE cpm_product_sys_id = $1`, linked)
	require.NoError(t, err)
	_, err = raw.ExecContext(ctx, `INSERT INTO cost_erp_item (cei_item_code, cei_item_name) VALUES ('POY0000275','x')`)
	require.NoError(t, err)
	_ = e.actualCost(linked, period, "APPROVED", "USD", "2")
	_ = e.actualCost(linked, "202606", "APPROVED", "USD", "2") // other period
	_ = e.actualCost(noShade, period, "VERIFIED", "USD", "3")

	t.Run("linked", func(t *testing.T) {
		got, err := src.ListLinkedAxProducts(ctx, period)
		require.NoError(t, err)
		require.Len(t, got, 2)
		p := got[0]
		assert.Equal(t, linked, p.SysID)
		assert.Equal(t, "POY0000275", p.ItemCode)
		assert.Equal(t, "x419t", p.ShadeCode)
		assert.Equal(t, "ITY", p.TypeCode)
		assert.Equal(t, "DTY 150 WHITE", p.ProductName)
		assert.Equal(t, "DTY", p.Attrs.FgType)
		assert.Equal(t, "12.50000", p.Attrs.PrdPerDay)
		assert.Empty(t, p.Attrs.ChpItemCode)
		assert.True(t, p.InReplica)
		assert.Equal(t, 1, p.ActualRows)
		q := got[1]
		assert.Equal(t, noShade, q.SysID)
		assert.Empty(t, q.ShadeCode)
		assert.Equal(t, "MB", q.TypeCode)
		assert.False(t, q.InReplica)
		assert.Equal(t, 1, q.ActualRows)
	})

	t.Run("unlinked", func(t *testing.T) {
		got, err := src.ListUnlinkedAxProducts(ctx, []string{"S2"})
		require.NoError(t, err)
		ids := make([]int64, len(got))
		for i, p := range got {
			ids[i] = p.SysID
		}
		assert.Equal(t, []int64{unlinked, blankItem}, ids)
		assert.Equal(t, "s2", got[0].ShadeCode)
		empty, err := src.ListUnlinkedAxProducts(ctx, nil)
		require.NoError(t, err)
		assert.Empty(t, empty)
	})

	t.Run("replica count", func(t *testing.T) {
		n, err := src.CountReplicaItems(ctx)
		require.NoError(t, err)
		assert.Equal(t, baseReplica+1, n)
	})

	t.Run("handler end-to-end on PG", func(t *testing.T) {
		rep, err := app.NewLinkReadinessHandler(nil, nil, src).Handle(ctx, app.LinkReadinessQuery{Period: period})
		require.NoError(t, err)
		require.Len(t, rep.LinkedNoShade, 1)
		assert.Equal(t, noShade, rep.LinkedNoShade[0].SysID)
		require.Len(t, rep.TypeMismatches, 1) // ACY item on MB product
		assert.Equal(t, app.MismatchMBOnYarn, rep.TypeMismatches[0].Direction)
		assert.Empty(t, rep.Duplicates)
	})
}
