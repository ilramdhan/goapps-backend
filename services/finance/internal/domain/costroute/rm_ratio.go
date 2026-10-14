package costroute

import (
	"fmt"
	"strings"
)

// RM ratio modes (cost_product_type.cpt_rm_ratio_mode, migration 000570).
const (
	// RMRatioModeInput keeps every route RM line's crm_route_rm_ratio as
	// entered. "" is treated identically (legacy callers, e.g. mbbatch).
	RMRatioModeInput = "INPUT"
	// RMRatioModeDenier derives the ratio of the denier lines (PRODUCT RMs and
	// GROUP RMs whose head has a denier) from their denier share.
	RMRatioModeDenier = "DENIER"
)

// Ratio sources recorded per RM line for DENIER-mode products.
const (
	// RatioSourceDenier marks a line whose ratio is its denier share.
	RatioSourceDenier = "DENIER"
	// RatioSourceInput marks a line of a DENIER-mode product that keeps its
	// route ratio (ITEM lines, GROUP lines whose head has no denier).
	RatioSourceInput = "INPUT"
)

// Product parameter codes read by the DENIER ratio mode.
const (
	// ParamCodeDenier is an upstream product's DENIER INPUT param (000407).
	ParamCodeDenier = "DENIER"
	// ParamCodeSpandexDraft is the product's own SPANDEX_DRAFT param (000570).
	ParamCodeSpandexDraft = "SPANDEX_DRAFT"
)

// IsDenierMode reports whether mode selects the denier-share ratio.
func IsDenierMode(mode string) bool {
	return strings.EqualFold(strings.TrimSpace(mode), RMRatioModeDenier)
}

// RatioSourceData is everything ComputeSeqRMRatios needs for one seq.
type RatioSourceData struct {
	// Mode is the seq product's cpt_rm_ratio_mode ("" / INPUT / DENIER).
	Mode string
	// UpstreamDenier maps upstream product sys id -> its master DENIER.
	UpstreamDenier map[int64]float64
	// GroupDenier maps RM group_code -> cst_rm_group_head.denier for groups
	// with a non-NULL denier; presence makes a GROUP line a denier line.
	GroupDenier map[string]float64
	// GroupUseDraft maps RM group_code -> cst_rm_group_head.denier_use_draft.
	// True (spandex) divides the group denier by SpandexDraft; false/absent
	// uses the group denier as-is (bought-out POY/FDY).
	GroupUseDraft map[string]bool
	// SpandexDraft is the seq product's SPANDEX_DRAFT value (0 = missing).
	// Only read for GROUP lines whose head has denier_use_draft.
	SpandexDraft float64
}

// LineRatio is the resolved ratio of one route RM line.
type LineRatio struct {
	// Ratio is the ratio the calc engine uses for the line.
	Ratio float64
	// Denier is the line denier (upstream DENIER, group denier / draft for
	// draft groups, or group denier as-is);
	// 0 for non-denier lines.
	Denier float64
	// Source is "" in INPUT mode, RatioSourceDenier for denier lines and
	// RatioSourceInput for DENIER-mode lines that keep their route ratio.
	Source string
	// Note explains a line-level fallback (e.g. missing denier treated as 0).
	Note string
}

// ComputeSeqRMRatios resolves the ratio of every RM line of seq, index-aligned
// with seq.Rms (a nil Rm yields a zero LineRatio).
//
// INPUT mode ("" / INPUT / anything but DENIER): every line passes through its
// RouteRmRatio with an empty Source and no notes.
//
// DENIER mode:
//   - PRODUCT line: d = UpstreamDenier[upstream]
//   - GROUP line whose head has a denier: d = head denier / SpandexDraft
//     when the head has denier_use_draft (spandex), else d = head denier
//     as-is (bought-out POY/FDY; no draft needed)
//   - any other line keeps RouteRmRatio (Source INPUT)
//
// ratio_i = d_i / Σd over the seq's denier lines. A missing or non-positive
// input sets that line's d to 0 and records a note; Σd = 0 sets every denier
// line's ratio to 0 and records a note. It never fails. The returned notes are
// deduplicated and in first-seen order.
func ComputeSeqRMRatios(seq *Seq, src RatioSourceData) ([]LineRatio, []string) {
	if seq == nil {
		return nil, nil
	}
	if !IsDenierMode(src.Mode) {
		return inputRatios(seq), nil
	}

	var notes noteSet
	out := make([]LineRatio, len(seq.Rms))
	var total float64
	denierIdx := make([]int, 0, len(seq.Rms))
	for i, rm := range seq.Rms {
		if rm == nil {
			continue
		}
		d, msg, ok := lineDenier(rm, src)
		if !ok {
			out[i] = LineRatio{Ratio: rm.RouteRmRatio, Source: RatioSourceInput}
			continue
		}
		notes.add(msg)
		out[i] = LineRatio{Denier: d, Source: RatioSourceDenier, Note: msg}
		denierIdx = append(denierIdx, i)
		total += d
	}
	switch {
	case len(denierIdx) == 0:
	case total <= 0:
		msg := fmt.Sprintf("DENIER ratio: total denier is 0 at route level %d; denier-based RM lines treated as ratio 0", seq.RouteLevel)
		notes.add(msg)
		for _, i := range denierIdx {
			if out[i].Note == "" {
				out[i].Note = msg
			}
		}
	default:
		for _, i := range denierIdx {
			out[i].Ratio = out[i].Denier / total
		}
	}
	return out, notes.list
}

// inputRatios is the INPUT-mode passthrough: every line keeps RouteRmRatio.
func inputRatios(seq *Seq) []LineRatio {
	out := make([]LineRatio, len(seq.Rms))
	for i, rm := range seq.Rms {
		if rm != nil {
			out[i] = LineRatio{Ratio: rm.RouteRmRatio}
		}
	}
	return out
}

// noteSet collects non-empty notes, deduplicated in first-seen order.
type noteSet struct {
	seen map[string]bool
	list []string
}

func (n *noteSet) add(msg string) {
	if msg == "" || n.seen[msg] {
		return
	}
	if n.seen == nil {
		n.seen = map[string]bool{}
	}
	n.seen[msg] = true
	n.list = append(n.list, msg)
}

// lineDenier resolves one line's denier. ok=false means the line is not a
// denier line and keeps its route ratio; msg is a non-empty note when an input
// is missing and the line's denier is treated as 0.
func lineDenier(rm *Rm, src RatioSourceData) (d float64, msg string, ok bool) {
	switch rm.RmType {
	case RmTypeProduct:
		v := src.UpstreamDenier[rm.RmProductSysID]
		if v <= 0 {
			return 0, fmt.Sprintf("DENIER ratio: upstream product %d has no DENIER; treated as 0", rm.RmProductSysID), true
		}
		return v, "", true
	case RmTypeGroup:
		headDenier, has := src.GroupDenier[rm.RmGroupCode]
		if !has {
			return 0, "", false
		}
		if headDenier <= 0 {
			return 0, fmt.Sprintf("DENIER ratio: RM group %s has no denier; treated as 0", rm.RmGroupCode), true
		}
		if !src.GroupUseDraft[rm.RmGroupCode] {
			return headDenier, "", true
		}
		if src.SpandexDraft <= 0 {
			return 0, fmt.Sprintf("DENIER ratio: SPANDEX_DRAFT missing or not positive; RM group %s treated as 0", rm.RmGroupCode), true
		}
		return headDenier / src.SpandexDraft, "", true
	default:
		return 0, "", false
	}
}

// RatioSources is the master data needed to annotate a whole route graph with
// its computed RM ratios.
type RatioSources struct {
	// Modes maps product sys id -> cpt_rm_ratio_mode (absent = INPUT).
	Modes map[int64]string
	// UpstreamDenier maps upstream product sys id -> master DENIER.
	UpstreamDenier map[int64]float64
	// GroupDenier maps RM group_code -> head denier (non-NULL only).
	GroupDenier map[string]float64
	// GroupUseDraft maps RM group_code -> head denier_use_draft.
	GroupUseDraft map[string]bool
	// SpandexDraft maps seq product sys id -> its SPANDEX_DRAFT value.
	SpandexDraft map[int64]float64
}

// NormalizedRMRatioMode returns DENIER for a DENIER mode and INPUT otherwise.
func NormalizedRMRatioMode(mode string) string {
	if IsDenierMode(mode) {
		return RMRatioModeDenier
	}
	return RMRatioModeInput
}

// AnnotateRMRatios stamps every seq of g with its product's RM ratio mode and
// every RM line with the ratio the calc engine uses for it
// (ComputeSeqRMRatios, per seq, with that seq product's mode and
// SPANDEX_DRAFT). DENIER-source lines also get EffectiveRatio = ComputedRatio
// so nested-MB compounding and edge labels follow the engine's ratio. Only the
// read-only annotation fields are written.
func (g *Graph) AnnotateRMRatios(src RatioSources) {
	if g == nil {
		return
	}
	if g.Head != nil {
		g.Head.RMRatioMode = NormalizedRMRatioMode(src.Modes[g.Head.ProductSysID])
	}
	for _, seq := range g.Seqs {
		if seq == nil {
			continue
		}
		seq.RMRatioMode = NormalizedRMRatioMode(src.Modes[seq.ProductSysID])
		lines, _ := ComputeSeqRMRatios(seq, RatioSourceData{
			Mode:           seq.RMRatioMode,
			UpstreamDenier: src.UpstreamDenier,
			GroupDenier:    src.GroupDenier,
			GroupUseDraft:  src.GroupUseDraft,
			SpandexDraft:   src.SpandexDraft[seq.ProductSysID],
		})
		for i, rm := range seq.Rms {
			if rm == nil {
				continue
			}
			l := lines[i]
			rm.ComputedRatio, rm.RatioSource, rm.Denier, rm.RatioNote = l.Ratio, l.Source, l.Denier, l.Note
			if l.Source == RatioSourceDenier {
				rm.EffectiveRatio = l.Ratio
			}
		}
	}
}

// DenierInputKeys lists what a DENIER-mode annotation of g needs loaded,
// given each seq product's mode: the upstream PRODUCT RM ids and GROUP RM
// codes of DENIER-mode seqs, and the DENIER-mode seq products themselves (for
// SPANDEX_DRAFT). All slices are deduplicated; all are empty when no seq is
// DENIER-mode.
func (g *Graph) DenierInputKeys(modes map[int64]string) (upstreamIDs []int64, groupCodes []string, denierProducts []int64) {
	if g == nil {
		return nil, nil, nil
	}
	seenUp, seenGrp, seenProd := map[int64]bool{}, map[string]bool{}, map[int64]bool{}
	for _, seq := range g.Seqs {
		if seq == nil || !IsDenierMode(modes[seq.ProductSysID]) {
			continue
		}
		if !seenProd[seq.ProductSysID] {
			seenProd[seq.ProductSysID] = true
			denierProducts = append(denierProducts, seq.ProductSysID)
		}
		for _, rm := range seq.Rms {
			switch {
			case rm == nil:
			case rm.RmType == RmTypeProduct && !seenUp[rm.RmProductSysID]:
				seenUp[rm.RmProductSysID] = true
				upstreamIDs = append(upstreamIDs, rm.RmProductSysID)
			case rm.RmType == RmTypeGroup && rm.RmGroupCode != "" && !seenGrp[rm.RmGroupCode]:
				seenGrp[rm.RmGroupCode] = true
				groupCodes = append(groupCodes, rm.RmGroupCode)
			}
		}
	}
	return upstreamIDs, groupCodes, denierProducts
}
