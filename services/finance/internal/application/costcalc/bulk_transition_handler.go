package costcalc

import (
	"context"
	"errors"
	"fmt"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// BulkTransitionMaxRows caps one synchronous bulk verify / approve.
// ponytail: synchronous loop, one tx per row; move to an async job if 2000 rows
// ever take longer than the gateway deadline.
const BulkTransitionMaxRows = 2000

// bulkListPageSize matches the repository's ListResults page cap.
const bulkListPageSize = 500

// ErrBulkSelectionTooLarge is returned when a selection exceeds BulkTransitionMaxRows.
var ErrBulkSelectionTooLarge = fmt.Errorf("selection exceeds %d rows; narrow the filter", BulkTransitionMaxRows)

// ErrBulkPeriodRequired is returned when filter mode has no period.
var ErrBulkPeriodRequired = errors.New("period is required when no cost ids are given")

// BulkSelection picks rows by explicit ids, or (when CostIDs is empty) by the
// list filter restricted to the transition's source status.
type BulkSelection struct {
	CostIDs        []int64
	Period         string
	CalcType       costcalcdom.CalculationType
	Search         string
	ProductTypeIDs []int32
	ShadeCodes     []string
	RMGroupCodes   []string
}

// BulkFailure is one refused row.
type BulkFailure struct {
	CostID      int64
	ProductCode string
	Err         error
}

// BulkResult summarizes a bulk transition.
type BulkResult struct {
	OK       int
	Failures []BulkFailure
}

// BulkTransitionHandler applies the single-row verify or approve handler to
// many rows. Each row keeps its own rules and transaction; a failure is
// recorded and the loop continues.
type BulkTransitionHandler struct {
	svc          *Service
	sourceStatus string
	apply        func(ctx context.Context, costID int64, actor string) error
}

// NewBulkVerifyHandler wraps the single-row verify handler (CALCULATED → VERIFIED).
func NewBulkVerifyHandler(svc *Service, verify *VerifyCostHandler) *BulkTransitionHandler {
	return &BulkTransitionHandler{svc: svc, sourceStatus: "CALCULATED",
		apply: func(ctx context.Context, id int64, actor string) error {
			return verify.Handle(ctx, VerifyCostCommand{CostID: id, Actor: actor})
		}}
}

// NewBulkApproveHandler wraps the single-row approve handler (VERIFIED → APPROVED).
func NewBulkApproveHandler(svc *Service, approve *ApproveCostHandler) *BulkTransitionHandler {
	return &BulkTransitionHandler{svc: svc, sourceStatus: "VERIFIED",
		apply: func(ctx context.Context, id int64, actor string) error {
			return approve.Handle(ctx, ApproveCostCommand{CostID: id, Actor: actor})
		}}
}

// Handle resolves the selection and transitions every row.
func (h *BulkTransitionHandler) Handle(ctx context.Context, sel BulkSelection, actor string) (*BulkResult, error) {
	if actor == "" {
		return nil, errors.New(errMsgActorRequired)
	}
	ids, codes, err := h.resolve(ctx, sel)
	if err != nil {
		return nil, err
	}
	out := &BulkResult{}
	for i, id := range ids {
		if ctxErr := ctx.Err(); ctxErr != nil {
			for _, rest := range ids[i:] {
				out.Failures = append(out.Failures, BulkFailure{CostID: rest, ProductCode: codes[rest], Err: ctxErr})
			}
			break
		}
		if err := h.apply(ctx, id, actor); err != nil {
			out.Failures = append(out.Failures, BulkFailure{CostID: id, ProductCode: codes[id], Err: err})
			continue
		}
		out.OK++
	}
	return out, nil
}

// resolve returns the ids to process (input order, de-duplicated) plus product
// codes when the filter path knows them.
func (h *BulkTransitionHandler) resolve(ctx context.Context, sel BulkSelection) ([]int64, map[int64]string, error) {
	if len(sel.CostIDs) > 0 {
		ids, err := dedupCostIDs(sel.CostIDs)
		return ids, map[int64]string{}, err
	}
	if sel.Period == "" {
		return nil, nil, ErrBulkPeriodRequired
	}
	return h.resolveByFilter(ctx, sel)
}

func dedupCostIDs(in []int64) ([]int64, error) {
	seen := make(map[int64]struct{}, len(in))
	ids := make([]int64, 0, len(in))
	for _, id := range in {
		if id <= 0 {
			return nil, errors.New(errMsgCostIDPositive)
		}
		if _, dup := seen[id]; !dup {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	if len(ids) > BulkTransitionMaxRows {
		return nil, ErrBulkSelectionTooLarge
	}
	return ids, nil
}

// resolveByFilter pages the list query restricted to the source status.
func (h *BulkTransitionHandler) resolveByFilter(ctx context.Context, sel BulkSelection) ([]int64, map[int64]string, error) {
	f := costcalcdom.ResultListFilter{
		Period: sel.Period, CalcType: sel.CalcType, Status: h.sourceStatus, Search: sel.Search,
		ProductTypeIDs: sel.ProductTypeIDs, ShadeCodes: sel.ShadeCodes, RMGroupCodes: sel.RMGroupCodes,
		PageSize: bulkListPageSize,
	}
	var ids []int64
	codes := map[int64]string{}
	for page := 1; ; page++ {
		f.Page = page
		items, total, _, err := h.svc.resultRepo.ListResults(ctx, f)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve bulk selection: %w", err)
		}
		if total > BulkTransitionMaxRows {
			return nil, nil, ErrBulkSelectionTooLarge
		}
		for _, s := range items {
			ids = append(ids, s.CostID)
			codes[s.CostID] = s.ProductCode
		}
		if len(items) < bulkListPageSize || len(ids) >= total {
			return ids, codes, nil
		}
	}
}
