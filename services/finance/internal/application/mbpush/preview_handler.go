package mbpush

import (
	"context"
	"fmt"
	"strings"
)

const (
	skipReasonMissingActual   = "MISSING_ACTUAL_COST"
	skipReasonMissingSelling  = "MISSING_SELLING_COST"
	skipReasonMissingForecast = "MISSING_FORECAST_COST"
	skipReasonNoCostProduct   = "NO_COST_PRODUCT_LINKED"
)

// PushableMBHead is an MB Head eligible for the push, with per-cost-type availability flags.
type PushableMBHead struct {
	MBHID       string
	Code        string
	Name        string
	HasActual   bool
	HasSelling  bool
	HasForecast bool
	// NeedsRepush marks a head already pushed for the period whose pushed cost went stale after a
	// later MB Batch run. Informational only — the head stays pushable, since re-pushing is the
	// remedy. Never auto-acted on.
	NeedsRepush bool
}

// SkippedMBHead is an MB Head excluded from the push, with the reason(s) why.
type SkippedMBHead struct {
	MBHID  string
	Code   string
	Name   string
	Reason string
}

// PreviewResult is the full outcome of a push preview: the pushable set, the skipped set, and
// the count of pushable heads that are pushable *because* their previous push went stale.
type PreviewResult struct {
	Pushable         []PushableMBHead
	Skipped          []SkippedMBHead
	NeedsRepushCount int32
}

// PreviewHandler computes which VALIDATED MB Heads are ready for a push-to-head execution.
type PreviewHandler struct {
	mbHeadReader    MBHeadReader
	costReader      CostReader
	stalePushReader StalePushReader
}

// NewPreviewHandler constructs a PreviewHandler.
func NewPreviewHandler(mbHeadReader MBHeadReader, costReader CostReader, stalePushReader StalePushReader) *PreviewHandler {
	return &PreviewHandler{mbHeadReader: mbHeadReader, costReader: costReader, stalePushReader: stalePushReader}
}

// Preview lists VALIDATED MB Heads split into pushable (all 3 cost types CALCULATED) and skipped
// (with the reason(s) why), per PR-02. Pushable heads whose already-pushed cost has gone stale
// after a later MB Batch run are additionally flagged NeedsRepush — a label only, it neither
// moves a head between buckets nor pushes anything.
func (h *PreviewHandler) Preview(ctx context.Context, period string) (*PreviewResult, error) {
	candidates, err := h.mbHeadReader.ListValidated(ctx)
	if err != nil {
		return nil, fmt.Errorf("list validated mb heads: %w", err)
	}
	stale, err := h.staleSet(ctx, period)
	if err != nil {
		return nil, err
	}

	result := &PreviewResult{
		Pushable: make([]PushableMBHead, 0, len(candidates)),
		Skipped:  make([]SkippedMBHead, 0, len(candidates)),
	}
	for _, c := range candidates {
		if c.CostProductID == 0 {
			result.Skipped = append(result.Skipped, SkippedMBHead{MBHID: c.MBHID, Code: c.Code, Name: c.Name, Reason: skipReasonNoCostProduct})
			continue
		}
		p, reasons := h.checkCostTypes(ctx, c, period)
		if len(reasons) > 0 {
			result.Skipped = append(result.Skipped, SkippedMBHead{MBHID: c.MBHID, Code: c.Code, Name: c.Name, Reason: strings.Join(reasons, ", ")})
			continue
		}
		if _, ok := stale[c.MBHID]; ok {
			p.NeedsRepush = true
			result.NeedsRepushCount++
		}
		result.Pushable = append(result.Pushable, p)
	}
	return result, nil
}

// staleSet resolves the stale-push mbh_id set for period. A nil reader yields an empty set so
// the flag degrades to "never set" rather than breaking the preview.
func (h *PreviewHandler) staleSet(ctx context.Context, period string) (map[string]struct{}, error) {
	if h.stalePushReader == nil {
		return map[string]struct{}{}, nil
	}
	ids, err := h.stalePushReader.ListStalePushedMBHIDs(ctx, period)
	if err != nil {
		return nil, fmt.Errorf("list stale pushed mb heads: %w", err)
	}
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set, nil
}

func (h *PreviewHandler) checkCostTypes(ctx context.Context, c MBHeadCandidate, period string) (PushableMBHead, []string) {
	p := PushableMBHead{MBHID: c.MBHID, Code: c.Code, Name: c.Name}
	var reasons []string

	_, _, foundActual, err := h.costReader.GetActiveCalculated(ctx, c.CostProductID, period, "ACTUAL")
	p.HasActual = err == nil && foundActual
	if !p.HasActual {
		reasons = append(reasons, skipReasonMissingActual)
	}
	_, _, foundSelling, err := h.costReader.GetActiveCalculated(ctx, c.CostProductID, period, "SELLING")
	p.HasSelling = err == nil && foundSelling
	if !p.HasSelling {
		reasons = append(reasons, skipReasonMissingSelling)
	}
	_, _, foundForecast, err := h.costReader.GetActiveCalculated(ctx, c.CostProductID, period, "FORECAST")
	p.HasForecast = err == nil && foundForecast
	if !p.HasForecast {
		reasons = append(reasons, skipReasonMissingForecast)
	}
	return p, reasons
}
