package dashboard_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
)

func validParams() dashboard.NewDashboardParams {
	return dashboard.NewDashboardParams{
		Code:               "EBITDA",
		Title:              "EBITDA Performance",
		Description:        "Earnings before interest, tax, depreciation & amortization",
		FilterType:         "MIS",
		FilterGroup1:       "EBITDA",
		PeriodGrain:        "MONTHLY",
		DefaultPeriod:      "L12M",
		ChartType:          "waterfall",
		ChartConfigRaw:     map[string]any{"x_axis_field": "group_2", "y_axis_field": "display_value"},
		CompareModes:       []string{"MoM", "YoY"},
		DrillEnabled:       true,
		MaxDrillLevel:      3,
		CacheTTLSec:        1800,
		RefreshIntervalSec: 0,
		DisplayOrder:       10,
		GroupID:            uuid.New(),
		IsActive:           true,
		AllowedRoleCodes:   []string{"CFO", "FINANCE_MGR", "CFO"}, // duplicate intentional
		CreatedBy:          uuid.New(),
	}
}

func TestNewDashboard_HappyPath(t *testing.T) {
	p := validParams()
	d, err := dashboard.NewDashboard(p)
	if err != nil {
		t.Fatal(err)
	}
	if d.Code().String() != "EBITDA" {
		t.Errorf("code: want EBITDA, got %q", d.Code().String())
	}
	if d.Title() != "EBITDA Performance" {
		t.Errorf("title mismatch: %q", d.Title())
	}
	if !d.DrillEnabled() {
		t.Error("drill enabled lost")
	}
	if d.MaxDrillLevel().Int() != 3 {
		t.Errorf("max drill level: want 3, got %d", d.MaxDrillLevel().Int())
	}
	if d.CacheTTL().Seconds() != 1800 {
		t.Errorf("cache ttl: %d", d.CacheTTL().Seconds())
	}
	// duplicates removed
	roles := d.AllowedRoleCodes()
	if len(roles) != 2 {
		t.Errorf("want 2 unique roles, got %d (%v)", len(roles), roles)
	}
	if d.IsDeleted() {
		t.Error("freshly constructed should not be deleted")
	}
}

func TestNewDashboard_InvalidCode(t *testing.T) {
	p := validParams()
	p.Code = "bad-code"
	_, err := dashboard.NewDashboard(p)
	if !errors.Is(err, dashboard.ErrInvalidCode) {
		t.Errorf("want ErrInvalidCode, got %v", err)
	}
}

func TestNewDashboard_EmptyTitle(t *testing.T) {
	p := validParams()
	p.Title = "   "
	_, err := dashboard.NewDashboard(p)
	if !errors.Is(err, dashboard.ErrInvalidTitle) {
		t.Errorf("want ErrInvalidTitle, got %v", err)
	}
}

func TestNewDashboard_MissingChartConfigRequired(t *testing.T) {
	p := validParams()
	p.ChartConfigRaw = map[string]any{} // missing required x_axis_field/y_axis_field
	_, err := dashboard.NewDashboard(p)
	if !errors.Is(err, dashboard.ErrInvalidChartConfig) {
		t.Errorf("want ErrInvalidChartConfig, got %v", err)
	}
}

func TestNewDashboard_NilGroupID(t *testing.T) {
	p := validParams()
	p.GroupID = uuid.Nil
	_, err := dashboard.NewDashboard(p)
	if !errors.Is(err, dashboard.ErrInvalidChartConfig) {
		t.Errorf("want ErrInvalidChartConfig, got %v", err)
	}
}

func TestUpdate_AppliesPartial(t *testing.T) {
	p := validParams()
	d, _ := dashboard.NewDashboard(p)
	newTitle := "EBITDA Performance v2"
	newTTL := 600
	if err := d.Update(dashboard.UpdateParams{
		Title:       &newTitle,
		CacheTTLSec: &newTTL,
		UpdatedBy:   uuid.New(),
	}); err != nil {
		t.Fatal(err)
	}
	if d.Title() != newTitle {
		t.Errorf("title not updated: %q", d.Title())
	}
	if d.CacheTTL().Seconds() != 600 {
		t.Errorf("cache ttl not updated: %d", d.CacheTTL().Seconds())
	}
	// untouched
	if d.Code().String() != "EBITDA" {
		t.Errorf("code mutated: %q", d.Code().String())
	}
}

func TestUpdate_RejectsInvalidWithoutMutation(t *testing.T) {
	p := validParams()
	d, _ := dashboard.NewDashboard(p)
	originalTitle := d.Title()
	bad := -5
	err := d.Update(dashboard.UpdateParams{
		Title:       ptr("Some new title"),
		CacheTTLSec: &bad,
	})
	if !errors.Is(err, dashboard.ErrInvalidCacheTTL) {
		t.Errorf("want ErrInvalidCacheTTL, got %v", err)
	}
	// title must NOT be mutated despite being parsed before the bad TTL
	if d.Title() != originalTitle {
		t.Errorf("partial mutation leaked: %q", d.Title())
	}
}

func TestUpdate_ChartTypeChange_RevalidatesExistingConfig(t *testing.T) {
	p := validParams()
	d, _ := dashboard.NewDashboard(p)
	// switch to donut, which requires label_field/value_field — existing waterfall config has neither
	newType := "donut"
	err := d.Update(dashboard.UpdateParams{ChartType: &newType})
	if !errors.Is(err, dashboard.ErrInvalidChartConfig) {
		t.Errorf("want ErrInvalidChartConfig when new type has different required fields, got %v", err)
	}
}

func TestSoftDelete(t *testing.T) {
	p := validParams()
	d, _ := dashboard.NewDashboard(p)
	by := uuid.New()
	d.SoftDelete(by)
	if !d.IsDeleted() {
		t.Error("should be marked deleted")
	}
	if d.IsActive() {
		t.Error("should not be active after delete")
	}
	if d.DeletedBy() != by {
		t.Errorf("deletedBy: want %v, got %v", by, d.DeletedBy())
	}
}

func TestIsAccessibleBy(t *testing.T) {
	p := validParams()
	p.AllowedRoleCodes = []string{"CFO"}
	d, _ := dashboard.NewDashboard(p)

	if !d.IsAccessibleBy([]string{"CFO"}, false) {
		t.Error("CFO should access")
	}
	if d.IsAccessibleBy([]string{"INTERN"}, false) {
		t.Error("INTERN should NOT access")
	}
	if !d.IsAccessibleBy([]string{"INTERN"}, true) {
		t.Error("super-admin must bypass")
	}

	// when no whitelist, open to anyone view-permitted
	p.AllowedRoleCodes = nil
	d2, _ := dashboard.NewDashboard(p)
	if !d2.IsAccessibleBy([]string{"GUEST"}, false) {
		t.Error("open dashboard should be accessible by any view-permitted user")
	}
}

func ptr[T any](v T) *T { return &v }
