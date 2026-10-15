// Package grpc provides gRPC server implementation for the finance service.
package grpc

import (
	"context"
	"errors"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/lookupsync"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/boxbobbincost"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/intermingling"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/machine"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/productgrade"
)

// mbHeadNumericReaders aliases the lookupsync reader map so delivery-layer
// reader tests keep asserting against the live map.
var mbHeadNumericReaders = lookupsync.MBHeadNumericReaders

// LookupReaderColumns returns every lookup_source_column the fill handler can
// actually resolve, keyed by lookup master code. It re-exports
// lookupsync.LookupReaderColumns, the inventory derived from the reader maps
// that the startup registry-divergence check consumes.
func LookupReaderColumns() map[string]map[string]struct{} {
	return lookupsync.LookupReaderColumns()
}

// YarnLookupFillHandler implements financev1.YarnLookupFillServiceServer.
// It is a thin adapter over lookupsync.Resolver: it resolves the selected
// master record and maps the result (or error) onto the BaseResponse pattern.
type YarnLookupFillHandler struct {
	financev1.UnimplementedYarnLookupFillServiceServer
	machineRepo       machine.Repository
	interminglingRepo intermingling.Repository
	productGradeRepo  productgrade.Repository
	mbHeadRepo        mbhead.Repository
	mbSpinRepo        mbspin.Repository
	boxBobbinRepo     boxbobbincost.Repository
	paramRepo         parameter.Repository
	// superbaFallback resolves a SUPERBA shade stored in MB_SP_CODE (optional; see
	// WithSuperbaFallback).
	superbaFallback mbsource.Provider
}

// WithSuperbaFallback lets the MB_SPIN fill resolve a Superba shade (stored in MB_SP_CODE by the
// shade-driven auto-fill) instead of returning NotFound: it fills MB_SP_DYE with the color name.
func (h *YarnLookupFillHandler) WithSuperbaFallback(p mbsource.Provider) *YarnLookupFillHandler {
	h.superbaFallback = p
	return h
}

// NewYarnLookupFillHandler creates a new YarnLookupFillHandler.
func NewYarnLookupFillHandler(
	machineRepo machine.Repository,
	interminglingRepo intermingling.Repository,
	productGradeRepo productgrade.Repository,
	mbHeadRepo mbhead.Repository,
	mbSpinRepo mbspin.Repository,
	boxBobbinRepo boxbobbincost.Repository,
	paramRepo parameter.Repository,
) (*YarnLookupFillHandler, error) {
	return &YarnLookupFillHandler{
		machineRepo:       machineRepo,
		interminglingRepo: interminglingRepo,
		productGradeRepo:  productGradeRepo,
		mbHeadRepo:        mbHeadRepo,
		mbSpinRepo:        mbSpinRepo,
		boxBobbinRepo:     boxBobbinRepo,
		paramRepo:         paramRepo,
	}, nil
}

// resolver builds a lookupsync.Resolver over the handler's repositories. It is
// built per call (a plain struct, no I/O) so handlers assembled field-by-field
// in tests behave exactly like those from NewYarnLookupFillHandler.
func (h *YarnLookupFillHandler) resolver() *lookupsync.Resolver {
	return lookupsync.NewResolver(
		h.machineRepo, h.interminglingRepo, h.productGradeRepo, h.mbHeadRepo,
		h.mbSpinRepo, h.boxBobbinRepo, h.paramRepo,
	).WithSuperbaFallback(h.superbaFallback)
}

// GetLookupFillValues routes to master-specific fill logic by lookup_master_code.
func (h *YarnLookupFillHandler) GetLookupFillValues(ctx context.Context, req *financev1.GetLookupFillValuesRequest) (*financev1.GetLookupFillValuesResponse, error) { //nolint:nilerr // BaseResponse pattern
	return h.fill(ctx, req.GetLookupMasterCode(), req.GetSelectedKey(), req.GetSourceParamCode()), nil
}

// fill resolves one master record and maps the outcome onto the response.
func (h *YarnLookupFillHandler) fill(ctx context.Context, masterCode, selectedKey, sourceParamCode string) *financev1.GetLookupFillValuesResponse {
	res, err := h.resolver().Resolve(ctx, masterCode, selectedKey, sourceParamCode)
	if err != nil {
		return &financev1.GetLookupFillValuesResponse{Base: fillErrorToBaseResponse(masterCode, err)}
	}
	return &financev1.GetLookupFillValuesResponse{
		Base:         successResponse(res.Message),
		NumericFills: res.Numeric,
		TextFills:    res.Text,
		DisplayLabel: res.DisplayLabel,
	}
}

// fillErrorToBaseResponse maps a Resolver error onto the BaseResponse the fill
// RPC has always returned: an unknown master is a 404, an MB_SPIN fill-group
// load failure is a 500 carrying the raw message, and every other error goes
// through domainErrorToBaseResponse.
func fillErrorToBaseResponse(masterCode string, err error) *commonv1.BaseResponse {
	if errors.Is(err, lookupsync.ErrUnknownMaster) {
		return ErrorResponse("404", err.Error())
	}
	var fgErr *lookupsync.FillGroupError
	if masterCode == lookupsync.MasterMBSpin && errors.As(err, &fgErr) {
		return ErrorResponse("500", err.Error())
	}
	return domainErrorToBaseResponse(err)
}

// fillFromIntermingling, fillFromBoxBobbinCost and fillFromMBSpin are per-master
// entry points kept for the existing delivery tests; each delegates to fill.
//
//nolint:unparam // signature kept for existing delivery tests; error is always nil (BaseResponse pattern).
func (h *YarnLookupFillHandler) fillFromIntermingling(ctx context.Context, intmCode, triggerParamCode string) (*financev1.GetLookupFillValuesResponse, error) {
	return h.fill(ctx, lookupsync.MasterIntermingling, intmCode, triggerParamCode), nil
}

//nolint:unparam // signature kept for existing delivery tests; error is always nil (BaseResponse pattern).
func (h *YarnLookupFillHandler) fillFromBoxBobbinCost(ctx context.Context, bbcCode, triggerParamCode string) (*financev1.GetLookupFillValuesResponse, error) {
	return h.fill(ctx, lookupsync.MasterBoxBobbinCost, bbcCode, triggerParamCode), nil
}

//nolint:unparam // signature kept for existing delivery tests; error is always nil (BaseResponse pattern).
func (h *YarnLookupFillHandler) fillFromMBSpin(ctx context.Context, selectedKey, sourceParamCode string) (*financev1.GetLookupFillValuesResponse, error) {
	return h.fill(ctx, lookupsync.MasterMBSpin, selectedKey, sourceParamCode), nil
}

// resolveMBSpinForFill resolves selectedKey to an MB Spin entity for the read
// (lookup-fill) path; see lookupsync.Resolver.ResolveMBSpin.
func (h *YarnLookupFillHandler) resolveMBSpinForFill(ctx context.Context, selectedKey string) (*mbspin.Entity, error) {
	return h.resolver().ResolveMBSpin(ctx, selectedKey)
}

// compile-time interface check.
var _ financev1.YarnLookupFillServiceServer = (*YarnLookupFillHandler)(nil)
