package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// memoCandidates serves FgCandidates from a precomputed map (avoids a second query).
type memoCandidates map[int64][]int64

// FgCandidates implements mbsource.FgCandidateFinder.
func (m memoCandidates) FgCandidates(_ context.Context, ids []int64) (map[int64][]int64, error) {
	out := make(map[int64][]int64, len(ids))
	for _, id := range ids {
		if c, ok := m[id]; ok {
			out[id] = c
		}
	}
	return out, nil
}

// buildOverlay turns the valid APPLY entries into simulated (shade, POY) -> spin links,
// mirroring the apply re-checks (spin live, FG empty, POY active POY, shade equal). The
// first entry per key wins (as apply would: later ones hit conflict_unique). It also
// returns the POY ids of all valid entries (the simulated FG set).
func buildOverlay(ctx context.Context, d reportDeps, entries []applyEntry) (map[mbsource.ShadeFgKey]*mbspin.Entity, []int64, map[string]int, error) {
	extra := make(map[mbsource.ShadeFgKey]*mbspin.Entity)
	skipped := make(map[string]int)
	poyIDs := make([]int64, 0, len(entries))
	for _, e := range entries {
		if e.Err == nil {
			poyIDs = append(poyIDs, e.PoySys)
		}
	}
	poyShades, err := d.src.ActivePoyShades(ctx, poyIDs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load poy shades: %w", err)
	}
	usedSpin := make(map[uuid.UUID]bool)
	for _, e := range entries {
		key, spin, reason, err := overlayEntry(ctx, d.getter, poyShades, e)
		if err != nil {
			return nil, nil, nil, err
		}
		reason = overlayDupReason(reason, key, spin, extra, usedSpin)
		if reason != "" {
			skipped[reason]++
			continue
		}
		extra[key] = spin
		usedSpin[spin.ID()] = true
	}
	return extra, poyIDs, skipped, nil
}

// overlayDupReason mirrors apply ordering: a second row for an already simulated spin
// would find its FG set; a second row for an already simulated (shade, POY) would hit
// the live unique index.
func overlayDupReason(reason string, key mbsource.ShadeFgKey, spin *mbspin.Entity, extra map[mbsource.ShadeFgKey]*mbspin.Entity, usedSpin map[uuid.UUID]bool) string {
	if reason != "" {
		return reason
	}
	if usedSpin[spin.ID()] {
		return overlaySkipFgSet
	}
	if _, dup := extra[key]; dup {
		return overlaySkipDuplicate
	}
	return ""
}

// entrySpinID returns the spin id of a well-formed APPLY entry (false when malformed).
func entrySpinID(e applyEntry) (uuid.UUID, bool) {
	if e.Err != nil {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(e.SpinID)
	return id, err == nil
}

// overlayEntry validates one APPLY entry; a non-empty reason means it is not simulated.
func overlayEntry(ctx context.Context, g spinGetter, poyShades map[int64]string, e applyEntry) (mbsource.ShadeFgKey, *mbspin.Entity, string, error) {
	var none mbsource.ShadeFgKey
	id, valid := entrySpinID(e)
	if !valid {
		return none, nil, overlaySkipMalformed, nil
	}
	spin, err := g.GetByID(ctx, id)
	if errors.Is(err, mbspin.ErrNotFound) {
		return none, nil, overlaySkipSpinMissing, nil
	}
	if err != nil {
		return none, nil, "", fmt.Errorf("get spin %s: %w", id, err)
	}
	if !spin.IsActive() || spin.IsSuperseded() {
		return none, nil, overlaySkipNotLive, nil
	}
	if spin.FgProductID() != nil {
		return none, nil, overlaySkipFgSet, nil
	}
	poyShade, ok := poyShades[e.PoySys]
	if !ok {
		return none, nil, overlaySkipPoyInvalid, nil
	}
	shade := ""
	if s := spin.ShadeCode(); s != nil {
		shade = mbsource.NormalizeShade(*s)
	}
	if shade == "" || shade != poyShade {
		return none, nil, overlaySkipShade, nil
	}
	return mbsource.ShadeFgKey{Shade: shade, FgProductID: e.PoySys}, spin, "", nil
}
