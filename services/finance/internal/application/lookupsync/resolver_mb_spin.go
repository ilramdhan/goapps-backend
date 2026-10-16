package lookupsync

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

func (r *Resolver) fillFromMBSpin(ctx context.Context, selectedKey, sourceParamCode string) (FillResult, error) {
	spin, err := r.ResolveMBSpin(ctx, selectedKey)
	if err != nil {
		return r.mbSpinFillMiss(ctx, selectedKey, err)
	}

	children, err := r.fillChildren(ctx, sourceParamCode)
	if err != nil {
		return FillResult{}, err
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
	return FillResult{
		Numeric:      nums,
		Text:         texts,
		DisplayLabel: label,
		Message:      successFillRetrieved,
	}, nil
}

// mbSpinFillMiss answers an MB_SPIN fill whose key is not a spin: a Superba shade (when the
// fallback is wired) fills the color name, anything else returns the original error.
func (r *Resolver) mbSpinFillMiss(ctx context.Context, selectedKey string, err error) (FillResult, error) {
	if errors.Is(err, mbspin.ErrNotFound) {
		if res, ok := r.fillFromSuperbaShade(ctx, selectedKey); ok {
			return res, nil
		}
	}
	return FillResult{}, err
}

// fillFromSuperbaShade answers an MB_SPIN fill for a key that is not a spin but a Superba shade.
// Only text children the superba source knows (MB_SP_DYE) are returned; numeric children stay
// empty (the MB cost of a Superba product comes from the IS_SUPERBA branch, not from rate/dozing).
func (r *Resolver) fillFromSuperbaShade(ctx context.Context, selectedKey string) (FillResult, bool) {
	if r.superbaFallback == nil {
		return FillResult{}, false
	}
	got, err := r.superbaFallback.ResolveByShades(ctx, []string{selectedKey})
	if err != nil {
		log.Warn().Err(err).Str("key", selectedKey).Msg("superba fallback for MB_SPIN fill failed")
		return FillResult{}, false
	}
	res, ok := got[mbsource.NormalizeShade(selectedKey)]
	if !ok {
		return FillResult{}, false
	}
	texts := make(map[string]string)
	for code, v := range res.Children {
		if v.Text != nil {
			texts[code] = *v.Text
		}
	}
	label := selectedKey
	if res.DyeName != "" {
		label = fmt.Sprintf("%s — %s", selectedKey, res.DyeName)
	}
	return FillResult{
		Numeric:      map[string]float64{},
		Text:         texts,
		DisplayLabel: label,
		Message:      "Fill values retrieved (Superba shade)",
	}, true
}

// ResolveMBSpin resolves selectedKey to an MB Spin entity for the read
// (lookup-fill) path.
//
// If selectedKey itself is a valid UUID, it is tried FIRST via GetByID — this is
// the permanent mst_mb_spin.mbs_id, e.g. the value carried by the new companion
// column cpp_value_mb_spin_id (migration 000494). A permanent-ID lookup is
// unambiguous by construction (primary key), so it takes priority over the
// legacy ORION-item-code / mb_costing chain, which can be ambiguous for the 177
// ORION codes shared by more than one spin. If selectedKey is not a UUID, or the
// UUID does not resolve, this falls through UNCHANGED to the original chain —
// existing legacy-row behavior (selectedKey = ORION code or mb_costing text) is
// not altered in any way.
func (r *Resolver) ResolveMBSpin(ctx context.Context, selectedKey string) (*mbspin.Entity, error) {
	if id, parseErr := uuid.Parse(selectedKey); parseErr == nil {
		if spin, err := r.mbSpinRepo.GetByID(ctx, id); err == nil && spin != nil {
			return spin, nil
		}
	}

	// Try ORION item code first (product params use CMBS_ORION_ITEM_CODE as key).
	spin, err := r.mbSpinRepo.GetByOrionItemCode(ctx, selectedKey)
	if err != nil {
		// Fallback to mb_costing lookup (legacy / direct entry).
		spin, err = r.mbSpinRepo.GetByMBCosting(ctx, selectedKey)
		if err != nil {
			return nil, err
		}
	}
	return spin, nil
}
