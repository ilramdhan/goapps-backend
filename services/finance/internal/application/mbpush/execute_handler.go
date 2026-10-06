package mbpush

import (
	"context"
	"database/sql"
	"fmt"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbpushlog"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

const pushCostTypesLabel = "ACTUAL,SELLING,FORECAST"

// PushError records a single MB's failure during an otherwise-successful batch.
type PushError struct {
	MBHID string
	Error string
}

// ExecuteResult summarizes a push-to-head batch execution.
type ExecuteResult struct {
	Period    string
	MBCount   int32
	RowCount  int32
	Errors    []PushError
	PushLogID string
}

// ExecuteHandler executes a push-to-head batch: for each requested MB Head, upserts its 3
// CALCULATED cost types into cst_mb_cost and flips the source cst_product_cost rows to APPROVED,
// all inside one advisory-locked transaction (PR-06), with per-MB savepoint isolation so one MB's
// failure does not abort the whole batch.
type ExecuteHandler struct {
	db           *postgres.DB
	mbHeadReader MBHeadReader
	costReader   CostReader
	mbCostWriter MBCostWriter
	pushLogRepo  mbpushlog.Repository
	periodLock   costcalcdom.PeriodLockChecker
}

// ExecuteOption customizes the handler at construction.
type ExecuteOption func(*ExecuteHandler)

// WithPeriodLock installs the period-lock guard (design §5.5, plan-02 P1-T3).
// A push flips the period's ACTUAL cst_product_cost rows to APPROVED, so a
// locked (period, ACTUAL) refuses the whole push with ErrPeriodLocked: once
// before any read or write, and again inside the push transaction under a
// FOR SHARE row lock, so a lock taken concurrently cannot slip through.
// Omitting it (nil) keeps the pre-lock behavior exactly; tests omit it.
func WithPeriodLock(c costcalcdom.PeriodLockChecker) ExecuteOption {
	return func(h *ExecuteHandler) { h.periodLock = c }
}

// NewExecuteHandler constructs an ExecuteHandler.
func NewExecuteHandler(db *postgres.DB, mbHeadReader MBHeadReader, costReader CostReader, mbCostWriter MBCostWriter, pushLogRepo mbpushlog.Repository, opts ...ExecuteOption) *ExecuteHandler {
	h := &ExecuteHandler{
		db:           db,
		mbHeadReader: mbHeadReader,
		costReader:   costReader,
		mbCostWriter: mbCostWriter,
		pushLogRepo:  pushLogRepo,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Execute runs the push-to-head batch for period across mbhIDs, per PR-01 (bulk-only) and PR-03
// (idempotent re-push via UPSERT).
func (h *ExecuteHandler) Execute(ctx context.Context, period string, mbhIDs []string, actorUserID string) (*ExecuteResult, error) {
	if err := costcalcdom.CheckPeriodUnlocked(ctx, h.periodLock, period, costcalcdom.CalcTypeActual); err != nil {
		return nil, fmt.Errorf("push to head period %s: %w", period, err)
	}
	candidates, err := h.mbHeadReader.ListValidated(ctx)
	if err != nil {
		return nil, fmt.Errorf("list validated mb heads: %w", err)
	}
	byID := make(map[string]MBHeadCandidate, len(candidates))
	for _, c := range candidates {
		byID[c.MBHID] = c
	}

	result := &ExecuteResult{Period: period}
	err = h.db.Transaction(ctx, func(tx *sql.Tx) error {
		return h.executeBatch(ctx, tx, byID, mbhIDs, period, actorUserID, result)
	})
	if err != nil {
		return nil, fmt.Errorf("execute push to head: %w", err)
	}
	return result, nil
}

func (h *ExecuteHandler) executeBatch(ctx context.Context, tx *sql.Tx, byID map[string]MBHeadCandidate, mbhIDs []string, period, actorUserID string, result *ExecuteResult) error {
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, period); err != nil {
		return fmt.Errorf("acquire push lock for period %s: %w", period, err)
	}
	if err := h.checkLockInTx(ctx, tx, period); err != nil {
		return err
	}
	for _, mbhID := range mbhIDs {
		c, ok := byID[mbhID]
		if !ok {
			result.Errors = append(result.Errors, PushError{MBHID: mbhID, Error: "not in VALIDATED set (race with preview)"})
			continue
		}
		if pushErr := h.pushOneMB(ctx, tx, c, period, actorUserID); pushErr != nil {
			result.Errors = append(result.Errors, PushError{MBHID: mbhID, Error: pushErr.Error()})
			continue
		}
		result.MBCount++
		result.RowCount += 3
	}
	if result.MBCount == 0 {
		return nil
	}
	entity, err := mbpushlog.NewEntity(period, actorUserID, result.MBCount, result.RowCount, pushCostTypesLabel)
	if err != nil {
		return fmt.Errorf("build push log entity: %w", err)
	}
	if err := h.pushLogRepo.Create(ctx, entity); err != nil {
		return fmt.Errorf("create push log: %w", err)
	}
	result.PushLogID = entity.ID()
	return nil
}

func (h *ExecuteHandler) pushOneMB(ctx context.Context, tx *sql.Tx, c MBHeadCandidate, period, actorUserID string) error {
	const savepoint = "sp_mb_push"
	if _, err := tx.ExecContext(ctx, "SAVEPOINT "+savepoint); err != nil {
		return fmt.Errorf("savepoint: %w", err)
	}
	if err := h.pushOneMBInner(ctx, tx, c, period, actorUserID); err != nil {
		if _, rbErr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+savepoint); rbErr != nil {
			return fmt.Errorf("rollback to savepoint after %w: %w", err, rbErr)
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT "+savepoint); err != nil {
		return fmt.Errorf("release savepoint: %w", err)
	}
	return nil
}

func (h *ExecuteHandler) pushOneMBInner(ctx context.Context, tx *sql.Tx, c MBHeadCandidate, period, actorUserID string) error {
	for _, costType := range []string{"ACTUAL", "SELLING", "FORECAST"} {
		costID, costValue, found, err := h.costReader.GetActiveCalculatedTx(ctx, tx, c.CostProductID, period, costType)
		if err != nil {
			return fmt.Errorf("get %s cost: %w", costType, err)
		}
		if !found {
			return fmt.Errorf("no CALCULATED %s cost for period %s (race with preview)", costType, period)
		}
		if err := h.mbCostWriter.Upsert(ctx, tx, c.MBHID, period, costType, costValue, costID, actorUserID); err != nil {
			return fmt.Errorf("upsert %s: %w", costType, err)
		}
		if err := h.costReader.MarkApprovedFromCalculatedTx(ctx, tx, costID, actorUserID); err != nil {
			return fmt.Errorf("mark %s approved: %w", costType, err)
		}
	}
	return nil
}

// checkLockInTx re-checks the period lock inside the push transaction with
// FOR SHARE, closing the window between the pre-check in Execute and a lock
// taken concurrently. It runs only when the guard is installed, so an
// unguarded handler issues exactly the pre-lock SQL.
func (h *ExecuteHandler) checkLockInTx(ctx context.Context, tx *sql.Tx, period string) error {
	if h.periodLock == nil {
		return nil
	}
	locked, err := postgres.IsPeriodLockedForShare(ctx, tx, period, string(costcalcdom.CalcTypeActual))
	if err != nil {
		return err
	}
	if locked {
		return fmt.Errorf("push to head period %s: %w", period, costcalcdom.ErrPeriodLocked)
	}
	return nil
}
