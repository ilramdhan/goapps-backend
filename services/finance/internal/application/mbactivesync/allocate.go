package mbactivesync

import (
	"bytes"
	"math"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// Allocation match levels (port of L-14 c2.lvl). Lower is better.
const (
	AllocLvlDenFilCS = 2 // denier, filament and cross-section equal
	AllocLvlDenFil   = 3 // denier and filament equal
)

// L-14 compares LDR and denier in exact NUMERIC (mbs_run_ldr_pct NUMERIC(10,4),
// doz 2 decimals, denier NUMERIC(10,2)). Doing it in float64 lets rounding noise
// decide the 0.005 KEEP boundary and break ldr_d ties, so all such comparisons
// are done on int64 fixed-point ten-thousandths (Fix4).
const (
	fixScale     = 1e4
	ldrMissing   = -99.0 // COALESCE(mbs_run_ldr_pct, -99)
	keepLDRDiff4 = 50    // KEEP when |run_ldr - doz| <= 0.005 (ten-thousandths)
	ldrEqual4    = 0     // SET_LDR when |run_ldr - doz| > this (ten-thousandths)
)

// Fix4 converts a NUMERIC value carried as float64 to exact ten-thousandths.
func Fix4(x float64) int64 { return int64(math.Round(x * fixScale)) }

func absInt64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// AllocInput is one file row taking part in the allocation (it has a head, a POY
// product and a parsed denier — design §3.4).
type AllocInput struct {
	Row    FileRow
	HeadID uuid.UUID
	// TargetFG is the row's target FG (POY product sys id); nil = no FG preference.
	TargetFG *int64
}

// AllocResult is the best candidate spin of one row and its rank on that spin.
type AllocResult struct {
	Ln   int
	Spin *SpinSnap
	Lvl  int
	// ShEq is Loose(COALESCE(spin shade, cc)) == Loose(file shade); nil when either side is empty (SQL NULL).
	ShEq *bool
	// LDRD is |COALESCE(run_ldr_pct,-99) - doz| in exact ten-thousandths (see Fix4);
	// nil when the file has no RP-Doz (SQL NULL).
	LDRD *int64
	Rank int // 1 = the row owns the spin; >1 = spin taken by a better row
	// FGRank is the FG affinity of the spin for this row: 0 = spin FG equals the
	// row's target FG, 1 = spin FG NULL (or row has no target), 2 = another FG.
	FGRank int
}

// fgRank is the first allocation key (P2h-4a): once a head carries several clones
// identical on den/fil/cs/shade/LDR that differ only by FG, each row must prefer
// the clone that already holds its own target FG, otherwise every row converges
// on the same clone (uuid tiebreak) and the re-plan never reaches a fixed point.
func fgRank(spinFG, targetFG *int64) int {
	switch {
	case spinFG == nil || targetFG == nil:
		return 1
	case *spinFG == *targetFG:
		return 0
	}
	return 2
}

// shadeEq is the tri-state sh_eq of L-14.
func shadeEq(spinShade, spinCC, fileShade string) *bool {
	s := NormShadeLoose(spinShade)
	if s == "" { // COALESCE(shade_code, cc); "" is treated as NULL
		s = NormShadeLoose(spinCC)
	}
	f := NormShadeLoose(fileShade)
	if s == "" || f == "" {
		return nil
	}
	v := s == f
	return &v
}

func ldrDiff(spin *SpinSnap, r FileRow) *int64 {
	if !r.DozOK {
		return nil
	}
	cur := ldrMissing
	if spin.RunLDRPct != nil {
		cur = *spin.RunLDRPct
	}
	d := absInt64(Fix4(cur) - Fix4(r.Doz))
	return &d
}

// matchLevel ports the L-14 CASE: 2 = den & fil & UPPER(cs) equal, 3 = den & fil equal, 0 = not a candidate.
// A NULL on either side (nil pointer / empty cs) never matches, as in SQL.
func matchLevel(spin *SpinSnap, r FileRow) int {
	if spin.Denier == nil || spin.Filament == nil || r.Den == nil || r.Fil == nil {
		return 0
	}
	if Fix4(*spin.Denier) != Fix4(*r.Den) || *spin.Filament != *r.Fil {
		return 0
	}
	if spin.CrossSection != "" && r.CS != "" && strings.EqualFold(spin.CrossSection, r.CS) {
		return AllocLvlDenFilCS
	}
	return AllocLvlDenFil
}

// shEqRank maps sh_eq to "DESC NULLS LAST" order: true, false, NULL.
func shEqRank(b *bool) int {
	switch {
	case b == nil:
		return 2
	case *b:
		return 0
	default:
		return 1
	}
}

// cmpNullableAsc compares two nullable fixed-point values ascending, NULLs last (Postgres ASC default).
func cmpNullableAsc(a, b *int64) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	case *a < *b:
		return -1
	case *a > *b:
		return 1
	}
	return 0
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func cmpBoolTrueFirst(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return -1
	}
	return 1
}

// cmpUUID orders like Postgres uuid (byte-wise).
func cmpUUID(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) }

// Allocate ports L-14 best/alloc: every row picks its best candidate spin on its
// own head (live or inactive, non-deleted) ordered fg_rank ASC, lvl ASC, sh_eq DESC
// NULLS LAST, ldr_d ASC, live DESC, mbs_id ASC; then the rows choosing the same spin
// are ranked fg_rank ASC, sh_eq DESC NULLS LAST, ldr_d ASC, ln ASC. fg_rank (P2h-4a,
// not in L-14) is 0 = spin FG equals the row target FG, 1 = spin FG NULL, 2 = other
// FG; without it the re-plan after phase C oscillates. There is deliberately no fallback to a
// second candidate (design §3.4). The result is keyed by ln and does not depend on
// the order of rows or spins.
func Allocate(rows []AllocInput, spinsByHead map[uuid.UUID][]*SpinSnap) map[int]*AllocResult {
	out := make(map[int]*AllocResult, len(rows))
	bySpin := make(map[uuid.UUID][]*AllocResult)
	for _, in := range rows {
		best := bestCandidate(in, spinsByHead[in.HeadID])
		if best == nil {
			continue
		}
		out[in.Row.Ln] = best
		bySpin[best.Spin.ID] = append(bySpin[best.Spin.ID], best)
	}
	for _, list := range bySpin {
		sort.Slice(list, func(i, j int) bool {
			a, b := list[i], list[j]
			if c := cmpInt(a.FGRank, b.FGRank); c != 0 {
				return c < 0
			}
			if c := cmpInt(shEqRank(a.ShEq), shEqRank(b.ShEq)); c != 0 {
				return c < 0
			}
			if c := cmpNullableAsc(a.LDRD, b.LDRD); c != 0 {
				return c < 0
			}
			return a.Ln < b.Ln
		})
		for i, r := range list {
			r.Rank = i + 1
		}
	}
	return out
}

// bestCandidate returns the L-14 "best" spin of one row, nil when no spin matches den/fil.
func bestCandidate(in AllocInput, spins []*SpinSnap) *AllocResult {
	var best *AllocResult
	for _, sp := range spins {
		lvl := matchLevel(sp, in.Row)
		if lvl == 0 {
			continue
		}
		c := &AllocResult{Ln: in.Row.Ln, Spin: sp, Lvl: lvl, ShEq: shadeEq(sp.ShadeCode, sp.CC, in.Row.Shade), LDRD: ldrDiff(sp, in.Row),
			FGRank: fgRank(sp.FGProductID, in.TargetFG)}
		if best == nil || betterCandidate(c, best) {
			best = c
		}
	}
	return best
}

func betterCandidate(a, b *AllocResult) bool {
	if c := cmpInt(a.FGRank, b.FGRank); c != 0 {
		return c < 0
	}
	if c := cmpInt(a.Lvl, b.Lvl); c != 0 {
		return c < 0
	}
	if c := cmpInt(shEqRank(a.ShEq), shEqRank(b.ShEq)); c != 0 {
		return c < 0
	}
	if c := cmpNullableAsc(a.LDRD, b.LDRD); c != 0 {
		return c < 0
	}
	if c := cmpBoolTrueFirst(a.Spin.Live(), b.Spin.Live()); c != 0 {
		return c < 0
	}
	return cmpUUID(a.Spin.ID, b.Spin.ID) < 0
}

// absDiffFix4OrMax returns |a-b| in ten-thousandths, or MaxInt64 when a side is missing (sorts last).
func absDiffFix4OrMax(a *float64, b *float64) int64 {
	if a == nil || b == nil {
		return math.MaxInt64
	}
	return absInt64(Fix4(*a) - Fix4(*b))
}

func absDiffIntOrMax(a *int, b *int) int64 {
	if a == nil || b == nil {
		return math.MaxInt64
	}
	return absInt64(int64(*a - *b))
}

// SelectDuplicateSource picks the source spin for DUPLICATE_SPIN (design §3.5):
// a root spin (parent NULL) of the head ordered |den-row.den| ASC, |fil-row.fil| ASC,
// sh_eq DESC NULLS LAST, live DESC, created_at ASC, mbs_id ASC. nil when the head has no root.
func SelectDuplicateSource(spins []*SpinSnap, r FileRow) *SpinSnap {
	var best *SpinSnap
	var bestKey [3]int64
	for _, sp := range spins {
		if sp.ParentID != nil {
			continue
		}
		key := [3]int64{absDiffFix4OrMax(sp.Denier, r.Den), absDiffIntOrMax(sp.Filament, r.Fil),
			int64(shEqRank(shadeEq(sp.ShadeCode, sp.CC, r.Shade)))}
		if best == nil || sourceLess(key, sp, bestKey, best) {
			best, bestKey = sp, key
		}
	}
	return best
}

func sourceLess(ka [3]int64, a *SpinSnap, kb [3]int64, b *SpinSnap) bool {
	for i := range ka {
		if ka[i] != kb[i] {
			return ka[i] < kb[i]
		}
	}
	if c := cmpBoolTrueFirst(a.Live(), b.Live()); c != 0 {
		return c < 0
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.Before(b.CreatedAt)
	}
	return cmpUUID(a.ID, b.ID) < 0
}
