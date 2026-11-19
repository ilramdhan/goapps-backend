package mblegacyfill

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/google/uuid"

	appmbspin "github.com/mutugading/goapps-backend/services/finance/internal/application/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbdozing"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// SpinRecalcer adapts mbspin.RecalcService.Apply to Recalculator and OrionResolver.
type SpinRecalcer struct {
	Repo mbspin.Repository
	Svc  *appmbspin.RecalcService
}

var (
	_ Recalculator  = (*SpinRecalcer)(nil)
	_ OrionResolver = (*SpinRecalcer)(nil)
)

// OrionCodeOf returns the current orion item code of a spin ("" when none).
func (r *SpinRecalcer) OrionCodeOf(ctx context.Context, spinID string) (string, error) {
	id, err := uuid.Parse(spinID)
	if err != nil {
		return "", fmt.Errorf("spin id: %w", err)
	}
	sp, err := r.Repo.GetByID(ctx, id)
	if err != nil {
		return "", fmt.Errorf("load spin: %w", err)
	}
	if oc := sp.OrionItemCode(); oc != nil {
		return *oc, nil
	}
	return "", nil
}

// RecalcSpin reloads the (post-commit) parent spin and applies the child recalc.
func (r *SpinRecalcer) RecalcSpin(ctx context.Context, t RecalcTarget, actor string) (RecalcOutcome, error) {
	var o RecalcOutcome
	id, err := uuid.Parse(t.SpinID)
	if err != nil {
		return o, fmt.Errorf("spin id: %w", err)
	}
	parent, err := r.Repo.GetByID(ctx, id)
	if err != nil {
		return o, fmt.Errorf("load spin: %w", err)
	}
	if oc := parent.OrionItemCode(); oc != nil {
		o.OrionCode = *oc
	}
	res, err := r.Svc.Apply(ctx, appmbspin.ApplyInput{Parent: parent, OldDozing: t.OldDozing, Actor: actor})
	if err != nil {
		return o, err
	}
	o.Result, o.Recalculated, o.Incomplete = RecalcOK, len(res.Recalculated), len(res.Incomplete)+len(res.LDRIncomplete)
	seen := map[string]bool{}
	for _, c := range res.Recalculated {
		seen[c.SpinID.String()] = true
	}
	for _, c := range res.LDRRecalculated {
		seen[c.SpinID.String()] = true
	}
	for id := range seen {
		o.ChildIDs = append(o.ChildIDs, id)
	}
	sort.Strings(o.ChildIDs)
	// The recalc rewrote these children's mbs_dozing after the head tx committed.
	// Only dozing-rewritten children can have lost a legacy dozing.
	doz := make([]string, 0, len(res.Recalculated))
	for _, c := range res.Recalculated {
		doz = append(doz, c.SpinID.String())
	}
	o.OverwroteLegacy = OverwrittenLegacy(doz, t.LegacyDozing)
	sort.Strings(o.OverwroteLegacy)
	return o, nil
}

// OrionResolver resolves a spin's current orion item code (read-only).
type OrionResolver interface {
	OrionCodeOf(ctx context.Context, spinID string) (string, error)
}

// productCols are spin columns whose change can stale MB_SP_* values on products.
var productCols = map[string]bool{
	"mbs_denier": true, "mbs_filament": true, "mbs_dozing": true,
	"mbs_orion_item_code": true, "mbs_shade_code": true, "mbs_shade_name": true, "mbs_mb_costing": true,
}

const orionCol = "mbs_orion_item_code"

// AffectedCode is one orion code to look up, with the reason it is affected.
type AffectedCode struct{ code, reason string }

// AffectedOrionCodes derives, deduped and sorted by code, every orion code whose bound
// products may hold stale MB_SP_* values: recalc parents and rewritten R and D children
// (current code), spins whose orion/shade/costing/den/fil/doz changed directly or via the
// head cascade (current code), and the OLD orion code of any orion change (what products
// still reference). Resolver errors are returned.
func AffectedOrionCodes(ctx context.Context, rs OrionResolver, results []RowResult, outs []RecalcOutcome) ([]AffectedCode, error) {
	c := &codeCollector{ctx: ctx, rs: rs, reasons: map[string]string{}}
	if err := c.fromOutcomes(outs); err != nil {
		return nil, err
	}
	for _, rr := range results {
		if rr.Result != ResultApplied {
			continue
		}
		if err := c.fromResult(rr); err != nil {
			return nil, err
		}
	}
	out := make([]AffectedCode, 0, len(c.reasons))
	for code, r := range c.reasons {
		out = append(out, AffectedCode{code, r})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].code < out[j].code })
	return out, nil
}

// codeCollector accumulates affected orion codes (first reason wins).
type codeCollector struct {
	ctx     context.Context
	rs      OrionResolver
	reasons map[string]string
}

func (c *codeCollector) add(code, reason string) {
	if code == "" {
		return
	}
	if _, ok := c.reasons[code]; !ok {
		c.reasons[code] = reason
	}
}

func (c *codeCollector) cur(spinID, reason string) error {
	code, err := c.rs.OrionCodeOf(c.ctx, spinID)
	if err != nil {
		return fmt.Errorf("orion of spin %s: %w", spinID, err)
	}
	c.add(code, reason)
	return nil
}

func (c *codeCollector) fromOutcomes(outs []RecalcOutcome) error {
	for _, o := range outs {
		c.add(o.OrionCode, "spin "+o.SpinID+" recalculated (orion "+o.OrionCode+")")
		for _, ch := range o.ChildIDs {
			if err := c.cur(ch, "child spin "+ch+" rewritten by recalc of "+o.SpinID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *codeCollector) fromResult(rr RowResult) error {
	if rr.Row.Entity == EntitySpin && productCols[rr.Row.Spec.Column] {
		if err := c.cur(rr.Row.ID, "spin "+rr.Row.ID+" "+rr.Row.Spec.Column+" changed"); err != nil {
			return err
		}
		if rr.Row.Spec.Column == orionCol {
			addOld(rr.BeforeJSON, c.add, rr.Row.ID)
		}
	}
	if rr.SideEffects == "" {
		return nil
	}
	fx, ok := parseFx(rr.SideEffects)
	if !ok {
		return nil
	}
	for id, ch := range fx.SpinChanges {
		if !touchesProductCol(ch.After) {
			continue
		}
		if err := c.cur(id, "spin "+id+" changed by head cascade"); err != nil {
			return err
		}
		if _, ok := ch.After[orionCol]; ok {
			addOldMap(ch.Before, c.add, id)
		}
	}
	return nil
}

func parseFx(raw string) (fxJSON, bool) {
	var fx fxJSON
	err := json.Unmarshal([]byte(raw), &fx)
	return fx, err == nil
}

func touchesProductCol[V any](after map[string]V) bool {
	for col := range after {
		if productCols[col] {
			return true
		}
	}
	return false
}

func addOld(beforeJSON string, add func(code, reason string), id string) {
	var m map[string]string
	if json.Unmarshal([]byte(beforeJSON), &m) == nil {
		addOldMap(m, add, id)
	}
}

func addOldMap(before map[string]string, add func(code, reason string), id string) {
	if v := before[orionCol]; v != "" {
		add(v, "spin "+id+" orion changed from "+v+" (old code still bound)")
	}
}

// ImpactLimit is the per-code row cap of ImpactRepository (see postgres maxImpactLimit).
const ImpactLimit = 200

// CollectAffectedProducts lists yarn products bound to the affected orion codes
// (read-only impact SELECT), deduped by product id, for affected-products.csv.
// A code with more bound products than were returned (repo cap) adds a TRUNCATED
// row (product_id 0) so the operator knows the list is incomplete.
func CollectAffectedProducts(ctx context.Context, impact mbdozing.ImpactRepository, rs OrionResolver, results []RowResult, outs []RecalcOutcome) ([]AffectedProduct, error) {
	codes, err := AffectedOrionCodes(ctx, rs, results, outs)
	if err != nil {
		return nil, err
	}
	seen := map[int64]bool{}
	var rows []AffectedProduct
	for _, ac := range codes {
		items, totals, err := impact.ImpactBySpin(ctx, ac.code, ImpactLimit)
		if err != nil {
			return nil, fmt.Errorf("impact %s: %w", ac.code, err)
		}
		for _, it := range items {
			if seen[it.ProductSysID] {
				continue
			}
			seen[it.ProductSysID] = true
			rows = append(rows, AffectedProduct{ProductID: it.ProductSysID, ItemCode: it.ProductCode, Reason: ac.reason})
		}
		if totals.TotalAffected > len(items) {
			rows = append(rows, AffectedProduct{ItemCode: ac.code, Reason: fmt.Sprintf(
				"TRUNCATED: orion %s has %d bound products, only %d listed; lookup-sync the rest by orion code",
				ac.code, totals.TotalAffected, len(items))})
		}
	}
	return rows, nil
}
