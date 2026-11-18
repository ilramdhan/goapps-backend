package mbsource

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// OverlaySpinShadeFgFinder answers FindByShadeAndFg from the Base finder first and fills
// only the keys the Base does not resolve from the in-memory simulated Extra links (for
// example the APPLY rows of a dry run). Base resolutions are never overridden.
type OverlaySpinShadeFgFinder struct {
	Base  SpinShadeFgFinder
	Extra map[ShadeFgKey]*mbspin.Entity
}

// FindByShadeAndFg implements SpinShadeFgFinder.
func (o *OverlaySpinShadeFgFinder) FindByShadeAndFg(ctx context.Context, keys []ShadeFgKey) (map[ShadeFgKey]*mbspin.Entity, error) {
	out, err := o.Base.FindByShadeAndFg(ctx, keys)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = make(map[ShadeFgKey]*mbspin.Entity)
	}
	for _, k := range keys {
		nk := ShadeFgKey{Shade: NormalizeShade(k.Shade), FgProductID: k.FgProductID}
		if _, ok := out[k]; ok {
			continue
		}
		if e, ok := o.Extra[nk]; ok {
			out[k] = e
		}
	}
	return out, nil
}
