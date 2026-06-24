// Package grpc provides gRPC server implementation for the finance service.
package grpc

import (
	"context"
	"fmt"
	"sort"

	"github.com/rs/zerolog/log"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/boxbobbincost"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/intermingling"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/machine"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/productgrade"
)

// machineNumericReaders maps lookup_source_column → value extractor for mst_machine entity.
// Add new entries here when new fillable columns are added to mst_lookup_master_column.
var machineNumericReaders = map[string]func(*machine.Entity) (float64, bool){
	"mc_speed":       func(e *machine.Entity) (float64, bool) { return e.MCSpeed(), true },
	"mc_efficiency":  func(e *machine.Entity) (float64, bool) { return e.MCEfficiency(), true },
	"no_of_position": func(e *machine.Entity) (float64, bool) { return float64(e.NoOfPosition()), true },
	"no_of_end":      func(e *machine.Entity) (float64, bool) { return float64(e.NoOfEnd()), true },
	"machine_rpm": func(e *machine.Entity) (float64, bool) {
		if v := e.MachineRPM(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"power_per_day": func(e *machine.Entity) (float64, bool) {
		if v := e.PowerPerDay(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"mp_per_day": func(e *machine.Entity) (float64, bool) {
		if v := e.MpPerDay(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"ohs_per_day": func(e *machine.Entity) (float64, bool) {
		if v := e.OhsPerDay(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"spares_per_day": func(e *machine.Entity) (float64, bool) {
		if v := e.SparesPerDay(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"kgs_lost_change": func(e *machine.Entity) (float64, bool) {
		if v := e.KgsLostChange(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"vb1_qty": func(e *machine.Entity) (float64, bool) {
		if v := e.Vb1Qty(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"vb2_qty": func(e *machine.Entity) (float64, bool) {
		if v := e.Vb2Qty(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"vb3_qty": func(e *machine.Entity) (float64, bool) {
		if v := e.Vb3Qty(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"vb4_qty": func(e *machine.Entity) (float64, bool) {
		if v := e.Vb4Qty(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"vb5_qty": func(e *machine.Entity) (float64, bool) {
		if v := e.Vb5Qty(); v != nil {
			return *v, true
		}
		return 0, false
	},
}

// interminglingNumericReaders maps lookup_source_column → value extractor for mst_intermingling entity.
var interminglingNumericReaders = map[string]func(*intermingling.Entity) (float64, bool){
	"intm_cost_per_kg": func(e *intermingling.Entity) (float64, bool) { return e.CostPerKg(), true },
}

// productGradeNumericReaders maps lookup_source_column → value extractor for mst_product_grade entity.
var productGradeNumericReaders = map[string]func(*productgrade.Entity) (float64, bool){
	"bc_perc":           func(e *productgrade.Entity) (float64, bool) { return e.BCPerc(), true },
	"non_std_perc":      func(e *productgrade.Entity) (float64, bool) { return e.NonStdPerc(), true },
	"bc_recovery_rate":  func(e *productgrade.Entity) (float64, bool) { return e.BCRecoveryRate(), true },
	"std_selling_price": func(e *productgrade.Entity) (float64, bool) { return e.StdSellingPrice(), true },
	"sp_value":          func(e *productgrade.Entity) (float64, bool) { return e.SpValue(), true },
}

// productGradeTextReaders maps lookup_source_column → text value extractor for mst_product_grade entity.
var productGradeTextReaders = map[string]func(*productgrade.Entity) (string, bool){
	"pg_detail_product": func(e *productgrade.Entity) (string, bool) {
		if v := e.PgDetailProduct(); v != "" {
			return v, true
		}
		return "", false
	},
	"pg_grade_label": func(e *productgrade.Entity) (string, bool) {
		if v := e.PgGradeLabel(); v != "" {
			return v, true
		}
		return "", false
	},
}

// mbHeadNumericReaders maps lookup_source_column → numeric value extractor for mst_mb_head entity.
var mbHeadNumericReaders = map[string]func(*mbhead.Entity) (float64, bool){
	"mbh_dozing": func(e *mbhead.Entity) (float64, bool) {
		if v := e.Dozing(); v != nil {
			return *v, true
		}
		return 0, false
	},
}

// mbHeadTextReaders maps lookup_source_column → text value extractor for mst_mb_head entity.
var mbHeadTextReaders = map[string]func(*mbhead.Entity) (string, bool){
	"mbh_mgt_name": func(e *mbhead.Entity) (string, bool) {
		if v := e.MgtName(); v != nil && *v != "" {
			return *v, true
		}
		return "", false
	},
}

// mbSpinNumericReaders maps lookup_source_column → numeric value extractor for mst_mb_spin entity.
var mbSpinNumericReaders = map[string]func(*mbspin.Entity) (float64, bool){
	"mbs_denier": func(e *mbspin.Entity) (float64, bool) {
		if v := e.Denier(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"mbs_dozing": func(e *mbspin.Entity) (float64, bool) {
		if v := e.Dozing(); v != nil {
			return *v, true
		}
		return 0, false
	},
	"mbs_filament": func(e *mbspin.Entity) (float64, bool) {
		if v := e.Filament(); v != nil {
			return float64(*v), true
		}
		return 0, false
	},
	"mbs_cost_rate_mkt": func(e *mbspin.Entity) (float64, bool) {
		if v := e.CostRateMkt(); v != nil {
			return *v, true
		}
		return 0, false
	},
}

// mbSpinTextReaders maps lookup_source_column → text value extractor for mst_mb_spin entity.
var mbSpinTextReaders = map[string]func(*mbspin.Entity) (string, bool){
	"mbs_mgt_name": func(e *mbspin.Entity) (string, bool) {
		if v := e.MgtName(); v != "" {
			return v, true
		}
		return "", false
	},
	"mbs_cc": func(e *mbspin.Entity) (string, bool) {
		if v := e.CC(); v != nil && *v != "" {
			return *v, true
		}
		return "", false
	},
}

// YarnLookupFillHandler implements financev1.YarnLookupFillServiceServer.
// It routes GetLookupFillValues requests to master-specific fill logic.
type YarnLookupFillHandler struct {
	financev1.UnimplementedYarnLookupFillServiceServer
	machineRepo       machine.Repository
	interminglingRepo intermingling.Repository
	productGradeRepo  productgrade.Repository
	mbHeadRepo        mbhead.Repository
	mbSpinRepo        mbspin.Repository
	boxBobbinRepo     boxbobbincost.Repository
	paramRepo         parameter.Repository
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

// GetLookupFillValues routes to master-specific fill logic by lookup_master_code.
func (h *YarnLookupFillHandler) GetLookupFillValues(ctx context.Context, req *financev1.GetLookupFillValuesRequest) (*financev1.GetLookupFillValuesResponse, error) { //nolint:nilerr // BaseResponse pattern
	switch req.GetLookupMasterCode() {
	case "MACHINE":
		return h.fillFromMachine(ctx, req.GetSelectedKey(), req.GetSourceParamCode())
	case "INTERMINGLING":
		return h.fillFromIntermingling(ctx, req.GetSelectedKey(), req.GetSourceParamCode())
	case "PRODUCT_GRADE":
		return h.fillFromProductGrade(ctx, req.GetSelectedKey(), req.GetSourceParamCode())
	case "MB_HEAD":
		return h.fillFromMBHead(ctx, req.GetSelectedKey(), req.GetSourceParamCode())
	case "MB_SPIN":
		return h.fillFromMBSpin(ctx, req.GetSelectedKey(), req.GetSourceParamCode())
	case "BOX_BOBBIN_COST":
		return h.fillFromBoxBobbinCost(ctx, req.GetSelectedKey(), req.GetSourceParamCode())
	default:
		return &financev1.GetLookupFillValuesResponse{
			Base: ErrorResponse("404", fmt.Sprintf("unknown lookup_master_code: %q", req.GetLookupMasterCode())),
		}, nil //nolint:nilerr // BaseResponse pattern
	}
}

func (h *YarnLookupFillHandler) fillFromMachine(ctx context.Context, mcCode, triggerParamCode string) (*financev1.GetLookupFillValuesResponse, error) {
	mc, err := h.machineRepo.GetByCode(ctx, mcCode)
	if err != nil {
		return &financev1.GetLookupFillValuesResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // BaseResponse pattern
	}

	childParams, err := h.paramRepo.GetByFillGroup(ctx, triggerParamCode)
	if err != nil {
		return &financev1.GetLookupFillValuesResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // BaseResponse pattern
	}

	nums := make(map[string]float64, len(childParams))
	for _, p := range childParams {
		if reader, ok := machineNumericReaders[p.LookupSourceColumn()]; ok {
			if val, hasVal := reader(mc); hasVal {
				nums[p.Code().String()] = val
			}
		}
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

func (h *YarnLookupFillHandler) fillFromIntermingling(ctx context.Context, intmCode, triggerParamCode string) (*financev1.GetLookupFillValuesResponse, error) {
	intm, err := h.interminglingRepo.GetByCode(ctx, intmCode)
	if err != nil {
		return &financev1.GetLookupFillValuesResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // BaseResponse pattern
	}

	childParams, err := h.paramRepo.GetByFillGroup(ctx, triggerParamCode)
	if err != nil {
		return &financev1.GetLookupFillValuesResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // BaseResponse pattern
	}

	nums := make(map[string]float64, len(childParams))
	for _, p := range childParams {
		if reader, ok := interminglingNumericReaders[p.LookupSourceColumn()]; ok {
			if val, hasVal := reader(intm); hasVal {
				nums[p.Code().String()] = val
			}
		}
	}

	label := fmt.Sprintf("%s (%s) — %.4f USD/kg", intm.Name(), intm.Code(), intm.CostPerKg())
	return &financev1.GetLookupFillValuesResponse{
		Base:         successResponse("Intermingling fill values retrieved"),
		NumericFills: nums,
		TextFills:    map[string]string{},
		DisplayLabel: label,
	}, nil
}

func (h *YarnLookupFillHandler) fillFromProductGrade(ctx context.Context, pgCode, triggerParamCode string) (*financev1.GetLookupFillValuesResponse, error) {
	grade, err := h.productGradeRepo.GetByCode(ctx, pgCode)
	if err != nil {
		return &financev1.GetLookupFillValuesResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // BaseResponse pattern
	}

	childParams, err := h.paramRepo.GetByFillGroup(ctx, triggerParamCode)
	if err != nil {
		return &financev1.GetLookupFillValuesResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // BaseResponse pattern
	}

	nums := make(map[string]float64, len(childParams))
	texts := make(map[string]string, len(childParams))
	for _, p := range childParams {
		if reader, ok := productGradeNumericReaders[p.LookupSourceColumn()]; ok {
			if val, hasVal := reader(grade); hasVal {
				nums[p.Code().String()] = val
			}
		}
		if reader, ok := productGradeTextReaders[p.LookupSourceColumn()]; ok {
			if val, hasVal := reader(grade); hasVal {
				texts[p.Code().String()] = val
			}
		}
	}

	label := fmt.Sprintf("%s — BC %.1f%%, NonStd %.1f%%, Recovery %.1f%%",
		grade.Name(), grade.BCPerc(), grade.NonStdPerc(), grade.BCRecoveryRate())
	return &financev1.GetLookupFillValuesResponse{
		Base:         successResponse("Product grade fill values retrieved"),
		NumericFills: nums,
		TextFills:    texts,
		DisplayLabel: label,
	}, nil
}

func (h *YarnLookupFillHandler) fillFromMBHead(ctx context.Context, mbCosting, triggerParamCode string) (*financev1.GetLookupFillValuesResponse, error) {
	mbh, err := h.mbHeadRepo.GetByMBCosting(ctx, mbCosting)
	if err != nil {
		return &financev1.GetLookupFillValuesResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // BaseResponse pattern
	}

	childParams, err := h.paramRepo.GetByFillGroup(ctx, triggerParamCode)
	if err != nil {
		return &financev1.GetLookupFillValuesResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // BaseResponse pattern
	}

	nums := make(map[string]float64, len(childParams))
	texts := make(map[string]string, len(childParams))
	for _, p := range childParams {
		if reader, ok := mbHeadNumericReaders[p.LookupSourceColumn()]; ok {
			if val, hasVal := reader(mbh); hasVal {
				nums[p.Code().String()] = val
			}
		}
		if reader, ok := mbHeadTextReaders[p.LookupSourceColumn()]; ok {
			if val, hasVal := reader(mbh); hasVal {
				texts[p.Code().String()] = val
			}
		}
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

func (h *YarnLookupFillHandler) fillFromBoxBobbinCost(ctx context.Context, bbcCode, triggerParamCode string) (*financev1.GetLookupFillValuesResponse, error) {
	bbc, err := h.boxBobbinRepo.GetByCode(ctx, bbcCode)
	if err != nil {
		return &financev1.GetLookupFillValuesResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // BaseResponse pattern
	}

	childParams, err := h.paramRepo.GetByFillGroup(ctx, triggerParamCode)
	if err != nil {
		return &financev1.GetLookupFillValuesResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // BaseResponse pattern
	}

	// Get latest rates (best-effort).
	var latestBobRateMkt, latestBoxRateMkt float64
	rates, rateErr := h.boxBobbinRepo.ListRates(ctx, bbc.ID())
	if rateErr != nil {
		log.Ctx(ctx).Warn().Err(rateErr).Str("bbc_code", bbcCode).Msg("ListRates failed — returning without rate fills")
	} else if len(rates) > 0 {
		sort.Slice(rates, func(i, j int) bool { return rates[i].Period() > rates[j].Period() })
		latestBobRateMkt = rates[0].BobRateMkt()
		latestBoxRateMkt = rates[0].BoxRateMkt()
	}

	nums := make(map[string]float64, len(childParams))
	for _, p := range childParams {
		switch p.LookupSourceColumn() {
		case "no_of_bob":
			nums[p.Code().String()] = float64(bbc.NoOfBob())
		case "bbcr_bob_rate_mkt":
			if latestBobRateMkt > 0 {
				nums[p.Code().String()] = latestBobRateMkt
			}
		case "bbcr_box_rate_mkt":
			if latestBoxRateMkt > 0 {
				nums[p.Code().String()] = latestBoxRateMkt
			}
		}
	}

	label := fmt.Sprintf("%s — %d bob/box", bbc.Name(), bbc.NoOfBob())
	return &financev1.GetLookupFillValuesResponse{
		Base:         successResponse("Box bobbin cost fill values retrieved"),
		NumericFills: nums,
		TextFills:    map[string]string{},
		DisplayLabel: label,
	}, nil
}

func (h *YarnLookupFillHandler) fillFromMBSpin(ctx context.Context, selectedKey, sourceParamCode string) (*financev1.GetLookupFillValuesResponse, error) {
	spin, err := h.mbSpinRepo.GetByMBCosting(ctx, selectedKey)
	if err != nil {
		return &financev1.GetLookupFillValuesResponse{ //nolint:nilerr // BaseResponse pattern: error surfaced in response body
			Base: ErrorResponse("404", fmt.Sprintf("MB Spin not found: %s", selectedKey)),
		}, nil
	}

	children, err := h.paramRepo.GetByFillGroup(ctx, sourceParamCode)
	if err != nil {
		return &financev1.GetLookupFillValuesResponse{Base: ErrorResponse("500", err.Error())}, nil //nolint:nilerr // BaseResponse pattern
	}

	nums := make(map[string]float64)
	texts := make(map[string]string)
	for _, p := range children {
		col := p.LookupSourceColumn()
		if reader, ok := mbSpinNumericReaders[col]; ok {
			if val, has := reader(spin); has {
				nums[p.Code().String()] = val
			}
		}
		if reader, ok := mbSpinTextReaders[col]; ok {
			if val, has := reader(spin); has {
				texts[p.Code().String()] = val
			}
		}
	}

	label := fmt.Sprintf("%s — %s", selectedKey, spin.MgtName())
	return &financev1.GetLookupFillValuesResponse{
		Base:         successResponse("Fill values retrieved"),
		NumericFills: nums,
		TextFills:    texts,
		DisplayLabel: label,
	}, nil
}

// compile-time interface check.
var _ financev1.YarnLookupFillServiceServer = (*YarnLookupFillHandler)(nil)
