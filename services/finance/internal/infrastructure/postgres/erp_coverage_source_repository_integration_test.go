// Integration test for ErpCoverageSourceRepository (plan-04 P3-T4). LOCAL
// PostgreSQL only via internal/testutil/pgcontainer; migrations applied on a
// fresh DB. Skipped unless INTEGRATION_TEST=true.
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

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/erpintegration"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

func (e erpLinkIT) actualCost(pid int64, period, status, cur, val string) int64 {
	var hid, id int64
	err := e.raw.QueryRowContext(e.ctx, `SELECT crh_head_id FROM cost_route_head WHERE crh_product_sys_id = $1
		ORDER BY crh_head_id LIMIT 1`, pid).Scan(&hid)
	if err == sql.ErrNoRows {
		err = e.raw.QueryRowContext(e.ctx, `INSERT INTO cost_route_head (crh_product_sys_id, crh_created_by)
			VALUES ($1, 'it') RETURNING crh_head_id`, pid).Scan(&hid)
	}
	require.NoError(e.t, err)
	require.NoError(e.t, e.raw.QueryRowContext(e.ctx, `INSERT INTO cst_product_cost
		(cpc_product_sys_id, cpc_period, cpc_calculation_type, cpc_route_head_id, cpc_cost_per_unit,
		 cpc_calculated_by, cpc_currency_code, cpc_status, cpc_verified_at, cpc_verified_by, cpc_approved_at, cpc_approved_by)
		VALUES ($1, $2, 'ACTUAL', $3, $4::numeric, 'it', $5, $6::varchar,
		        CASE WHEN $6::varchar IN ('VERIFIED','APPROVED') THEN NOW() END,
		        CASE WHEN $6::varchar IN ('VERIFIED','APPROVED') THEN 'v' END,
		        CASE WHEN $6::varchar = 'APPROVED' THEN NOW() END,
		        CASE WHEN $6::varchar = 'APPROVED' THEN 'a' END)
		RETURNING cpc_cost_id`, pid, period, hid, val, cur, status).Scan(&id))
	return id
}

func TestErpCoverageSourceIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, "erp_coverage_source_it", "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))

	src := postgres.NewErpCoverageSourceRepository(postgres.NewDBFromSQL(raw))
	e := erpLinkIT{ctx: ctx, t: t, raw: raw}
	yarn := e.typeID("ITY")
	mb := e.typeID("MB")
	const period = "202607"

	okP := e.product(yarn, shadeOf(" x419t"), "AX", true, "poy0000275 ")
	_ = e.product(yarn, shadeOf("X419T"), "B", true, "POY0000275")   // non-AX ignored
	_ = e.product(yarn, shadeOf("X419T"), "AX", false, "POY0000275") // inactive ignored
	dup1 := e.product(yarn, shadeOf("D1"), "AX", true, "POY0000300")
	dup2 := e.product(yarn, shadeOf("D1"), "", true, "POY0000300")
	noCost := e.product(yarn, sql.NullString{}, "AX", true, "POY0000400")
	notAppr := e.product(yarn, shadeOf("N1"), "AX", true, "POY0000500")
	idr := e.product(yarn, shadeOf("I1"), "AX", true, "POY0000600")
	yarnOnMB := e.product(mb, shadeOf("Y1"), "AX", true, "POY0000700")
	cmbOnYarn := e.product(yarn, shadeOf("RED"), "AX", true, "CMB0000001")

	okCost := e.actualCost(okP, period, "APPROVED", "USD", "2.345600")
	_ = e.actualCost(okP, "202606", "APPROVED", "USD", "9") // other period ignored
	_ = e.actualCost(notAppr, period, "VERIFIED", "USD", "2")
	_ = e.actualCost(idr, period, "APPROVED", "IDR", "35000")
	_ = e.actualCost(yarnOnMB, period, "APPROVED", "USD", "2")
	_ = e.actualCost(cmbOnYarn, period, "APPROVED", "USD", "2")

	demand := []struct{ item, shade string }{
		{"POY0000275", "X419T"}, {"POY0000300", "D1"}, {"POY0000400", ""}, {"POY0000500", "N1"},
		{"POY0000600", "I1"}, {"POY0000700", "Y1"}, {"CMB0000001", "RED"}, {"POY0009999", "ZZ"},
	}
	keys := make([]app.ErpProductKey, len(demand))
	for i, d := range demand {
		keys[i] = app.NewErpProductKey(d.item, d.shade)
	}

	t.Run("resolve D-LINK keys", func(t *testing.T) {
		got, err := src.ResolveProducts(ctx, keys)
		require.NoError(t, err)
		require.Len(t, got[keys[0]], 1)
		assert.Equal(t, okP, got[keys[0]][0].SysID)
		assert.Equal(t, "ITY", got[keys[0]][0].TypeCode)
		require.Len(t, got[keys[1]], 2)
		assert.Equal(t, []int64{dup1, dup2}, []int64{got[keys[1]][0].SysID, got[keys[1]][1].SysID})
		assert.Equal(t, noCost, got[keys[2]][0].SysID)
		assert.Equal(t, "MB", got[keys[5]][0].TypeCode)
		assert.Empty(t, got[keys[7]])
	})

	t.Run("actual costs and full classification", func(t *testing.T) {
		products, err := src.ResolveProducts(ctx, keys)
		require.NoError(t, err)
		var ids []int64
		for _, k := range keys {
			if c := products[k]; len(c) == 1 {
				ids = append(ids, c[0].SysID)
			}
		}
		costs, err := src.ActualCosts(ctx, period, ids)
		require.NoError(t, err)
		assert.Equal(t, okCost, costs[okP].CostID)
		assert.Equal(t, "2.3456", costs[okP].CostPerUnit.String())
		want := []domain.CoverageStatus{
			domain.CoverageOK, domain.CoverageDupMapping, domain.CoverageNoCost, domain.CoverageNotApproved,
			domain.CoverageNotUSD, domain.CoverageInvalid, domain.CoverageInvalid, domain.CoverageNoMapping,
		}
		for i, d := range demand {
			combo := domain.CoverageLine{BatchID: 1, Kind: domain.ItemKindForCode(d.item), ItemCode: d.item, ShadeCode: d.shade}
			got := app.ClassifyCoverage(period, combo, products[keys[i]], costs)
			assert.Equal(t, want[i], got.Status, "%s/%s: %s", d.item, d.shade, got.Reason)
		}
	})

	t.Run("empty inputs", func(t *testing.T) {
		got, err := src.ResolveProducts(ctx, nil)
		require.NoError(t, err)
		assert.Empty(t, got)
		costs, err := src.ActualCosts(ctx, period, nil)
		require.NoError(t, err)
		assert.Empty(t, costs)
	})
}

type itDemandReader struct{ rows []domain.ErpDemandRow }

func (r itDemandReader) LoadAdjDemand(context.Context, string) ([]domain.ErpDemandRow, error) {
	return r.rows, nil
}

// TestErpBatchStepsIntegration runs LoadDemand + ComputeCoverage end to end
// on PG through the G11 runner (demand from a fake reader; no Oracle).
func TestErpBatchStepsIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, "erp_batch_steps_it", "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))

	db := postgres.NewDBFromSQL(raw)
	e := erpLinkIT{ctx: ctx, t: t, raw: raw}
	yarn := e.typeID("ITY")
	const period = "202607"
	p := e.product(yarn, shadeOf("X419T"), "AX", true, "POY0000275")
	_ = e.actualCost(p, period, "APPROVED", "USD", "2.5")

	batches := postgres.NewErpIntBatchRepository(db)
	b, err := domain.NewBatch(period, domain.ModeLive, "it", time.Now())
	require.NoError(t, err)
	b, err = batches.Create(ctx, b)
	require.NoError(t, err)

	row := func(item, grade, shade, qty string) domain.ErpDemandRow {
		return domain.ErpDemandRow{Period: period, TxnCode: domain.TxnInvAdj, ItemCode: item, GradeCode: grade,
			ShadeCode: shade, HeadCount: 1, ItemCount: 1, RateVariants: 1, QtyKg: decimal.RequireFromString(qty)}
	}
	reader := itDemandReader{rows: []domain.ErpDemandRow{row("POY0000275", "AA", "X419T", "10"), row("POY0000275", "AB", "X419T", "5")}}
	runner := postgres.NewErpBatchTxRunner(db)
	load := app.NewLoadDemandStep(runner, reader, nil)
	cov := app.NewCoverageStep(runner, postgres.NewErpCoverageSourceRepository(db), time.Hour)

	first, err := load.Run(ctx, b.ID(), "it", nil)
	require.NoError(t, err)
	second, err := load.Run(ctx, b.ID(), "it", nil)
	require.NoError(t, err)
	assert.Equal(t, first.Digest, second.Digest, "AC-05")
	assert.Equal(t, first.Rows, second.Rows)

	sum, err := cov.Run(ctx, b.ID(), "it", nil)
	require.NoError(t, err)
	assert.True(t, sum.AllOK)
	got, err := batches.GetByID(ctx, b.ID())
	require.NoError(t, err)
	assert.Equal(t, domain.StatusCovered, got.Status())
	counts, err := postgres.NewErpCoverageRepository(db).Counts(ctx, b.ID())
	require.NoError(t, err)
	assert.Equal(t, int64(1), counts[domain.CoverageOK])

	// A gap after COVERED reopens to DEMAND_LOADED and keeps the demand.
	_, err = raw.ExecContext(ctx, `UPDATE cst_product_cost SET cpc_status = 'SUPERSEDED' WHERE cpc_product_sys_id = $1`, p)
	require.NoError(t, err)
	gap, err := cov.Run(ctx, b.ID(), "it", nil)
	require.NoError(t, err)
	assert.False(t, gap.AllOK)
	got, err = batches.GetByID(ctx, b.ID())
	require.NoError(t, err)
	assert.Equal(t, domain.StatusDemandLoaded, got.Status())
	n, err := postgres.NewErpDemandRepository(db).Count(ctx, b.ID())
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
}
