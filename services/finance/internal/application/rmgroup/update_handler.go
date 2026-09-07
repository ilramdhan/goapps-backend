// Package rmgroup provides application layer handlers for RM group head and detail operations.
package rmgroup

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// UpdateCommand is the partial-update command for a head. Pointer fields stay nil
// when the caller wants to leave them unchanged. The three ClearInitVal flags
// force the corresponding init_val columns to NULL.
type UpdateCommand struct {
	HeadID string
	Period string

	Name           *string
	Description    *string
	Colorant       *string
	CIName         *string
	CostPercentage *float64
	CostPerKg      *float64

	FlagValuation  *string
	FlagMarketing  *string
	FlagSimulation *string

	InitValValuation  *float64
	InitValMarketing  *float64
	InitValSimulation *float64

	ClearInitValValuation  bool
	ClearInitValMarketing  bool
	ClearInitValSimulation bool

	IsActive *bool

	UpdatedBy string

	// V2 marketing fields.
	MarketingFreightRate    *float64
	MarketingAntiDumpingPct *float64
	MarketingDefaultValue   *float64
	ValuationFlag           *string // explicit "" allowed = AUTO
	MarketingFlag           *string

	ClearMarketingFreightRate    bool
	ClearMarketingAntiDumpingPct bool
	ClearMarketingDefaultValue   bool
}

// UpdateHandler handles UpdateHead commands.
type UpdateHandler struct {
	repo rmgroup.Repository
}

// NewUpdateHandler builds an UpdateHandler.
func NewUpdateHandler(repo rmgroup.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle resolves the period-scoped snapshot for cmd.Period (get-or-create from
// the anchor head when none exists yet), applies the partial update onto that
// snapshot, and upserts it. When cmd.Period is the current latest sync period,
// the same patch is additionally applied to the anchor cst_rm_group_head row
// (write-through rule, design doc §2.2/§5.1) so every period-blind reader of the
// anchor row keeps seeing up-to-date "current" config; edits to an older period
// leave the anchor row untouched.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*rmgroup.HeadPeriodSnapshot, error) {
	id, err := uuid.Parse(cmd.HeadID)
	if err != nil {
		return nil, rmgroup.ErrNotFound
	}
	if err := rmgroup.ValidatePeriodFormat(cmd.Period); err != nil {
		return nil, err
	}

	// Step 1: load the anchor Head — source of "what fields currently look like"
	// for building the get-or-create default, and the write-through target below.
	head, err := h.repo.GetHeadByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Step 2: resolve the period-scoped working copy (get-or-create semantics).
	snap, err := h.resolveHeadSnapshot(ctx, id, cmd.Period, head)
	if err != nil {
		return nil, err
	}

	// Step 3: apply the patch onto the snapshot's fields.
	if err := applyHeadSnapshotPatch(snap, cmd); err != nil {
		return nil, err
	}

	// Step 4: upsert the snapshot.
	if err := h.repo.UpsertHeadPeriod(ctx, snap); err != nil {
		return nil, fmt.Errorf("persist head period snapshot: %w", err)
	}

	// Step 5: conditional write-through to the anchor row when editing the
	// latest sync period.
	latest, err := h.repo.LatestSyncPeriod(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve latest sync period: %w", err)
	}
	if cmd.Period == latest {
		if err := h.writeThroughAnchor(ctx, head, cmd); err != nil {
			return nil, err
		}
	}

	// Step 6: return the up-to-date view for the requested period.
	return snap, nil
}

// resolveHeadSnapshot loads the (headID, period) snapshot, falling back to a
// fresh snapshot built from the anchor head's current values when none exists
// yet (get-or-create baseline, design §2.3).
func (h *UpdateHandler) resolveHeadSnapshot(
	ctx context.Context, headID uuid.UUID, period string, head *rmgroup.Head,
) (*rmgroup.HeadPeriodSnapshot, error) {
	snap, err := h.repo.GetHeadPeriodSnapshot(ctx, headID, period)
	if err == nil {
		return snap, nil
	}
	if !errors.Is(err, rmgroup.ErrNotFound) {
		return nil, fmt.Errorf("load head period snapshot: %w", err)
	}
	fresh := rmgroup.NewHeadPeriodSnapshotFromHead(period, head)
	return &fresh, nil
}

// writeThroughAnchor re-applies the same command patch to the anchor Head and
// persists it, exactly as the pre-versioning write path did.
func (h *UpdateHandler) writeThroughAnchor(ctx context.Context, head *rmgroup.Head, cmd UpdateCommand) error {
	in, err := buildHeadUpdateInput(cmd)
	if err != nil {
		return err
	}
	if err := head.Update(in, cmd.UpdatedBy); err != nil {
		return err
	}
	if err := applyV2MarketingPatch(head, cmd); err != nil {
		return err
	}
	if err := h.repo.UpdateHead(ctx, head); err != nil {
		return fmt.Errorf("persist head update: %w", err)
	}
	return nil
}

// applyHeadSnapshotPatch applies the command's patch fields directly onto a
// HeadPeriodSnapshot's exported fields. HeadPeriodSnapshot is a plain value
// type (not an aggregate root), so validation that on Head lives behind
// private setters is re-asserted here explicitly, mirroring Head.Update's
// rules field-for-field. IsActive is intentionally not patched — activity
// status is not period-scoped (design §4).
func applyHeadSnapshotPatch(snap *rmgroup.HeadPeriodSnapshot, cmd UpdateCommand) error {
	if err := applySnapshotNameField(snap, cmd.Name); err != nil {
		return err
	}
	applySnapshotTextFields(snap, cmd.Description, cmd.Colorant, cmd.CIName)
	if err := applySnapshotCostFields(snap, cmd.CostPercentage, cmd.CostPerKg); err != nil {
		return err
	}
	if err := applySnapshotInitValues(snap, cmd); err != nil {
		return err
	}
	if err := applySnapshotFlagFields(snap, cmd.FlagValuation, cmd.FlagMarketing, cmd.FlagSimulation); err != nil {
		return err
	}
	if err := assertSnapshotFlagInitConsistency(snap); err != nil {
		return err
	}
	if err := applySnapshotMarketingPatch(snap, cmd); err != nil {
		return err
	}

	if snap.CreatedBy == "" {
		snap.CreatedBy = cmd.UpdatedBy
	}
	updatedBy := cmd.UpdatedBy
	snap.UpdatedBy = &updatedBy
	return nil
}

func applySnapshotNameField(snap *rmgroup.HeadPeriodSnapshot, name *string) error {
	if name == nil {
		return nil
	}
	if *name == "" {
		return rmgroup.ErrEmptyName
	}
	if len(*name) > 200 {
		return rmgroup.ErrNameTooLong
	}
	snap.Name = *name
	return nil
}

func applySnapshotTextFields(snap *rmgroup.HeadPeriodSnapshot, description, colorant, ciName *string) {
	if description != nil {
		snap.Description = *description
	}
	if colorant != nil {
		snap.Colorant = *colorant
	}
	if ciName != nil {
		snap.CIName = *ciName
	}
}

func applySnapshotCostFields(snap *rmgroup.HeadPeriodSnapshot, costPercentage, costPerKg *float64) error {
	if costPercentage != nil {
		if *costPercentage < 0 {
			return rmgroup.ErrNegativeCostPercentage
		}
		snap.CostPercentage = *costPercentage
	}
	if costPerKg != nil {
		if *costPerKg < 0 {
			return rmgroup.ErrNegativeCostPerKg
		}
		snap.CostPerKg = *costPerKg
	}
	return nil
}

func applySnapshotInitValues(snap *rmgroup.HeadPeriodSnapshot, cmd UpdateCommand) error {
	if err := assignInitVal(&snap.InitValValuation, cmd.InitValValuation, cmd.ClearInitValValuation); err != nil {
		return err
	}
	if err := assignInitVal(&snap.InitValMarketing, cmd.InitValMarketing, cmd.ClearInitValMarketing); err != nil {
		return err
	}
	return assignInitVal(&snap.InitValSimulation, cmd.InitValSimulation, cmd.ClearInitValSimulation)
}

// assignInitVal applies the same clear/patch/validate rule as the domain
// entity's private assignInitVal — duplicated here because HeadPeriodSnapshot
// exposes plain fields rather than a private-setter aggregate.
func assignInitVal(target **float64, incoming *float64, reset bool) error {
	if reset {
		*target = nil
		return nil
	}
	if incoming == nil {
		return nil
	}
	if *incoming < 0 {
		return rmgroup.ErrNegativeInitValue
	}
	v := *incoming
	*target = &v
	return nil
}

func applySnapshotFlagFields(snap *rmgroup.HeadPeriodSnapshot, valuation, marketing, simulation *string) error {
	if err := assignSnapshotFlag(&snap.FlagValuation, valuation); err != nil {
		return err
	}
	if err := assignSnapshotFlag(&snap.FlagMarketing, marketing); err != nil {
		return err
	}
	return assignSnapshotFlag(&snap.FlagSimulation, simulation)
}

func assignSnapshotFlag(target *rmgroup.Flag, raw *string) error {
	if raw == nil {
		return nil
	}
	flag, err := rmgroup.ParseFlag(*raw)
	if err != nil {
		return err
	}
	*target = flag
	return nil
}

// assertSnapshotFlagInitConsistency mirrors Head.assertFlagInitConsistency:
// a flag set to INIT requires a non-nil init_val.
func assertSnapshotFlagInitConsistency(snap *rmgroup.HeadPeriodSnapshot) error {
	if snap.FlagValuation == rmgroup.FlagInit && snap.InitValValuation == nil {
		return rmgroup.ErrInitValueRequired
	}
	if snap.FlagMarketing == rmgroup.FlagInit && snap.InitValMarketing == nil {
		return rmgroup.ErrInitValueRequired
	}
	if snap.FlagSimulation == rmgroup.FlagInit && snap.InitValSimulation == nil {
		return rmgroup.ErrInitValueRequired
	}
	return nil
}

// applySnapshotMarketingPatch merges the V2 marketing patch onto the
// snapshot's existing MarketingInputs, reusing the same low-level patch
// helpers as applyV2MarketingPatch (which operate on a plain MarketingInputs
// value, not a Head-specific receiver).
func applySnapshotMarketingPatch(snap *rmgroup.HeadPeriodSnapshot, cmd UpdateCommand) error {
	if !hasV2MarketingPatch(cmd) {
		return nil
	}
	mi := snap.MarketingInputs
	mi.FreightRate = patchOptFloat(mi.FreightRate, cmd.MarketingFreightRate, cmd.ClearMarketingFreightRate)
	mi.AntiDumpingPct = patchOptFloat(mi.AntiDumpingPct, cmd.MarketingAntiDumpingPct, cmd.ClearMarketingAntiDumpingPct)
	mi.DefaultValue = patchOptFloat(mi.DefaultValue, cmd.MarketingDefaultValue, cmd.ClearMarketingDefaultValue)
	if err := applyV2ValuationFlag(&mi, cmd.ValuationFlag); err != nil {
		return err
	}
	if err := applyV2MarketingFlag(&mi, cmd.MarketingFlag); err != nil {
		return err
	}
	snap.MarketingInputs = mi
	return nil
}

func hasV2MarketingPatch(cmd UpdateCommand) bool {
	return cmd.MarketingFreightRate != nil || cmd.MarketingAntiDumpingPct != nil || cmd.MarketingDefaultValue != nil ||
		cmd.ValuationFlag != nil || cmd.MarketingFlag != nil ||
		cmd.ClearMarketingFreightRate || cmd.ClearMarketingAntiDumpingPct || cmd.ClearMarketingDefaultValue
}

// applyV2MarketingPatch merges the V2 marketing patch onto the head's
// existing MarketingInputs and re-attaches them through the validating setter.
func applyV2MarketingPatch(head *rmgroup.Head, cmd UpdateCommand) error {
	if !hasV2MarketingPatch(cmd) {
		return nil
	}
	mi := head.MarketingInputs()
	mi.FreightRate = patchOptFloat(mi.FreightRate, cmd.MarketingFreightRate, cmd.ClearMarketingFreightRate)
	mi.AntiDumpingPct = patchOptFloat(mi.AntiDumpingPct, cmd.MarketingAntiDumpingPct, cmd.ClearMarketingAntiDumpingPct)
	mi.DefaultValue = patchOptFloat(mi.DefaultValue, cmd.MarketingDefaultValue, cmd.ClearMarketingDefaultValue)
	if err := applyV2ValuationFlag(&mi, cmd.ValuationFlag); err != nil {
		return err
	}
	if err := applyV2MarketingFlag(&mi, cmd.MarketingFlag); err != nil {
		return err
	}
	return head.AttachMarketingInputs(mi)
}

func applyV2ValuationFlag(mi *rmgroup.MarketingInputs, raw *string) error {
	if raw == nil {
		return nil
	}
	vf, err := rmgroup.ParseValuationFlag(*raw)
	if err != nil {
		return err
	}
	mi.ValuationFlag = vf
	return nil
}

func applyV2MarketingFlag(mi *rmgroup.MarketingInputs, raw *string) error {
	if raw == nil {
		return nil
	}
	mf, err := rmgroup.ParseMarketingFlag(*raw)
	if err != nil {
		return err
	}
	mi.MarketingFlag = mf
	return nil
}

func patchOptFloat(cur, in *float64, clearField bool) *float64 {
	if clearField {
		return nil
	}
	if in == nil {
		return cur
	}
	v := *in
	return &v
}

// buildHeadUpdateInput maps command pointers to the domain UpdateInput, parsing
// the three optional flag strings into typed Flag values.
func buildHeadUpdateInput(cmd UpdateCommand) (rmgroup.UpdateInput, error) {
	in := rmgroup.UpdateInput{
		Name:                   cmd.Name,
		Description:            cmd.Description,
		Colorant:               cmd.Colorant,
		CIName:                 cmd.CIName,
		CostPercentage:         cmd.CostPercentage,
		CostPerKg:              cmd.CostPerKg,
		InitValValuation:       cmd.InitValValuation,
		InitValMarketing:       cmd.InitValMarketing,
		InitValSimulation:      cmd.InitValSimulation,
		ClearInitValValuation:  cmd.ClearInitValValuation,
		ClearInitValMarketing:  cmd.ClearInitValMarketing,
		ClearInitValSimulation: cmd.ClearInitValSimulation,
		IsActive:               cmd.IsActive,
	}

	if err := assignFlag(&in.FlagValuation, cmd.FlagValuation); err != nil {
		return in, err
	}
	if err := assignFlag(&in.FlagMarketing, cmd.FlagMarketing); err != nil {
		return in, err
	}
	if err := assignFlag(&in.FlagSimulation, cmd.FlagSimulation); err != nil {
		return in, err
	}
	return in, nil
}

func assignFlag(target **rmgroup.Flag, raw *string) error {
	if raw == nil {
		return nil
	}
	flag, err := rmgroup.ParseFlag(*raw)
	if err != nil {
		return err
	}
	*target = &flag
	return nil
}
