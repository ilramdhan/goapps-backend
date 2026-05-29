// Package dashboard contains application-layer handlers for BI dashboard CRUD,
// duplication, role mapping, and viewer-side accessibility listing.
//
// All handlers follow the Command/Query pattern: each struct represents one
// operation, and Handle(ctx, cmd) is the single entry point. Construction is
// via NewXxxHandler(repo). The application layer mediates between gRPC delivery
// (which constructs commands from proto requests) and domain logic + repo.
package dashboard

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	redisinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/redis"
)

// Cache is the minimal interface this package needs from the BI chart cache.
type Cache interface {
	InvalidateDashboard(ctx context.Context, dashboardCode string) error
}

// =========================================================================
// CreateHandler — create a new dashboard
// =========================================================================

// CreateCommand is the payload for CreateHandler.
type CreateCommand struct {
	Code               string
	Title              string
	Description        string
	FilterType         string
	FilterGroup1       string
	PeriodGrain        string
	DefaultPeriod      string
	ChartType          string
	ChartConfigRaw     map[string]any
	LayoutConfigRaw    map[string]any
	KpiConfigRaw       []map[string]any
	CompareModes       []string
	DrillEnabled       bool
	MaxDrillLevel      int
	CacheTTLSec        int
	RefreshIntervalSec int
	DisplayOrder       int
	GroupID            uuid.UUID
	AllowedRoleCodes   []string
	IsActive           bool
	CreatedBy          uuid.UUID
}

// CreateHandler validates the command, constructs the Dashboard, and persists it.
type CreateHandler struct {
	repo dashboarddomain.Repository
}

// NewCreateHandler constructs a CreateHandler.
func NewCreateHandler(repo dashboarddomain.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the create-dashboard use case.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*dashboarddomain.Dashboard, error) {
	d, err := dashboarddomain.NewDashboard(dashboarddomain.NewDashboardParams{
		Code:               cmd.Code,
		Title:              cmd.Title,
		Description:        cmd.Description,
		FilterType:         cmd.FilterType,
		FilterGroup1:       cmd.FilterGroup1,
		PeriodGrain:        cmd.PeriodGrain,
		DefaultPeriod:      cmd.DefaultPeriod,
		ChartType:          cmd.ChartType,
		ChartConfigRaw:     cmd.ChartConfigRaw,
		LayoutConfigRaw:    cmd.LayoutConfigRaw,
		KpiConfigRaw:       cmd.KpiConfigRaw,
		CompareModes:       cmd.CompareModes,
		DrillEnabled:       cmd.DrillEnabled,
		MaxDrillLevel:      cmd.MaxDrillLevel,
		CacheTTLSec:        cmd.CacheTTLSec,
		RefreshIntervalSec: cmd.RefreshIntervalSec,
		DisplayOrder:       cmd.DisplayOrder,
		GroupID:            cmd.GroupID,
		IsActive:           cmd.IsActive,
		AllowedRoleCodes:   cmd.AllowedRoleCodes,
		CreatedBy:          cmd.CreatedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("validate dashboard: %w", err)
	}
	if err := h.repo.Create(ctx, d); err != nil {
		return nil, fmt.Errorf("persist dashboard: %w", err)
	}
	return d, nil
}

// =========================================================================
// GetHandler — fetch by ID or code
// =========================================================================

// GetByIDQuery is the payload for GetHandler.HandleByID.
type GetByIDQuery struct{ ID uuid.UUID }

// GetByCodeQuery is the payload for GetHandler.HandleByCode.
type GetByCodeQuery struct{ Code string }

// GetHandler reads dashboards by ID or code.
type GetHandler struct {
	repo dashboarddomain.Repository
}

// NewGetHandler constructs a GetHandler.
func NewGetHandler(repo dashboarddomain.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// HandleByID looks up by primary key.
func (h *GetHandler) HandleByID(ctx context.Context, q GetByIDQuery) (*dashboarddomain.Dashboard, error) {
	return h.repo.GetByID(ctx, q.ID)
}

// HandleByCode looks up by business code.
func (h *GetHandler) HandleByCode(ctx context.Context, q GetByCodeQuery) (*dashboarddomain.Dashboard, error) {
	return h.repo.GetByCode(ctx, q.Code)
}

// =========================================================================
// ListHandler — paginated admin list
// =========================================================================

// ListQuery is the payload for ListHandler.
type ListQuery = dashboarddomain.ListFilter

// ListResult bundles the page + total count.
type ListResult struct {
	Items []*dashboarddomain.Dashboard
	Total int64
}

// ListHandler returns paginated dashboards.
type ListHandler struct {
	repo dashboarddomain.Repository
}

// NewListHandler constructs a ListHandler.
func NewListHandler(repo dashboarddomain.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the list query.
func (h *ListHandler) Handle(ctx context.Context, q ListQuery) (ListResult, error) {
	items, total, err := h.repo.List(ctx, q)
	if err != nil {
		return ListResult{}, fmt.Errorf("list dashboards: %w", err)
	}
	return ListResult{Items: items, Total: total}, nil
}

// =========================================================================
// UpdateHandler — partial update + cache invalidation
// =========================================================================

// UpdateCommand is the payload for UpdateHandler. All non-ID fields are optional.
type UpdateCommand struct {
	ID                 uuid.UUID
	Title              *string
	Description        *string
	FilterType         *string
	FilterGroup1       *string
	PeriodGrain        *string
	DefaultPeriod      *string
	ChartType          *string
	ChartConfigRaw     map[string]any
	LayoutConfigRaw    map[string]any
	KpiConfigRaw       []map[string]any
	CompareModes       []string
	DrillEnabled       *bool
	MaxDrillLevel      *int
	CacheTTLSec        *int
	RefreshIntervalSec *int
	DisplayOrder       *int
	GroupID            *uuid.UUID
	IsActive           *bool
	IsFeatured         *bool
	FeatureOrder       *int
	AllowedRoleCodes   []string
	UpdatedBy          uuid.UUID
}

// UpdateHandler applies a partial update + invalidates the Redis cache for that dashboard.
type UpdateHandler struct {
	repo  dashboarddomain.Repository
	cache Cache
}

// NewUpdateHandler constructs an UpdateHandler. cache may be nil (no-op invalidation).
func NewUpdateHandler(repo dashboarddomain.Repository, cache Cache) *UpdateHandler {
	return &UpdateHandler{repo: repo, cache: cache}
}

// Handle executes the update.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*dashboarddomain.Dashboard, error) {
	d, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if err := d.Update(dashboarddomain.UpdateParams{
		Title:              cmd.Title,
		Description:        cmd.Description,
		FilterType:         cmd.FilterType,
		FilterGroup1:       cmd.FilterGroup1,
		PeriodGrain:        cmd.PeriodGrain,
		DefaultPeriod:      cmd.DefaultPeriod,
		ChartType:          cmd.ChartType,
		ChartConfigRaw:     cmd.ChartConfigRaw,
		LayoutConfigRaw:    cmd.LayoutConfigRaw,
		KpiConfigRaw:       cmd.KpiConfigRaw,
		CompareModes:       cmd.CompareModes,
		DrillEnabled:       cmd.DrillEnabled,
		MaxDrillLevel:      cmd.MaxDrillLevel,
		CacheTTLSec:        cmd.CacheTTLSec,
		RefreshIntervalSec: cmd.RefreshIntervalSec,
		DisplayOrder:       cmd.DisplayOrder,
		GroupID:            cmd.GroupID,
		IsActive:           cmd.IsActive,
		IsFeatured:         cmd.IsFeatured,
		FeatureOrder:       cmd.FeatureOrder,
		AllowedRoleCodes:   cmd.AllowedRoleCodes,
		UpdatedBy:          cmd.UpdatedBy,
	}); err != nil {
		return nil, fmt.Errorf("apply update: %w", err)
	}
	if err := h.repo.Update(ctx, d); err != nil {
		return nil, fmt.Errorf("persist update: %w", err)
	}
	if h.cache != nil {
		if err := h.cache.InvalidateDashboard(ctx, d.Code().String()); err != nil {
			_ = err //nolint:errcheck // best-effort cache invalidation; stale cache is acceptable
		}
	}
	return d, nil
}

// =========================================================================
// DeleteHandler — soft delete + cache invalidation
// =========================================================================

// DeleteCommand is the payload for DeleteHandler.
type DeleteCommand struct {
	ID        uuid.UUID
	DeletedBy uuid.UUID
}

// DeleteHandler performs a soft delete and invalidates cache.
type DeleteHandler struct {
	repo  dashboarddomain.Repository
	cache Cache
}

// NewDeleteHandler constructs a DeleteHandler.
func NewDeleteHandler(repo dashboarddomain.Repository, cache Cache) *DeleteHandler {
	return &DeleteHandler{repo: repo, cache: cache}
}

// Handle executes the soft-delete.
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	// Fetch first so we know the code for cache invalidation
	d, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return err
	}
	if err := h.repo.SoftDelete(ctx, cmd.ID, cmd.DeletedBy); err != nil {
		return fmt.Errorf("soft delete: %w", err)
	}
	if h.cache != nil {
		if err := h.cache.InvalidateDashboard(ctx, d.Code().String()); err != nil {
			_ = err //nolint:errcheck // best-effort cache invalidation; stale cache is acceptable
		}
	}
	return nil
}

// =========================================================================
// DuplicateHandler — clone with new code/title
// =========================================================================

// DuplicateCommand is the payload for DuplicateHandler.
type DuplicateCommand struct {
	SourceID  uuid.UUID
	NewCode   string
	NewTitle  string
	CreatedBy uuid.UUID
}

// DuplicateHandler clones a dashboard.
type DuplicateHandler struct {
	repo dashboarddomain.Repository
}

// NewDuplicateHandler constructs a DuplicateHandler.
func NewDuplicateHandler(repo dashboarddomain.Repository) *DuplicateHandler {
	return &DuplicateHandler{repo: repo}
}

// Handle executes the duplicate.
func (h *DuplicateHandler) Handle(ctx context.Context, cmd DuplicateCommand) (*dashboarddomain.Dashboard, error) {
	return h.repo.Duplicate(ctx, cmd.SourceID, cmd.NewCode, cmd.NewTitle, cmd.CreatedBy)
}

// =========================================================================
// SetRolesHandler — overwrite role whitelist + invalidate cache
// =========================================================================

// SetRolesCommand is the payload for SetRolesHandler.
type SetRolesCommand struct {
	DashboardID uuid.UUID
	RoleCodes   []string
	UpdatedBy   uuid.UUID
}

// SetRolesHandler overwrites the dashboard role mapping.
type SetRolesHandler struct {
	repo  dashboarddomain.Repository
	cache Cache
}

// NewSetRolesHandler constructs a SetRolesHandler.
func NewSetRolesHandler(repo dashboarddomain.Repository, cache Cache) *SetRolesHandler {
	return &SetRolesHandler{repo: repo, cache: cache}
}

// Handle executes the role mapping update.
func (h *SetRolesHandler) Handle(ctx context.Context, cmd SetRolesCommand) ([]string, error) {
	if err := h.repo.SetRoles(ctx, cmd.DashboardID, cmd.RoleCodes, cmd.UpdatedBy); err != nil {
		return nil, fmt.Errorf("set roles: %w", err)
	}
	current, err := h.repo.GetRoles(ctx, cmd.DashboardID)
	if err != nil {
		return nil, err
	}
	// Invalidate the cache for this dashboard so role-gated viewer queries pick up the change.
	if h.cache != nil {
		if d, getErr := h.repo.GetByID(ctx, cmd.DashboardID); getErr == nil {
			if invErr := h.cache.InvalidateDashboard(ctx, d.Code().String()); invErr != nil {
				_ = invErr //nolint:errcheck // best-effort cache invalidation; stale cache is acceptable
			}
		}
	}
	return current, nil
}

// =========================================================================
// ListAccessibleHandler — viewer sidebar (per-user filter)
// =========================================================================

// ListAccessibleQuery carries the calling user's identity / roles.
type ListAccessibleQuery struct {
	UserRoles    []string
	IsSuperAdmin bool
}

// ListAccessibleHandler returns active dashboards visible to the user.
type ListAccessibleHandler struct {
	repo dashboarddomain.Repository
}

// NewListAccessibleHandler constructs a ListAccessibleHandler.
func NewListAccessibleHandler(repo dashboarddomain.Repository) *ListAccessibleHandler {
	return &ListAccessibleHandler{repo: repo}
}

// Handle executes the visibility query.
func (h *ListAccessibleHandler) Handle(ctx context.Context, q ListAccessibleQuery) ([]*dashboarddomain.Dashboard, error) {
	return h.repo.ListAccessible(ctx, q.UserRoles, q.IsSuperAdmin)
}

// =========================================================================
// ListFeaturedHandler — featured dashboards for the Executive Dashboard landing page
// =========================================================================

// ListFeaturedHandler returns dashboards pinned to the Executive Dashboard landing page.
type ListFeaturedHandler struct {
	repo dashboarddomain.Repository
}

// NewListFeaturedHandler constructs a ListFeaturedHandler.
func NewListFeaturedHandler(repo dashboarddomain.Repository) *ListFeaturedHandler {
	return &ListFeaturedHandler{repo: repo}
}

// Handle executes the featured-dashboard query.
func (h *ListFeaturedHandler) Handle(ctx context.Context) ([]*dashboarddomain.Dashboard, error) {
	return h.repo.ListFeatured(ctx)
}

// Compile-time assertion that the redis cache satisfies the local Cache interface
// (lets the wiring layer pass a *redisinfra.ChartCache directly).
var _ Cache = (*redisinfra.ChartCache)(nil)
