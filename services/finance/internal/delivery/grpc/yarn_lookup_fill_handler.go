// Package grpc provides gRPC server implementation for the finance service.
package grpc

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/rs/zerolog/log"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/boxbobbincost"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/intermingling"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/machine"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/productgrade"
)

// YarnLookupFillHandler implements financev1.YarnLookupFillServiceServer.
// It routes GetLookupFillValues requests to master-specific fill logic.
type YarnLookupFillHandler struct {
	financev1.UnimplementedYarnLookupFillServiceServer
	machineRepo       machine.Repository
	interminglingRepo intermingling.Repository
	productGradeRepo  productgrade.Repository
	mbHeadRepo        mbhead.Repository
	boxBobbinRepo     boxbobbincost.Repository
}

// NewYarnLookupFillHandler creates a new YarnLookupFillHandler.
func NewYarnLookupFillHandler(
	machineRepo machine.Repository,
	interminglingRepo intermingling.Repository,
	productGradeRepo productgrade.Repository,
	mbHeadRepo mbhead.Repository,
	boxBobbinRepo boxbobbincost.Repository,
) (*YarnLookupFillHandler, error) {
	return &YarnLookupFillHandler{
		machineRepo:       machineRepo,
		interminglingRepo: interminglingRepo,
		productGradeRepo:  productGradeRepo,
		mbHeadRepo:        mbHeadRepo,
		boxBobbinRepo:     boxBobbinRepo,
	}, nil
}

// GetLookupFillValues routes to master-specific fill logic by lookup_master_code.
func (h *YarnLookupFillHandler) GetLookupFillValues(ctx context.Context, req *financev1.GetLookupFillValuesRequest) (*financev1.GetLookupFillValuesResponse, error) { //nolint:nilerr // BaseResponse pattern
	switch req.GetLookupMasterCode() {
	case "MACHINE":
		return h.fillFromMachine(ctx, req.GetSelectedKey())
	case "INTERMINGLING":
		return h.fillFromIntermingling(ctx, req.GetSelectedKey())
	case "PRODUCT_GRADE":
		return h.fillFromProductGrade(ctx, req.GetSelectedKey())
	case "MB_HEAD":
		return h.fillFromMBHead(ctx, req.GetSelectedKey())
	case "BOX_BOBBIN_COST":
		return h.fillFromBoxBobbinCost(ctx, req.GetSelectedKey(), req.GetSourceParamCode())
	default:
		return &financev1.GetLookupFillValuesResponse{
			Base: ErrorResponse("404", fmt.Sprintf("unknown lookup_master_code: %q", req.GetLookupMasterCode())),
		}, nil //nolint:nilerr // BaseResponse pattern
	}
}

func (h *YarnLookupFillHandler) fillFromMachine(ctx context.Context, mcCode string) (*financev1.GetLookupFillValuesResponse, error) {
	mc, err := h.machineRepo.GetByCode(ctx, mcCode)
	if err != nil {
		return &financev1.GetLookupFillValuesResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // BaseResponse pattern
	}
	nums := map[string]float64{
		"MC_SPEED":       mc.MCSpeed(),
		"MC_EFFICIENCY":  mc.MCEfficiency(),
		"NO_OF_POSITION": float64(mc.NoOfPosition()),
		"NO_OF_END":      float64(mc.NoOfEnd()),
	}
	if rpm := mc.MachineRPM(); rpm != nil {
		nums["MACHINE_RPM"] = *rpm
	}
	if power := mc.PowerPerDay(); power != nil {
		nums["POWER_PER_DAY"] = *power
	}
	label := fmt.Sprintf("%s (%s) — %d pos, %.0f m/min, %.1f%% eff",
		mc.Name(), mc.MCType(), mc.NoOfPosition(), mc.MCSpeed(), mc.MCEfficiency())
	return &financev1.GetLookupFillValuesResponse{
		Base:         successResponse("Machine fill values retrieved"),
		NumericFills: nums,
		TextFills:    map[string]string{},
		DisplayLabel: label,
	}, nil
}

func (h *YarnLookupFillHandler) fillFromIntermingling(ctx context.Context, intmCode string) (*financev1.GetLookupFillValuesResponse, error) {
	intm, err := h.interminglingRepo.GetByCode(ctx, intmCode)
	if err != nil {
		return &financev1.GetLookupFillValuesResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // BaseResponse pattern
	}
	label := fmt.Sprintf("%s (%s) — %.4f USD/kg", intm.Name(), intm.Code(), intm.CostPerKg())
	return &financev1.GetLookupFillValuesResponse{
		Base:         successResponse("Intermingling fill values retrieved"),
		NumericFills: map[string]float64{"INTERMINGLE_COST": intm.CostPerKg()},
		TextFills:    map[string]string{},
		DisplayLabel: label,
	}, nil
}

func (h *YarnLookupFillHandler) fillFromProductGrade(ctx context.Context, pgCode string) (*financev1.GetLookupFillValuesResponse, error) {
	grade, err := h.productGradeRepo.GetByCode(ctx, pgCode)
	if err != nil {
		return &financev1.GetLookupFillValuesResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // BaseResponse pattern
	}
	nums := map[string]float64{
		"BC_PERC":          grade.BCPerc(),
		"NON_STD_PERC":     grade.NonStdPerc(),
		"BC_RECOVERY_RATE": grade.BCRecoveryRate(),
	}
	label := fmt.Sprintf("%s — BC %.1f%%, NonStd %.1f%%, Recovery %.1f%%",
		grade.Name(), grade.BCPerc(), grade.NonStdPerc(), grade.BCRecoveryRate())
	return &financev1.GetLookupFillValuesResponse{
		Base:         successResponse("Product grade fill values retrieved"),
		NumericFills: nums,
		TextFills:    map[string]string{},
		DisplayLabel: label,
	}, nil
}

func (h *YarnLookupFillHandler) fillFromMBHead(ctx context.Context, mbCosting string) (*financev1.GetLookupFillValuesResponse, error) {
	mbh, err := h.mbHeadRepo.GetByMBCosting(ctx, mbCosting)
	if err != nil {
		return &financev1.GetLookupFillValuesResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // BaseResponse pattern
	}
	nums := map[string]float64{}
	texts := map[string]string{}
	if d := mbh.Dozing(); d != nil {
		nums["MB_DOZING_PCT"] = *d
	}
	if n := mbh.MgtName(); n != nil && *n != "" {
		texts["MB_DYE_NAME"] = *n
	}
	label := mbh.MBCosting()
	if d := mbh.Dozing(); d != nil {
		label = fmt.Sprintf("%s — %.2f%% dozing", mbh.MBCosting(), *d)
	}
	return &financev1.GetLookupFillValuesResponse{
		Base:         successResponse("MB Head fill values retrieved"),
		NumericFills: nums,
		TextFills:    texts,
		DisplayLabel: label,
	}, nil
}

func (h *YarnLookupFillHandler) fillFromBoxBobbinCost(ctx context.Context, bbcCode, sourceParamCode string) (*financev1.GetLookupFillValuesResponse, error) {
	bbc, err := h.boxBobbinRepo.GetByCode(ctx, bbcCode)
	if err != nil {
		return &financev1.GetLookupFillValuesResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // BaseResponse pattern
	}
	prefix := "CAP"
	if strings.HasPrefix(strings.ToUpper(sourceParamCode), "DEL_") {
		prefix = "DEL"
	}
	nums := map[string]float64{
		prefix + "_NO_OF_BOB": float64(bbc.NoOfBob()),
	}
	rates, rateErr := h.boxBobbinRepo.ListRates(ctx, bbc.ID())
	if rateErr != nil {
		log.Ctx(ctx).Warn().Err(rateErr).Str("bbc_code", bbcCode).Msg("ListRates failed — returning without rate fills")
	} else if len(rates) > 0 {
		sort.Slice(rates, func(i, j int) bool { return rates[i].Period() > rates[j].Period() })
		latest := rates[0]
		nums[prefix+"_BOB_RATE"] = latest.BobRateMkt()
		nums[prefix+"_BOX_RATE"] = latest.BoxRateMkt()
	}
	label := fmt.Sprintf("%s — %d bob/box", bbc.Name(), bbc.NoOfBob())
	return &financev1.GetLookupFillValuesResponse{
		Base:         successResponse("Box bobbin cost fill values retrieved"),
		NumericFills: nums,
		TextFills:    map[string]string{},
		DisplayLabel: label,
	}, nil
}

// compile-time interface check.
var _ financev1.YarnLookupFillServiceServer = (*YarnLookupFillHandler)(nil)
