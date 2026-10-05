package erpintegration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

type countingReloader struct{ n int }

func (r *countingReloader) Reload(context.Context) { r.n++ }

func TestScheduleHandler_Get(t *testing.T) {
	ctx := context.Background()
	loc := jkt(t)
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, loc)
	cfg := ScheduleConfig{Enabled: true, Cron: "0 0 2 5 * *", Timezone: "Asia/Jakarta"}

	t.Run("permission_denied", func(t *testing.T) {
		_, err := NewScheduleHandler(&memScheduleRepo{}, cfg, nil).Get(ctx, false)
		require.ErrorIs(t, err, ErrScheduleViewDenied)
	})
	t.Run("no_row_uses_config", func(t *testing.T) {
		v, err := NewScheduleHandler(&memScheduleRepo{}, cfg, nil).WithClock(func() time.Time { return now }).Get(ctx, true)
		require.NoError(t, err)
		assert.Nil(t, v.Setting)
		assert.Equal(t, ScheduleSourceConfigCron, v.Effective.Source)
		require.NotNil(t, v.NextRun)
		assert.Equal(t, time.Date(2026, 10, 5, 2, 0, 0, 0, loc), *v.NextRun)
	})
	t.Run("seeded_disabled_row_falls_back", func(t *testing.T) {
		repo := &memScheduleRepo{row: &domain.ScheduleSetting{Timezone: "Asia/Jakarta"}}
		v, err := NewScheduleHandler(repo, cfg, nil).WithClock(func() time.Time { return now }).Get(ctx, true)
		require.NoError(t, err)
		require.NotNil(t, v.Setting)
		assert.Equal(t, ScheduleSourceConfigCron, v.Effective.Source)
	})
	t.Run("invalid_config_reports_errors_no_next", func(t *testing.T) {
		bad := cfg
		bad.Cron = "0 2 5 * *"
		v, err := NewScheduleHandler(&memScheduleRepo{}, bad, nil).Get(ctx, true)
		require.NoError(t, err)
		assert.False(t, v.Effective.Enabled)
		assert.NotEmpty(t, v.Effective.Errors)
		assert.Nil(t, v.NextRun)
	})
	t.Run("repo_error", func(t *testing.T) {
		_, err := NewScheduleHandler(&memScheduleRepo{err: errors.New("down")}, cfg, nil).Get(ctx, true)
		require.Error(t, err)
	})
	t.Run("repo_nil", func(t *testing.T) {
		_, err := NewScheduleHandler(nil, cfg, nil).Get(ctx, true)
		require.ErrorIs(t, err, ErrScheduleNotConfigured)
	})
}

func TestScheduleHandler_Update(t *testing.T) {
	ctx := context.Background()
	loc := jkt(t)
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, loc)
	cfg := ScheduleConfig{Enabled: true, Cron: "0 0 2 5 * *", Timezone: "Asia/Jakarta"}

	t.Run("permission_denied_writes_nothing", func(t *testing.T) {
		repo, audit := &memScheduleRepo{}, &recordingAudit{}
		_, err := NewScheduleHandler(repo, cfg, audit).Update(ctx, UpdateScheduleCommand{
			Setting: domain.ScheduleSetting{Enabled: true, Mode: domain.ScheduleEndOfMonth}, Actor: "alice",
		})
		require.ErrorIs(t, err, ErrScheduleUpdateDenied)
		assert.Zero(t, repo.saves)
		assert.Empty(t, audit.inputs)
	})
	t.Run("invalid_rejected_writes_nothing", func(t *testing.T) {
		bad := []domain.ScheduleSetting{
			{Enabled: true, Mode: domain.ScheduleCron, Cron: "0 2 5 * *"},
			{Enabled: true, Mode: domain.ScheduleDayOfMonth, DayOfMonth: 32},
			{Enabled: true, Mode: domain.ScheduleDayOfMonth},
			{Enabled: true, Mode: domain.ScheduleSpecificDate, RunDate: "20261001"},
			{Enabled: true, Mode: domain.ScheduleEndOfMonth, RunTime: "24:00"},
			{Enabled: true, Mode: domain.ScheduleEndOfMonth, Timezone: "Nowhere/City"},
			{Enabled: true},
			{Mode: "HOURLY"},
		}
		for _, s := range bad {
			repo, audit := &memScheduleRepo{}, &recordingAudit{}
			_, err := NewScheduleHandler(repo, cfg, audit).Update(ctx, UpdateScheduleCommand{Setting: s, Actor: "alice", HasPermission: true})
			require.ErrorIs(t, err, domain.ErrInvalidSchedule, "%+v", s)
			assert.Zero(t, repo.saves)
			assert.Empty(t, audit.inputs)
		}
	})
	t.Run("actor_required", func(t *testing.T) {
		_, err := NewScheduleHandler(&memScheduleRepo{}, cfg, nil).Update(ctx, UpdateScheduleCommand{
			Setting: domain.ScheduleSetting{Mode: domain.ScheduleEndOfMonth}, HasPermission: true,
		})
		require.ErrorIs(t, err, domain.ErrActorRequired)
	})
	t.Run("valid_saves_audits_reloads", func(t *testing.T) {
		repo := &memScheduleRepo{row: &domain.ScheduleSetting{Timezone: "Asia/Jakarta"}}
		audit, rl := &recordingAudit{}, &countingReloader{}
		h := NewScheduleHandler(repo, cfg, audit).WithReloader(rl).WithClock(func() time.Time { return now })
		v, err := h.Update(ctx, UpdateScheduleCommand{
			Setting: domain.ScheduleSetting{
				Enabled: true, Mode: " day_of_month ", DayOfMonth: 31, RunTime: "04:00",
				Cron: "ignored", RunDate: "2026-01-01",
			},
			Actor: "alice", HasPermission: true,
		})
		require.NoError(t, err)
		assert.Equal(t, 1, repo.saves)
		assert.Equal(t, 1, rl.n)
		require.NotNil(t, repo.row)
		assert.Equal(t, domain.ScheduleDayOfMonth, repo.row.Mode)
		assert.Empty(t, repo.row.Cron, "fields the mode ignores are cleared")
		assert.Empty(t, repo.row.RunDate)
		assert.Equal(t, "Asia/Jakarta", repo.row.Timezone)
		assert.Equal(t, ScheduleSourceSettings, v.Effective.Source)
		require.NotNil(t, v.NextRun)
		assert.Equal(t, time.Date(2026, 10, 31, 4, 0, 0, 0, loc), *v.NextRun)

		require.Len(t, audit.inputs, 1)
		in := audit.inputs[0]
		assert.Equal(t, "cst_erp_integration_setting", in.EntityType)
		assert.Equal(t, int64(1), in.EntityID)
		assert.Equal(t, auditdomain.OpUpdate, in.Operation)
		assert.Equal(t, "alice", in.UserID)
		require.NoError(t, in.Validate())
		assert.Contains(t, in.AfterData, `"mode":"DAY_OF_MONTH"`)
		assert.Contains(t, in.AfterData, AuditEventScheduleUpdate)
		assert.Contains(t, in.BeforeData, `"enabled":false`)
	})
	t.Run("audit_failure_is_best_effort", func(t *testing.T) {
		repo := &memScheduleRepo{}
		_, err := NewScheduleHandler(repo, cfg, &recordingAudit{err: errors.New("x")}).Update(ctx, UpdateScheduleCommand{
			Setting: domain.ScheduleSetting{Mode: domain.ScheduleEndOfMonth}, Actor: "alice", HasPermission: true,
		})
		require.NoError(t, err)
		assert.Equal(t, 1, repo.saves)
	})
}
