package shared

// Bobbin weight helpers for TXT/TWT production quantity computation.
//
// TXT TRN_STS convention is the inverse of SPG: 0=Full, 1=Unfull. A production
// quantity in kilograms is the sum of each bobbin count multiplied by the lot's
// standard weight for that fill state.

// QtyTXT computes TXT bobbin production in kilograms. Full bobbins use the lot
// standard full weight; unfull bobbins use the standard unfull weight.
func QtyTXT(fullBobbins, unfullBobbins int, stdWeightFull, stdWeightUnfull float64) float64 {
	return float64(fullBobbins)*stdWeightFull + float64(unfullBobbins)*stdWeightUnfull
}

// QtyTXTNormal computes the QC-released TXT quantity in kilograms used by the
// suggest P2 branch. Normal (TQM-passed) bobbins are treated as full-weight;
// released-unfull bobbins use the standard unfull weight. When a distinct
// released-unfull count is unavailable, pass 0 for relUnfullBobs to fall back
// to normalBobs*stdWeightFull only.
func QtyTXTNormal(normalBobs, relUnfullBobs int, stdWeightFull, stdWeightUnfull float64) float64 {
	return float64(normalBobs)*stdWeightFull + float64(relUnfullBobs)*stdWeightUnfull
}

// SPG DOFF_OPTION encodes the doff fill state. It is the INVERSE of the TXT
// TRN_STS convention (TXT: 0=Full, 1=Unfull).
const (
	SpgDoffOptionFull   = 1 // DOFF_OPTION=1 => Full bobbin
	SpgDoffOptionUnfull = 2 // DOFF_OPTION=2 => Unfull bobbin
)

// IsSpgFullDoff reports whether an SPG DOFF_OPTION denotes a full doff (option
// 1). This is the opposite of TXT where TRN_STS=0 means full.
func IsSpgFullDoff(doffOption int) bool { return doffOption == SpgDoffOptionFull }

// QtySPG computes SPG production kilograms for a bobbin count. The SPG summary
// carries a measured per-bobbin weight (DOFF_WT); when weightPerBob > 0 it is
// used directly. Otherwise the doffOption selects a standard weight as a
// fallback: option 1 (Full) uses stdWeightFull, option 2 (Unfull) uses
// stdWeightUnfull — the inverse of the TXT convention.
func QtySPG(bobbins, doffOption int, weightPerBob, stdWeightFull, stdWeightUnfull float64) float64 {
	if weightPerBob > 0 {
		return float64(bobbins) * weightPerBob
	}
	if IsSpgFullDoff(doffOption) {
		return float64(bobbins) * stdWeightFull
	}
	return float64(bobbins) * stdWeightUnfull
}
