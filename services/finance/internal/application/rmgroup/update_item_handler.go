// Package rmgroup — V2 update-one-item handler. Patches valuation fields,
// sort_order, and is_active on a single Detail row.
package rmgroup

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// UpdateItemCommand carries optional patches for one Detail row.
type UpdateItemCommand struct {
	HeadID        string
	GroupDetailID string
	Period        string

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

// Handle resolves the period-scoped snapshot for cmd.Period (get-or-create
// from the anchor detail when none exists yet), applies the partial update
// onto that snapshot, and upserts it. When cmd.Period is the current latest
// sync period, the same patch is additionally applied to the anchor
// cst_rm_group_detail row (write-through rule, design doc §2.2/§5.2) so every
// period-blind reader of the anchor row keeps seeing up-to-date "current"
// config; edits to an older period leave the anchor row untouched.
func (h *UpdateItemHandler) Handle(ctx context.Context, cmd UpdateItemCommand) (*rmgroup.DetailPeriodSnapshot, error) {
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
	if err := rmgroup.ValidatePeriodFormat(cmd.Period); err != nil {
		return nil, err
	}

	// Step 1: load the anchor Detail — source of "what fields currently look
	// like" for building the get-or-create default, and the write-through
	// target below.
	d, err := h.repo.GetDetailByID(ctx, detailID)
	if err != nil {
		return nil, err
	}
	if d.HeadID() != headID {
		return nil, rmgroup.ErrDetailNotFound
	}

	// Step 2: resolve the period-scoped working copy (get-or-create semantics).
	snap, err := h.resolveDetailSnapshot(ctx, detailID, cmd.Period, d)
	if err != nil {
		return nil, err
	}

	// Step 3: apply the patch onto the snapshot's fields.
	if err := applyDetailSnapshotPatch(snap, cmd); err != nil {
		return nil, err
	}

	// Step 4: upsert the snapshot.
	if err := h.repo.UpsertDetailPeriod(ctx, snap); err != nil {
		return nil, fmt.Errorf("persist detail period snapshot: %w", err)
	}

	// Step 5: conditional write-through to the anchor row when editing the
	// latest sync period.
	latest, err := h.repo.LatestSyncPeriod(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve latest sync period: %w", err)
	}
	if cmd.Period == latest {
		if err := h.writeThroughAnchorDetail(ctx, d, cmd); err != nil {
			return nil, err
		}
	}

	// Step 6: return the up-to-date view for the requested period.
	return snap, nil
}

// resolveDetailSnapshot loads the (detailID, period) snapshot, falling back
// to a fresh snapshot built from the anchor detail's current values when
// none exists yet (get-or-create baseline, design §2.3).
func (h *UpdateItemHandler) resolveDetailSnapshot(
	ctx context.Context, detailID uuid.UUID, period string, detail *rmgroup.Detail,
) (*rmgroup.DetailPeriodSnapshot, error) {
	snap, err := h.repo.GetDetailPeriodSnapshot(ctx, detailID, period)
	if err == nil {
		return snap, nil
	}
	if !errors.Is(err, rmgroup.ErrNotFound) {
		return nil, fmt.Errorf("load detail period snapshot: %w", err)
	}
	fresh := rmgroup.NewDetailPeriodSnapshotFromDetail(period, detail)
	return &fresh, nil
}

// writeThroughAnchorDetail re-applies the same command patch to the anchor
// Detail and persists it, exactly as the pre-versioning write path did.
func (h *UpdateItemHandler) writeThroughAnchorDetail(ctx context.Context, d *rmgroup.Detail, cmd UpdateItemCommand) error {
	if cmd.SortOrder != nil || cmd.IsActive != nil {
		v1 := rmgroup.DetailUpdateInput{
			SortOrder: cmd.SortOrder,
			IsActive:  cmd.IsActive,
		}
		if err := d.Update(v1, cmd.UpdatedBy); err != nil {
			return err
		}
	}

	if hasV2DetailPatch(cmd) {
		cur := d.ValuationInputs()
		applyDetailValuationPatch(&cur, cmd)
		if err := d.AttachValuationInputs(cur); err != nil {
			return err
		}
	}

	if err := h.repo.UpdateDetail(ctx, d); err != nil {
		return fmt.Errorf("persist detail update: %w", err)
	}
	return nil
}

// applyDetailSnapshotPatch applies the command's patch fields directly onto
// a DetailPeriodSnapshot's exported fields. DetailPeriodSnapshot is a plain
// value type (not an aggregate root), so validation that on Detail lives
// behind private setters is re-asserted here explicitly, mirroring
// Detail.Update / Detail.AttachValuationInputs' rules field-for-field.
func applyDetailSnapshotPatch(snap *rmgroup.DetailPeriodSnapshot, cmd UpdateItemCommand) error {
	if cmd.SortOrder != nil {
		snap.SortOrder = *cmd.SortOrder
	}
	if cmd.IsActive != nil {
		snap.IsActive = *cmd.IsActive
	}

	if hasV2DetailPatch(cmd) {
		cur := snap.ValuationInputs
		applyDetailValuationPatch(&cur, cmd)
		if err := assertValuationInputsNonNegative(cur); err != nil {
			return err
		}
		snap.ValuationInputs = cur
	}

	if snap.CreatedBy == "" {
		snap.CreatedBy = cmd.UpdatedBy
	}
	updatedBy := cmd.UpdatedBy
	snap.UpdatedBy = &updatedBy
	return nil
}

// applyDetailValuationPatch merges the command's V2 valuation patch onto an
// existing ValuationInputs value in place, reusing the same low-level
// patchOptFloat helper as the head/marketing patch path.
func applyDetailValuationPatch(cur *rmgroup.ValuationInputs, cmd UpdateItemCommand) {
	cur.FreightRate = patchOptFloat(cur.FreightRate, cmd.ValuationFreightRate, cmd.ClearValuationFreightRate)
	cur.AntiDumpingPct = patchOptFloat(cur.AntiDumpingPct, cmd.ValuationAntiDumpingPct, cmd.ClearValuationAntiDumpingPct)
	cur.DutyPct = patchOptFloat(cur.DutyPct, cmd.ValuationDutyPct, cmd.ClearValuationDutyPct)
	cur.TransportRate = patchOptFloat(cur.TransportRate, cmd.ValuationTransportRate, cmd.ClearValuationTransportRate)
	cur.DefaultValue = patchOptFloat(cur.DefaultValue, cmd.ValuationDefaultValue, cmd.ClearValuationDefaultValue)
}

// assertValuationInputsNonNegative mirrors Detail.AttachValuationInputs'
// validation: every non-nil valuation input must be non-negative.
func assertValuationInputsNonNegative(in rmgroup.ValuationInputs) error {
	for _, p := range []*float64{in.FreightRate, in.AntiDumpingPct, in.DutyPct, in.TransportRate, in.DefaultValue} {
		if p != nil && *p < 0 {
			return rmgroup.ErrNegativeMarketValue
		}
	}
	return nil
}

func hasV2DetailPatch(cmd UpdateItemCommand) bool {
	return cmd.ValuationFreightRate != nil || cmd.ValuationAntiDumpingPct != nil ||
		cmd.ValuationDutyPct != nil || cmd.ValuationTransportRate != nil || cmd.ValuationDefaultValue != nil ||
		cmd.ClearValuationFreightRate || cmd.ClearValuationAntiDumpingPct ||
		cmd.ClearValuationDutyPct || cmd.ClearValuationTransportRate || cmd.ClearValuationDefaultValue
}
