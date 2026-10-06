package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// ErpIntegrationSettingRepository implements
// erpintegration.ScheduleSettingRepository over the single
// cst_erp_integration_setting row (migration 000558; plan-06 P5-T11).
type ErpIntegrationSettingRepository struct{ db *DB }

// NewErpIntegrationSettingRepository constructs the repository.
func NewErpIntegrationSettingRepository(db *DB) *ErpIntegrationSettingRepository {
	return &ErpIntegrationSettingRepository{db: db}
}

var _ erpintegration.ScheduleSettingRepository = (*ErpIntegrationSettingRepository)(nil)

const erpSettingColumns = `ceis_schedule_enabled, COALESCE(ceis_schedule_mode, ''), COALESCE(ceis_run_day, 0),
	COALESCE(ceis_run_time, ''), COALESCE(TO_CHAR(ceis_run_date, 'YYYY-MM-DD'), ''), COALESCE(ceis_cron, ''),
	ceis_timezone, created_at, created_by, updated_at, COALESCE(updated_by, '')`

const selectErpSettingSQL = `SELECT ` + erpSettingColumns + ` FROM cst_erp_integration_setting WHERE ceis_setting_id = 1`

// upsertErpSettingSQL replaces the schedule fields of row 1 (inserting it if
// a DBA removed it). created_* are kept on update.
const upsertErpSettingSQL = `
	INSERT INTO cst_erp_integration_setting
		(ceis_setting_id, ceis_schedule_enabled, ceis_schedule_mode, ceis_run_day, ceis_run_time,
		 ceis_run_date, ceis_cron, ceis_timezone, created_by, updated_by, updated_at)
	VALUES (1, $1, $2, $3, $4, $5::date, $6, $7, $8, $8, NOW())
	ON CONFLICT (ceis_setting_id) DO UPDATE SET
		ceis_schedule_enabled = EXCLUDED.ceis_schedule_enabled,
		ceis_schedule_mode    = EXCLUDED.ceis_schedule_mode,
		ceis_run_day          = EXCLUDED.ceis_run_day,
		ceis_run_time         = EXCLUDED.ceis_run_time,
		ceis_run_date         = EXCLUDED.ceis_run_date,
		ceis_cron             = EXCLUDED.ceis_cron,
		ceis_timezone         = EXCLUDED.ceis_timezone,
		updated_by            = EXCLUDED.updated_by,
		updated_at            = NOW()
	RETURNING ` + erpSettingColumns

// GetSchedule returns the settings row or ErrScheduleSettingNotFound.
func (r *ErpIntegrationSettingRepository) GetSchedule(ctx context.Context) (erpintegration.ScheduleSetting, error) {
	s, err := scanErpSetting(r.db.QueryRowContext(ctx, selectErpSettingSQL))
	if errors.Is(err, sql.ErrNoRows) {
		return erpintegration.ScheduleSetting{}, erpintegration.ErrScheduleSettingNotFound
	}
	if err != nil {
		return erpintegration.ScheduleSetting{}, fmt.Errorf("erp setting get: %w", err)
	}
	return s, nil
}

// SaveSchedule upserts row 1 and returns it as stored.
func (r *ErpIntegrationSettingRepository) SaveSchedule(ctx context.Context, s erpintegration.ScheduleSetting, actor string) (erpintegration.ScheduleSetting, error) {
	tz := strings.TrimSpace(s.Timezone)
	if tz == "" {
		tz = "Asia/Jakarta"
	}
	out, err := scanErpSetting(r.db.QueryRowContext(ctx, upsertErpSettingSQL,
		s.Enabled, erpSettingNullStr(string(s.Mode)), erpSettingNullDay(s.DayOfMonth), erpSettingNullStr(s.RunTime),
		erpSettingNullStr(s.RunDate), erpSettingNullStr(s.Cron), tz, actor))
	if err != nil {
		return erpintegration.ScheduleSetting{}, fmt.Errorf("erp setting save: %w", err)
	}
	return out, nil
}

func scanErpSetting(row *sql.Row) (erpintegration.ScheduleSetting, error) {
	var (
		s    erpintegration.ScheduleSetting
		mode string
		day  int
		cAt  time.Time
		uAt  time.Time
	)
	if err := row.Scan(&s.Enabled, &mode, &day, &s.RunTime, &s.RunDate, &s.Cron,
		&s.Timezone, &cAt, &s.CreatedBy, &uAt, &s.UpdatedBy); err != nil {
		return erpintegration.ScheduleSetting{}, err
	}
	m, err := erpintegration.ParseScheduleMode(mode)
	if err != nil {
		return erpintegration.ScheduleSetting{}, err
	}
	s.Mode, s.DayOfMonth, s.CreatedAt, s.UpdatedAt = m, day, cAt, uAt
	return s, nil
}

func erpSettingNullStr(s string) sql.NullString {
	s = strings.TrimSpace(s)
	return sql.NullString{String: s, Valid: s != ""}
}

func erpSettingNullDay(d int) sql.NullInt16 {
	if d <= 0 || d > 31 {
		return sql.NullInt16{}
	}
	return sql.NullInt16{Int16: int16(d), Valid: true}
}
