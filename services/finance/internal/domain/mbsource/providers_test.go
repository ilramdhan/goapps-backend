package mbsource_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/superbacostsp"
)

type fakeSuperbaRepo struct {
	superbacostsp.Repository
	rows map[string]superbacostsp.Resolved
}

func (f fakeSuperbaRepo) ResolveByShades(_ context.Context, s []string) (map[string]superbacostsp.Resolved, error) {
	out := map[string]superbacostsp.Resolved{}
	for _, x := range s {
		if r, ok := f.rows[x]; ok {
			out[x] = r
		}
	}
	return out, nil
}

func TestSuperbaProvider(t *testing.T) {
	p := mbsource.NewSuperbaProvider(fakeSuperbaRepo{rows: map[string]superbacostsp.Resolved{"MC-0547": {ColourName: "SKY BLUE"}}})
	got, err := p.ResolveByShades(context.Background(), []string{" mc-0547", "nope"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	r := got["MC-0547"]
	assert.Equal(t, "MC-0547", r.SPCode)
	assert.Equal(t, "SKY BLUE", r.DyeName)
	assert.Equal(t, mbsource.SourceSuperbaCostSP, r.Source)
	assert.Nil(t, r.SpinID)
	require.NotNil(t, r.Children[mbsource.ParamSPDye].Text)
	assert.Equal(t, "SKY BLUE", *r.Children[mbsource.ParamSPDye].Text)
	assert.Len(t, r.Children, 1, "rate/dozing and other children stay empty")
}

func TestSpinPickOrderSQL_Rule(t *testing.T) {
	// The rule text is shared with migration 000569 (see migration test).
	for _, frag := range []string{"'Spinning' THEN 1", "'Boughtout' THEN 2", "'R and D' THEN 3", "DESC", "mbs_id"} {
		assert.Contains(t, mbsource.SpinPickOrderSQL, frag)
	}
}
