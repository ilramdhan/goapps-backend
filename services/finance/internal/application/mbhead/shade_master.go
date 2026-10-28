package mbhead

import (
	"context"
	"errors"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/shade"
)

// headShadeCode returns the shade code a recipe carries: the user-visible "Shade
// Code" (mbh_code), falling back to mbh_shade_code when mbh_code is empty.
func headShadeCode(entity *mbhead.Entity) string {
	if c := entity.MBHCode(); c != nil && strings.TrimSpace(*c) != "" {
		return *c
	}
	return entity.ShadeCode()
}

// ensureShadeMaster auto-creates a MANUAL shade master row (cost_erp_shade) for the
// recipe's shade code when none exists yet. It runs AFTER the head is persisted and
// is strictly best-effort: every failure is logged as a warning and swallowed, so a
// shade-master problem never fails the recipe save. An existing shade (any source)
// is left untouched. A nil repo disables the hook.
func ensureShadeMaster(ctx context.Context, repo shade.Repository, entity *mbhead.Entity, actor string) {
	if repo == nil || entity == nil {
		return
	}
	code := shade.NormalizeCode(headShadeCode(entity))
	if code == "" {
		return
	}
	logger := log.With().Str("mbh_id", entity.ID().String()).Str("shade_code", code).Logger()

	existing, err := repo.GetByCode(ctx, code)
	if err != nil && !errors.Is(err, shade.ErrNotFound) {
		logger.Warn().Err(err).Msg("MB recipe save: shade master lookup failed; shade not auto-created")
		return
	}
	if existing != nil {
		return
	}

	name := strings.TrimSpace(entity.ShadeName())
	if name == "" {
		name = code
	}
	s, err := shade.New(shade.NewParams{Code: code, Name: name, CreatedBy: actor})
	if err != nil {
		logger.Warn().Err(err).Msg("MB recipe save: shade master auto-create rejected by domain")
		return
	}
	if err := repo.Create(ctx, s); err != nil {
		logger.Warn().Err(err).Msg("MB recipe save: shade master auto-create failed")
		return
	}
	logger.Info().Msg("MB recipe save: shade master auto-created (MANUAL)")
}
