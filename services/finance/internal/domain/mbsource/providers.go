package mbsource

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/superbacostsp"
)

// SpinPickOrderSQL is THE deterministic pick rule when several live MB spin rows share a
// shade (decision Q2): Spinning > Boughtout > R and D (anything else last), then newest,
// then id. It is the ORDER BY of a DISTINCT ON (normalized shade) query; migration 000569
// embeds the same text and a test asserts both stay identical.
const SpinPickOrderSQL = `UPPER(TRIM(mbs_shade_code)),
	CASE mbs_status WHEN 'Spinning' THEN 1 WHEN 'Boughtout' THEN 2 WHEN 'R and D' THEN 3 ELSE 4 END,
	GREATEST(created_at, COALESCE(updated_at, created_at)) DESC,
	mbs_id`

// SpinShadeFinder returns, per normalized shade, the picked MB spin (see SpinPickOrderSQL).
type SpinShadeFinder interface {
	FindByShades(ctx context.Context, normShades []string) (map[string]*mbspin.Entity, error)
}

// FillGroupLister lists the fill-group children of MB_SP_CODE.
type FillGroupLister interface {
	GetByFillGroup(ctx context.Context, fillGroupCode string) ([]*parameter.Parameter, error)
}

// MBSpinProvider resolves shades against mst_mb_spin.
type MBSpinProvider struct {
	finder SpinShadeFinder
	params FillGroupLister
}

// NewMBSpinProvider constructs the spin provider.
func NewMBSpinProvider(finder SpinShadeFinder, params FillGroupLister) *MBSpinProvider {
	return &MBSpinProvider{finder: finder, params: params}
}

// Name implements Provider.
func (p *MBSpinProvider) Name() string { return SourceMBSpin }

// ResolveByShades implements Provider. Children are exactly what the interactive spin fill
// produces (same mbspin.NumericFillReaders / TextFillReaders), including the companion spin id.
func (p *MBSpinProvider) ResolveByShades(ctx context.Context, shades []string) (map[string]Resolution, error) {
	norm := normalizeAll(shades)
	out := make(map[string]Resolution, len(norm))
	if len(norm) == 0 {
		return out, nil
	}
	spins, err := p.finder.FindByShades(ctx, norm)
	if err != nil {
		return nil, fmt.Errorf("find spins by shade: %w", err)
	}
	if len(spins) == 0 {
		return out, nil
	}
	children, err := p.params.GetByFillGroup(ctx, FillGroupSPCode)
	if err != nil {
		return nil, fmt.Errorf("list %s fill group: %w", FillGroupSPCode, err)
	}
	for shade, spin := range spins {
		id := spin.ID()
		res := Resolution{
			SPCode:  SpinCode(spin),
			DyeName: spin.MgtName(),
			Source:  SourceMBSpin,
			SpinID:  &id,
		}
		res.Children = spinChildren(spin, children)
		out[shade] = res
	}
	return out, nil
}

// SpinCode is the MB_SP_CODE value stored for a spin: the ORION item code (the dropdown
// key), else the MB costing code, else the permanent id (resolveMBSpinForFill accepts it).
func SpinCode(s *mbspin.Entity) string {
	if v := s.OrionItemCode(); v != nil && *v != "" {
		return *v
	}
	if v := s.MBCosting(); v != nil && *v != "" {
		return *v
	}
	return s.ID().String()
}

// SuperbaProvider resolves shades against cost_superba_cost_sp (SUPERBA source): the code is
// the shade itself, the dye is the colour name; rate/dozing/other children stay empty
// because the MB cost comes from the IS_SUPERBA branch of the formula.
type SuperbaProvider struct{ repo superbacostsp.Repository }

// NewSuperbaProvider constructs the superba provider.
func NewSuperbaProvider(repo superbacostsp.Repository) *SuperbaProvider {
	return &SuperbaProvider{repo: repo}
}

// Name implements Provider.
func (p *SuperbaProvider) Name() string { return SourceSuperbaCostSP }

// ResolveByShades implements Provider.
func (p *SuperbaProvider) ResolveByShades(ctx context.Context, shades []string) (map[string]Resolution, error) {
	norm := normalizeAll(shades)
	out := make(map[string]Resolution, len(norm))
	if len(norm) == 0 {
		return out, nil
	}
	got, err := p.repo.ResolveByShades(ctx, norm)
	if err != nil {
		return nil, err
	}
	for shade, r := range got {
		res := Resolution{SPCode: shade, DyeName: r.ColourName, Source: SourceSuperbaCostSP, Children: map[string]ChildValue{}}
		if r.ColourName != "" {
			c := r.ColourName
			res.Children[ParamSPDye] = ChildValue{Text: &c}
		}
		out[shade] = res
	}
	return out, nil
}

func normalizeAll(shades []string) []string {
	seen := make(map[string]bool, len(shades))
	out := make([]string, 0, len(shades))
	for _, s := range shades {
		if n := NormalizeShade(s); n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

// spinChildren maps the spin to a value per fill-group child using the shared readers.
func spinChildren(spin *mbspin.Entity, children []*parameter.Parameter) map[string]ChildValue {
	out := make(map[string]ChildValue, len(children))
	for _, c := range children {
		col := c.LookupSourceColumn()
		code := c.Code().String()
		if rd, ok := mbspin.NumericFillReaders[col]; ok {
			if v, has := rd(spin); has {
				out[code] = ChildValue{Num: &v}
				continue
			}
		}
		if rd, ok := mbspin.TextFillReaders[col]; ok {
			if v, has := rd(spin); has {
				out[code] = ChildValue{Text: &v}
			}
		}
	}
	return out
}
