// Package rmgroup — V2 update-one-item handler. Patches valuation fields,
// sort_order, and is_active on a single Detail row.
package rmgroup

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// UpdateItemCommand carries optional patches for one Detail row.
type UpdateItemCommand struct {
	HeadID        string
	GroupDetailID string

	ValuationFreightRate    *float64
	ValuationAntiDumpingPct *float64
	ValuationDutyPct        *float64
	ValuationTransportRate  *float64
	ValuationDefaultValue   *float64
	SortOrder               *int32
	IsActive                *bool

	ClearValuationFreightRate    bool
	ClearValuationAntiDumpingPct bool
	ClearValuationDutyPct        bool
	ClearValuationTransportRate  bool
	ClearValuationDefaultValue   bool

	UpdatedBy string
}

// UpdateItemHandler patches one Detail row.
type UpdateItemHandler struct {
	repo rmgroup.Repository
}

// NewUpdateItemHandler constructs the handler.
func NewUpdateItemHandler(repo rmgroup.Repository) *UpdateItemHandler {
	return &UpdateItemHandler{repo: repo}
}

// Handle parses IDs, loads the Detail, applies V1 + V2 patches, persists.
func (h *UpdateItemHandler) Handle(ctx context.Context, cmd UpdateItemCommand) (*rmgroup.Detail, error) {
	if cmd.UpdatedBy == "" {
		return nil, rmgroup.ErrEmptyUpdatedBy
	}
	headID, err := uuid.Parse(cmd.HeadID)
	if err != nil {
		return nil, rmgroup.ErrNotFound
	}
	detailID, err := uuid.Parse(cmd.GroupDetailID)
	if err != nil {
		return nil, rmgroup.ErrDetailNotFound
	}
	d, err := h.repo.GetDetailByID(ctx, detailID)
	if err != nil {
		return nil, err
	}
	if d.HeadID() != headID {
		return nil, rmgroup.ErrDetailNotFound
	}

	// V1 patch (sort_order + is_active).
	if cmd.SortOrder != nil || cmd.IsActive != nil {
		v1 := rmgroup.DetailUpdateInput{
			SortOrder: cmd.SortOrder,
			IsActive:  cmd.IsActive,
		}
		if err := d.Update(v1, cmd.UpdatedBy); err != nil {
			return nil, err
		}
	}

	// V2 valuation patches.
	if hasV2DetailPatch(cmd) {
		cur := d.ValuationInputs()
		cur.FreightRate = patchOptFloat(cur.FreightRate, cmd.ValuationFreightRate, cmd.ClearValuationFreightRate)
		cur.AntiDumpingPct = patchOptFloat(cur.AntiDumpingPct, cmd.ValuationAntiDumpingPct, cmd.ClearValuationAntiDumpingPct)
		cur.DutyPct = patchOptFloat(cur.DutyPct, cmd.ValuationDutyPct, cmd.ClearValuationDutyPct)
		cur.TransportRate = patchOptFloat(cur.TransportRate, cmd.ValuationTransportRate, cmd.ClearValuationTransportRate)
		cur.DefaultValue = patchOptFloat(cur.DefaultValue, cmd.ValuationDefaultValue, cmd.ClearValuationDefaultValue)
		if err := d.AttachValuationInputs(cur); err != nil {
			return nil, err
		}
	}

	if err := h.repo.UpdateDetail(ctx, d); err != nil {
		return nil, fmt.Errorf("persist detail update: %w", err)
	}
	return d, nil
}

func hasV2DetailPatch(cmd UpdateItemCommand) bool {
	return cmd.ValuationFreightRate != nil || cmd.ValuationAntiDumpingPct != nil ||
		cmd.ValuationDutyPct != nil || cmd.ValuationTransportRate != nil || cmd.ValuationDefaultValue != nil ||
		cmd.ClearValuationFreightRate || cmd.ClearValuationAntiDumpingPct ||
		cmd.ClearValuationDutyPct || cmd.ClearValuationTransportRate || cmd.ClearValuationDefaultValue
}
