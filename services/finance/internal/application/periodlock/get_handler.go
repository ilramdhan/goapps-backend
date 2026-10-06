package periodlock

import (
	"context"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/periodlock"
)

// GetQuery reads the lock row for (period, calc type).
type GetQuery struct {
	Period   string
	CalcType string // "" defaults to ACTUAL
}

// GetHandler returns the lock row (locked or soft-unlocked), or
// domain.ErrLockNotFound when the period was never locked.
type GetHandler struct{ repo domain.Repository }

// NewGetHandler builds the handler.
func NewGetHandler(repo domain.Repository) *GetHandler { return &GetHandler{repo: repo} }

// Handle reads the lock.
func (h *GetHandler) Handle(ctx context.Context, q GetQuery) (*domain.PeriodLock, error) {
	calcType := q.CalcType
	if calcType == "" {
		calcType = domain.CalcTypeActual
	}
	if err := domain.ValidatePeriod(q.Period); err != nil {
		return nil, err
	}
	if err := domain.ValidateCalcType(calcType); err != nil {
		return nil, err
	}
	return h.repo.Get(ctx, q.Period, calcType)
}
