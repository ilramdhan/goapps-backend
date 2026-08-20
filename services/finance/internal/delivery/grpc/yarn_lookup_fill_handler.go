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
	// mc_weightage was registered in mst_lookup_master_column by 000425 but had no
	// reader here, so the fill-group UI could offer the column while the handler
	// could not resolve it. Wired up alongside the MC_WEIGHTAGE param (000475).
	"mc_weightage": func(e *machine.Entity) (float64, bool) {
		if v := e.McWeightage(); v != nil {
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
	// D30: mbh_run_ldr_pct is the actual LDR used in production — the correct value for costing.
	"mbh_run_ldr_pct": func(e *mbhead.Entity) (float64, bool) {
		if v := e.MBHRunLdrPct(); v != nil {
			return *v, true
		}
		return 0, false
	},
	// D30: mbh_ldr_prsn is the planned LDR, set while the product is still new.
	"mbh_ldr_prsn": func(e *mbhead.Entity) (float64, bool) {
		if v := e.MBHLdrPrsn(); v != nil {
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
	// D30: mbs_dozing is the retired, contaminated legacy column. Kept registered on
	// purpose until L1 repoints lookup_source_column — removing it now would empty out
	// the fills that are currently live.
	"mbs_dozing": func(e *mbspin.Entity) (float64, bool) {
		if v := e.Dozing(); v != nil {
			return *v, true
		}
		return 0, false
	},
	// D30: mbs_run_ldr_pct is the actual LDR used in production — the correct value for costing.
	"mbs_run_ldr_pct": func(e *mbspin.Entity) (float64, bool) {
		if v := e.MBSRunLdrPct(); v != nil {
			return *v, true
		}
		return 0, false
	},
	// D30: mbs_ldr_prsn is the planned LDR, set while the product is still new.
	"mbs_ldr_prsn": func(e *mbspin.Entity) (float64, bool) {
		if v := e.MBSLdrPrsn(); v != nil {
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

// boxBobbinCostColumns mirrors the `case` labels of fillFromBoxBobbinCost, which
// resolves columns with a literal switch instead of a reader map and therefore
// cannot be enumerated reflectively. TestBoxBobbinSwitchColumnsMatchSource
// asserts this slice against the handler source so it cannot drift from the
// switch it copies.
var boxBobbinCostColumns = []string{
	"no_of_bob",
	"bbcr_bob_rate_mkt",
	"bbcr_box_rate_mkt",
}

// LookupReaderColumns returns every lookup_source_column the fill handler can
// actually resolve, keyed by lookup master code.
//
// This is derived from the reader maps above — the single source of truth. It
// exists so the startup registry-divergence check can compare the live contents
// of mst_lookup_master_column against the readers without maintaining a second
// hand-written column list, which is the very duplication that produced the R30
// drift in the first place.
func LookupReaderColumns() map[string]map[string]struct{} {
	out := make(map[string]map[string]struct{}, 6)
	add := func(master, col string) {
		if out[master] == nil {
			out[master] = map[string]struct{}{}
		}
		out[master][col] = struct{}{}
	}
	for col := range machineNumericReaders {
		add("MACHINE", col)
	}
	for col := range interminglingNumericReaders {
		add("INTERMINGLING", col)
	}
	for col := range productGradeNumericReaders {
		add("PRODUCT_GRADE", col)
	}
	for col := range productGradeTextReaders {
		add("PRODUCT_GRADE", col)
	}
	for col := range mbHeadNumericReaders {
		add("MB_HEAD", col)
	}
	for col := range mbHeadTextReaders {
		add("MB_HEAD", col)
	}
	for col := range mbSpinNumericReaders {
		add("MB_SPIN", col)
	}
	for col := range mbSpinTextReaders {
		add("MB_SPIN", col)
	}
	for _, col := range boxBobbinCostColumns {
		add("BOX_BOBBIN_COST", col)
	}
	return out
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

// warnNoReader reports a lookup_source_column that is registered in
// mst_lookup_master_column but has no reader in the Go-side reader maps (or no
// switch case). Such a column is silently skipped and yields an empty fill, so
// the only trace of the failure is this log line. It stays a warning rather than
// an error on purpose: registered-but-unreadable columns still exist in the wild
// and hard-failing would break otherwise working lookups.
func warnNoReader(ctx context.Context, source, col, paramCode, sourceParamCode string) {
	log.Ctx(ctx).Warn().
		Str("lookup_source_column", col).
		Str("param_code", paramCode).
		Str("source_param_code", sourceParamCode).
		Msg(source + " fill: no reader registered for lookup_source_column — value skipped")
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
		col := p.LookupSourceColumn()
		numReader, hasNumReader := machineNumericReaders[col]
		if !hasNumReader {
			// Machine has no text readers, so an unmapped column is unfillable.
			warnNoReader(ctx, "Machine", col, p.Code().String(), triggerParamCode)
			continue
		}
		if val, hasVal := numReader(mc); hasVal {
			nums[p.Code().String()] = val
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
		col := p.LookupSourceColumn()
		numReader, hasNumReader := interminglingNumericReaders[col]
		if !hasNumReader {
			// Intermingling has no text readers, so an unmapped column is unfillable.
			warnNoReader(ctx, "Intermingling", col, p.Code().String(), triggerParamCode)
			continue
		}
		if val, hasVal := numReader(intm); hasVal {
			nums[p.Code().String()] = val
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
		col := p.LookupSourceColumn()
		numReader, hasNumReader := productGradeNumericReaders[col]
		if hasNumReader {
			if val, hasVal := numReader(grade); hasVal {
				nums[p.Code().String()] = val
			}
		}
		textReader, hasTextReader := productGradeTextReaders[col]
		if hasTextReader {
			if val, hasVal := textReader(grade); hasVal {
				texts[p.Code().String()] = val
			}
		}
		if !hasNumReader && !hasTextReader {
			warnNoReader(ctx, "Product grade", col, p.Code().String(), triggerParamCode)
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
		col := p.LookupSourceColumn()
		numReader, hasNumReader := mbHeadNumericReaders[col]
		if hasNumReader {
			if val, hasVal := numReader(mbh); hasVal {
				nums[p.Code().String()] = val
			}
		}
		textReader, hasTextReader := mbHeadTextReaders[col]
		if hasTextReader {
			if val, hasVal := textReader(mbh); hasVal {
				texts[p.Code().String()] = val
			}
		}
		if !hasNumReader && !hasTextReader {
			warnNoReader(ctx, "MB Head", col, p.Code().String(), triggerParamCode)
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
		col := p.LookupSourceColumn()
		switch col {
		case "no_of_bob":
			nums[p.Code().String()] = float64(bbc.NoOfBob())
		case "bbcr_bob_rate_mkt":
			// A zero/missing rate is a deliberate no-fill, not an unknown column.
			if latestBobRateMkt > 0 {
				nums[p.Code().String()] = latestBobRateMkt
			}
		case "bbcr_box_rate_mkt":
			if latestBoxRateMkt > 0 {
				nums[p.Code().String()] = latestBoxRateMkt
			}
		default:
			warnNoReader(ctx, "Box bobbin cost", col, p.Code().String(), triggerParamCode)
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
	// Try ORION item code first (product params use CMBS_ORION_ITEM_CODE as key).
	spin, err := h.mbSpinRepo.GetByOrionItemCode(ctx, selectedKey)
	if err != nil {
		// Fallback to mb_costing lookup (legacy / direct entry).
		spin, err = h.mbSpinRepo.GetByMBCosting(ctx, selectedKey)
		if err != nil {
			return &financev1.GetLookupFillValuesResponse{
				Base: domainErrorToBaseResponse(err),
			}, nil //nolint:nilerr // BaseResponse pattern
		}
	}

	children, err := h.paramRepo.GetByFillGroup(ctx, sourceParamCode)
	if err != nil {
		return &financev1.GetLookupFillValuesResponse{Base: ErrorResponse("500", err.Error())}, nil //nolint:nilerr // BaseResponse pattern
	}

	nums := make(map[string]float64)
	texts := make(map[string]string)
	for _, p := range children {
		col := p.LookupSourceColumn()
		numReader, hasNumReader := mbSpinNumericReaders[col]
		if hasNumReader {
			if val, has := numReader(spin); has {
				nums[p.Code().String()] = val
			}
		}
		textReader, hasTextReader := mbSpinTextReaders[col]
		if hasTextReader {
			if val, has := textReader(spin); has {
				texts[p.Code().String()] = val
			}
		}
		if !hasNumReader && !hasTextReader {
			warnNoReader(ctx, "MB Spin", col, p.Code().String(), sourceParamCode)
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
