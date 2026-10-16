package lookupsyncjob

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// ProductReader resolves product codes for the failures list (best-effort).
type ProductReader interface {
	GetBySysID(ctx context.Context, sysID int64) (*costproductmaster.CostProductMaster, error)
}

// Status is the progress of one lookup sync parent job.
type Status struct {
	Execution *job.Execution
	// SkippedChildren counts completed children whose product was skipped at
	// apply time (locked / vanished) — a skip is not a failure.
	SkippedChildren int
	// ValuesWritten is the total of values set plus values cleared.
	ValuesWritten int
}

// Failure is one hard-failed child product.
type Failure struct {
	ProductSysID int64
	ProductCode  string
	ErrorMessage string
}

// Tracker reads lookup sync job status and failures.
type Tracker struct {
	jobRepo  job.Repository
	products ProductReader
}

// NewTracker constructs the tracker; products may be nil.
func NewTracker(jobRepo job.Repository, products ProductReader) *Tracker {
	return &Tracker{jobRepo: jobRepo, products: products}
}

// Status returns the parent job plus skip / values tallies from its children.
func (t *Tracker) Status(ctx context.Context, jobID string) (Status, error) {
	id, err := uuid.Parse(jobID)
	if err != nil {
		return Status{}, fmt.Errorf("%w: %q", ErrInvalidJobID, jobID)
	}
	parent, err := t.jobRepo.GetByID(ctx, id)
	if err != nil {
		return Status{}, fmt.Errorf("load lookup sync job: %w", err)
	}
	children, err := t.jobRepo.ListChildren(ctx, id)
	if err != nil {
		return Status{}, fmt.Errorf("list lookup sync children: %w", err)
	}
	st := Status{Execution: parent}
	for _, c := range children {
		if c.Status() != job.StatusSuccess || len(c.ResultSummary()) == 0 {
			continue
		}
		var r ChildResult
		if json.Unmarshal(c.ResultSummary(), &r) != nil {
			continue
		}
		if r.Skipped {
			st.SkippedChildren++
		}
		st.ValuesWritten += r.ValuesWritten + r.ValuesCleared
	}
	return st, nil
}

// Failures lists every hard-failed child (skips are not failures).
func (t *Tracker) Failures(ctx context.Context, jobID string) ([]Failure, error) {
	id, err := uuid.Parse(jobID)
	if err != nil {
		return nil, fmt.Errorf("%w: %q", ErrInvalidJobID, jobID)
	}
	if _, err := t.jobRepo.GetByID(ctx, id); err != nil {
		return nil, fmt.Errorf("load lookup sync job: %w", err)
	}
	children, err := t.jobRepo.ListChildren(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list lookup sync children: %w", err)
	}
	out := make([]Failure, 0)
	for _, c := range children {
		if c.Status() != job.StatusFailed {
			continue
		}
		out = append(out, t.toFailure(ctx, c))
	}
	return out, nil
}

func (t *Tracker) toFailure(ctx context.Context, c *job.Execution) Failure {
	f := Failure{ErrorMessage: c.ErrorMessage()}
	var p ChildParams
	if json.Unmarshal(c.Params(), &p) != nil {
		return f
	}
	f.ProductSysID = p.ProductSysID
	if t.products == nil || p.ProductSysID == 0 {
		return f
	}
	if pm, err := t.products.GetBySysID(ctx, p.ProductSysID); err == nil && pm != nil {
		f.ProductCode = pm.ProductCode()
	}
	return f
}

// StatusString maps a parent job onto QUEUED/PROCESSING/DONE/FAILED/PARTIAL.
func StatusString(exec *job.Execution) string {
	switch exec.Status() {
	case job.StatusSuccess:
		if exec.FailedChildren() > 0 {
			return "PARTIAL"
		}
		return "DONE"
	case job.StatusFailed:
		return "FAILED"
	case job.StatusQueued, job.StatusProcessing, job.StatusCancelled:
		return string(exec.Status())
	default:
		return string(exec.Status())
	}
}
