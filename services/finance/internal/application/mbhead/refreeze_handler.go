package mbhead

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
)

// RefreezeCommand represents a request to re-freeze an already-VALIDATED MB head's cost
// parameters without changing its entry_status or bumping its version.
type RefreezeCommand struct {
	MbhID       uuid.UUID
	ActorUserID string
}

// RefreezeHandler handles the re-freeze operation for already-VALIDATED MB heads whose
// frozen param values were incorrect (e.g. throughput/no_of_process taking the param-master
// default instead of the head's own values — ENG-MB-01).
type RefreezeHandler struct {
	repo      mbhead.Repository
	paramRepo mbparam.Repository
	validate  *ValidateHandler
}

// NewRefreezeHandler creates a RefreezeHandler.
func NewRefreezeHandler(repo mbhead.Repository, paramRepo mbparam.Repository) *RefreezeHandler {
	return &RefreezeHandler{
		repo:      repo,
		paramRepo: paramRepo,
		validate:  NewValidateHandler(repo, paramRepo),
	}
}

// Handle re-resolves the param snapshot (with head-override logic from
// applyHeadParamOverrides) and atomically updates both mst_mb_head and
// cost_product_parameter via RefreezeCostParams. Does not change entry_status,
// bump the version, or create a workflow log.
func (h *RefreezeHandler) Handle(ctx context.Context, cmd RefreezeCommand) error {
	entity, err := h.repo.GetByID(ctx, cmd.MbhID)
	if err != nil {
		return err
	}
	if entity.CostProductID() == 0 {
		return fmt.Errorf("refreeze %s: cost product has not been auto-generated yet; validate first", entity.MBCosting())
	}

	params, err := h.validate.resolveParamSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("resolve param snapshot: %w", err)
	}
	applyHeadParamOverrides(params, entity)

	// Freeze the corrected values onto the in-memory entity too, so
	// mbFreezeCostParams inside RefreezeCostParams reads the right values from
	// entity.ParamThroughputPerHour() etc. (the CPP write uses getters, not params).
	entity.FreezeParams(
		params.Waste, params.QualityLoss, params.Efficiency, params.DevExpense,
		params.Packing, params.MBProdPerDay, params.ThroughputPerHour, params.NoOfProcess,
	)

	return h.repo.RefreezeCostParams(ctx, cmd.MbhID, entity, params)
}
