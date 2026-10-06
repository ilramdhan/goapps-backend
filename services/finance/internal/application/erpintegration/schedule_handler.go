package erpintegration

// schedule_handler.go implements Get/UpdateErpIntegrationSchedule (plan-06
// P5-T11 step 5; design Part 2 §9.3). The RPC surface is P6: delivery passes
// the RBAC outcome (finance.cost.erpintegration.schedule.view / .update) as
// bools and maps domain.ErrInvalidSchedule to InvalidArgument. PUT edits only
// the cst_erp_integration_setting row; the config and the
// ERP_SCHEDULE_ENABLED master switch are never changed from here.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// Schedule audit identifiers. cal_entity_id is the fixed settings row id.
const (
	AuditEventScheduleUpdate = "ERP_SCHEDULE_UPDATE"
	auditEntityErpSetting    = "cst_erp_integration_setting"
	erpSettingRowID          = 1
)

// Schedule permission errors (G3; delivery maps them to PermissionDenied).
var (
	ErrScheduleViewDenied   = errors.New("erpintegration: permission denied to view the ERP schedule")
	ErrScheduleUpdateDenied = errors.New("erpintegration: permission denied to update the ERP schedule")
	// ErrScheduleNotConfigured is returned when no settings repository is wired.
	ErrScheduleNotConfigured = errors.New("erpintegration: schedule settings repository not configured")
)

// ScheduleView is the GetErpIntegrationSchedule result.
type ScheduleView struct {
	Effective ResolvedSchedule
	Setting   *domain.ScheduleSetting // nil when the row is absent
	NextRun   *time.Time              // nil when disabled or no future run
}

// UpdateScheduleCommand replaces the settings row.
type UpdateScheduleCommand struct {
	Setting       domain.ScheduleSetting
	Actor         string
	HasPermission bool
}

// ScheduleReloader is notified after a successful update so a running
// scheduler picks up the change without a restart (optional).
type ScheduleReloader interface {
	Reload(ctx context.Context)
}

// ScheduleHandler serves the schedule RPCs.
type ScheduleHandler struct {
	repo     domain.ScheduleSettingRepository
	cfg      ScheduleConfig
	audit    AuditSink
	reloader ScheduleReloader
	now      func() time.Time
}

// NewScheduleHandler builds the handler; audit may be nil.
func NewScheduleHandler(repo domain.ScheduleSettingRepository, cfg ScheduleConfig, audit AuditSink) *ScheduleHandler {
	return &ScheduleHandler{repo: repo, cfg: cfg, audit: audit, now: time.Now}
}

// WithReloader sets the scheduler to notify after an update.
func (h *ScheduleHandler) WithReloader(r ScheduleReloader) *ScheduleHandler {
	h.reloader = r
	return h
}

// WithClock overrides the clock (tests).
func (h *ScheduleHandler) WithClock(now func() time.Time) *ScheduleHandler {
	h.now = now
	return h
}

// Get returns the effective schedule, its source, validation errors and the
// next run.
func (h *ScheduleHandler) Get(ctx context.Context, hasPermission bool) (ScheduleView, error) {
	if err := checkPermission(hasPermission, ErrScheduleViewDenied); err != nil {
		return ScheduleView{}, err
	}
	setting, err := loadScheduleSetting(ctx, h.repo)
	if err != nil {
		return ScheduleView{}, err
	}
	return h.view(setting), nil
}

// Update validates and stores the settings row, emits the audit entry and
// reloads the scheduler. An invalid schedule is rejected (wraps
// domain.ErrInvalidSchedule) and nothing is written.
func (h *ScheduleHandler) Update(ctx context.Context, cmd UpdateScheduleCommand) (ScheduleView, error) {
	if err := checkPermission(cmd.HasPermission, ErrScheduleUpdateDenied); err != nil {
		return ScheduleView{}, err
	}
	actor := strings.TrimSpace(cmd.Actor)
	if actor == "" || len(actor) > 64 {
		return ScheduleView{}, domain.ErrActorRequired
	}
	if h.repo == nil {
		return ScheduleView{}, ErrScheduleNotConfigured
	}
	s := normalizeSetting(cmd.Setting)
	if err := ValidateScheduleSetting(s); err != nil {
		return ScheduleView{}, err
	}
	before, err := loadScheduleSetting(ctx, h.repo)
	if err != nil {
		return ScheduleView{}, err
	}
	saved, err := h.repo.SaveSchedule(ctx, s, actor)
	if err != nil {
		return ScheduleView{}, fmt.Errorf("save erp schedule: %w", err)
	}
	h.emitAudit(ctx, actor, before, saved)
	if h.reloader != nil {
		h.reloader.Reload(ctx)
	}
	return h.view(&saved), nil
}

func (h *ScheduleHandler) view(setting *domain.ScheduleSetting) ScheduleView {
	eff := ResolveSchedule(h.cfg, setting)
	v := ScheduleView{Effective: eff, Setting: setting}
	if next := eff.Next(h.now()); !next.IsZero() {
		v.NextRun = &next
	}
	return v
}

// loadScheduleSetting returns the row, or nil when it is absent.
func loadScheduleSetting(ctx context.Context, repo domain.ScheduleSettingRepository) (*domain.ScheduleSetting, error) {
	if repo == nil {
		return nil, ErrScheduleNotConfigured
	}
	s, err := repo.GetSchedule(ctx)
	switch {
	case errors.Is(err, domain.ErrScheduleSettingNotFound):
		return nil, nil //nolint:nilnil // an absent row is valid: the config schedule applies
	case err != nil:
		return nil, fmt.Errorf("load erp schedule: %w", err)
	default:
		return &s, nil
	}
}

// normalizeSetting trims strings and clears the fields the mode ignores, so
// the stored row holds only what is in force.
func normalizeSetting(s domain.ScheduleSetting) domain.ScheduleSetting {
	s.Mode = domain.ScheduleMode(strings.ToUpper(strings.TrimSpace(string(s.Mode))))
	s.RunTime = strings.TrimSpace(s.RunTime)
	s.RunDate = strings.TrimSpace(s.RunDate)
	s.Cron = strings.TrimSpace(s.Cron)
	s.Timezone = strings.TrimSpace(s.Timezone)
	if s.Timezone == "" {
		s.Timezone = DefaultScheduleTimezone
	}
	if s.Mode != domain.ScheduleDayOfMonth {
		s.DayOfMonth = 0
	}
	if s.Mode != domain.ScheduleSpecificDate {
		s.RunDate = ""
	}
	if s.Mode != domain.ScheduleCron {
		s.Cron = ""
	} else {
		s.RunTime = ""
	}
	if s.Mode == "" {
		s.RunTime = ""
	}
	return s
}

// scheduleAuditData is the cal_before_data / cal_after_data payload.
type scheduleAuditData struct {
	EventType  string `json:"event_type"`
	Enabled    bool   `json:"enabled"`
	Mode       string `json:"mode,omitempty"`
	DayOfMonth int    `json:"day_of_month,omitempty"`
	RunTime    string `json:"run_time,omitempty"`
	RunDate    string `json:"run_date,omitempty"`
	Cron       string `json:"cron,omitempty"`
	Timezone   string `json:"timezone,omitempty"`
}

func scheduleAuditJSON(s *domain.ScheduleSetting) string {
	if s == nil {
		return ""
	}
	raw, err := json.Marshal(scheduleAuditData{
		EventType: AuditEventScheduleUpdate, Enabled: s.Enabled, Mode: string(s.Mode),
		DayOfMonth: s.DayOfMonth, RunTime: s.RunTime, RunDate: s.RunDate, Cron: s.Cron, Timezone: s.Timezone,
	})
	if err != nil {
		return ""
	}
	return string(raw)
}

func (h *ScheduleHandler) emitAudit(ctx context.Context, actor string, before *domain.ScheduleSetting, after domain.ScheduleSetting) {
	if h.audit == nil {
		return
	}
	in := auditdomain.NewInput{
		EntityType: auditEntityErpSetting,
		EntityID:   erpSettingRowID,
		Operation:  auditdomain.OpUpdate,
		BeforeData: scheduleAuditJSON(before),
		AfterData:  scheduleAuditJSON(&after),
		UserID:     actor,
	}
	if err := h.audit.Emit(ctx, in); err != nil {
		// Best effort: the setting has already been stored.
		log.Warn().Err(err).Str("event", AuditEventScheduleUpdate).Msg("erp schedule update: audit emit failed")
	}
}
