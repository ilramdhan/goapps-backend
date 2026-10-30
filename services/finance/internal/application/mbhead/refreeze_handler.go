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
}

// NewRefreezeHandler creates a RefreezeHandler.
func NewRefreezeHandler(repo mbhead.Repository, paramRepo mbparam.Repository) *RefreezeHandler {
	return &RefreezeHandler{repo: repo, paramRepo: paramRepo}
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

	params, err := resolveRefreezeSnapshot(ctx, h.paramRepo, entity)
	if err != nil {
		return err
	}
	return h.repo.RefreezeCostParams(ctx, cmd.MbhID, entity, params)
}

// resolveRefreezeSnapshot resolves the param-master snapshot, lets the head's own values
// win (applyHeadParamOverrides) and freezes the result onto the in-memory entity too, so
// mbFreezeCostParams — which reads entity.ParamThroughputPerHour() etc., not params —
// writes the corrected values. Shared by RefreezeHandler and the UpdateHandler cascade.
func resolveRefreezeSnapshot(ctx context.Context, paramRepo mbparam.Repository, entity *mbhead.Entity) (*mbhead.ParamSnapshot, error) {
	params, err := resolveParamSnapshotFrom(ctx, paramRepo)
	if err != nil {
		return nil, fmt.Errorf("resolve param snapshot: %w", err)
	}
	applyHeadParamOverrides(params, entity)
	entity.FreezeParams(
		params.Waste, params.QualityLoss, params.Efficiency, params.DevExpense,
		params.Packing, params.MBProdPerDay, params.ThroughputPerHour, params.NoOfProcess,
	)
	return params, nil
}
