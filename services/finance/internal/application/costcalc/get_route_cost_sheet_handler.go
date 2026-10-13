package costcalc

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// GetRouteCostSheetQuery selects every route stage of one product for a given
// period and calculation type.
type GetRouteCostSheetQuery struct {
	ProductSysID int64
	Period       string
	CalcType     costcalcdom.CalculationType
}

// RouteCostSheetStage is one route stage — one column of the product cost sheet.
// ParamSnapshot is stringified so the transport layer can hand it straight to a
// map<string,string> without a second numeric round-trip; absent params stay
// absent so the renderer can print "-" rather than a fabricated zero.
type RouteCostSheetStage struct {
	RouteLevel    int32
	RouteSeq      int32
	RouteName     string
	ItemCode      string
	ProductName   string
	ShadeCode     string
	ShadeName     string
	ProductSysID  int64
	HasCost       bool
	ParamSnapshot map[string]string
	// LeftSysID is the legacy Oracle sys id (cpm_flex_02) — the "Left Sys ID"
	// column of the flat "all data" sheet. Empty for products that were never
	// imported from the legacy system.
	LeftSysID string
	// YarnType is the legacy product type label (cpm_flex_03) — "POY",
	// "MELANGE". The "Yarn Type" column of the flat "all data" sheet.
	YarnType string
	// OilGroupName is the display name of the RM group the stage's OIL_NAME
	// code points at (cst_rm_group_head.group_name), for the flat sheet's
	// "59.Oil Name" column (D18). Empty when the stage has no OIL_NAME, the
	// code matches no active group, or the loader cannot resolve names — the
	// exporter then falls back to the raw code. ParamSnapshot["OIL_NAME"]
	// keeps the code itself, so the gRPC read model is unchanged.
	OilGroupName string
}

// GetRouteCostSheetHandler assembles the full N-column cost sheet for one
// product in a single call: resolve the route, then batch-load every stage's
// cost snapshot.
type GetRouteCostSheetHandler struct {
	svc *Service
}

// NewGetRouteCostSheetHandler constructs the handler.
func NewGetRouteCostSheetHandler(svc *Service) *GetRouteCostSheetHandler {
	return &GetRouteCostSheetHandler{svc: svc}
}

// GetRouteCostSheetBatchQuery selects every route stage of several products
// for one period and calculation type.
type GetRouteCostSheetBatchQuery struct {
	ProductSysIDs []int64
	Period        string
	CalcType      costcalcdom.CalculationType
}

// Handle executes the query. A stage with no cost row for the requested
// period/type gets HasCost=false and an empty snapshot — never an error, since
// a partially-calculated route must still export.
func (h *GetRouteCostSheetHandler) Handle(ctx context.Context, q GetRouteCostSheetQuery) ([]RouteCostSheetStage, error) {
	if q.ProductSysID <= 0 {
		return nil, errors.New(errMsgProductIDPositive)
	}
	if len(q.Period) != 6 {
		return nil, errors.New(errMsgPeriodFormat)
	}
	out, err := h.buildSheets(ctx, []int64{q.ProductSysID}, q.Period, q.CalcType)
	if err != nil {
		return nil, err
	}
	if stages, ok := out[q.ProductSysID]; ok {
		return stages, nil
	}
	return []RouteCostSheetStage{}, nil
}

// HandleBatch is Handle for many products at once: the routes of every
// product are resolved in one loader call and every stage's cost row, product
// master row and text params are then loaded once for the union of stages,
// instead of ~8 queries per product. The result is keyed by product_sys_id and
// each value is exactly what Handle returns for that product (an empty slice
// for a product with no route or no stages), so callers can swap one for the
// other without changing output.
func (h *GetRouteCostSheetHandler) HandleBatch(ctx context.Context, q GetRouteCostSheetBatchQuery) (map[int64][]RouteCostSheetStage, error) {
	for _, id := range q.ProductSysIDs {
		if id <= 0 {
			return nil, errors.New(errMsgProductIDPositive)
		}
	}
	if len(q.Period) != 6 {
		return nil, errors.New(errMsgPeriodFormat)
	}
	out, err := h.buildSheets(ctx, q.ProductSysIDs, q.Period, q.CalcType)
	if err != nil {
		return nil, err
	}
	for _, id := range q.ProductSysIDs {
		if _, ok := out[id]; !ok {
			out[id] = []RouteCostSheetStage{}
		}
	}
	return out, nil
}

// buildSheets assembles the stage lists of every product in productSysIDs.
// Products without a route (or with an empty one) are absent from the result.
// Every lookup below is keyed per stage product (or per OIL_NAME code), so
// resolving several products together yields, per product, exactly what
// resolving it alone would.
func (h *GetRouteCostSheetHandler) buildSheets(
	ctx context.Context, productSysIDs []int64, period string, calcType costcalcdom.CalculationType,
) (map[int64][]RouteCostSheetStage, error) {
	seqsByProduct, err := h.resolveStagesBatch(ctx, productSysIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[int64][]RouteCostSheetStage, len(seqsByProduct))
	if len(seqsByProduct) == 0 {
		return out, nil
	}

	stageIDs := make([]int64, 0, len(seqsByProduct)*4)
	seen := map[int64]bool{}
	for _, pid := range productSysIDs {
		for _, s := range seqsByProduct[pid] {
			if s.ProductSysID > 0 && !seen[s.ProductSysID] {
				seen[s.ProductSysID] = true
				stageIDs = append(stageIDs, s.ProductSysID)
			}
		}
	}
	costs, err := h.svc.resultRepo.ListByProductIDsPeriodType(ctx, stageIDs, period, calcType)
	if err != nil {
		return nil, fmt.Errorf("load stage costs: %w", err)
	}
	products, err := h.svc.loader.LoadProducts(ctx, stageIDs)
	if err != nil {
		return nil, fmt.Errorf("load stage products: %w", err)
	}
	// Text-valued params (machine name, pack codes, loss types) never survive
	// into cpc_param_snapshot — the evaluator scope is float-only — so they are
	// read from current master data instead. See ProductLoader.LoadCAPPText.
	capText, err := h.svc.loader.LoadCAPPText(ctx, stageIDs)
	if err != nil {
		return nil, fmt.Errorf("load stage text params: %w", err)
	}

	if err := h.overrideSuperbaColours(ctx, stageIDs, capText); err != nil {
		return nil, err
	}

	oilNames, err := h.loadOilGroupNames(ctx, capText)
	if err != nil {
		return nil, err
	}

	for pid, seqs := range seqsByProduct {
		stages := make([]RouteCostSheetStage, 0, len(seqs))
		for _, s := range seqs {
			stage := buildStage(s, costs[s.ProductSysID], products, capText[s.ProductSysID])
			stage.OilGroupName = oilNames[strings.TrimSpace(capText[s.ProductSysID][oilNameParamCode])]
			stages = append(stages, stage)
		}
		out[pid] = stages
	}
	return out, nil
}

// overrideSuperbaColours replaces capText[pid][MB_SP_DYE] with the Superba colour
// name for SUPERBA products that resolve to a master row (display only: stored
// params are untouched). A loader without SuperbaColourLoader (test fakes) or a
// product without a resolved row keeps its stored value.
func (h *GetRouteCostSheetHandler) overrideSuperbaColours(ctx context.Context, stageIDs []int64, capText map[int64]map[string]string) error {
	cl, ok := h.svc.loader.(SuperbaColourLoader)
	if !ok {
		return nil
	}
	names, err := cl.LoadSuperbaColours(ctx, stageIDs)
	if err != nil {
		return fmt.Errorf("load superba names: %w", err)
	}
	for pid, name := range names {
		if capText[pid] == nil {
			capText[pid] = map[string]string{}
		}
		capText[pid][ParamCodeMBSpDye] = name
	}
	return nil
}

// oilNameParamCode is the text param holding a product's oil RM group code.
const oilNameParamCode = "OIL_NAME"

// loadOilGroupNames resolves every distinct OIL_NAME code among the stages to
// its RM group name. A loader without the OilGroupNameLoader capability (test
// fakes) yields an empty map, i.e. the exporter's raw-code fallback.
func (h *GetRouteCostSheetHandler) loadOilGroupNames(ctx context.Context, capText map[int64]map[string]string) (map[string]string, error) {
	nl, ok := h.svc.loader.(OilGroupNameLoader)
	if !ok {
		return map[string]string{}, nil
	}
	seen := map[string]bool{}
	codes := make([]string, 0, len(capText))
	for _, texts := range capText {
		code := strings.TrimSpace(texts[oilNameParamCode])
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		codes = append(codes, code)
	}
	if len(codes) == 0 {
		return map[string]string{}, nil
	}
	names, err := nl.LoadRMGroupNames(ctx, codes)
	if err != nil {
		return nil, fmt.Errorf("load oil group names: %w", err)
	}
	return names, nil
}

// resolveStagesBatch returns each product's route stages ordered by
// (level, seq), in one loader call. A product with no route, or a route with
// no stages, is absent from the result.
func (h *GetRouteCostSheetHandler) resolveStagesBatch(ctx context.Context, productSysIDs []int64) (map[int64][]*costroute.Seq, error) {
	graphs, err := h.svc.loader.LoadRoutesByProducts(ctx, productSysIDs)
	if err != nil {
		return nil, fmt.Errorf("load route: %w", err)
	}
	out := make(map[int64][]*costroute.Seq, len(productSysIDs))
	for _, pid := range productSysIDs {
		g, ok := graphs[pid]
		if !ok || g == nil {
			continue
		}
		seqs := make([]*costroute.Seq, 0, len(g.Seqs))
		for _, s := range g.Seqs {
			if s != nil {
				seqs = append(seqs, s)
			}
		}
		if len(seqs) == 0 {
			continue
		}
		sort.SliceStable(seqs, func(i, j int) bool {
			if seqs[i].RouteLevel != seqs[j].RouteLevel {
				return seqs[i].RouteLevel < seqs[j].RouteLevel
			}
			return seqs[i].RouteSeq < seqs[j].RouteSeq
		})
		out[pid] = seqs
	}
	return out, nil
}

// buildStage zips one route seq with its (possibly missing) cost row. Identity
// columns fall back to the route seq's own denormalized labels when the product
// master lookup misses, so the sheet header is never blank.
//
// capText carries the stage's text-valued params, read from master data rather
// than the snapshot. It is applied last and wins over any same-code numeric:
// a non-empty cpp_value_text means the master explicitly stores a label there
// ("NA", a machine name), which is more truthful than the 0 that
// buildInitialScope zero-fills into the scope for an absent numeric.
func buildStage(
	s *costroute.Seq,
	res *costcalcdom.Result,
	products map[int64]*costproductmaster.CostProductMaster,
	capText map[string]string,
) RouteCostSheetStage {
	stage := RouteCostSheetStage{
		RouteLevel:    s.RouteLevel,
		RouteSeq:      s.RouteSeq,
		RouteName:     s.RouteName,
		ItemCode:      s.RouteItemCode,
		ProductName:   s.ProductName,
		ShadeCode:     s.RouteShadeCode,
		ShadeName:     s.RouteShadeName,
		ProductSysID:  s.ProductSysID,
		ParamSnapshot: map[string]string{},
	}
	if p := products[s.ProductSysID]; p != nil {
		fillIdentityFromProductMaster(&stage, p)
	}
	fillCostFromResult(&stage, res)
	applyCapText(&stage, capText)
	return stage
}

// fillIdentityFromProductMaster backfills blank identity columns from the
// product master lookup, so the sheet header is never blank when the route
// seq's own denormalized labels are missing.
func fillIdentityFromProductMaster(stage *RouteCostSheetStage, p *costproductmaster.CostProductMaster) {
	if stage.ItemCode == "" {
		stage.ItemCode = p.ProductCode()
	}
	if stage.ProductName == "" {
		stage.ProductName = p.ProductName()
	}
	if stage.ShadeCode == "" {
		stage.ShadeCode = p.ShadeCode()
	}
	if stage.ShadeName == "" {
		stage.ShadeName = p.ShadeName()
	}
	// Legacy identity columns are only ever on the product master — the route
	// seq carries no denormalized copy — so these are assigned, not defaulted.
	stage.LeftSysID = p.Flex02()
	stage.YarnType = p.Flex03()
}

func fillCostFromResult(stage *RouteCostSheetStage, res *costcalcdom.Result) {
	if res == nil {
		return
	}
	stage.HasCost = true
	snap := map[string]float64{}
	// A corrupt snapshot degrades those columns to "-" rather than failing
	// the whole sheet; the text params below still land.
	if err := decodeJSONBlob(res.ParamSnapshot(), &snap); err == nil {
		for k, v := range snap {
			stage.ParamSnapshot[k] = strconv.FormatFloat(v, 'f', -1, 64)
		}
	}
}

// applyCapText overlays the stage's text-valued params, read from master data
// rather than the snapshot. It is applied last and wins over any same-code
// numeric: a non-empty cpp_value_text means the master explicitly stores a
// label there ("NA", a machine name), which is more truthful than the 0 that
// buildInitialScope zero-fills into the scope for an absent numeric.
func applyCapText(stage *RouteCostSheetStage, capText map[string]string) {
	maps.Copy(stage.ParamSnapshot, capText)
}
