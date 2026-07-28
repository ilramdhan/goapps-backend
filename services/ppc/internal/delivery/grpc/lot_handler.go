// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	lotapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/lot"
	lotdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/lot"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// lotUnavailable is reported when the lot service was never wired.
const lotUnavailable = "lot service unavailable"

// lotHandler implements the lot-master RPCs of PPCService.
type lotHandler struct {
	svc *lotapp.Service
}

func newLotHandler(svc *lotapp.Service) *lotHandler {
	return &lotHandler{svc: svc}
}

// CreateLotMaster creates a new lot master.
func (h *lotHandler) CreateLotMaster(ctx context.Context, req *ppcv1.CreateLotMasterRequest) (*ppcv1.CreateLotMasterResponse, error) {
	if h.svc == nil {
		return &ppcv1.CreateLotMasterResponse{Base: errorResponse(nilServiceCode, lotUnavailable)}, nil
	}
	stdFull, base := decimalField("std_weight_full", req.GetStdWeightFull())
	if base != nil {
		return &ppcv1.CreateLotMasterResponse{Base: base}, nil
	}
	stdUnfull, base := decimalField("std_weight_unfull", req.GetStdWeightUnfull())
	if base != nil {
		return &ppcv1.CreateLotMasterResponse{Base: base}, nil
	}

	entity, err := h.svc.Create(ctx, lotapp.CreateCommand{
		LotNo:           req.GetLotNo(),
		ItemCode:        req.GetItemCode(),
		ShadeCode:       req.GetShadeCode(),
		StdWeightFull:   stdFull,
		StdWeightUnfull: stdUnfull,
		Notes:           req.GetNotes(),
		CreatedBy:       getUserFromContext(ctx),
	})
	if err != nil {
		return &ppcv1.CreateLotMasterResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.CreateLotMasterResponse{
		Base: successResponse("Lot master created successfully"),
		Data: lotToProto(entity),
	}, nil
}

// GetLotMaster retrieves a lot master by lot number.
func (h *lotHandler) GetLotMaster(ctx context.Context, req *ppcv1.GetLotMasterRequest) (*ppcv1.GetLotMasterResponse, error) {
	if h.svc == nil {
		return &ppcv1.GetLotMasterResponse{Base: errorResponse(nilServiceCode, lotUnavailable)}, nil
	}
	entity, err := h.svc.Get(ctx, req.GetLotNo())
	if err != nil {
		return &ppcv1.GetLotMasterResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.GetLotMasterResponse{
		Base: successResponse("Lot master retrieved successfully"),
		Data: lotToProto(entity),
	}, nil
}

// UpdateLotMaster updates an existing lot master. A supplied spec replaces the
// stored yarn/packing specification wholesale; omitting it leaves it untouched.
func (h *lotHandler) UpdateLotMaster(ctx context.Context, req *ppcv1.UpdateLotMasterRequest) (*ppcv1.UpdateLotMasterResponse, error) {
	if h.svc == nil {
		return &ppcv1.UpdateLotMasterResponse{Base: errorResponse(nilServiceCode, lotUnavailable)}, nil
	}
	stdFull, base := optionalDecimalField("std_weight_full", req.GetStdWeightFull())
	if base != nil {
		return &ppcv1.UpdateLotMasterResponse{Base: base}, nil
	}
	stdUnfull, base := optionalDecimalField("std_weight_unfull", req.GetStdWeightUnfull())
	if base != nil {
		return &ppcv1.UpdateLotMasterResponse{Base: base}, nil
	}
	spec, base := specFromProto(req.GetSpec())
	if base != nil {
		return &ppcv1.UpdateLotMasterResponse{Base: base}, nil
	}

	entity, err := h.svc.Update(ctx, lotapp.UpdateCommand{
		LotNo:           req.GetLotNo(),
		ItemCode:        req.ItemCode,
		ShadeCode:       req.ShadeCode,
		StdWeightFull:   stdFull,
		StdWeightUnfull: stdUnfull,
		Notes:           req.Notes,
		Spec:            spec,
		UpdatedBy:       getUserFromContext(ctx),
	})
	if err != nil {
		return &ppcv1.UpdateLotMasterResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.UpdateLotMasterResponse{
		Base: successResponse("Lot master updated successfully"),
		Data: lotToProto(entity),
	}, nil
}

// DeleteLotMaster deletes a lot master.
func (h *lotHandler) DeleteLotMaster(ctx context.Context, req *ppcv1.DeleteLotMasterRequest) (*ppcv1.DeleteLotMasterResponse, error) {
	if h.svc == nil {
		return &ppcv1.DeleteLotMasterResponse{Base: errorResponse(nilServiceCode, lotUnavailable)}, nil
	}
	if err := h.svc.Delete(ctx, req.GetLotNo()); err != nil {
		return &ppcv1.DeleteLotMasterResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.DeleteLotMasterResponse{Base: successResponse("Lot master deleted successfully")}, nil
}

// ListLotMasters lists lot masters with filtering and pagination.
func (h *lotHandler) ListLotMasters(ctx context.Context, req *ppcv1.ListLotMastersRequest) (*ppcv1.ListLotMastersResponse, error) {
	if h.svc == nil {
		return &ppcv1.ListLotMastersResponse{Base: errorResponse(nilServiceCode, lotUnavailable)}, nil
	}
	result, err := h.svc.List(ctx, lotapp.ListQuery{
		Page:      int(req.GetPage()),
		PageSize:  int(req.GetPageSize()),
		Search:    req.GetSearch(),
		ItemCode:  req.GetItemCode(),
		ShadeCode: req.GetShadeCode(),
		Source:    req.GetSource(),
		ProdType:  req.GetProdType(),
		SortBy:    req.GetSortBy(),
		SortOrder: req.GetSortOrder(),
	})
	if err != nil {
		return &ppcv1.ListLotMastersResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	items := make([]*ppcv1.LotMaster, len(result.Items))
	for i, entity := range result.Items {
		items[i] = lotToProto(entity)
	}
	return &ppcv1.ListLotMastersResponse{
		Base: successResponse("Lot masters retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// SyncLots triggers an on-demand read-only import from Oracle ASPAK.MMSMERGE.
func (h *lotHandler) SyncLots(ctx context.Context, _ *ppcv1.SyncLotsRequest) (*ppcv1.SyncLotsResponse, error) {
	if h.svc == nil {
		return &ppcv1.SyncLotsResponse{Base: errorResponse(nilServiceCode, lotUnavailable)}, nil
	}
	res, err := h.svc.SyncFromOracle(ctx)
	if err != nil {
		return &ppcv1.SyncLotsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.SyncLotsResponse{
		Base:       successResponse("Lot sync completed"),
		Inserted:   safeconv.IntToInt32(res.Inserted),
		Updated:    safeconv.IntToInt32(res.Updated),
		Skipped:    safeconv.IntToInt32(res.Skipped),
		OracleUsed: res.OracleUsed,
	}, nil
}

// specFromProto maps an optional proto LotSpec onto the domain struct. A nil
// message yields a nil spec, which the service reads as "leave it alone". The
// second return is a populated 400 BaseResponse when a decimal fails to parse.
func specFromProto(p *ppcv1.LotSpec) (*lotdomain.Spec, *commonv1.BaseResponse) {
	if p == nil {
		return nil, nil
	}
	tareBox, base := optionalDecimalField("spec.tare_box_weight", p.GetTareBoxWeight())
	if base != nil {
		return nil, base
	}
	tareBobbin, base := optionalDecimalField("spec.tare_bobbin_weight", p.GetTareBobbinWeight())
	if base != nil {
		return nil, base
	}
	srcBob, base := optionalDecimalField("spec.source_bob_weight", p.GetSourceBobWeight())
	if base != nil {
		return nil, base
	}
	return &lotdomain.Spec{
		ProdType:         p.GetProdType(),
		YarnType:         p.GetYarnType(),
		Denier:           p.GetDenier(),
		Filament:         p.Filament,
		CrossSection:     p.GetCrossSection(),
		QCGrade:          p.GetQcGrade(),
		Description:      p.GetDescription(),
		ShadeColor:       p.GetShadeColor(),
		TareBoxWeight:    tareBox,
		TareBobbinWeight: tareBobbin,
		BobbinsPerBox:    p.BobbinsPerBox,
		SourceBobWeight:  srcBob,
		OrionItemCode:    p.GetOrionItemCode(),
		MachineNo:        p.GetMachineNo(),
		EfficiencyPct:    p.EfficiencyPct,
		SourceStatus:     p.GetSourceStatus(),
		SourcePakStatus:  p.GetSourcePakStatus(),
	}, nil
}

// specToProto maps the domain spec onto its proto message. Optional numerics
// stay absent rather than collapsing to zero, because a missing tare weight and
// a tare weight of nothing are different answers.
func specToProto(s lotdomain.Spec) *ppcv1.LotSpec {
	return &ppcv1.LotSpec{
		ProdType:         s.ProdType,
		YarnType:         s.YarnType,
		Denier:           s.Denier,
		Filament:         s.Filament,
		CrossSection:     s.CrossSection,
		QcGrade:          s.QCGrade,
		Description:      s.Description,
		ShadeColor:       s.ShadeColor,
		TareBoxWeight:    formatOptionalDecimal(s.TareBoxWeight),
		TareBobbinWeight: formatOptionalDecimal(s.TareBobbinWeight),
		BobbinsPerBox:    s.BobbinsPerBox,
		SourceBobWeight:  formatOptionalDecimal(s.SourceBobWeight),
		OrionItemCode:    s.OrionItemCode,
		MachineNo:        s.MachineNo,
		EfficiencyPct:    s.EfficiencyPct,
		SourceStatus:     s.SourceStatus,
		SourcePakStatus:  s.SourcePakStatus,
	}
}

func lotToProto(e *lotdomain.Master) *ppcv1.LotMaster {
	proto := &ppcv1.LotMaster{
		LotNo:           e.LotNo(),
		ItemCode:        e.ItemCode(),
		ShadeCode:       e.ShadeCode(),
		StdWeightFull:   formatDecimal(e.StdWeightFull()),
		StdWeightUnfull: formatDecimal(e.StdWeightUnfull()),
		Notes:           e.Notes(),
		Spec:            specToProto(e.Spec()),
		Source:          e.Source(),
		SourceKey:       e.SourceKey(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt().Format(time.RFC3339),
			CreatedBy: e.CreatedBy(),
		},
	}
	if e.SyncedAt() != nil {
		proto.SyncedAt = e.SyncedAt().Format(time.RFC3339)
	}
	if e.UpdatedAt() != nil {
		proto.Audit.UpdatedAt = e.UpdatedAt().Format(time.RFC3339)
	}
	if e.UpdatedBy() != nil {
		proto.Audit.UpdatedBy = *e.UpdatedBy()
	}
	return proto
}
