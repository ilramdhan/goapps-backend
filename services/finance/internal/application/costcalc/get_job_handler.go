package costcalc

import (
	"context"
	"errors"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// GetJobQuery carries inputs for fetching a single calc job by ID.
type GetJobQuery struct {
	JobID int64
}

// GetJobHandler fetches one calc job by id.
type GetJobHandler struct {
	svc *Service
}

// NewGetJobHandler constructs the handler.
func NewGetJobHandler(svc *Service) *GetJobHandler {
	return &GetJobHandler{svc: svc}
}

// Handle executes the query.
func (h *GetJobHandler) Handle(ctx context.Context, q GetJobQuery) (*costcalcdom.Job, error) {
	if q.JobID <= 0 {
		return nil, errors.New(errMsgJobIDPositive)
	}
	return h.svc.jobRepo.GetByID(ctx, q.JobID)
}
