package costfillassignment

import (
	"context"
	"errors"
	"fmt"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
)

// ConfigResolverAdapter resolves the effective fill config for a (request, product, level) tuple
// by reading all three tiers and calling domain.Resolve.
type ConfigResolverAdapter struct {
	repo domain.ConfigRepository
}

// NewConfigResolverAdapter constructs the adapter.
func NewConfigResolverAdapter(repo domain.ConfigRepository) *ConfigResolverAdapter {
	return &ConfigResolverAdapter{repo: repo}
}

// Resolve returns the merged effective config for a route level. Returns ErrConfigNotFound
// if no global config exists for that level.
func (a *ConfigResolverAdapter) Resolve(ctx context.Context, productSysID, requestID int64, routeLevel int32) (domain.ResolvedConfig, error) {
	global, err := a.repo.GetGlobal(ctx, routeLevel)
	if err != nil {
		if errors.Is(err, domain.ErrConfigNotFound) {
			return domain.ResolvedConfig{}, domain.ErrConfigNotFound
		}
		return domain.ResolvedConfig{}, fmt.Errorf("get global config level %d: %w", routeLevel, err)
	}
	var product *domain.Config
	if productSysID > 0 {
		product, err = a.repo.GetProduct(ctx, productSysID, routeLevel)
		if err != nil {
			return domain.ResolvedConfig{}, fmt.Errorf("get product config level %d: %w", routeLevel, err)
		}
	}
	var request *domain.Config
	if requestID > 0 {
		request, err = a.repo.GetRequest(ctx, requestID, routeLevel)
		if err != nil {
			return domain.ResolvedConfig{}, fmt.Errorf("get request config level %d: %w", routeLevel, err)
		}
	}
	return domain.Resolve(global, product, request)
}
