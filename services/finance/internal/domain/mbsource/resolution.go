// Package mbsource resolves the "MB source" of a product (MB_SP_CODE / MB_SP_DYE and the
// spin fill-group children) from the product's SHADE code.
//
// It is the single place that knows which master a shade lives in. Providers are ordered
// (first hit wins) and batch-only, so the resolver can be called from a single product
// save as well as from bulk import / backfill without per-row round trips.
//
// Future merge: when the Superba Cost SP rows move into MB spin keyed by shade,
// SuperbaProvider is deleted, MBSpinProvider hits first for those shades, and the
// IS_SUPERBA branch of F_YARN_MB_COST is dropped by a guarded formula migration. Nothing
// persisted per product records the source, so no product rewrite is needed.
package mbsource

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Source identifiers carried on a Resolution.
const (
	// SourceMBSpin marks a resolution found in mst_mb_spin.
	SourceMBSpin = "MB_SPIN"
	// SourceSuperbaCostSP marks a resolution found in cost_superba_cost_sp.
	SourceSuperbaCostSP = "SUPERBA_COST_SP"
)

// Parameter codes and the marker written to cost_product_parameter.cpp_filled_by.
const (
	// ParamSPCode is the MASTER_LOOKUP trigger parameter.
	ParamSPCode = "MB_SP_CODE"
	// ParamSPDye is the dye / colour child parameter.
	ParamSPDye = "MB_SP_DYE"
	// FillGroupSPCode is the lookup_fill_group_code of the MB_SP_CODE children.
	FillGroupSPCode = "MB_SP_CODE"
	// FilledBy marks values written by the auto-fill, so a future "re-resolve" can find them.
	FilledBy = "auto_mb_source"
)

// ChildValue is the value of one fill-group child parameter (exactly one field is set).
type ChildValue struct {
	Num  *float64
	Text *string
}

// Resolution is the MB source found for one normalized shade.
type Resolution struct {
	// SPCode is the value for MB_SP_CODE (ORION item code for a spin, the shade for superba).
	SPCode string
	// DyeName is the value for MB_SP_DYE (spin MGT name / superba colour name).
	DyeName string
	// Source is SourceMBSpin or SourceSuperbaCostSP.
	Source string
	// SpinID is the chosen mst_mb_spin.mbs_id (nil for superba); stored as the CPP companion.
	SpinID *uuid.UUID
	// Children is the value per fill-group child param code (includes MB_SP_DYE when known).
	Children map[string]ChildValue
}

// NormalizeShade is THE shade match rule: UPPER(TRIM()) on both sides, exact.
func NormalizeShade(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

// Provider resolves a batch of shades against one master.
type Provider interface {
	// Name identifies the provider in logs.
	Name() string
	// ResolveByShades returns a Resolution per normalized shade it knows. Missing shades are
	// absent. Input shades may be un-normalized; keys of the result are normalized.
	ResolveByShades(ctx context.Context, shades []string) (map[string]Resolution, error)
}

// Resolver composes ordered providers; the first provider that knows a shade wins.
type Resolver struct{ providers []Provider }

// NewResolver builds a resolver; order matters (MB spin before superba).
func NewResolver(providers ...Provider) *Resolver { return &Resolver{providers: providers} }

// Resolve returns a Resolution per normalized shade. A provider error aborts the whole call:
// falling through to a later provider after a failure would mis-assign a shade.
func (r *Resolver) Resolve(ctx context.Context, shades []string) (map[string]Resolution, error) {
	out := make(map[string]Resolution, len(shades))
	pending := make([]string, 0, len(shades))
	seen := make(map[string]bool, len(shades))
	for _, s := range shades {
		n := NormalizeShade(s)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		pending = append(pending, n)
	}
	for _, p := range r.providers {
		if len(pending) == 0 {
			break
		}
		got, err := p.ResolveByShades(ctx, pending)
		if err != nil {
			return nil, fmt.Errorf("mb source provider %s: %w", p.Name(), err)
		}
		next := pending[:0:0]
		for _, s := range pending {
			if res, ok := got[s]; ok {
				out[s] = res
			} else {
				next = append(next, s)
			}
		}
		pending = next
	}
	return out, nil
}
