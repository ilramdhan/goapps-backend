package grpc

import (
	"context"

	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	"github.com/mutugading/goapps-backend/services/ppc/internal/application/etl"
)

// suggestHandler serves the SuggestWOActual RPC. It is embedded into PPCHandler
// by the server orchestrator.
type suggestHandler struct {
	svc *etl.SuggestService
}

// newSuggestHandler builds the suggest handler.
func newSuggestHandler(svc *etl.SuggestService) *suggestHandler {
	return &suggestHandler{svc: svc}
}

// SuggestWOActual computes the suggested WO-actual quantity and its provenance
// for a (wo, date, shift) via the suggest priority chain.
func (h *suggestHandler) SuggestWOActual(ctx context.Context, req *ppcv1.SuggestWOActualRequest) (*ppcv1.SuggestWOActualResponse, error) {
	if h.svc == nil {
		return &ppcv1.SuggestWOActualResponse{
			Base: errorResponse("500", "suggest service unavailable"),
		}, nil
	}
	if req.GetWoId() <= 0 {
		return &ppcv1.SuggestWOActualResponse{Base: errorResponse("400", "invalid wo_id")}, nil
	}
	if req.GetShift() == "" {
		return &ppcv1.SuggestWOActualResponse{Base: errorResponse("400", "shift is required")}, nil
	}
	date, badResp := dateField("date", req.GetDate())
	if badResp != nil {
		return &ppcv1.SuggestWOActualResponse{Base: badResp}, nil
	}

	qty, source, err := h.svc.Suggest(ctx, req.GetWoId(), date, req.GetShift())
	if err != nil {
		return &ppcv1.SuggestWOActualResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &ppcv1.SuggestWOActualResponse{
		Base:           successResponse("suggested WO actual computed"),
		SuggestedQtyKg: formatDecimal(qty),
		QtySource:      suggestSourceToProto(source),
	}, nil
}

// suggestSourceToProto maps the domain suggest source to its proto QtySource.
func suggestSourceToProto(s etl.SuggestSource) ppcv1.QtySource {
	switch s {
	case etl.SuggestManualOverride:
		return ppcv1.QtySource_QTY_SOURCE_MANUAL_OVERRIDE
	case etl.SuggestPackingDone:
		return ppcv1.QtySource_QTY_SOURCE_PACKING_DONE
	case etl.SuggestQCReleased:
		return ppcv1.QtySource_QTY_SOURCE_QC_RELEASED
	case etl.SuggestSPGTransferred:
		return ppcv1.QtySource_QTY_SOURCE_SPG_TRANSFERRED
	case etl.SuggestTXTTransferred:
		return ppcv1.QtySource_QTY_SOURCE_TXT_TRANSFERRED
	case etl.SuggestDoffEstimate:
		return ppcv1.QtySource_QTY_SOURCE_DOFF_ESTIMATE
	case etl.SuggestNoData:
		return ppcv1.QtySource_QTY_SOURCE_NO_DATA
	default:
		return ppcv1.QtySource_QTY_SOURCE_NO_DATA
	}
}
