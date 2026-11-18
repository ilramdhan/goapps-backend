package mbsource_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

type overlayFakeShadeFg struct {
	m   map[mbsource.ShadeFgKey]*mbspin.Entity
	err error
}

func (f *overlayFakeShadeFg) FindByShadeAndFg(_ context.Context, keys []mbsource.ShadeFgKey) (map[mbsource.ShadeFgKey]*mbspin.Entity, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := map[mbsource.ShadeFgKey]*mbspin.Entity{}
	for _, k := range keys {
		if e, ok := f.m[k]; ok {
			out[k] = e
		}
	}
	return out, nil
}

func overlaySpin(t *testing.T, orion, shade string, active bool, fg *int64) *mbspin.Entity {
	t.Helper()
	o, s := orion, shade
	e := mbspin.Reconstruct(uuid.New(), nil, &o, uuid.New(), "DYE", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		active, time.Now(), "t", nil, nil, nil, nil)
	e.HydrateShadeAndLDR(mbspin.ShadeAndLDR{ShadeCode: &s, LDRType: "NOT_CALCULATED"})
	e.HydrateFg(fg, nil, nil, mbspin.FgSupersede{})
	return e
}

func overlayKey(s string, fg int64) mbsource.ShadeFgKey {
	return mbsource.ShadeFgKey{Shade: s, FgProductID: fg}
}

func TestOverlaySpinShadeFgFinder_BaseWinsOverlayFillsOnlyMissing(t *testing.T) {
	dbSpin := overlaySpin(t, "DB", "A", true, nil)
	simA := overlaySpin(t, "SIM-A", "A", true, nil)
	simB := overlaySpin(t, "SIM-B", "B", true, nil)
	o := &mbsource.OverlaySpinShadeFgFinder{
		Base:  &overlayFakeShadeFg{m: map[mbsource.ShadeFgKey]*mbspin.Entity{overlayKey("A", 1): dbSpin}},
		Extra: map[mbsource.ShadeFgKey]*mbspin.Entity{overlayKey("A", 1): simA, overlayKey("B", 2): simB},
	}
	got, err := o.FindByShadeAndFg(context.Background(), []mbsource.ShadeFgKey{overlayKey("A", 1), overlayKey("B", 2), overlayKey("C", 3)})
	require.NoError(t, err)
	assert.Same(t, dbSpin, got[overlayKey("A", 1)], "DB resolution is never overridden")
	assert.Same(t, simB, got[overlayKey("B", 2)])
	assert.NotContains(t, got, overlayKey("C", 3))

	_, err = (&mbsource.OverlaySpinShadeFgFinder{Base: &overlayFakeShadeFg{err: errors.New("db")}}).
		FindByShadeAndFg(context.Background(), []mbsource.ShadeFgKey{overlayKey("A", 1)})
	assert.Error(t, err)
}
