// Package rmgroup provides application layer handlers for RM group head and detail operations.
package rmgroup

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// AddItemsCommand assigns a batch of raw-material item codes to an existing head.
// Per-item optional fields (name, uom, grade, market rows) can be supplied; when
// omitted, the detail is created with defaults and can be edited later.
type AddItemsCommand struct {
	HeadID    string
	CreatedBy string
	Items     []AddItemInput
}

// AddItemInput describes a single item to assign.
type AddItemInput struct {
	ItemCode         string
	ItemName         string
	ItemTypeCode     string
	GradeCode        string
	ItemGrade        string
	UOMCode          string
	MarketPercentage *float64
	MarketValueRp    *float64
	SortOrder        int32
}

// AddItemsResult summarises the outcome of an add-items call.
type AddItemsResult struct {
	HeadID  uuid.UUID
	Added   []*rmgroup.Detail
	Skipped []SkippedItem
}

// SkippedItem describes an item that could not be added, with the reason.
type SkippedItem struct {
	ItemCode       string
	Reason         string
	OwningGroupID  *uuid.UUID
	OwningDetailID *uuid.UUID
}

// AddItemsHandler handles AddItems commands.
type AddItemsHandler struct {
	repo rmgroup.Repository
}

// NewAddItemsHandler builds an AddItemsHandler.
func NewAddItemsHandler(repo rmgroup.Repository) *AddItemsHandler {
	return &AddItemsHandler{repo: repo}
}

// Handle enforces the "one item, one active group" invariant: any item already
// assigned to another active detail is reported in Skipped and not inserted.
// Items that already belong to THIS head are also skipped (idempotent re-add).
func (h *AddItemsHandler) Handle(ctx context.Context, cmd AddItemsCommand) (*AddItemsResult, error) {
	if cmd.CreatedBy == "" {
		return nil, rmgroup.ErrEmptyCreatedBy
	}
	headID, err := uuid.Parse(cmd.HeadID)
	if err != nil {
		return nil, rmgroup.ErrNotFound
	}

	head, err := h.repo.GetHeadByID(ctx, headID)
	if err != nil {
		return nil, err
	}
	if head.IsDeleted() {
		return nil, rmgroup.ErrAlreadyDeleted
	}

	result := &AddItemsResult{HeadID: headID}
	for i := range cmd.Items {
		detail, skip, err := h.processItem(ctx, headID, cmd.CreatedBy, cmd.Items[i])
		if err != nil {
			return nil, err
		}
		if skip != nil {
			result.Skipped = append(result.Skipped, *skip)
			continue
		}
		result.Added = append(result.Added, detail)
	}
	return result, nil
}

// processItem validates a single item, checks cross-group ownership, and creates
// the detail. Returns (detail, nil, nil) on insert, (nil, skipped, nil) when the
// item is skipped, and (nil, nil, err) on fatal errors.
func (h *AddItemsHandler) processItem(
	ctx context.Context,
	headID uuid.UUID,
	createdBy string,
	in AddItemInput,
) (*rmgroup.Detail, *SkippedItem, error) {
	itemCode, err := rmgroup.NewItemCode(in.ItemCode)
	if err != nil {
		return nil, &SkippedItem{ItemCode: in.ItemCode, Reason: err.Error()}, nil
	}

	existing, err := h.repo.GetActiveDetailByItemCodeGrade(ctx, itemCode, in.GradeCode)
	if err != nil && !errors.Is(err, rmgroup.ErrDetailNotFound) {
		return nil, nil, fmt.Errorf("lookup active detail for %q: %w", in.ItemCode, err)
	}
	if existing != nil {
		owningGroup := existing.HeadID()
		owningDetail := existing.ID()
		reason := rmgroup.ErrItemAlreadyInOtherGroup.Error()
		if owningGroup == headID {
			reason = "item already assigned to this group"
			// Backfill snapshot metadata if the existing detail has empty fields
			// but the request now carries enriched data from the sync feed.
			if needsBackfill(existing) && hasMetadata(in) {
				if err := applyItemMetadata(existing, in, createdBy); err == nil {
					if saveErr := h.repo.UpdateDetail(ctx, existing); saveErr != nil {
						return nil, nil, fmt.Errorf("backfill detail for %q: %w", in.ItemCode, saveErr)
					}
				}
			}
		}
		return nil, &SkippedItem{
			ItemCode:       in.ItemCode,
			Reason:         reason,
			OwningGroupID:  &owningGroup,
			OwningDetailID: &owningDetail,
		}, nil
	}

	detail, err := rmgroup.NewDetail(headID, itemCode, createdBy)
	if err != nil {
		return nil, nil, err
	}

	if err := applyItemMetadata(detail, in, createdBy); err != nil {
		return nil, nil, err
	}

	if err := h.repo.AddDetail(ctx, detail); err != nil {
		return nil, nil, fmt.Errorf("persist detail for %q: %w", in.ItemCode, err)
	}
	return detail, nil, nil
}

// needsBackfill reports whether an existing detail has empty snapshot columns
// that could be filled in from the sync feed.
func needsBackfill(d *rmgroup.Detail) bool {
	return d.ItemName() == "" || d.GradeCode() == "" || d.UOMCode() == ""
}

// hasMetadata reports whether the incoming request carries any snapshot metadata
// (name/grade/uom) that would be worth applying to an existing detail.
func hasMetadata(in AddItemInput) bool {
	return in.ItemName != "" || in.GradeCode != "" || in.ItemGrade != "" || in.UOMCode != ""
}

func applyItemMetadata(detail *rmgroup.Detail, in AddItemInput, createdBy string) error {
	upd := rmgroup.DetailUpdateInput{
		MarketPercentage: in.MarketPercentage,
		MarketValueRp:    in.MarketValueRp,
	}
	if in.ItemName != "" {
		v := in.ItemName
		upd.ItemName = &v
	}
	if in.ItemTypeCode != "" {
		v := in.ItemTypeCode
		upd.ItemTypeCode = &v
	}
	if in.GradeCode != "" {
		v := in.GradeCode
		upd.GradeCode = &v
	}
	if in.ItemGrade != "" {
		v := in.ItemGrade
		upd.ItemGrade = &v
	}
	if in.UOMCode != "" {
		v := in.UOMCode
		upd.UOMCode = &v
	}
	if in.SortOrder > 0 {
		v := in.SortOrder
		upd.SortOrder = &v
	}
	if upd.ItemName == nil && upd.ItemTypeCode == nil && upd.GradeCode == nil &&
		upd.ItemGrade == nil && upd.UOMCode == nil && upd.SortOrder == nil &&
		upd.MarketPercentage == nil && upd.MarketValueRp == nil {
		return nil
	}
	return detail.Update(upd, createdBy)
}
