// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/shade"
)

// UpdateCommand represents the update MB Head command.
type UpdateCommand struct {
	ID        uuid.UUID
	MBCosting *string
	MgtName   *string
	Denier    *float64
	Filament  *int
	Dozing    *float64
	// ⛔ MBHCheckStatus removed (§11 item 106): the frozen Oracle trace is not a
	// writable command field on any path. The gRPC layer rejects requests carrying it.
	MBHStatus       *string
	MBHLdrPrsn      *float64
	MBHRunLdrPct    *float64
	MBHFinalProduct *string
	MBHCode         *string
	IsActive        *bool
	DevCode         *string
	ShadeCode       *string
	ShadeName       *string
	CrossSection    *string
	LustureCode     *string
	MachineID       *uuid.UUID
	UpdatedBy       string

	// VSNumber / NoOfProcess: nil means "absent from the payload" and leaves the
	// stored value untouched (D13). ⛔ No default is applied on update either — the
	// no_of_process default is still an open user decision (U-B, plan §11 item 70).
	VSNumber    *string
	NoOfProcess *string

	// AdditionalShades is only consulted when ReplaceAdditionalShades is true.
	AdditionalShades []mbhead.Shade

	// ReplaceAdditionalShades is the explicit opt-in for rewriting the shade rows.
	//
	// 🔴 When false or absent the stored shades are LEFT ALONE. Without this gate
	// the legacy edit form — which never sends additional_shades — would wipe every
	// shade row on each save.
	ReplaceAdditionalShades bool
}

// LookupSyncTrigger queues a master-lookup sync for the yarn products that reference an
// MB master record, after an MB Recipe edit cascaded into it. Implementations are
// best-effort: the caller only logs a returned error, it never fails the save.
type LookupSyncTrigger interface {
	// TriggerSpinSync queues a sync for products referencing one MB Spin (key mbs_id).
	// There is deliberately no head-keyed (mb_costing) trigger: yarn rows never hold the
	// head costing text, so every cascaded change reaches them through the spins.
	TriggerSpinSync(ctx context.Context, spinID uuid.UUID, actor string) error
}

// lookupSyncTriggerTimeout bounds the detached after-commit lookup-sync queueing.
const lookupSyncTriggerTimeout = 2 * time.Minute

// UpdateHandler handles the UpdateMBHead command.
type UpdateHandler struct {
	repo      mbhead.Repository
	shadeRepo shade.Repository
	// paramRepo backs the CPP re-freeze of the cascade; nil skips the re-freeze.
	paramRepo mbparam.Repository
	// lookupSync is the after-commit lookup-sync port; nil disables it.
	lookupSync LookupSyncTrigger
}

// NewUpdateHandler creates a new UpdateHandler. shadeRepo backs the best-effort
// shade-master auto-create after a successful save; nil disables it.
func NewUpdateHandler(repo mbhead.Repository, shadeRepo shade.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo, shadeRepo: shadeRepo}
}

// WithShadeRepo attaches the shade master repository used by the best-effort
// shade auto-create and returns the handler for chaining.
func (h *UpdateHandler) WithShadeRepo(shadeRepo shade.Repository) *UpdateHandler {
	h.shadeRepo = shadeRepo
	return h
}

// WithParamRepo attaches the MB param master repository so an edit of a head that
// already has an MB cost product re-freezes its CPP params in the same transaction.
func (h *UpdateHandler) WithParamRepo(paramRepo mbparam.Repository) *UpdateHandler {
	h.paramRepo = paramRepo
	return h
}

// WithLookupSyncTrigger attaches the after-commit lookup-sync port. nil disables it.
func (h *UpdateHandler) WithLookupSyncTrigger(t LookupSyncTrigger) *UpdateHandler {
	h.lookupSync = t
	return h
}

// Handle executes the update MB Head command.
//
// ⛔ The domain's recipe required-field check is NOT invoked here (nor anywhere
// else): 573 legacy heads still carry a NULL cross-section column (plan K6) and
// must remain editable. Required-field enforcement is blocked behind the P4
// backfill.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*mbhead.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	if err := h.assertVSNumberFree(ctx, entity, cmd.VSNumber); err != nil {
		return nil, err
	}

	if err := h.assertDevCodeFree(ctx, entity, cmd.DevCode); err != nil {
		return nil, err
	}

	oldCosting := entity.MBCosting()
	oldMachineID := copyUUIDPtr(entity.MachineID())
	costingChanged := mbCostingChanged(oldCosting, cmd.MBCosting)
	mbCosting := cmd.MBCosting
	if costingChanged {
		if err := h.assertMBCostingFree(ctx, *cmd.MBCosting); err != nil {
			return nil, err
		}
	} else {
		// A trim-equal value is a no-op: never let a whitespace-only edit drift the
		// head away from the cost product / spin rows that copied its costing.
		mbCosting = nil
	}

	if err := entity.Update(mbhead.UpdateInput{
		MBCosting:       mbCosting,
		MgtName:         cmd.MgtName,
		Denier:          cmd.Denier,
		Filament:        cmd.Filament,
		Dozing:          cmd.Dozing,
		MBHStatus:       cmd.MBHStatus,
		MBHLdrPrsn:      cmd.MBHLdrPrsn,
		MBHRunLdrPct:    cmd.MBHRunLdrPct,
		MBHFinalProduct: cmd.MBHFinalProduct,
		MBHCode:         cmd.MBHCode,
		IsActive:        cmd.IsActive,
		DevCode:         cmd.DevCode,
		ShadeCode:       cmd.ShadeCode,
		ShadeName:       cmd.ShadeName,
		CrossSection:    cmd.CrossSection,
		LustureCode:     cmd.LustureCode,
		MachineID:       cmd.MachineID,
		VSNumber:        cmd.VSNumber,
		NoOfProcess:     cmd.NoOfProcess,
	}, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	if cmd.ReplaceAdditionalShades {
		if err := entity.SetAdditionalShades(cmd.AdditionalShades); err != nil {
			return nil, err
		}
	}

	refreeze := cppInputsChanged(oldMachineID, entity)
	spinIDs, err := h.persist(ctx, entity, oldCosting, costingChanged, refreeze)
	if err != nil {
		return nil, err
	}
	// The cascade transaction is committed at this point, so queue the lookup sync now:
	// a later ReplaceShades failure must not stop the yarn rows following the spins.
	h.triggerLookupSync(ctx, entity, spinIDs, cmd.UpdatedBy)

	if cmd.ReplaceAdditionalShades {
		if err := h.repo.ReplaceShades(ctx, entity.ID(), entity.AdditionalShades(), cmd.UpdatedBy); err != nil {
			return nil, err
		}
	}

	ensureShadeMaster(ctx, h.shadeRepo, entity, cmd.UpdatedBy)

	return entity, nil
}

// persist writes the head and returns the spin IDs the cascade touched.
//
// A head that already has an MB cost product (it has been validated, so its spins and
// product exist) goes through UpdateWithCascade: spins, cost product and — when the
// param master is wired — the CPP re-freeze change in the same transaction as the
// head (MB Recipe cascade design §3.2). The re-freeze runs ONLY when refreeze is true, i.e.
// a head field feeding the frozen CPP changed in this update (see cppInputsChanged): a
// cosmetic edit must neither pull changed mst_mb_param / mst_machine master values into a
// validated recipe nor be blocked by a broken master row. A costing rename on a head without a product
// keeps the pre-cascade rename of its spins; any other edit is a plain Update.
func (h *UpdateHandler) persist(ctx context.Context, entity *mbhead.Entity, oldCosting string, costingChanged, refreeze bool) ([]uuid.UUID, error) {
	if entity.CostProductID() == 0 {
		if costingChanged {
			_, err := h.repo.UpdateWithCascade(ctx, entity, oldCosting, nil)
			return nil, err
		}
		return nil, h.repo.Update(ctx, entity)
	}

	var params *mbhead.ParamSnapshot
	if refreeze && h.paramRepo != nil {
		p, err := resolveRefreezeSnapshot(ctx, h.paramRepo, entity)
		if err != nil {
			return nil, err
		}
		params = p
	}
	return h.repo.UpdateWithCascade(ctx, entity, oldCosting, params)
}

// cppInputsChanged reports whether this update changed a head field that feeds the frozen
// CPP rows written by mbFreezeCostParams (mbBuildParamValues). Of the inputs there — the
// frozen mbh_param_* snapshot, is_boughtout, current_version and machine_id — only
// machine_id (MACHINE_MB_FIXED_TOTAL) is writable through UpdateInput. mbh_no_of_process
// (the live choice) is NOT mbh_param_no_of_process (the frozen snapshot), so it does not
// count.
func cppInputsChanged(oldMachineID *uuid.UUID, entity *mbhead.Entity) bool {
	return !uuidPtrEqual(oldMachineID, entity.MachineID())
}

func copyUUIDPtr(id *uuid.UUID) *uuid.UUID {
	if id == nil {
		return nil
	}
	v := *id
	return &v
}

func uuidPtrEqual(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// triggerLookupSync queues, AFTER commit and detached from the request, a lookup sync
// for every cascaded spin so yarn products holding copies of the spin values follow
// (design §3.3). Strictly best-effort: failures are logged as warnings and never
// surface to the caller.
func (h *UpdateHandler) triggerLookupSync(ctx context.Context, entity *mbhead.Entity, spinIDs []uuid.UUID, actor string) {
	if h.lookupSync == nil || entity.CostProductID() == 0 || len(spinIDs) == 0 {
		return
	}
	trigger := h.lookupSync
	logger := log.With().Str("mbh_id", entity.ID().String()).Logger()
	bg := context.WithoutCancel(ctx)
	go func() {
		bgCtx, cancel := context.WithTimeout(bg, lookupSyncTriggerTimeout)
		defer cancel()
		for _, id := range spinIDs {
			if err := trigger.TriggerSpinSync(bgCtx, id, actor); err != nil {
				logger.Warn().Err(err).Str("mbs_id", id.String()).Msg("MB recipe cascade: lookup sync for spin not queued")
			}
		}
	}()
}

// mbCostingChanged reports whether an incoming MB Costing really renames the head.
// nil (field absent) and a value equal to the stored one after trimming are no-ops.
func mbCostingChanged(current string, incoming *string) bool {
	if incoming == nil {
		return false
	}
	return strings.TrimSpace(*incoming) != strings.TrimSpace(current)
}

// assertMBCostingFree rejects a rename onto an MB Costing another live head already
// uses (unique index uix_mst_mb_head_mb_costing). Only called when the value changes,
// so the head's own row can never collide with itself.
func (h *UpdateHandler) assertMBCostingFree(ctx context.Context, incoming string) error {
	taken, err := h.repo.ExistsByMBCosting(ctx, incoming)
	if err != nil {
		return err
	}
	if taken {
		return mbhead.ErrAlreadyExists
	}
	return nil
}

// assertVSNumberFree checks VS Number uniqueness ONLY when the value actually
// changes (B9). An unchanged value is never re-checked, so the 177 legacy heads
// carrying '0' — and the two sharing '16728' — stay editable. '0' and "" are
// exempt outright. See CreateHandler.assertVSNumberFree for the full rationale.
func (h *UpdateHandler) assertVSNumberFree(ctx context.Context, entity *mbhead.Entity, incoming *string) error {
	if !vsNumberNeedsUniquenessCheck(incoming) {
		return nil
	}
	if current := entity.VSNumber(); current != nil && *current == *incoming {
		return nil
	}
	taken, err := h.repo.ExistsByVSNumber(ctx, *incoming, entity.ID())
	if err != nil {
		return err
	}
	if taken {
		return mbhead.ErrDuplicateVSNumber
	}
	return nil
}

// assertDevCodeFree checks Dev Code uniqueness ONLY when the value actually
// changes (U-D, 2026-08-22), mirroring assertVSNumberFree. An unchanged value is
// never re-checked, so a head that already shares a legacy Dev Code with another
// head stays editable — the user's rule is "unique for new data, leave legacy
// duplicates alone". nil (field absent from the payload) and "" are exempt.
func (h *UpdateHandler) assertDevCodeFree(ctx context.Context, entity *mbhead.Entity, incoming *string) error {
	if incoming == nil || !devCodeNeedsUniquenessCheck(*incoming) {
		return nil
	}
	if entity.DevCode() == *incoming {
		return nil
	}
	taken, err := h.repo.ExistsByDevCode(ctx, *incoming, entity.ID())
	if err != nil {
		return err
	}
	if taken {
		return mbhead.ErrDuplicateDevCode
	}
	return nil
}
