package costcalc

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strconv"

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

	seqs, err := h.resolveStages(ctx, q.ProductSysID)
	if err != nil {
		return nil, err
	}
	if len(seqs) == 0 {
		return []RouteCostSheetStage{}, nil
	}

	stageIDs := make([]int64, 0, len(seqs))
	for _, s := range seqs {
		if s.ProductSysID > 0 {
			stageIDs = append(stageIDs, s.ProductSysID)
		}
	}
	costs, err := h.svc.resultRepo.ListByProductIDsPeriodType(ctx, stageIDs, q.Period, q.CalcType)
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

	out := make([]RouteCostSheetStage, 0, len(seqs))
	for _, s := range seqs {
		out = append(out, buildStage(s, costs[s.ProductSysID], products, capText[s.ProductSysID]))
	}
	return out, nil
}

// resolveStages returns the product's route stages ordered by (level, seq).
// An empty slice (no route, or a route with no stages) is a valid outcome.
func (h *GetRouteCostSheetHandler) resolveStages(ctx context.Context, productSysID int64) ([]*costroute.Seq, error) {
	graphs, err := h.svc.loader.LoadRoutesByProducts(ctx, []int64{productSysID})
	if err != nil {
		return nil, fmt.Errorf("load route: %w", err)
	}
	g, ok := graphs[productSysID]
	if !ok || g == nil {
		return nil, nil
	}
	seqs := make([]*costroute.Seq, 0, len(g.Seqs))
	for _, s := range g.Seqs {
		if s != nil {
			seqs = append(seqs, s)
		}
	}
	sort.SliceStable(seqs, func(i, j int) bool {
		if seqs[i].RouteLevel != seqs[j].RouteLevel {
			return seqs[i].RouteLevel < seqs[j].RouteLevel
		}
		return seqs[i].RouteSeq < seqs[j].RouteSeq
	})
	return seqs, nil
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
