// Integration test for the ERP std-cost repository and the DeriveStore side of
// the G11 tx store (plan-05 P4-T4a). LOCAL throwaway PostgreSQL only, via
// internal/testutil/pgcontainer. Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

func stdDec(s string) decimal.NullDecimal {
	return decimal.NewNullDecimal(decimal.RequireFromString(s))
}

// erpStdRows builds n deterministic rows: mostly OK AX/derived rows, every 7th
// a NO_RULE row with an issue and no numbers.
func erpStdRows(n int, salt string) []erpintegration.StdRow {
	out := make([]erpintegration.StdRow, 0, n)
	for i := 0; i < n; i++ {
		item, grade := fmt.Sprintf("YRN%06d", i/2), erpintegration.GradeAX
		if i%2 == 1 {
			grade = "B"
		}
		k := erpintegration.ErpKey{ItemCode: item, GradeCode: grade, ShadeCode: "S1"}
		r := erpintegration.StdRow{Key: k, Kind: erpintegration.ItemKindYarn, ItemName: "Yarn " + item,
			ShadeName: "Shade one", Status: erpintegration.DeriveOK}
		if i%7 == 6 {
			r.Status = erpintegration.DeriveNoRule
			r.Issues = []erpintegration.Issue{{Key: k, Code: erpintegration.IssueV08,
				Severity: erpintegration.SeverityError, Message: "no rule"}}
			out = append(out, r)
			continue
		}
		cost, ver, pid := int64(1000+i), int32(2), int64(500+i)
		r.AxCostSysID, r.AxCostVersion, r.ProductSysID = &cost, &ver, &pid
		r.Source, r.Basis, r.ProdType = erpintegration.SourceAX, erprule.BasisCost, erprule.ProdType("POY")
		r.GradeGroup, r.FgType, r.ChpItemCode = erprule.GradeGroupAX, "Type 1", "CHP01"
		r.MsBatchItem, r.ItemType = "MSB1", "FG"
		if grade != erpintegration.GradeAX {
			r.Source = erpintegration.SourceDerived
		}
		v := fmt.Sprintf("%d.%05d", i%13, (i*37)%100000)
		r.AxCost, r.StdCost = stdDec(v), stdDec(salt+v)
		r.ConvCost, r.AxConvCost, r.ProdValLoss = stdDec("0.12345"), stdDec("0.1"), stdDec("-0.00001")
		r.ChpCost, r.ChpConKg, r.SellingPrice, r.ValueLoss = stdDec("1.5"), stdDec("1.01"), stdDec("3.25"), stdDec("0.5")
		r.ConvCost1, r.ConvCost2 = stdDec("0.1"), stdDec("0.02345")
		if i%3 == 0 {
			r.PrdPerDay = stdDec("120.5")
		}
		out = append(out, r)
	}
	return out
}

func requireStdDigest(ctx context.Context, t *testing.T, repo *postgres.ErpStdCostRepository, id int64, rows []erpintegration.StdRow) erpintegration.StdDigest {
	t.Helper()
	want, err := erpintegration.ComputeStdDigest(rows)
	require.NoError(t, err)
	got, err := repo.Digest(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, want.RowsMD5, got.RowsMD5, "SQL md5 = in-memory md5")
	assert.True(t, want.Totals.Equal(got.Totals), "SQL totals = in-memory totals: %+v vs %+v", want.Totals, got.Totals)
	return got
}

func TestErpStdCostRepositoryIntegration(t *testing.T) {
	ctx := context.Background()
	f := newErpBatchFixture(ctx, t)
	repo := postgres.NewErpStdCostRepository(f.db)

	t.Run("list_recon_returns_only_classified_rows", func(t *testing.T) {
		id := insertBatch(ctx, t, f.raw, "202611", 1, "DERIVED")
		rows := erpStdRows(4, "")
		_, err := repo.Replace(ctx, id, "202611", rows)
		require.NoError(t, err)
		got, err := repo.ListRecon(ctx, id)
		require.NoError(t, err)
		assert.Empty(t, got, "no recon status yet")

		_, err = f.raw.ExecContext(ctx,
			`UPDATE cst_erp_std_cost SET cesc_recon_status = 'MATCH' WHERE cesc_batch_id = $1`, id)
		require.NoError(t, err)
		got, err = repo.ListRecon(ctx, id)
		require.NoError(t, err)
		assert.Len(t, got, len(rows))
	})

	t.Run("replace_idempotent_chunked_roundtrip", func(t *testing.T) {
		id := insertBatch(ctx, t, f.raw, "202601", 1, "DERIVED")
		rows := erpStdRows(2503, "") // > 2 chunks of 1000
		n, err := repo.Replace(ctx, id, "202601", rows)
		require.NoError(t, err)
		assert.Equal(t, int64(len(rows)), n)
		d1 := requireStdDigest(ctx, t, repo, id, rows)

		n, err = repo.Replace(ctx, id, "202601", rows)
		require.NoError(t, err)
		assert.Equal(t, int64(len(rows)), n)
		d2 := requireStdDigest(ctx, t, repo, id, rows)
		assert.True(t, d1.Equal(d2), "re-run gives identical totals and md5")

		got, err := repo.List(ctx, id)
		require.NoError(t, err)
		require.Len(t, got, len(rows))
		byKey := make(map[erpintegration.ErpKey]erpintegration.StdRow, len(rows))
		for _, r := range rows {
			byKey[r.Key] = r
		}
		for i := 1; i < len(got); i++ {
			assert.Negative(t, compareKey(got[i-1].Key, got[i].Key), "ordered by (item, grade, shade)")
		}
		for _, g := range got {
			w := byKey[g.Key]
			assert.Equal(t, erpintegration.CanonicalStdLine(w), erpintegration.CanonicalStdLine(g))
			assert.Equal(t, w.Status, g.Status)
			assert.Equal(t, w.Kind, g.Kind)
			assert.Equal(t, w.ItemName, g.ItemName)
			assert.Equal(t, w.ProdType, g.ProdType)
			assert.Equal(t, w.GradeGroup, g.GradeGroup)
			assert.Equal(t, w.AxCostSysID, g.AxCostSysID)
			assert.Equal(t, w.AxCostVersion, g.AxCostVersion)
			assert.Equal(t, w.ProductSysID, g.ProductSysID)
			assert.Equal(t, w.PrdPerDay.Valid, g.PrdPerDay.Valid)
			assert.Equal(t, w.Issues, g.Issues)
		}

		// Recomputed digest from the read-back rows matches too.
		back, err := erpintegration.ComputeStdDigest(got)
		require.NoError(t, err)
		assert.True(t, back.Equal(d1))

		// A value change moves the md5.
		changed := erpStdRows(2503, "1")
		_, err = repo.Replace(ctx, id, "202601", changed)
		require.NoError(t, err)
		d3 := requireStdDigest(ctx, t, repo, id, changed)
		assert.NotEqual(t, d1.RowsMD5, d3.RowsMD5)

		// Empty replace clears; md5 of the empty set.
		n, err = repo.Replace(ctx, id, "202601", nil)
		require.NoError(t, err)
		assert.Zero(t, n)
		d4 := requireStdDigest(ctx, t, repo, id, nil)
		assert.Equal(t, "d41d8cd98f00b204e9800998ecf8427e", d4.RowsMD5)
	})

	t.Run("batch_isolation_and_atomic_failure", func(t *testing.T) {
		a := insertBatch(ctx, t, f.raw, "202602", 1, "DERIVED")
		b := insertBatch(ctx, t, f.raw, "202603", 1, "DERIVED")
		rowsB := erpStdRows(40, "")
		_, err := repo.Replace(ctx, b, "202603", rowsB)
		require.NoError(t, err)
		dB := requireStdDigest(ctx, t, repo, b, rowsB)

		_, err = repo.Replace(ctx, a, "202602", erpStdRows(10, ""))
		require.NoError(t, err)
		_, err = repo.Replace(ctx, a, "202602", erpStdRows(3, "2"))
		require.NoError(t, err)
		gotA, err := repo.List(ctx, a)
		require.NoError(t, err)
		assert.Len(t, gotA, 3)
		dB2 := requireStdDigest(ctx, t, repo, b, rowsB)
		assert.True(t, dB.Equal(dB2), "replacing A leaves B untouched")

		// A failing insert (OK key longer than 12, chk_cesc_ok_key_len) rolls
		// back the delete too.
		bad := erpStdRows(3, "")
		bad[0].Key.ItemCode = "YRN-TOO-LONG-KEY"
		_, err = repo.Replace(ctx, a, "202602", bad)
		require.Error(t, err)
		gotA, err = repo.List(ctx, a)
		require.NoError(t, err)
		assert.Len(t, gotA, 3, "failed replace keeps the previous rows")

		_, err = repo.Replace(ctx, a, "2026", nil)
		require.Error(t, err, "invalid period")
		_, err = repo.Replace(ctx, 0, "202602", nil)
		require.Error(t, err, "invalid batch id")
	})

	t.Run("derive_store_in_locked_tx", func(t *testing.T) {
		const period = "202604"
		e := erpLinkIT{ctx: ctx, t: t, raw: f.raw}
		yarn := e.typeID("ITY")
		pid := e.product(yarn, shadeOf("S1"), "AX", true, "YRNDS0001")
		_, err := f.raw.ExecContext(ctx, `UPDATE cost_product_master SET cpm_erp_fg_type='Type 3',
			cpm_erp_chp_item_code='CHP9', cpm_erp_ms_batch_item='MSB9', cpm_erp_item_type='FG',
			cpm_erp_prd_per_day=77.12345 WHERE cpm_product_sys_id=$1`, pid)
		require.NoError(t, err)
		okCost := e.actualCost(pid, period, "APPROVED", "USD", "2.345678")
		noRM := e.actualCost(e.product(yarn, shadeOf("S2"), "AX", true, "YRNDS0002"), period, "APPROVED", "USD", "1")
		verified := e.actualCost(e.product(yarn, shadeOf("S3"), "AX", true, "YRNDS0003"), period, "VERIFIED", "USD", "1")
		_, err = f.raw.ExecContext(ctx, `UPDATE cst_product_cost SET cpc_total_rm_cost = 1.234567
			WHERE cpc_cost_id = ANY($1::bigint[])`, fmt.Sprintf("{%d,%d}", okCost, verified))
		require.NoError(t, err)
		_, err = f.raw.ExecContext(ctx, `INSERT INTO cost_erp_shade (ces_shade_code, ces_shade_name)
			VALUES (' s1 ', 'Shade One'), ('S9', NULL)`)
		require.NoError(t, err)

		id := insertBatch(ctx, t, f.raw, period, 1, "COVERED")
		cov := []erpintegration.CoverageLine{{BatchID: id, Kind: erpintegration.ItemKindYarn, ItemCode: "YRNDS0001",
			ShadeCode: "S1", GradeCodes: []string{"AX", "B"}, ProductSysID: &pid, CostID: &okCost,
			Status: erpintegration.CoverageOK, QtyKg: decimal.RequireFromString("3")}}
		_, err = f.coverage.Replace(ctx, id, cov)
		require.NoError(t, err)

		rows := erpStdRows(20, "")
		snapshot := []byte(`{"version": 1, "rules": [{"basis": "COST"}]}`)
		hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		digest, err := erpintegration.ComputeStdDigest(rows)
		require.NoError(t, err)

		err = f.runner.RunLocked(ctx, id, func(ctx context.Context, bs erpintegration.BatchStore) error {
			s, ok := bs.(erpintegration.DeriveStore)
			require.True(t, ok, "tx store implements DeriveStore")

			lines, err := s.ListCoverage(ctx)
			require.NoError(t, err)
			require.Len(t, lines, 1)
			assert.Equal(t, []string{"AX", "B"}, lines[0].GradeCodes)

			locked, err := s.IsPeriodLocked(ctx, period, "ACTUAL")
			require.NoError(t, err)
			assert.False(t, locked, "no lock row")

			ax, err := s.LoadAxComponents(ctx, period, []int64{okCost, noRM, verified, 999999})
			require.NoError(t, err)
			require.Len(t, ax, 1, "only APPROVED with total_rm_cost")
			a := ax[okCost]
			assert.Equal(t, pid, a.ProductSysID)
			assert.Equal(t, int32(1), a.Version)
			assert.True(t, a.CostPerUnit.Equal(decimal.RequireFromString("2.345678")))
			assert.True(t, a.TotalRMCost.Equal(decimal.RequireFromString("1.234567")))
			assert.Equal(t, "Type 3", a.FgType)
			assert.Equal(t, "CHP9", a.ChpItemCode)
			assert.Equal(t, "MSB9", a.MsBatchItem)
			assert.Equal(t, "FG", a.ItemType)
			require.NotNil(t, a.PrdPerDay)
			assert.True(t, a.PrdPerDay.Equal(decimal.RequireFromString("77.12345")))
			other, err := s.LoadAxComponents(ctx, "202605", []int64{okCost})
			require.NoError(t, err)
			assert.Empty(t, other, "other period")
			none, err := s.LoadAxComponents(ctx, period, nil)
			require.NoError(t, err)
			assert.Empty(t, none)

			names, err := s.ShadeNames(ctx, []string{"s1", " S9", "NOPE", ""})
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"S1": "Shade One", "S9": ""}, names)

			b, err := s.GetForUpdate(ctx)
			require.NoError(t, err)
			inv, err := b.Transition(erpintegration.StatusDerived, "integ", erpT0)
			require.NoError(t, err)
			require.True(t, inv.StdRows, "derive invalidates std rows")
			n, err := s.ReplaceStdRows(ctx, period, rows)
			require.NoError(t, err)
			assert.Equal(t, int64(len(rows)), n)
			require.NoError(t, b.SetDerivation(snapshot, hash, digest.Totals, "integ", erpT0))
			b.SetSummary([]byte(fmt.Sprintf(`{"derive":{"rows_md5":%q,"digest_version":%d}}`,
				digest.RowsMD5, erpintegration.StdDigestVersion)))
			return s.Save(ctx, b, erpintegration.StatusCovered, inv)
		})
		require.NoError(t, err)

		requireStdDigest(ctx, t, repo, id, rows) // Save kept the replaced rows
		got, err := f.batches.GetByID(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, erpintegration.StatusDerived, got.Status())
		assert.JSONEq(t, string(snapshot), string(got.RuleSnapshot()))
		assert.Equal(t, hash, got.RuleHash())
		require.NotNil(t, got.Totals())
		assert.True(t, digest.Totals.Equal(*got.Totals()))
		assert.JSONEq(t, fmt.Sprintf(`{"derive":{"rows_md5":%q,"digest_version":1}}`, digest.RowsMD5), string(got.Summary()))

		// A later rule change on another batch leaves this snapshot intact.
		other := insertBatch(ctx, t, f.raw, "202605", 1, "DERIVED")
		ob, err := f.batches.GetByID(ctx, other)
		require.NoError(t, err)
		require.NoError(t, ob.SetDerivation([]byte(`{"version": 2}`),
			"fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210", digest.Totals, "integ", erpT0))
		require.NoError(t, f.runner.RunLocked(ctx, other, func(ctx context.Context, s erpintegration.BatchStore) error {
			return s.Save(ctx, ob, erpintegration.StatusDerived, erpintegration.Invalidation{})
		}))
		again, err := f.batches.GetByID(ctx, id)
		require.NoError(t, err)
		assert.JSONEq(t, string(snapshot), string(again.RuleSnapshot()))
		assert.Equal(t, hash, again.RuleHash())

		// Period lock: a lock row makes IsPeriodLocked true inside the tx.
		_, err = f.raw.ExecContext(ctx, `INSERT INTO cst_period_lock (cpl_period, cpl_calc_type, cpl_locked_by, cpl_reason)
			VALUES ($1, 'ACTUAL', 'integ', 'it')`, period)
		require.NoError(t, err)
		require.NoError(t, f.runner.RunLocked(ctx, id, func(ctx context.Context, bs erpintegration.BatchStore) error {
			locked, err := bs.(erpintegration.DeriveStore).IsPeriodLocked(ctx, period, "ACTUAL")
			require.NoError(t, err)
			assert.True(t, locked)
			return nil
		}))

		// Without a replace in the tx, a derive Save does clear the std rows.
		require.NoError(t, f.runner.RunLocked(ctx, id, func(ctx context.Context, s erpintegration.BatchStore) error {
			b, err := s.GetForUpdate(ctx)
			require.NoError(t, err)
			return s.Save(ctx, b, erpintegration.StatusDerived, erpintegration.Invalidation{StdRows: true})
		}))
		left, err := repo.List(ctx, id)
		require.NoError(t, err)
		assert.Empty(t, left)
	})
}

func compareKey(a, b erpintegration.ErpKey) int {
	switch {
	case a.ItemCode != b.ItemCode:
		return cmpStr(a.ItemCode, b.ItemCode)
	case a.GradeCode != b.GradeCode:
		return cmpStr(a.GradeCode, b.GradeCode)
	default:
		return cmpStr(a.ShadeCode, b.ShadeCode)
	}
}

func cmpStr(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
