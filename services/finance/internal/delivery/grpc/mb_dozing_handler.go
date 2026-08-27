package grpc

import (
	"context"
	"errors"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appmbdozing "github.com/mutugading/goapps-backend/services/finance/internal/application/mbdozing"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/formula"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcrosssection"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbdozing"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// MBDozingHandler implements financev1.MBDozingServiceServer.
//
// Both RPCs are READ-ONLY (user decision K-18).
type MBDozingHandler struct {
	financev1.UnimplementedMBDozingServiceServer
	calculateHandler *appmbdozing.CalculateHandler
	impactHandler    *appmbdozing.ImpactPreviewHandler
	validation       *ValidationHelper
}

// NewMBDozingHandler constructs an MBDozingHandler. The cross-section factor and
// formula repositories are the same instances used elsewhere in the service.
func NewMBDozingHandler(
	factorRepo mbcrosssection.FactorRepository,
	formulaRepo formula.Repository,
	spinRepo mbspin.Repository,
	impactRepo mbdozing.ImpactRepository,
) (*MBDozingHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &MBDozingHandler{
		calculateHandler: appmbdozing.NewCalculateHandler(factorRepo, formulaRepo),
		impactHandler:    appmbdozing.NewImpactPreviewHandler(spinRepo, impactRepo),
		validation:       v,
	}, nil
}

// CalculateDozing computes an LDR for the requested mode.
func (h *MBDozingHandler) CalculateDozing(ctx context.Context, req *financev1.CalculateDozingRequest) (*financev1.CalculateDozingResponse, error) { //nolint:nilerr // BaseResponse pattern
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.CalculateDozingResponse{Base: baseResp}, nil
	}

	result, err := h.calculateHandler.Handle(ctx, appmbdozing.CalculateCommand{
		Mode:             req.GetMode(),
		LDRRef:           req.LdrRef,
		DenierRef:        req.DenierRef,
		FilamentRef:      req.FilamentRef,
		DenierTarget:     req.DenierTarget,
		FilamentTarget:   req.FilamentTarget,
		LDRSource:        req.LdrSource,
		FromCrossSection: req.FromCrossSection,
		ToCrossSection:   req.ToCrossSection,
	})
	if err != nil {
		return &financev1.CalculateDozingResponse{Base: calculateErrorToBaseResponse(err)}, nil //nolint:nilerr // BaseResponse pattern
	}

	// A missing conversion factor is a SUCCESSFUL response carrying
	// factor_available = false (constraint D13). ResultLdr stays unset — the
	// forbidden alternative would be substituting a 1.0 factor.
	resp := &financev1.CalculateDozingResponse{
		Base:             successResponse(baseMessage(result)),
		ResultLdr:        result.ResultLDR,
		FormulaCode:      result.FormulaCode,
		CalculationTrace: result.CalculationTrace,
		FactorAvailable:  result.FactorAvailable,
	}
	return resp, nil
}

// baseMessage picks the response message: the explanatory text when no factor
// exists, the generic success text otherwise.
func baseMessage(result *appmbdozing.CalculateResult) string {
	if result.Message != "" {
		return result.Message
	}
	return "Dozing calculated successfully"
}

// calculateErrorToBaseResponse maps calculate-use-case errors to a BaseResponse.
// Bad operands are 400s, not 500s.
func calculateErrorToBaseResponse(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, mbdozing.ErrInvalidMode),
		errors.Is(err, mbdozing.ErrZeroFilament),
		errors.Is(err, appmbdozing.ErrMissingScaleInput),
		errors.Is(err, appmbdozing.ErrMissingXSectionInput),
		errors.Is(err, mbcrosssection.ErrFactorNotPositive),
		errors.Is(err, mbcrosssection.ErrFactorInvalidOperation):
		return BadRequestResponse(err.Error())
	default:
		return domainErrorToBaseResponse(err)
	}
}

// PreviewDozingImpact lists the cost products a dozing change on this spin would reach.
func (h *MBDozingHandler) PreviewDozingImpact(ctx context.Context, req *financev1.PreviewDozingImpactRequest) (*financev1.PreviewDozingImpactResponse, error) { //nolint:nilerr // BaseResponse pattern
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.PreviewDozingImpactResponse{Base: baseResp}, nil
	}

	id, parseErr := uuid.Parse(req.GetMbsId())
	if parseErr != nil {
		//nolint:nilerr // BaseResponse pattern: validation failures ride in Base, not as a gRPC error
		return &financev1.PreviewDozingImpactResponse{
			Base: BadRequestResponse("invalid mbs_id: must be a UUID"),
		}, nil
	}

	result, err := h.impactHandler.Handle(ctx, appmbdozing.ImpactCommand{
		MBSID: id,
		Limit: int(req.GetLimit()),
	})
	if err != nil {
		return &financev1.PreviewDozingImpactResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // BaseResponse pattern
	}

	rows := make([]*financev1.DozingImpactRow, 0, len(result.Rows))
	for i := range result.Rows {
		r := result.Rows[i]
		rows = append(rows, &financev1.DozingImpactRow{
			CpmProductSysId: r.ProductSysID,
			CpmProductCode:  r.ProductCode,
			CpmProductName:  r.ProductName,
			CpmIsLocked:     r.IsLocked,
			FrozenDozing:    r.FrozenDozing,
		})
	}

	return &financev1.PreviewDozingImpactResponse{
		Base:          successResponse("Dozing impact preview retrieved successfully"),
		Data:          rows,
		TotalAffected: int32(result.Totals.TotalAffected), //nolint:gosec // counts are bounded by table size
		TotalLocked:   int32(result.Totals.TotalLocked),   //nolint:gosec // counts are bounded by table size
		Truncated:     result.Truncated,
		Note:          result.Note,
	}, nil
}
