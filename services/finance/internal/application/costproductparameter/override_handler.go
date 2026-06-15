package costproductparameter

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	cpp "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
	pginfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// ErrRouteLocked is returned when the linked route is locked and param override is requested.
var ErrRouteLocked = errors.New("route is locked — unlock before editing param values")

// RouteLockReader checks whether the route linked to a CPR is currently locked.
type RouteLockReader interface {
	IsLinkedRouteLocked(ctx context.Context, requestID int64) (bool, error)
}

// FillTaskApprovalResetter resets fill-task status to APPROVAL_PENDING for a level
// that has an approver configured, after param values are overridden.
type FillTaskApprovalResetter interface {
	ResetFillTaskApprovalIfNeeded(ctx context.Context, requestID int64, routeLevel int) error
}

// OverrideNotifier emits a single notification summarizing the override.
type OverrideNotifier interface {
	NotifyParamOverride(ctx context.Context, requestID int64, routeLevel int, changedCodes []string, actorID, actorName string) error
}

// OverrideParamItem is one value to override.
type OverrideParamItem struct {
	ProductSysID int64
	ParamID      uuid.UUID
	ValueNumeric *string
	ValueText    *string
	ValueFlag    *bool
}

// OverrideCommand bundles all inputs for one override call.
type OverrideCommand struct {
	RequestID  int64
	RouteLevel int
	Items      []OverrideParamItem
	ActorID    string
	ActorName  string
}

// OverrideParamValuesHandler handles admin overriding of param values before route lock.
type OverrideParamValuesHandler struct {
	repo         cpp.Repository
	lockReader   RouteLockReader
	editLogRepo  *pginfra.CostParamEditLogRepository
	taskResetter FillTaskApprovalResetter
	notifier     OverrideNotifier
}

// NewOverrideParamValuesHandler constructs the handler.
func NewOverrideParamValuesHandler(
	repo cpp.Repository,
	lockReader RouteLockReader,
	editLogRepo *pginfra.CostParamEditLogRepository,
) *OverrideParamValuesHandler {
	return &OverrideParamValuesHandler{
		repo:        repo,
		lockReader:  lockReader,
		editLogRepo: editLogRepo,
	}
}

// WithTaskResetter attaches an optional fill-task approval resetter.
func (h *OverrideParamValuesHandler) WithTaskResetter(r FillTaskApprovalResetter) *OverrideParamValuesHandler {
	h.taskResetter = r
	return h
}

// WithNotifier attaches an optional notification emitter.
func (h *OverrideParamValuesHandler) WithNotifier(n OverrideNotifier) *OverrideParamValuesHandler {
	h.notifier = n
	return h
}

// Handle executes the override: lock-check → old-value capture → upsert → audit → approval-reset → notify.
func (h *OverrideParamValuesHandler) Handle(ctx context.Context, cmd OverrideCommand) (int, error) {
	if cmd.RequestID <= 0 {
		return 0, fmt.Errorf("invalid request_id")
	}
	if cmd.RouteLevel < 1 {
		return 0, fmt.Errorf("invalid route_level")
	}
	if len(cmd.Items) == 0 {
		return 0, fmt.Errorf("no items provided")
	}

	// 1. Reject if the route is already locked.
	locked, err := h.lockReader.IsLinkedRouteLocked(ctx, cmd.RequestID)
	if err != nil {
		return 0, fmt.Errorf("check route lock: %w", err)
	}
	if locked {
		return 0, ErrRouteLocked
	}

	// 2. Capture old values and param codes for the audit log.
	type changeRecord struct {
		paramCode string
		oldVal    string
		newVal    string
	}
	changes := make([]changeRecord, 0, len(cmd.Items))
	for _, item := range cmd.Items {
		code, codeErr := h.repo.GetParamCodeByID(ctx, item.ParamID)
		if codeErr != nil {
			code = item.ParamID.String()
		}
		oldVal, _ := h.repo.GetCurrentValueAsText(ctx, item.ProductSysID, item.ParamID)
		changes = append(changes, changeRecord{
			paramCode: code,
			oldVal:    oldVal,
			newVal:    overrideNewValueText(item),
		})
	}

	// 3. Upsert new values (reuse Upsert to keep CAPP validation consistent).
	var count int
	for i, item := range cmd.Items {
		v := &cpp.Value{
			ProductSysID: item.ProductSysID,
			ParamID:      item.ParamID,
			ValueNumeric: item.ValueNumeric,
			ValueText:    item.ValueText,
			ValueFlag:    item.ValueFlag,
			FilledBy:     cmd.ActorID,
			CreatedBy:    cmd.ActorID,
		}
		if upsertErr := h.repo.Upsert(ctx, v); upsertErr != nil {
			log.Warn().Err(upsertErr).Int64("request_id", cmd.RequestID).
				Str("param_code", changes[i].paramCode).
				Msg("override: upsert failed, skipping item")
			continue
		}
		count++
	}

	// 4. Record audit log entries — only for params whose value actually changed.
	logEntries := make([]pginfra.ParamEditLogEntry, 0, len(changes))
	for _, c := range changes {
		if c.oldVal == c.newVal {
			continue
		}
		logEntries = append(logEntries, pginfra.ParamEditLogEntry{
			RequestID:  cmd.RequestID,
			RouteLevel: cmd.RouteLevel,
			ParamCode:  c.paramCode,
			OldValue:   c.oldVal,
			NewValue:   c.newVal,
			ChangedBy:  cmd.ActorID,
		})
	}
	if logErr := h.editLogRepo.BulkInsert(ctx, logEntries); logErr != nil {
		log.Warn().Err(logErr).Int64("request_id", cmd.RequestID).
			Msg("override: audit log insert failed (non-blocking)")
	}

	// 5. If the fill task for this level had an approver, reset it to APPROVAL_PENDING.
	if h.taskResetter != nil {
		if resetErr := h.taskResetter.ResetFillTaskApprovalIfNeeded(ctx, cmd.RequestID, cmd.RouteLevel); resetErr != nil {
			log.Warn().Err(resetErr).Int64("request_id", cmd.RequestID).
				Msg("override: fill task approval reset failed (non-blocking)")
		}
	}

	// 6. Send one notification listing all changed param codes.
	if h.notifier != nil {
		codes := make([]string, len(changes))
		for i, c := range changes {
			codes[i] = c.paramCode
		}
		if notifErr := h.notifier.NotifyParamOverride(ctx, cmd.RequestID, cmd.RouteLevel, codes, cmd.ActorID, cmd.ActorName); notifErr != nil {
			log.Warn().Err(notifErr).Int64("request_id", cmd.RequestID).
				Msg("override: notification failed (non-blocking)")
		}
	}

	return count, nil
}

// overrideNewValueText converts the incoming value to a human-readable string for the audit log.
func overrideNewValueText(item OverrideParamItem) string {
	if item.ValueNumeric != nil {
		return *item.ValueNumeric
	}
	if item.ValueText != nil {
		return *item.ValueText
	}
	if item.ValueFlag != nil {
		if *item.ValueFlag {
			return "true"
		}
		return "false"
	}
	return ""
}
