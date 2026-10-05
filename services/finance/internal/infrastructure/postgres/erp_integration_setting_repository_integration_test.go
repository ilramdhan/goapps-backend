// Integration test for ErpIntegrationSettingRepository (migration 000558;
// plan-06 P5-T11). LOCAL PostgreSQL only, via internal/testutil/pgcontainer.
//
// Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

func TestErpIntegrationSettingRepositoryIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, "erp_integration_setting_it", "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))

	repo := postgres.NewErpIntegrationSettingRepository(postgres.NewDBFromSQL(raw))

	t.Run("seeded_row_is_disabled_and_unconfigured", func(t *testing.T) {
		s, err := repo.GetSchedule(ctx)
		require.NoError(t, err)
		assert.False(t, s.Enabled)
		assert.False(t, s.IsConfigured())
		assert.Equal(t, "Asia/Jakarta", s.Timezone)
		assert.Equal(t, "migration:000558", s.CreatedBy)
	})

	t.Run("save_get_roundtrip_per_mode", func(t *testing.T) {
		cases := []erpintegration.ScheduleSetting{
			{Enabled: true, Mode: erpintegration.ScheduleDayOfMonth, DayOfMonth: 31, RunTime: "04:00", Timezone: "Asia/Jakarta"},
			{Enabled: true, Mode: erpintegration.ScheduleSpecificDate, RunDate: "2026-10-20", RunTime: "08:15", Timezone: "UTC"},
			{Enabled: true, Mode: erpintegration.ScheduleCron, Cron: "0 0 2 5 * *"},
			{Enabled: false, Mode: erpintegration.ScheduleEndOfMonth, RunTime: "23:00"},
		}
		for _, in := range cases {
			out, err := repo.SaveSchedule(ctx, in, "alice")
			require.NoError(t, err, "%+v", in)
			got, err := repo.GetSchedule(ctx)
			require.NoError(t, err)
			assert.Equal(t, out, got)
			assert.Equal(t, in.Enabled, got.Enabled)
			assert.Equal(t, in.Mode, got.Mode)
			assert.Equal(t, in.DayOfMonth, got.DayOfMonth)
			assert.Equal(t, in.RunDate, got.RunDate)
			assert.Equal(t, in.Cron, got.Cron)
			assert.Equal(t, in.RunTime, got.RunTime)
			if in.Timezone == "" {
				assert.Equal(t, "Asia/Jakarta", got.Timezone)
			} else {
				assert.Equal(t, in.Timezone, got.Timezone)
			}
			assert.Equal(t, "alice", got.UpdatedBy)
			assert.Equal(t, "migration:000558", got.CreatedBy, "created_by kept on update")
		}
		var n int
		require.NoError(t, raw.QueryRowContext(ctx, `SELECT COUNT(*) FROM cst_erp_integration_setting`).Scan(&n))
		assert.Equal(t, 1, n, "singleton row")
	})

	t.Run("check_constraints_reject_inconsistent_rows", func(t *testing.T) {
		bad := []erpintegration.ScheduleSetting{
			{Enabled: true}, // enabled needs mode
			{Enabled: true, Mode: erpintegration.ScheduleDayOfMonth}, // needs day
			{Enabled: true, Mode: erpintegration.ScheduleSpecificDate},
			{Enabled: true, Mode: erpintegration.ScheduleCron},
			{Enabled: true, Mode: erpintegration.ScheduleEndOfMonth, RunTime: "24:00"},
			{Enabled: true, Mode: "WEEKLY"},
		}
		for _, in := range bad {
			_, err := repo.SaveSchedule(ctx, in, "alice")
			require.Error(t, err, "%+v", in)
		}
		_, err := raw.ExecContext(ctx, `INSERT INTO cst_erp_integration_setting (ceis_setting_id, created_by) VALUES (2, 'x')`)
		require.Error(t, err, "only row 1 allowed")
	})

	t.Run("missing_row_not_found", func(t *testing.T) {
		_, err := raw.ExecContext(ctx, `DELETE FROM cst_erp_integration_setting`)
		require.NoError(t, err)
		_, err = repo.GetSchedule(ctx)
		require.ErrorIs(t, err, erpintegration.ErrScheduleSettingNotFound)
		out, err := repo.SaveSchedule(ctx, erpintegration.ScheduleSetting{Mode: erpintegration.ScheduleStartOfMonth}, "bob")
		require.NoError(t, err)
		assert.Equal(t, "bob", out.CreatedBy)
	})
}
