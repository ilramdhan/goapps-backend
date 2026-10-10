// Package mbsourceautofill fills the MB source parameters (MB_SP_CODE, MB_SP_DYE and the spin
// fill-group children) of products from their SHADE code.
//
// Rules (user-approved 2026-10-10):
//   - Only EMPTY values are written; a product whose MB_SP_CODE already holds a value is
//     skipped entirely (its children belong to that manual pick). Locked and MB-typed
//     products are skipped.
//   - Params are attached (CAPP) via the AddApplicableWithChildren semantics, but ONLY for
//     products whose shade resolves, so non-MB products are not polluted.
//   - Values are tagged cpp_filled_by = mbsource.FilledBy so a future "re-resolve" can find them.
//   - Best-effort: callers log and continue on error; AutoFill never panics the caller.
package mbsourceautofill

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

// Target is one product eligible for auto-fill (unlocked, non-MB, shade set, MB_SP_CODE empty).
type Target struct {
	ProductSysID int64
	Shade        string
}

// ParamSet carries the parameter ids needed to write the MB source group.
type ParamSet struct {
	TriggerID uuid.UUID
	// Children maps child param code (MB_SP_DYE, ...) to its id.
	Children map[string]uuid.UUID
}

// Plan is the write for one product.
type Plan struct {
	ProductSysID int64
	Resolution   mbsource.Resolution
}

// Stats summarizes one run.
type Stats struct {
	Considered int
	Filled     int
	NoMatch    int
	Failed     int
	BySource   map[string]int
}

// Store is the persistence port.
type Store interface {
	// ListTargets returns the eligible subset of productSysIDs.
	ListTargets(ctx context.Context, productSysIDs []int64) ([]Target, error)
	// Apply attaches the group (idempotent) and writes empty values for one product.
	Apply(ctx context.Context, params ParamSet, plan Plan, actor string) error
}

// ParamReader is the slice of parameter.Repository used here.
type ParamReader interface {
	GetByCode(ctx context.Context, code parameter.Code) (*parameter.Parameter, error)
	GetByFillGroup(ctx context.Context, fillGroupCode string) ([]*parameter.Parameter, error)
}

// ShadeResolver is the slice of mbsource.Resolver used here.
type ShadeResolver interface {
	Resolve(ctx context.Context, shades []string) (map[string]mbsource.Resolution, error)
}

// Service is the auto-fill application service.
type Service struct {
	store    Store
	resolver ShadeResolver
	params   ParamReader
}

// New constructs the service.
func New(store Store, resolver ShadeResolver, params ParamReader) *Service {
	return &Service{store: store, resolver: resolver, params: params}
}

// AutoFillMBSource fills MB source params for the given products. Per-product write failures
// are logged and counted, not returned; a returned error means nothing could be attempted.
func (s *Service) AutoFillMBSource(ctx context.Context, productSysIDs []int64, actor string) (Stats, error) {
	stats := Stats{BySource: map[string]int{}}
	if len(productSysIDs) == 0 {
		return stats, nil
	}
	targets, err := s.store.ListTargets(ctx, productSysIDs)
	if err != nil {
		return stats, fmt.Errorf("list mb source targets: %w", err)
	}
	stats.Considered = len(targets)
	if len(targets) == 0 {
		return stats, nil
	}
	shades := make([]string, 0, len(targets))
	for _, t := range targets {
		shades = append(shades, t.Shade)
	}
	resolved, err := s.resolver.Resolve(ctx, shades)
	if err != nil {
		return stats, fmt.Errorf("resolve mb source: %w", err)
	}
	plans := make([]Plan, 0, len(targets))
	for _, t := range targets {
		res, ok := resolved[mbsource.NormalizeShade(t.Shade)]
		if !ok {
			stats.NoMatch++
			continue
		}
		plans = append(plans, Plan{ProductSysID: t.ProductSysID, Resolution: res})
	}
	if len(plans) == 0 {
		return stats, nil
	}
	ps, err := s.paramSet(ctx)
	if err != nil {
		return stats, err
	}
	for _, p := range plans {
		if err := s.store.Apply(ctx, ps, p, actor); err != nil {
			stats.Failed++
			log.Warn().Err(err).Int64("product_sys_id", p.ProductSysID).Msg("mb source auto-fill: apply failed")
			continue
		}
		stats.Filled++
		stats.BySource[p.Resolution.Source]++
	}
	return stats, nil
}

// BestEffort runs AutoFillMBSource and only logs failures; used by create/update/import hooks
// where auto-fill must never fail the surrounding operation.
func (s *Service) BestEffort(ctx context.Context, productSysIDs []int64, actor string) {
	defer func() {
		if r := recover(); r != nil {
			log.Error().Interface("panic", r).Msg("mb source auto-fill: recovered panic")
		}
	}()
	stats, err := s.AutoFillMBSource(ctx, productSysIDs, actor)
	if err != nil {
		log.Warn().Err(err).Msg("mb source auto-fill failed (ignored)")
		return
	}
	if stats.Filled > 0 || stats.Failed > 0 {
		log.Info().Int("filled", stats.Filled).Int("no_match", stats.NoMatch).Int("failed", stats.Failed).
			Interface("by_source", stats.BySource).Msg("mb source auto-fill")
	}
}

func (s *Service) paramSet(ctx context.Context) (ParamSet, error) {
	code, err := parameter.NewCode(mbsource.ParamSPCode)
	if err != nil {
		return ParamSet{}, err
	}
	trig, err := s.params.GetByCode(ctx, code)
	if err != nil {
		return ParamSet{}, fmt.Errorf("get %s param: %w", mbsource.ParamSPCode, err)
	}
	kids, err := s.params.GetByFillGroup(ctx, mbsource.FillGroupSPCode)
	if err != nil {
		return ParamSet{}, fmt.Errorf("get %s fill group: %w", mbsource.FillGroupSPCode, err)
	}
	ps := ParamSet{TriggerID: trig.ID(), Children: make(map[string]uuid.UUID, len(kids))}
	for _, k := range kids {
		ps.Children[k.Code().String()] = k.ID()
	}
	return ps, nil
}
