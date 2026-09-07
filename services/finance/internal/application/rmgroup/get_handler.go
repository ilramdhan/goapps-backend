// Package rmgroup provides application layer handlers for RM group head and detail operations.
package rmgroup

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// GetQuery retrieves a head (and optionally its details) by ID.
type GetQuery struct {
	HeadID      string
	WithDetails bool
	ActiveOnly  bool

	// Period optionally selects a period-scoped read overlay (design §2.3).
	// Empty means anchor-row-only read -- exactly today's behavior, unchanged
	// for any caller that does not pass a period.
	Period string
}

// GetResult bundles the head with optional detail rows. When Query.Period was
// supplied, Head/Details reflect the period overlay: editable snapshot fields
// from cst_rm_group_head_period / cst_rm_group_detail_period when a snapshot
// exists for that period, falling back to the anchor row's own values
// otherwise (period never edited, or a brand-new period with no snapshot
// yet). Identity fields -- ID, Code, item code, created_at/by -- always come
// from the anchor row regardless of Period.
type GetResult struct {
	Head    *rmgroup.Head
	Details []*rmgroup.Detail
}

// GetHandler handles GetHead queries.
type GetHandler struct {
	repo rmgroup.Repository
}

// NewGetHandler builds a GetHandler.
func NewGetHandler(repo rmgroup.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle returns the head and, when requested, its detail rows. Soft-deleted rows
// are omitted from the detail list regardless of ActiveOnly; ActiveOnly further
// restricts the result to rows where is_active = true. When query.Period is
// non-empty, the returned Head/Details are overlaid with that period's
// snapshot (design §2.3); when empty, the anchor rows are returned unchanged,
// exactly as before period-versioning existed.
func (h *GetHandler) Handle(ctx context.Context, query GetQuery) (*GetResult, error) {
	id, err := uuid.Parse(query.HeadID)
	if err != nil {
		return nil, rmgroup.ErrNotFound
	}

	head, err := h.repo.GetHeadByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if query.Period != "" {
		head, err = h.overlayHeadPeriod(ctx, head, query.Period)
		if err != nil {
			return nil, err
		}
	}

	result := &GetResult{Head: head}
	if !query.WithDetails {
		return result, nil
	}

	details, err := h.loadDetails(ctx, id, query.ActiveOnly)
	if err != nil {
		return nil, fmt.Errorf("load details: %w", err)
	}
	if query.Period != "" {
		details, err = h.overlayDetailsPeriod(ctx, details, query.Period)
		if err != nil {
			return nil, err
		}
	}
	result.Details = details
	return result, nil
}

func (h *GetHandler) loadDetails(ctx context.Context, headID uuid.UUID, activeOnly bool) ([]*rmgroup.Detail, error) {
	if activeOnly {
		return h.repo.ListActiveDetailsByHeadID(ctx, headID)
	}
	return h.repo.ListDetailsByHeadID(ctx, headID)
}

// overlayHeadPeriod resolves the (head.ID(), period) snapshot -- falling back
// to a fresh snapshot built from the anchor head's current values when none
// exists yet (get-or-create baseline, design §2.3) -- and merges it onto a new
// Head value so the anchor entity itself is never mutated.
func (h *GetHandler) overlayHeadPeriod(ctx context.Context, head *rmgroup.Head, period string) (*rmgroup.Head, error) {
	if err := rmgroup.ValidatePeriodFormat(period); err != nil {
		return nil, err
	}
	snap, err := h.repo.GetHeadPeriodSnapshot(ctx, head.ID(), period)
	if err != nil {
		if !errors.Is(err, rmgroup.ErrNotFound) {
			return nil, fmt.Errorf("load head period snapshot: %w", err)
		}
		fresh := rmgroup.NewHeadPeriodSnapshotFromHead(period, head)
		snap = &fresh
	}
	return overlayHeadFromSnapshot(head, snap)
}

// overlayHeadFromSnapshot builds a new Head carrying the anchor row's
// identity/audit/activity fields plus the snapshot's editable fields. IsActive
// is intentionally taken from the anchor, not the snapshot -- activity status
// is not period-scoped (design §4).
func overlayHeadFromSnapshot(head *rmgroup.Head, snap *rmgroup.HeadPeriodSnapshot) (*rmgroup.Head, error) {
	overlaid := rmgroup.ReconstructHead(
		head.ID(), head.Code(),
		snap.Name, snap.Description, snap.Colorant, snap.CIName,
		snap.CostPercentage, snap.CostPerKg,
		snap.FlagValuation, snap.FlagMarketing, snap.FlagSimulation,
		snap.InitValValuation, snap.InitValMarketing, snap.InitValSimulation,
		head.IsActive(),
		head.CreatedAt(), head.CreatedBy(),
		head.UpdatedAt(), head.UpdatedBy(),
		head.DeletedAt(), head.DeletedBy(),
	)
	if err := overlaid.AttachMarketingInputs(snap.MarketingInputs); err != nil {
		return nil, fmt.Errorf("attach head period overlay marketing inputs: %w", err)
	}
	return overlaid, nil
}

// overlayDetailsPeriod overlays every detail row with its (detailID, period)
// snapshot, applying the same get-or-create fallback as overlayHeadPeriod.
func (h *GetHandler) overlayDetailsPeriod(
	ctx context.Context, details []*rmgroup.Detail, period string,
) ([]*rmgroup.Detail, error) {
	overlaid := make([]*rmgroup.Detail, len(details))
	for i, d := range details {
		snap, err := h.repo.GetDetailPeriodSnapshot(ctx, d.ID(), period)
		if err != nil {
			if !errors.Is(err, rmgroup.ErrNotFound) {
				return nil, fmt.Errorf("load detail period snapshot: %w", err)
			}
			fresh := rmgroup.NewDetailPeriodSnapshotFromDetail(period, d)
			snap = &fresh
		}
		merged, err := overlayDetailFromSnapshot(d, snap)
		if err != nil {
			return nil, err
		}
		overlaid[i] = merged
	}
	return overlaid, nil
}

// overlayDetailFromSnapshot builds a new Detail carrying the anchor row's
// identity/audit fields plus the snapshot's editable fields (including
// IsActive/IsDummy/SortOrder, which -- unlike Head's IsActive -- are
// period-scoped per design §3).
func overlayDetailFromSnapshot(detail *rmgroup.Detail, snap *rmgroup.DetailPeriodSnapshot) (*rmgroup.Detail, error) {
	overlaid := rmgroup.ReconstructDetail(
		detail.ID(), detail.HeadID(), detail.ItemCode(),
		detail.ItemName(), detail.ItemTypeCode(), detail.GradeCode(), detail.ItemGrade(), detail.UOMCode(),
		snap.MarketPercentage, snap.MarketValueRp,
		snap.SortOrder, snap.IsActive, snap.IsDummy,
		detail.CreatedAt(), detail.CreatedBy(),
		detail.UpdatedAt(), detail.UpdatedBy(),
		detail.DeletedAt(), detail.DeletedBy(),
	)
	if err := overlaid.AttachValuationInputs(snap.ValuationInputs); err != nil {
		return nil, fmt.Errorf("attach detail period overlay valuation inputs: %w", err)
	}
	return overlaid, nil
}
