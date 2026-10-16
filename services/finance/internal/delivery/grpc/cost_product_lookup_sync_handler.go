package grpc

import (
	"context"
	"errors"
	"fmt"
	"sort"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/lookupsync"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/lookupsyncjob"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

const (
	lookupSyncDefaultPageSize = 20
	lookupSyncMaxPageSize     = 100
	lookupSyncEnqueueMissing  = "lookup sync queue unavailable"
	lookupSyncTrackerMissing  = "lookup sync job tracking not implemented"
	lookupSyncDriftMessage    = "the plan changed since the preview (master or product data was modified); re-run the preview and confirm again"
)

// LookupSyncJobInfo describes a queued/running lookup sync job.
type LookupSyncJobInfo struct {
	JobID             string
	JobCode           string
	Status            string // QUEUED / PROCESSING / DONE / FAILED / PARTIAL
	TotalChildren     int
	CompletedChildren int
	FailedChildren    int
}

// LookupSyncJobStatus is the status of a lookup sync job.
type LookupSyncJobStatus struct {
	Info            LookupSyncJobInfo
	SkippedChildren int
	ValuesWritten   int
}

// LookupSyncJobFailure is one failed product of a lookup sync job.
type LookupSyncJobFailure struct {
	ProductSysID int64
	ProductCode  string
	ErrorMessage string
}

// LookupSyncEnqueuer queues the background apply job (implemented in P6).
type LookupSyncEnqueuer interface {
	EnqueueLookupSync(ctx context.Context, scope lookupsync.Scope, fingerprint, actor string) (LookupSyncJobInfo, error)
}

// LookupSyncJobTracker reads job progress (implemented in P6).
type LookupSyncJobTracker interface {
	GetLookupSyncJobStatus(ctx context.Context, jobID string) (LookupSyncJobStatus, error)
	ListLookupSyncJobFailures(ctx context.Context, jobID string) ([]LookupSyncJobFailure, error)
}

// LookupSyncPlanner is the subset of *lookupsync.Planner the handler uses.
type LookupSyncPlanner interface {
	Walk(ctx context.Context, scope lookupsync.Scope, visit func(lookupsync.Item) error) (lookupsync.Summary, error)
}

// CostProductLookupSyncHandler implements financev1.CostProductLookupSyncServiceServer.
type CostProductLookupSyncHandler struct {
	financev1.UnimplementedCostProductLookupSyncServiceServer
	planner  LookupSyncPlanner
	enqueuer LookupSyncEnqueuer
	tracker  LookupSyncJobTracker
}

// NewCostProductLookupSyncHandler constructs the handler; planner is required.
func NewCostProductLookupSyncHandler(planner LookupSyncPlanner) (*CostProductLookupSyncHandler, error) {
	if planner == nil {
		return nil, errors.New("lookup sync planner is required")
	}
	return &CostProductLookupSyncHandler{planner: planner}, nil
}

// WithEnqueuer attaches the apply-job enqueuer.
func (h *CostProductLookupSyncHandler) WithEnqueuer(e LookupSyncEnqueuer) *CostProductLookupSyncHandler {
	h.enqueuer = e
	return h
}

// WithJobTracker attaches the job status/failures reader.
func (h *CostProductLookupSyncHandler) WithJobTracker(t LookupSyncJobTracker) *CostProductLookupSyncHandler {
	h.tracker = t
	return h
}

// lookupSyncScopeFromProto maps the proto scope onto lookupsync.Scope.
func lookupSyncScopeFromProto(s *financev1.LookupSyncScope) (lookupsync.Scope, error) {
	switch v := s.GetScope().(type) {
	case *financev1.LookupSyncScope_MasterRef:
		ref := v.MasterRef
		if ref.GetLookupMasterCode() == "" || ref.GetMasterKey() == "" {
			return lookupsync.Scope{}, errors.New("master_ref requires lookup_master_code and master_key")
		}
		return lookupsync.Scope{MasterRef: &lookupsync.MasterRef{
			LookupMasterCode: ref.GetLookupMasterCode(), MasterKey: ref.GetMasterKey(),
		}}, nil
	case *financev1.LookupSyncScope_ProductIds:
		ids := v.ProductIds.GetProductSysIds()
		if len(ids) == 0 {
			return lookupsync.Scope{}, errors.New("product_ids must not be empty")
		}
		return lookupsync.Scope{ProductSysIDs: ids}, nil
	case *financev1.LookupSyncScope_Filter:
		f := v.Filter
		return lookupsync.Scope{Filter: &lookupsync.ProductFilter{
			Search: f.GetSearch(), ProductTypeIDs: f.GetProductTypeIds(),
			ShadeCode: f.GetShadeCode(), ActiveFilter: f.GetActiveFilter(),
		}}, nil
	default:
		return lookupsync.Scope{}, errors.New("scope is required (master_ref, product_ids or filter)")
	}
}

// lookupSyncItemActionable reports whether an item belongs in an only_actionable
// preview: items with changes, plus items skipped for a reason the user can act
// on. KEY_EMPTY (product never picked that master) is noise and stays hidden.
func lookupSyncItemActionable(it lookupsync.Item) bool {
	if it.SkipReason != "" {
		return it.SkipReason != lookupsync.SkipKeyEmpty
	}
	return len(it.Changes) > 0
}

func lookupSyncSummaryToProto(s lookupsync.Summary) *financev1.LookupSyncSummary {
	counts := make(map[string]int32, len(s.SkipCounts)+1)
	for k, v := range s.SkipCounts {
		counts[k] = safeIntToInt32(v)
	}
	if s.NotApplicableChildren > 0 {
		counts[lookupsync.SkipChildNotApplicable] = safeIntToInt32(s.NotApplicableChildren)
	}
	return &financev1.LookupSyncSummary{
		TotalProducts:       safeIntToInt32(s.TotalProducts),
		ProductsWithChanges: safeIntToInt32(s.ProductsWithChanges),
		TotalValueChanges:   safeIntToInt32(s.TotalValueChanges),
		LockedProducts:      safeIntToInt32(s.LockedProducts),
		SkipCounts:          counts,
		PlanFingerprint:     s.Fingerprint,
	}
}

func lookupSyncItemToProto(it lookupsync.Item) *financev1.LookupSyncItem {
	changes := make([]*financev1.LookupSyncValueChange, 0, len(it.Changes))
	for _, c := range it.Changes {
		changes = append(changes, &financev1.LookupSyncValueChange{
			ParamId: c.ParamID.String(), ParamCode: c.ParamCode, ParamName: c.ParamName,
			OldValue: c.OldValue, NewValue: c.NewValue, ChangeKind: c.Kind,
		})
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].ParamCode < changes[j].ParamCode })
	return &financev1.LookupSyncItem{
		ProductSysId: it.ProductSysID, ProductCode: it.ProductCode, ProductName: it.ProductName,
		TriggerParamCode: it.TriggerParamCode, LookupMasterCode: it.LookupMasterCode,
		MasterKey: it.MasterKey, SkipReason: it.SkipReason, Changes: changes,
	}
}

func lookupSyncJobInfoToProto(i LookupSyncJobInfo) *financev1.BulkParamJobInfo {
	return &financev1.BulkParamJobInfo{
		JobId: i.JobID, JobCode: i.JobCode, Status: i.Status,
		TotalChildren:     safeIntToInt32(i.TotalChildren),
		CompletedChildren: safeIntToInt32(i.CompletedChildren),
		FailedChildren:    safeIntToInt32(i.FailedChildren),
	}
}

func lookupSyncPage(p *commonv1.PaginationRequest) (page, size int32) {
	page, size = paginationFromProto(p)
	if p.GetPageSize() <= 0 {
		size = lookupSyncDefaultPageSize
	}
	if size > lookupSyncMaxPageSize {
		size = lookupSyncMaxPageSize
	}
	return page, size
}

// CountLookupUsage summarizes how many products a master record affects.
func (h *CostProductLookupSyncHandler) CountLookupUsage(
	ctx context.Context, req *financev1.CountLookupUsageRequest,
) (*financev1.CountLookupUsageResponse, error) {
	scope, err := lookupSyncScopeFromProto(&financev1.LookupSyncScope{Scope: &financev1.LookupSyncScope_MasterRef{
		MasterRef: &financev1.LookupMasterRef{LookupMasterCode: req.GetLookupMasterCode(), MasterKey: req.GetMasterKey()},
	}})
	if err != nil {
		return &financev1.CountLookupUsageResponse{Base: BadRequestResponse(err.Error())}, nil //nolint:nilerr // BaseResponse pattern
	}
	sum, err := h.planner.Walk(ctx, scope, nil)
	if err != nil {
		return &financev1.CountLookupUsageResponse{Base: lookupSyncErrToBase(err)}, nil
	}
	return &financev1.CountLookupUsageResponse{Base: cppSuccessResponse("OK"), Data: lookupSyncSummaryToProto(sum)}, nil
}

// PreviewLookupSync returns one page of plan items plus the whole-plan summary.
func (h *CostProductLookupSyncHandler) PreviewLookupSync(
	ctx context.Context, req *financev1.PreviewLookupSyncRequest,
) (*financev1.PreviewLookupSyncResponse, error) {
	scope, err := lookupSyncScopeFromProto(req.GetScope())
	if err != nil {
		return &financev1.PreviewLookupSyncResponse{Base: BadRequestResponse(err.Error())}, nil //nolint:nilerr // BaseResponse pattern
	}
	page, size := lookupSyncPage(req.GetPagination())
	lo := int64(page-1) * int64(size)
	hi := lo + int64(size)

	var total int64
	items := make([]*financev1.LookupSyncItem, 0, size)
	sum, err := h.planner.Walk(ctx, scope, func(it lookupsync.Item) error {
		if req.GetOnlyActionable() && !lookupSyncItemActionable(it) {
			return nil
		}
		if total >= lo && total < hi {
			items = append(items, lookupSyncItemToProto(it))
		}
		total++
		return nil
	})
	if err != nil {
		return &financev1.PreviewLookupSyncResponse{Base: lookupSyncErrToBase(err)}, nil
	}
	return &financev1.PreviewLookupSyncResponse{
		Base: cppSuccessResponse("OK"), Summary: lookupSyncSummaryToProto(sum), Items: items,
		Pagination: paginationResponse(page, size, total),
	}, nil
}

// ApplyLookupSync re-plans, rejects a drifted plan with 409, then queues the job.
func (h *CostProductLookupSyncHandler) ApplyLookupSync(
	ctx context.Context, req *financev1.ApplyLookupSyncRequest,
) (*financev1.ApplyLookupSyncResponse, error) {
	scope, err := lookupSyncScopeFromProto(req.GetScope())
	if err != nil {
		return &financev1.ApplyLookupSyncResponse{Base: BadRequestResponse(err.Error())}, nil //nolint:nilerr // BaseResponse pattern
	}
	if req.GetPlanFingerprint() == "" {
		return &financev1.ApplyLookupSyncResponse{Base: BadRequestResponse("plan_fingerprint is required")}, nil
	}
	if h.enqueuer == nil {
		return &financev1.ApplyLookupSyncResponse{Base: ErrorResponse("503", lookupSyncEnqueueMissing)}, nil
	}
	sum, err := h.planner.Walk(ctx, scope, nil)
	if err != nil {
		return &financev1.ApplyLookupSyncResponse{Base: lookupSyncErrToBase(err)}, nil
	}
	if sum.Fingerprint != req.GetPlanFingerprint() {
		return &financev1.ApplyLookupSyncResponse{
			Base: ConflictResponse(lookupSyncDriftMessage), Summary: lookupSyncSummaryToProto(sum),
		}, nil
	}
	info, err := h.enqueuer.EnqueueLookupSync(ctx, scope, sum.Fingerprint, getUserFromContext(ctx))
	if err != nil {
		return &financev1.ApplyLookupSyncResponse{Base: lookupSyncErrToBase(err), Summary: lookupSyncSummaryToProto(sum)}, nil
	}
	return &financev1.ApplyLookupSyncResponse{
		Base: cppSuccessResponse("Lookup sync queued"), Data: lookupSyncJobInfoToProto(info), Summary: lookupSyncSummaryToProto(sum),
	}, nil
}

// GetLookupSyncJobStatus delegates to the job tracker.
func (h *CostProductLookupSyncHandler) GetLookupSyncJobStatus(
	ctx context.Context, req *financev1.GetLookupSyncJobStatusRequest,
) (*financev1.GetLookupSyncJobStatusResponse, error) {
	if h.tracker == nil {
		return &financev1.GetLookupSyncJobStatusResponse{Base: ErrorResponse("501", lookupSyncTrackerMissing)}, nil
	}
	st, err := h.tracker.GetLookupSyncJobStatus(ctx, req.GetJobId())
	if err != nil {
		return &financev1.GetLookupSyncJobStatusResponse{Base: lookupSyncErrToBase(err)}, nil
	}
	return &financev1.GetLookupSyncJobStatusResponse{
		Base: cppSuccessResponse("OK"), Data: lookupSyncJobInfoToProto(st.Info),
		SkippedChildren: safeIntToInt32(st.SkippedChildren), ValuesWritten: safeIntToInt32(st.ValuesWritten),
	}, nil
}

// ListLookupSyncJobFailures delegates to the job tracker.
func (h *CostProductLookupSyncHandler) ListLookupSyncJobFailures(
	ctx context.Context, req *financev1.ListLookupSyncJobFailuresRequest,
) (*financev1.ListLookupSyncJobFailuresResponse, error) {
	if h.tracker == nil {
		return &financev1.ListLookupSyncJobFailuresResponse{Base: ErrorResponse("501", lookupSyncTrackerMissing)}, nil
	}
	fails, err := h.tracker.ListLookupSyncJobFailures(ctx, req.GetJobId())
	if err != nil {
		return &financev1.ListLookupSyncJobFailuresResponse{Base: lookupSyncErrToBase(err)}, nil
	}
	out := make([]*financev1.BulkProductParamJobFailure, 0, len(fails))
	for _, f := range fails {
		out = append(out, &financev1.BulkProductParamJobFailure{
			ProductSysId: f.ProductSysID, ProductCode: f.ProductCode, ErrorMessage: f.ErrorMessage,
		})
	}
	return &financev1.ListLookupSyncJobFailuresResponse{Base: cppSuccessResponse("OK"), Failures: out}, nil
}

func lookupSyncErrToBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, lookupsync.ErrUnknownMaster),
		errors.Is(err, lookupsyncjob.ErrInvalidJobID),
		errors.Is(err, lookupsyncjob.ErrNothingToApply):
		return BadRequestResponse(err.Error())
	case errors.Is(err, lookupsyncjob.ErrPublisherUnavailable):
		return ErrorResponse("503", err.Error())
	case errors.Is(err, job.ErrNotFound):
		return NotFoundResponse(err.Error())
	}
	return domainErrorToBaseResponse(fmt.Errorf("lookup sync: %w", err))
}
