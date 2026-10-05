// Integration test for the ValidateStore side of the G11 tx store (plan-06
// P5-T2). LOCAL throwaway PostgreSQL only, via internal/testutil/pgcontainer.
// Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

func withValidateStore(ctx context.Context, t *testing.T, f erpBatchFixture, id int64, fn func(s erpintegration.ValidateStore)) {
	t.Helper()
	require.NoError(t, f.runner.RunLocked(ctx, id, func(ctx context.Context, bs erpintegration.BatchStore) error {
		s, ok := bs.(erpintegration.ValidateStore)
		require.True(t, ok, "tx store implements ValidateStore")
		fn(s)
		return nil
	}))
}

func TestErpBatchValidateStoreIntegration(t *testing.T) {
	ctx := context.Background()
	f := newErpBatchFixture(ctx, t)
	repo := postgres.NewErpStdCostRepository(f.db)
	const period = "202608"

	cur := insertBatch(ctx, t, f.raw, period, 3, "DERIVED")
	rows := erpStdRows(10, "")
	_, err := repo.Replace(ctx, cur, period, rows)
	require.NoError(t, err)

	t.Run("std rows, validation write and derive-only read back", func(t *testing.T) {
		upd := make([]erpintegration.StdValidationUpdate, 0, len(rows))
		for i, r := range rows {
			u := erpintegration.StdValidationUpdate{Key: r.Key, Derived: r.Issues}
			if i%2 == 0 {
				u.Validation = []erpintegration.Issue{{Key: r.Key, Code: "V-05", Severity: erpintegration.SeverityWarning, Message: "delta"}}
				u.PrevStd = decimal.NewNullDecimal(decimal.RequireFromString("1.23456"))
			}
			upd = append(upd, u)
		}
		withValidateStore(ctx, t, f, cur, func(s erpintegration.ValidateStore) {
			n, err := s.UpdateStdValidation(ctx, upd)
			require.NoError(t, err)
			assert.Equal(t, int64(len(rows)), n)

			got, err := s.ListStdRowsForValidation(ctx)
			require.NoError(t, err)
			require.Len(t, got, len(rows))
			for _, r := range got {
				for _, is := range r.Issues {
					assert.NotEqual(t, erpintegration.IssueCode("V-05"), is.Code, "validate findings excluded")
				}
			}
		})
		// Other readers see both kinds of issue.
		all, err := repo.List(ctx, cur)
		require.NoError(t, err)
		var v05, derived int
		for _, r := range all {
			for _, is := range r.Issues {
				if is.Code == "V-05" {
					v05++
				} else {
					derived++
				}
			}
		}
		assert.Equal(t, 5, v05)
		assert.Positive(t, derived)

		var prev decimal.NullDecimal
		var raw string
		require.NoError(t, f.raw.QueryRowContext(ctx, `SELECT cesc_prev_std_cost::text, cesc_validation::text
			FROM cst_erp_std_cost WHERE cesc_batch_id=$1 AND cesc_item_code=$2 AND cesc_grade_code=$3 AND cesc_shade_code=$4`,
			cur, rows[0].Key.ItemCode, rows[0].Key.GradeCode, rows[0].Key.ShadeCode).Scan(&prev, &raw))
		assert.True(t, prev.Valid)
		assert.True(t, decimal.RequireFromString("1.23456").Equal(prev.Decimal))
		var js []map[string]string
		require.NoError(t, json.Unmarshal([]byte(raw), &js))
		require.Len(t, js, 1)
		assert.Equal(t, "validate", js[0]["src"])

		// A second write without findings clears them and the prev std.
		for i := range upd {
			upd[i].Validation, upd[i].PrevStd = nil, decimal.NullDecimal{}
		}
		withValidateStore(ctx, t, f, cur, func(s erpintegration.ValidateStore) {
			_, err := s.UpdateStdValidation(ctx, upd)
			require.NoError(t, err)
		})
		var nulls int
		require.NoError(t, f.raw.QueryRowContext(ctx, `SELECT count(*) FROM cst_erp_std_cost
			WHERE cesc_batch_id=$1 AND cesc_prev_std_cost IS NULL`, cur).Scan(&nulls))
		assert.Equal(t, len(rows), nulls)
	})

	t.Run("replica codes exact match", func(t *testing.T) {
		_, err := f.raw.ExecContext(ctx, `INSERT INTO cost_erp_item (cei_item_code, cei_is_active) VALUES ('YRNX01', true), ('YRNX02', false)`)
		require.NoError(t, err)
		_, err = f.raw.ExecContext(ctx, `INSERT INTO cost_erp_shade (ces_shade_code) VALUES ('SX1') ON CONFLICT DO NOTHING`)
		require.NoError(t, err)
		_, err = f.raw.ExecContext(ctx, `INSERT INTO cost_erp_grade (ceg_grade_code) VALUES ('GX') ON CONFLICT DO NOTHING`)
		require.NoError(t, err)
		withValidateStore(ctx, t, f, cur, func(s erpintegration.ValidateStore) {
			rc, err := s.ReplicaCodes(ctx, []string{"YRNX01", "YRNX02", "NOPE"}, []string{"SX1", "sx1"}, []string{"GX", "ZZ"})
			require.NoError(t, err)
			assert.Len(t, rc.Items, 2, "existence, active or not")
			assert.Contains(t, rc.Items, "YRNX02")
			assert.Len(t, rc.Shades, 1)
			assert.Len(t, rc.Grades, 1)
			empty, err := s.ReplicaCodes(ctx, nil, nil, nil)
			require.NoError(t, err)
			assert.Empty(t, empty.Items)
		})
	})

	t.Run("source cost labels unknown ids absent", func(t *testing.T) {
		withValidateStore(ctx, t, f, cur, func(s erpintegration.ValidateStore) {
			got, err := s.SourceCostLabels(ctx, []int64{987654321})
			require.NoError(t, err)
			assert.Empty(t, got)
			got, err = s.SourceCostLabels(ctx, nil)
			require.NoError(t, err)
			assert.Empty(t, got)
		})
	})

	t.Run("currency relabel log", func(t *testing.T) {
		withValidateStore(ctx, t, f, cur, func(s erpintegration.ValidateStore) {
			ok, err := s.CurrencyRelabelApplied(ctx, "209901")
			require.NoError(t, err)
			assert.False(t, ok)
		})
		_, err := f.raw.ExecContext(ctx, `INSERT INTO cst_currency_relabel_log
			(ccrl_cpc_cost_id, ccrl_product_sys_id, ccrl_period, ccrl_old_label, ccrl_new_label, ccrl_migration)
			VALUES (987654320, 1, '209901', 'IDR', 'USD', 'migration:000557')`)
		require.NoError(t, err)
		withValidateStore(ctx, t, f, cur, func(s erpintegration.ValidateStore) {
			ok, err := s.CurrencyRelabelApplied(ctx, "209901")
			require.NoError(t, err)
			assert.True(t, ok)
		})
	})

	t.Run("validation baseline", func(t *testing.T) {
		withValidateStore(ctx, t, f, cur, func(s erpintegration.ValidateStore) {
			bl, err := s.FindValidationBaseline(ctx, period)
			require.NoError(t, err)
			assert.Nil(t, bl, "no active batch yet")
		})
		old := insertBatch(ctx, t, f.raw, "202606", 1, "LOCKED")
		prev := insertBatch(ctx, t, f.raw, "202607", 1, "VALUATED")
		later := insertBatch(ctx, t, f.raw, "202609", 1, "VALUATED")
		_ = old
		_ = later
		_, err := f.raw.ExecContext(ctx, `UPDATE cst_erp_int_batch SET ceib_rule_snapshot = '{"version": 1}'::jsonb WHERE ceib_batch_id = $1`, prev)
		require.NoError(t, err)
		prevRows := erpStdRows(10, "1")
		_, err = repo.Replace(ctx, prev, "202607", prevRows)
		require.NoError(t, err)

		withValidateStore(ctx, t, f, cur, func(s erpintegration.ValidateStore) {
			bl, err := s.FindValidationBaseline(ctx, period)
			require.NoError(t, err)
			require.NotNil(t, bl)
			assert.Equal(t, prev, bl.BatchID, "latest active of same or earlier period")
			assert.Equal(t, "202607", bl.Period)
			assert.JSONEq(t, `{"version": 1}`, string(bl.RuleSnapshot))
			var ok int
			for _, r := range prevRows {
				if r.Status == erpintegration.DeriveOK && r.StdCost.Valid {
					ok++
					assert.True(t, r.StdCost.Decimal.Equal(bl.Std[r.Key]), r.Key.String())
				}
			}
			assert.Len(t, bl.Std, ok, "OK rows only")
		})
	})
}
