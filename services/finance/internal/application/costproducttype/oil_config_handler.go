package costproducttype

import (
	"context"
	"fmt"
	"strings"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
)

// GetOilConfigHandler returns a product type's oil class + allowed oil groups.
type GetOilConfigHandler struct{ repo domain.OilConfigRepository }

// NewGetOilConfigHandler constructs a GetOilConfigHandler.
func NewGetOilConfigHandler(r domain.OilConfigRepository) *GetOilConfigHandler {
	return &GetOilConfigHandler{repo: r}
}

// Handle loads the config.
func (h *GetOilConfigHandler) Handle(ctx context.Context, typeID int32) (*domain.OilConfig, error) {
	return h.repo.GetOilConfig(ctx, typeID)
}

// SetOilConfigCommand replaces a product type's oil config.
type SetOilConfigCommand struct {
	TypeID   int32
	OilClass string
	Groups   []domain.OilGroupEntry
	Actor    string
}

// SetOilConfigHandler validates and replaces a product type's oil config
// (oil-cost-rm-group spec §4.7).
type SetOilConfigHandler struct{ repo domain.OilConfigRepository }

// NewSetOilConfigHandler constructs a SetOilConfigHandler.
func NewSetOilConfigHandler(r domain.OilConfigRepository) *SetOilConfigHandler {
	return &SetOilConfigHandler{repo: r}
}

// Handle validates the shape, checks every group is an active oil RM group,
// replaces the config atomically and returns the stored result.
func (h *SetOilConfigHandler) Handle(ctx context.Context, cmd SetOilConfigCommand) (*domain.OilConfig, error) {
	class, groups, err := domain.NormalizeOilConfig(cmd.OilClass, cmd.Groups)
	if err != nil {
		return nil, err
	}
	// Existence check first so an unknown type is a 404, not a group error.
	if _, err := h.repo.GetOilConfig(ctx, cmd.TypeID); err != nil {
		return nil, err
	}
	rows, err := h.resolveGroups(ctx, groups)
	if err != nil {
		return nil, err
	}
	if err := h.repo.ReplaceOilConfig(ctx, cmd.TypeID, class, rows, cmd.Actor); err != nil {
		return nil, fmt.Errorf("replace oil config: %w", err)
	}
	return h.repo.GetOilConfig(ctx, cmd.TypeID)
}

// resolveGroups maps codes to group head ids, rejecting codes that are not
// active oil RM groups (ErrOilConfigGroupNotOil, listing every offender).
func (h *SetOilConfigHandler) resolveGroups(ctx context.Context, groups []domain.OilGroupEntry) ([]domain.ReplaceOilGroup, error) {
	if len(groups) == 0 {
		return nil, nil
	}
	codes := make([]string, 0, len(groups))
	for _, g := range groups {
		codes = append(codes, g.GroupCode)
	}
	resolved, err := h.repo.ResolveOilGroups(ctx, codes)
	if err != nil {
		return nil, fmt.Errorf("resolve oil groups: %w", err)
	}
	var missing []string
	out := make([]domain.ReplaceOilGroup, 0, len(groups))
	for _, g := range groups {
		r, ok := resolved[g.GroupCode]
		if !ok {
			missing = append(missing, g.GroupCode)
			continue
		}
		out = append(out, domain.ReplaceOilGroup{GroupHeadID: r.GroupHeadID, IsDefault: g.IsDefault})
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%w: %s", domain.ErrOilConfigGroupNotOil, strings.Join(missing, ", "))
	}
	return out, nil
}
