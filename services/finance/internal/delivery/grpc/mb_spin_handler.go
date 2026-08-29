// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appmbspin "github.com/mutugading/goapps-backend/services/finance/internal/application/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcrosssection"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbdozing"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// MBSpinHandler implements financev1.MBSpinServiceServer.
type MBSpinHandler struct {
	financev1.UnimplementedMBSpinServiceServer
	createHandler *appmbspin.CreateHandler
	getHandler    *appmbspin.GetHandler
	listHandler   *appmbspin.ListHandler
	updateHandler *appmbspin.UpdateHandler
	deleteHandler *appmbspin.DeleteHandler
	// duplicateHandler is nil when the service was built without the recalc
	// collaborators; DuplicateMBSpin then reports Unimplemented instead of
	// pretending to work.
	duplicateHandler *appmbspin.DuplicateHandler
	validation       *ValidationHelper
}

// NewMBSpinHandler constructs an MBSpinHandler without the duplicate/recalc
// collaborators. DuplicateMBSpin is unavailable on such a handler.
func NewMBSpinHandler(repo mbspin.Repository) (*MBSpinHandler, error) {
	return newMBSpinHandler(repo, nil, nil, nil)
}

// NewMBSpinHandlerWithRecalc constructs the full MBSpinHandler: duplicate spin
// plus the child-recalc cascade (dozing AND, Task D, LDR) on update.
//
// ⛔ None of the collaborators is a costing engine. recalcRepo writes spin rows
// and ONE audit row; impactRepo is READ-ONLY and merely counts the cost
// products bound to a spin; factorRepo (Task D) is READ-ONLY too — it only
// looks up a cross-section conversion factor, it never writes one.
// Recalculation stops at the child spin (D24) — ⛔ no yarn product is ever
// recomputed from here. factorRepo must be non-nil for the LDR cascade to be
// able to compute anything past the Scale step: RecalcService.NewRecalcService
// treats a nil factorRepo the same way recalcRepo is treated — never nil in
// practice — because GetByPair would otherwise be called on a nil interface.
func NewMBSpinHandlerWithRecalc(
	repo mbspin.Repository,
	recalcRepo mbspin.RecalcRepository,
	impactRepo mbdozing.ImpactRepository,
	factorRepo mbcrosssection.FactorRepository,
) (*MBSpinHandler, error) {
	return newMBSpinHandler(repo, recalcRepo, impactRepo, factorRepo)
}

func newMBSpinHandler(
	repo mbspin.Repository,
	recalcRepo mbspin.RecalcRepository,
	impactRepo mbdozing.ImpactRepository,
	factorRepo mbcrosssection.FactorRepository,
) (*MBSpinHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	h := &MBSpinHandler{
		createHandler: appmbspin.NewCreateHandler(repo),
		getHandler:    appmbspin.NewGetHandler(repo),
		listHandler:   appmbspin.NewListHandler(repo),
		updateHandler: appmbspin.NewUpdateHandler(repo),
		deleteHandler: appmbspin.NewDeleteHandler(repo),
		validation:    v,
	}
	if recalcRepo != nil {
		svc := appmbspin.NewRecalcService(repo, recalcRepo, impactRepo, factorRepo)
		h.updateHandler = appmbspin.NewUpdateHandlerWithRecalc(repo, svc)
		h.duplicateHandler = appmbspin.NewDuplicateHandler(repo, svc)
	}
	return h, nil
}

// CreateMBSpin creates a new MB spin record.
func (h *MBSpinHandler) CreateMBSpin(ctx context.Context, req *financev1.CreateMBSpinRequest) (*financev1.CreateMBSpinResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBSpinOperation("create", false)
		return &financev1.CreateMBSpinResponse{Base: baseResp}, nil
	}

	headID, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBSpinOperation("create", false)
		return &financev1.CreateMBSpinResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	var filament *int
	if req.MbsFilament != nil {
		v := int(*req.MbsFilament)
		filament = &v
	}

	entity, err := h.createHandler.Handle(ctx, appmbspin.CreateCommand{
		HeadID:          headID,
		MgtName:         req.MbsMgtName,
		OracleSysID:     req.MbsOracleSysId,
		Denier:          req.MbsDenier,
		Filament:        filament,
		Dozing:          req.MbsDozing,
		MBCosting:       req.MbsMbCosting,
		CC:              req.MbsCc,
		CostRateMkt:     req.MbsCostRateMkt,
		MBSStatus:       req.MbsStatus,
		MBSLdrPrsn:      req.MbsLdrPrsn,
		MBSRunLdrPct:    req.MbsRunLdrPct,
		MBSFinalProduct: req.MbsFinalProduct,
		LDRIsFixed:      req.MbsLdrIsFixed,
		DozingIsFixed:   req.MbsDozingIsFixed,
		CreatedBy:       getUserFromContext(ctx),
	})
	if err != nil {
		RecordMBSpinOperation("create", false)
		return &financev1.CreateMBSpinResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBSpinOperation("create", true)
	return &financev1.CreateMBSpinResponse{
		Base: successResponse("MB spin created successfully"),
		Data: mbSpinEntityToProto(entity),
	}, nil
}

// GetMBSpin retrieves an MB spin record by ID.
func (h *MBSpinHandler) GetMBSpin(ctx context.Context, req *financev1.GetMBSpinRequest) (*financev1.GetMBSpinResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBSpinOperation("get", false)
		return &financev1.GetMBSpinResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbsId)
	if err != nil {
		RecordMBSpinOperation("get", false)
		return &financev1.GetMBSpinResponse{Base: invalidIDResponse("mbs_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.getHandler.Handle(ctx, appmbspin.GetQuery{ID: id})
	if err != nil {
		RecordMBSpinOperation("get", false)
		return &financev1.GetMBSpinResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBSpinOperation("get", true)
	return &financev1.GetMBSpinResponse{
		Base: successResponse("MB spin retrieved successfully"),
		Data: mbSpinEntityToProto(entity),
	}, nil
}

// UpdateMBSpin updates an existing MB spin record.
func (h *MBSpinHandler) UpdateMBSpin(ctx context.Context, req *financev1.UpdateMBSpinRequest) (*financev1.UpdateMBSpinResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBSpinOperation("update", false)
		return &financev1.UpdateMBSpinResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbsId)
	if err != nil {
		RecordMBSpinOperation("update", false)
		return &financev1.UpdateMBSpinResponse{Base: invalidIDResponse("mbs_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	var filament *int
	if req.MbsFilament != nil {
		v := int(*req.MbsFilament)
		filament = &v
	}

	entity, err := h.updateHandler.Handle(ctx, appmbspin.UpdateCommand{
		ID:               id,
		MgtName:          req.MbsMgtName,
		MBCosting:        req.MbsMbCosting,
		Denier:           req.MbsDenier,
		Filament:         filament,
		Dozing:           req.MbsDozing,
		CC:               req.MbsCc,
		CostRateMkt:      req.MbsCostRateMkt,
		MBSStatus:        req.MbsStatus,
		MBSLdrPrsn:       req.MbsLdrPrsn,
		MBSRunLdrPct:     req.MbsRunLdrPct,
		MBSFinalProduct:  req.MbsFinalProduct,
		LDRIsFixed:       req.MbsLdrIsFixed,
		DozingIsFixed:    req.MbsDozingIsFixed,
		IsActive:         req.MbsIsActive,
		LDRAdjustmentPct: req.MbsLdrAdjustmentPct,
		LDRLockActual:    req.MbsLdrLockActual,
		UpdatedBy:        getUserFromContext(ctx),
	})
	if err != nil {
		RecordMBSpinOperation("update", false)
		return &financev1.UpdateMBSpinResponse{Base: mbSpinErrorToBaseResponse(err)}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	RecordMBSpinOperation("update", true)
	return &financev1.UpdateMBSpinResponse{
		Base: successResponse("MB spin updated successfully"),
		Data: mbSpinEntityToProto(entity),
	}, nil
}

// DeleteMBSpin soft-deletes an MB spin record.
func (h *MBSpinHandler) DeleteMBSpin(ctx context.Context, req *financev1.DeleteMBSpinRequest) (*financev1.DeleteMBSpinResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBSpinOperation("delete", false)
		return &financev1.DeleteMBSpinResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbsId)
	if err != nil {
		RecordMBSpinOperation("delete", false)
		return &financev1.DeleteMBSpinResponse{Base: invalidIDResponse("mbs_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	if err := h.deleteHandler.Handle(ctx, appmbspin.DeleteCommand{ID: id, DeletedBy: getUserFromContext(ctx)}); err != nil {
		RecordMBSpinOperation("delete", false)
		return &financev1.DeleteMBSpinResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBSpinOperation("delete", true)
	return &financev1.DeleteMBSpinResponse{Base: successResponse("MB spin deleted successfully")}, nil
}

// ListMBSpins lists MB spin records for a given head with search, filter, and pagination.
func (h *MBSpinHandler) ListMBSpins(ctx context.Context, req *financev1.ListMBSpinsRequest) (*financev1.ListMBSpinsResponse, error) {
	var headID uuid.UUID
	if req.MbhId != "" {
		var parseErr error
		headID, parseErr = uuid.Parse(req.MbhId)
		if parseErr != nil {
			RecordMBSpinOperation("list", false)
			return &financev1.ListMBSpinsResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
		}
	}

	page := int(req.Page)
	if page == 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 10
	}

	query := appmbspin.ListQuery{
		HeadID:    headID,
		Page:      page,
		PageSize:  pageSize,
		Search:    req.Search,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}

	switch req.ActiveFilter {
	case financev1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		t := true
		query.IsActive = &t
	case financev1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		f := false
		query.IsActive = &f
	default:
	}

	result, err := h.listHandler.Handle(ctx, query)
	if err != nil {
		RecordMBSpinOperation("list", false)
		return &financev1.ListMBSpinsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBSpinOperation("list", true)

	items := make([]*financev1.MBSpin, len(result.Items))
	for i, e := range result.Items {
		items[i] = mbSpinEntityToProto(e)
	}

	return &financev1.ListMBSpinsResponse{
		Base: successResponse("MB spins retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// ExportMBSpins is not yet implemented.
func (h *MBSpinHandler) ExportMBSpins(_ context.Context, _ *financev1.ExportMBSpinsRequest) (*financev1.ExportMBSpinsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ExportMBSpins not implemented")
}

// ImportMBSpins is not yet implemented.
func (h *MBSpinHandler) ImportMBSpins(_ context.Context, _ *financev1.ImportMBSpinsRequest) (*financev1.ImportMBSpinsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ImportMBSpins not implemented")
}

// DownloadMBSpinTemplate is not yet implemented.
func (h *MBSpinHandler) DownloadMBSpinTemplate(_ context.Context, _ *financev1.DownloadMBSpinTemplateRequest) (*financev1.DownloadMBSpinTemplateResponse, error) {
	return nil, status.Error(codes.Unimplemented, "DownloadMBSpinTemplate not implemented")
}

// mbSpinEntityToProto converts a domain MBSpin entity to its proto representation.
func mbSpinEntityToProto(e *mbspin.Entity) *financev1.MBSpin {
	p := &financev1.MBSpin{
		MbsId:       e.ID().String(),
		MbsMbhId:    e.HeadID().String(),
		MbsMgtName:  e.MgtName(),
		MbsIsActive: e.IsActive(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt().Format(time.RFC3339),
			CreatedBy: e.CreatedBy(),
		},
	}
	if e.OracleSysID() != nil {
		p.MbsOracleSysId = *e.OracleSysID()
	}
	p.MbsDenier = e.Denier()
	if e.Filament() != nil {
		v := safeconv.IntToInt32(*e.Filament())
		p.MbsFilament = &v
	}
	p.MbsDozing = e.Dozing()
	if e.MBCosting() != nil {
		p.MbsMbCosting = *e.MBCosting()
	}
	p.MbsCc = e.CC()
	p.MbsCostRateMkt = e.CostRateMkt()
	p.MbsStatus = e.MBSStatus()
	p.MbsLdrPrsn = e.MBSLdrPrsn()
	p.MbsRunLdrPct = e.MBSRunLdrPct()
	p.MbsFinalProduct = e.MBSFinalProduct()
	// Absence-vs-zero: nil stays nil so the UI can tell "unknown" from "computed".
	p.MbsLdrIsFixed = e.LDRIsFixed()
	p.MbsDozingIsFixed = e.DozingIsFixed()
	p.MbsLdrType = e.LDRType()
	p.MbsLdrCalculatedPct = e.LDRCalculatedPct()
	p.MbsLdrAdjustmentPct = e.LDRAdjustmentPct()
	p.MbsLdrIsActual = e.LDRIsActual()
	if e.UpdatedAt() != nil {
		p.Audit.UpdatedAt = e.UpdatedAt().Format(time.RFC3339)
	}
	if e.UpdatedBy() != nil {
		p.Audit.UpdatedBy = *e.UpdatedBy()
	}
	return p
}

// =============================================================================
// Duplicate spin (P8)
// =============================================================================

// DuplicateMBSpin clones one MB spin into a fresh R&D child.
//
// ⛔ THE RESPONSE'S impact_* FIELDS ARE A PREVIEW, NOT A RESULT (decision D24).
// Duplicating a spin does NOT recalculate a single yarn product: the numbers
// below are counted by a read-only SELECT over the products bound to the source
// spin's ORION item code. ⛔ No costing engine is invoked anywhere on this path.
//
// skipped[] reports the source spin's DIRECT children that a recalc would have
// left alone because they hold actual production values (rule A7). It is one
// level deep only (R13).
func (h *MBSpinHandler) DuplicateMBSpin(ctx context.Context, req *financev1.DuplicateMBSpinRequest) (*financev1.DuplicateMBSpinResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBSpinOperation("duplicate", false)
		return &financev1.DuplicateMBSpinResponse{Base: baseResp}, nil
	}
	if h.duplicateHandler == nil {
		RecordMBSpinOperation("duplicate", false)
		return nil, status.Error(codes.Unimplemented, "DuplicateMBSpin is not enabled on this server build")
	}

	spinID, err := uuid.Parse(req.GetMbsId())
	if err != nil {
		RecordMBSpinOperation("duplicate", false)
		return &financev1.DuplicateMBSpinResponse{Base: invalidIDResponse("mbs_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}
	headID, err := uuid.Parse(req.GetMbhId())
	if err != nil {
		RecordMBSpinOperation("duplicate", false)
		return &financev1.DuplicateMBSpinResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	var filament *int
	if req.MbsFilament != nil {
		v := int(*req.MbsFilament)
		filament = &v
	}

	result, err := h.duplicateHandler.Handle(ctx, appmbspin.DuplicateCommand{
		SourceSpinID: spinID,
		HeadID:       &headID,
		MgtName:      req.MbsMgtName,
		Denier:       req.MbsDenier,
		Filament:     filament,
		// OrionItemCode stays nil: D19 requires the clone's ERP keys to be NULL,
		// which is why DuplicateMBSpinRequest carries no such field.
		ActorUserID: getUserFromContext(ctx),
	})
	if err != nil {
		RecordMBSpinOperation("duplicate", false)
		return &financev1.DuplicateMBSpinResponse{Base: mbSpinErrorToBaseResponse(err)}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	RecordMBSpinOperation("duplicate", true)
	resp := &financev1.DuplicateMBSpinResponse{
		Base: successResponse("MB spin duplicated successfully"),
		Data: mbSpinEntityToProto(result.Clone),
	}
	applyRecalcToDuplicateResponse(resp, result.Recalc)
	return resp, nil
}

// applyRecalcToDuplicateResponse copies the skip list and the D24 impact PREVIEW
// onto the response.
//
// ⛔ Every impact number here comes from mbdozing.ImpactRepository, a read-only
// repository that SELECTs the products carrying this spin's code. ⛔ Nothing was
// recalculated to produce them.
func applyRecalcToDuplicateResponse(resp *financev1.DuplicateMBSpinResponse, recalc *appmbspin.RecalcResult) {
	if recalc == nil {
		return
	}
	skipped := make([]*financev1.MBSpinRecalcSkipped, 0, len(recalc.Skipped))
	for i := range recalc.Skipped {
		s := recalc.Skipped[i]
		skipped = append(skipped, &financev1.MBSpinRecalcSkipped{
			MbsId:      s.SpinID.String(),
			MbsMgtName: s.MgtName,
			MbsStatus:  s.Status,
			Reason:     s.Reason,
		})
	}
	resp.Skipped = skipped
	resp.SkippedCount = safeconv.IntToInt32(len(skipped))

	rows := make([]*financev1.DozingImpactRow, 0, len(recalc.ImpactRows))
	for i := range recalc.ImpactRows {
		r := recalc.ImpactRows[i]
		rows = append(rows, &financev1.DozingImpactRow{
			CpmProductSysId: r.ProductSysID,
			CpmProductCode:  r.ProductCode,
			CpmProductName:  r.ProductName,
			CpmIsLocked:     r.IsLocked,
			FrozenDozing:    r.FrozenDozing,
		})
	}
	resp.ImpactPreview = rows
	resp.ImpactTotalAffected = safeconv.IntToInt32(recalc.ImpactTotals.TotalAffected)
	resp.ImpactTotalLocked = safeconv.IntToInt32(recalc.ImpactTotals.TotalLocked)
	resp.ImpactTruncated = recalc.ImpactTruncated
}

// mbSpinErrorToBaseResponse maps the P8 duplicate/recalc sentinels, plus the
// Task E LDR lock sentinel, onto HTTP-ish status codes.
//
// The generic domainErrorToBaseResponse matches on substrings ("not found",
// "already exists", "invalid") and none of these sentinels contains one, so
// without this they would all surface as 500s — a cycle, a duplicate ERP code, a
// too-wide fan-out, or a rejected adjustment on a locked spin are all
// CLIENT-correctable conditions, not server faults.
func mbSpinErrorToBaseResponse(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, mbspin.ErrDuplicateOrionItemCode):
		return ConflictResponse(err.Error())
	case errors.Is(err, mbspin.ErrParentCycle),
		errors.Is(err, mbspin.ErrMaxDuplicateDepth),
		errors.Is(err, mbspin.ErrAlreadyDeleted),
		errors.Is(err, mbspin.ErrLDRLockedActual):
		return BadRequestResponse(err.Error())
	case errors.Is(err, mbspin.ErrTooManyChildren):
		return BadRequestResponse(err.Error() + "; recalculate the affected child spins manually")
	default:
		return domainErrorToBaseResponse(err)
	}
}
