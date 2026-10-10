package mbsource_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
)

type fakeProvider struct {
	name  string
	data  map[string]mbsource.Resolution
	err   error
	calls [][]string
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) ResolveByShades(_ context.Context, s []string) (map[string]mbsource.Resolution, error) {
	f.calls = append(f.calls, s)
	return f.data, f.err
}

func TestResolver_SpinWinsAndFallsBackToSuperba(t *testing.T) {
	spin := &fakeProvider{name: "spin", data: map[string]mbsource.Resolution{"A-1": {SPCode: "ORION1", Source: mbsource.SourceMBSpin}}}
	sup := &fakeProvider{name: "sup", data: map[string]mbsource.Resolution{
		"A-1":  {SPCode: "A-1", Source: mbsource.SourceSuperbaCostSP},
		"MC-9": {SPCode: "MC-9", DyeName: "RED", Source: mbsource.SourceSuperbaCostSP},
	}}
	got, err := mbsource.NewResolver(spin, sup).Resolve(context.Background(), []string{" a-1 ", "mc-9", "a-1", "zzz", ""})
	require.NoError(t, err)
	assert.Equal(t, mbsource.SourceMBSpin, got["A-1"].Source, "spin wins when both match")
	assert.Equal(t, mbsource.SourceSuperbaCostSP, got["MC-9"].Source)
	assert.NotContains(t, got, "ZZZ")
	// normalization + de-dup, and superba never sees the shade spin already resolved
	assert.ElementsMatch(t, []string{"A-1", "MC-9", "ZZZ"}, spin.calls[0])
	assert.ElementsMatch(t, []string{"MC-9", "ZZZ"}, sup.calls[0])
}

func TestResolver_ProviderErrorAborts(t *testing.T) {
	boom := &fakeProvider{name: "spin", err: errors.New("db down")}
	sup := &fakeProvider{name: "sup", data: map[string]mbsource.Resolution{"A": {}}}
	_, err := mbsource.NewResolver(boom, sup).Resolve(context.Background(), []string{"a"})
	require.Error(t, err)
	assert.Empty(t, sup.calls, "must not fall through after a provider failure")
}

func TestResolver_NoShadesNoCalls(t *testing.T) {
	p := &fakeProvider{name: "p"}
	got, err := mbsource.NewResolver(p).Resolve(context.Background(), []string{"", "  "})
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Empty(t, p.calls)
}

func TestNormalizeShade(t *testing.T) {
	assert.Equal(t, "Z-114-S", mbsource.NormalizeShade("  z-114-s\t"))
}
