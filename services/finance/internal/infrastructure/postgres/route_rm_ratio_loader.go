package postgres

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres/rmratioquery"
)

// RouteRMRatioLoader implements costroute application's RatioSourceLoader. It
// reuses the calc engine's exact SQL (rmratioquery), so the route graph shows
// the same ratio the engine computes. At most 4 batched queries per graph:
// ratio modes of the seq products, then -- only when some seq is DENIER-mode
// -- upstream DENIER, RM group head denier and SPANDEX_DRAFT.
type RouteRMRatioLoader struct {
	db *DB
}

// NewRouteRMRatioLoader constructs the loader.
func NewRouteRMRatioLoader(db *DB) *RouteRMRatioLoader {
	return &RouteRMRatioLoader{db: db}
}

// LoadRMRatioSources loads the ratio sources for g.
func (l *RouteRMRatioLoader) LoadRMRatioSources(ctx context.Context, g *costroute.Graph) (costroute.RatioSources, error) {
	var src costroute.RatioSources
	if g == nil {
		return src, nil
	}
	modes, err := rmratioquery.ProductRatioModes(ctx, l.db, graphProductIDs(g))
	if err != nil {
		return src, fmt.Errorf("route RM ratio: %w", err)
	}
	src.Modes = modes
	upstreamIDs, groupCodes, denierProducts := g.DenierInputKeys(modes)
	if len(denierProducts) == 0 {
		return src, nil
	}
	if src.UpstreamDenier, err = rmratioquery.ProductParamValues(ctx, l.db, upstreamIDs, costroute.ParamCodeDenier); err != nil {
		return src, fmt.Errorf("route RM ratio: %w", err)
	}
	if src.GroupDenier, src.GroupUseDraft, err = rmratioquery.GroupDeniers(ctx, l.db, groupCodes); err != nil {
		return src, fmt.Errorf("route RM ratio: %w", err)
	}
	if src.SpandexDraft, err = rmratioquery.ProductParamValues(ctx, l.db, denierProducts, costroute.ParamCodeSpandexDraft); err != nil {
		return src, fmt.Errorf("route RM ratio: %w", err)
	}
	return src, nil
}

// graphProductIDs returns the deduplicated head + seq product ids of g.
func graphProductIDs(g *costroute.Graph) []int64 {
	seen := map[int64]bool{}
	var out []int64
	add := func(id int64) {
		if id != 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if g.Head != nil {
		add(g.Head.ProductSysID)
	}
	for _, s := range g.Seqs {
		if s != nil {
			add(s.ProductSysID)
		}
	}
	return out
}
