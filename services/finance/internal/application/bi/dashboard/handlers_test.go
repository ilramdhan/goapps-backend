package dashboard_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	dashboardapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/dashboard"
	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
)

// fakeRepo records calls and lets tests script returns.
type fakeRepo struct {
	created   *dashboarddomain.Dashboard
	updated   *dashboarddomain.Dashboard
	deletedID uuid.UUID
	setRoles  []string
	byID      *dashboarddomain.Dashboard
	createErr error
}

func (r *fakeRepo) Create(_ context.Context, d *dashboarddomain.Dashboard) error {
	r.created = d
	return r.createErr
}
func (r *fakeRepo) GetByID(_ context.Context, _ uuid.UUID) (*dashboarddomain.Dashboard, error) {
	if r.byID == nil {
		return nil, dashboarddomain.ErrNotFound
	}
	return r.byID, nil
}
func (r *fakeRepo) GetByCode(context.Context, string) (*dashboarddomain.Dashboard, error) {
	return nil, dashboarddomain.ErrNotFound
}
func (r *fakeRepo) List(context.Context, dashboarddomain.ListFilter) ([]*dashboarddomain.Dashboard, int64, error) {
	return []*dashboarddomain.Dashboard{r.byID}, 1, nil
}
func (r *fakeRepo) Update(_ context.Context, d *dashboarddomain.Dashboard) error {
	r.updated = d
	return nil
}
func (r *fakeRepo) SoftDelete(_ context.Context, id uuid.UUID, _ uuid.UUID) error {
	r.deletedID = id
	return nil
}
func (r *fakeRepo) Duplicate(context.Context, uuid.UUID, string, string, uuid.UUID) (*dashboarddomain.Dashboard, error) {
	return r.byID, nil
}
func (r *fakeRepo) SetRoles(_ context.Context, _ uuid.UUID, roles []string, _ uuid.UUID) error {
	r.setRoles = roles
	return nil
}
func (r *fakeRepo) GetRoles(context.Context, uuid.UUID) ([]string, error) { return r.setRoles, nil }
func (r *fakeRepo) ListAccessible(context.Context, []string, bool) ([]*dashboarddomain.Dashboard, error) {
	return []*dashboarddomain.Dashboard{r.byID}, nil
}
func (r *fakeRepo) ListFeatured(context.Context) ([]*dashboarddomain.Dashboard, error) {
	return []*dashboarddomain.Dashboard{r.byID}, nil
}

// spyCache records invalidations.
type spyCache struct{ invalidated []string }

func (c *spyCache) InvalidateDashboard(_ context.Context, code string) error {
	c.invalidated = append(c.invalidated, code)
	return nil
}

func validCreateCmd() dashboardapp.CreateCommand {
	return dashboardapp.CreateCommand{
		Code:           "NEW_DASH",
		Title:          "New",
		FilterType:     "MIS",
		PeriodGrain:    "MONTHLY",
		DefaultPeriod:  "L12M",
		ChartType:      "bar",
		ChartConfigRaw: map[string]any{"x_axis_field": "group_1", "y_axis_field": "value"},
		MaxDrillLevel:  1,
		CacheTTLSec:    60,
		GroupID:        uuid.New(),
		IsActive:       true,
		CreatedBy:      uuid.New(),
	}
}

func TestCreateHandler_Success(t *testing.T) {
	repo := &fakeRepo{}
	h := dashboardapp.NewCreateHandler(repo)
	d, err := h.Handle(context.Background(), validCreateCmd())
	if err != nil {
		t.Fatal(err)
	}
	if d.Code().String() != "NEW_DASH" {
		t.Errorf("code: %v", d.Code().String())
	}
	if repo.created == nil {
		t.Error("repo.Create not called")
	}
}

func TestCreateHandler_InvalidConfigRejected(t *testing.T) {
	repo := &fakeRepo{}
	h := dashboardapp.NewCreateHandler(repo)
	cmd := validCreateCmd()
	cmd.ChartConfigRaw = map[string]any{} // missing required fields for bar
	_, err := h.Handle(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if repo.created != nil {
		t.Error("invalid command must not reach repo.Create")
	}
}

func TestUpdateHandler_InvalidatesCache(t *testing.T) {
	existing := buildDash(t, "EBITDA")
	repo := &fakeRepo{byID: existing}
	cache := &spyCache{}
	h := dashboardapp.NewUpdateHandler(repo, cache)

	newTitle := "Updated Title"
	_, err := h.Handle(context.Background(), dashboardapp.UpdateCommand{
		ID:        existing.ID(),
		Title:     &newTitle,
		UpdatedBy: uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo.updated == nil || repo.updated.Title() != "Updated Title" {
		t.Errorf("update not persisted: %+v", repo.updated)
	}
	if len(cache.invalidated) != 1 || cache.invalidated[0] != "EBITDA" {
		t.Errorf("expected cache invalidation for EBITDA, got %v", cache.invalidated)
	}
}

func TestDeleteHandler_InvalidatesCache(t *testing.T) {
	existing := buildDash(t, "EBITDA")
	repo := &fakeRepo{byID: existing}
	cache := &spyCache{}
	h := dashboardapp.NewDeleteHandler(repo, cache)

	err := h.Handle(context.Background(), dashboardapp.DeleteCommand{ID: existing.ID(), DeletedBy: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	if repo.deletedID != existing.ID() {
		t.Error("SoftDelete not called with correct ID")
	}
	if len(cache.invalidated) != 1 {
		t.Errorf("expected 1 cache invalidation, got %v", cache.invalidated)
	}
}

func TestDeleteHandler_NotFound(t *testing.T) {
	repo := &fakeRepo{} // byID nil → ErrNotFound
	h := dashboardapp.NewDeleteHandler(repo, &spyCache{})
	err := h.Handle(context.Background(), dashboardapp.DeleteCommand{ID: uuid.New()})
	if !errors.Is(err, dashboarddomain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSetRolesHandler(t *testing.T) {
	existing := buildDash(t, "EBITDA")
	repo := &fakeRepo{byID: existing}
	cache := &spyCache{}
	h := dashboardapp.NewSetRolesHandler(repo, cache)
	roles, err := h.Handle(context.Background(), dashboardapp.SetRolesCommand{
		DashboardID: existing.ID(),
		RoleCodes:   []string{"CFO", "FINANCE_MGR"},
		UpdatedBy:   uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 2 {
		t.Errorf("expected 2 roles, got %v", roles)
	}
	if len(cache.invalidated) != 1 {
		t.Errorf("set roles should invalidate cache, got %v", cache.invalidated)
	}
}

func buildDash(t *testing.T, code string) *dashboarddomain.Dashboard {
	t.Helper()
	d, err := dashboarddomain.NewDashboard(dashboarddomain.NewDashboardParams{
		Code:           code,
		Title:          "T",
		FilterType:     "MIS",
		PeriodGrain:    "MONTHLY",
		DefaultPeriod:  "L12M",
		ChartType:      "bar",
		ChartConfigRaw: map[string]any{"x_axis_field": "group_1", "y_axis_field": "value"},
		MaxDrillLevel:  1,
		CacheTTLSec:    60,
		GroupID:        uuid.New(),
		IsActive:       true,
		CreatedBy:      uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}
