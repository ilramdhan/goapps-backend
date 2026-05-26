package dashboard

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Dashboard is the BI Dashboard aggregate root.
//
// All fields are private; construction is via NewDashboard and mutation is via Update.
// Read-only accessors are provided per field. Soft delete is via SoftDelete.
type Dashboard struct {
	id                 uuid.UUID
	code               Code
	title              string
	description        string
	filterType         string
	filterGroup1       string
	periodGrain        PeriodGrain
	defaultPeriod      DefaultPeriod
	chartType          ChartType
	chartConfig        ChartConfig
	layoutConfig       map[string]any
	kpiConfig          KpiConfig
	compareModes       CompareModes
	drillEnabled       bool
	maxDrillLevel      MaxDrillLevel
	cacheTTL           CacheTTL
	refreshInterval    RefreshInterval
	displayOrder       int
	groupID            uuid.UUID
	isActive           bool
	allowedRoleCodes   []string
	createdAt          time.Time
	createdBy          uuid.UUID
	updatedAt          time.Time
	updatedBy          uuid.UUID
	deletedAt          time.Time
	deletedBy          uuid.UUID
}

// NewDashboardParams are the inputs to NewDashboard.
//
// All raw values (code, chart_type, period_grain, ...) are validated; pre-validated
// value objects are not accepted to keep the public surface unambiguous.
type NewDashboardParams struct {
	ID                  uuid.UUID
	Code                string
	Title               string
	Description         string
	FilterType          string
	FilterGroup1        string
	PeriodGrain         string
	DefaultPeriod       string
	ChartType           string
	ChartConfigRaw      map[string]any
	LayoutConfigRaw     map[string]any
	KpiConfigRaw        []map[string]any
	CompareModes        []string
	DrillEnabled        bool
	MaxDrillLevel       int
	CacheTTLSec         int
	RefreshIntervalSec  int
	DisplayOrder        int
	GroupID             uuid.UUID
	IsActive            bool
	AllowedRoleCodes    []string
	CreatedBy           uuid.UUID
}

// NewDashboard validates the inputs and constructs a Dashboard.
//
// Returns wrapped sentinel errors (ErrInvalidCode, ErrInvalidChartType, ...) on validation failure.
func NewDashboard(p NewDashboardParams) (*Dashboard, error) {
	code, err := NewCode(p.Code)
	if err != nil {
		return nil, err
	}

	title := strings.TrimSpace(p.Title)
	if title == "" || len(title) > 200 {
		return nil, fmt.Errorf("%w: title must be 1..200 chars, got %d", ErrInvalidTitle, len(title))
	}

	if len(p.Description) > 1000 {
		return nil, fmt.Errorf("%w: description must be <= 1000 chars, got %d", ErrInvalidTitle, len(p.Description))
	}

	if strings.TrimSpace(p.FilterType) == "" {
		return nil, fmt.Errorf("%w: filter_type must not be empty", ErrInvalidChartConfig)
	}

	grain, err := NewPeriodGrain(p.PeriodGrain)
	if err != nil {
		return nil, err
	}

	defaultPeriod, err := NewDefaultPeriod(p.DefaultPeriod)
	if err != nil {
		return nil, err
	}

	chartType, err := NewChartType(p.ChartType)
	if err != nil {
		return nil, err
	}

	chartConfig, err := ParseChartConfig(chartType, p.ChartConfigRaw)
	if err != nil {
		return nil, err
	}

	kpiConfig, err := ParseKpiConfig(p.KpiConfigRaw)
	if err != nil {
		return nil, err
	}

	compareModes, err := NewCompareModes(p.CompareModes)
	if err != nil {
		return nil, err
	}

	maxDrill, err := NewMaxDrillLevel(p.MaxDrillLevel)
	if err != nil {
		return nil, err
	}

	cacheTTL, err := NewCacheTTL(p.CacheTTLSec)
	if err != nil {
		return nil, err
	}

	refresh, err := NewRefreshInterval(p.RefreshIntervalSec)
	if err != nil {
		return nil, err
	}

	if p.GroupID == uuid.Nil {
		return nil, fmt.Errorf("%w: group_id is required", ErrInvalidChartConfig)
	}

	id := p.ID
	if id == uuid.Nil {
		id = uuid.New()
	}

	roleCodes := dedupRoles(p.AllowedRoleCodes)

	return &Dashboard{
		id:               id,
		code:             code,
		title:            title,
		description:      p.Description,
		filterType:       p.FilterType,
		filterGroup1:     p.FilterGroup1,
		periodGrain:      grain,
		defaultPeriod:    defaultPeriod,
		chartType:        chartType,
		chartConfig:      chartConfig,
		layoutConfig:     copyMap(p.LayoutConfigRaw),
		kpiConfig:        kpiConfig,
		compareModes:     compareModes,
		drillEnabled:     p.DrillEnabled,
		maxDrillLevel:    maxDrill,
		cacheTTL:         cacheTTL,
		refreshInterval:  refresh,
		displayOrder:     p.DisplayOrder,
		groupID:          p.GroupID,
		isActive:         p.IsActive,
		allowedRoleCodes: roleCodes,
		createdAt:        time.Now().UTC(),
		createdBy:        p.CreatedBy,
	}, nil
}

// UpdateParams are the optional fields that can be changed via Update.
//
// Nil pointer fields are left untouched. The dashboard_code is intentionally NOT
// included — codes are immutable once a dashboard is created (use Duplicate to fork).
type UpdateParams struct {
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
	AllowedRoleCodes   []string
	UpdatedBy          uuid.UUID
}

// Update applies the params to this dashboard with validation.
//
// On the first validation failure the dashboard is left unchanged. The chart_type
// field controls which chart_config validation is run; if ChartType is being changed
// AND ChartConfigRaw is provided, the new config is validated against the new type;
// if only ChartType is changed, the existing chart_config is re-validated against it.
func (d *Dashboard) Update(p UpdateParams) error {
	staged := *d // shallow copy as a transactional buffer

	if p.Title != nil {
		t := strings.TrimSpace(*p.Title)
		if t == "" || len(t) > 200 {
			return fmt.Errorf("%w: title must be 1..200 chars, got %d", ErrInvalidTitle, len(t))
		}
		staged.title = t
	}
	if p.Description != nil {
		if len(*p.Description) > 1000 {
			return fmt.Errorf("%w: description must be <= 1000 chars", ErrInvalidTitle)
		}
		staged.description = *p.Description
	}
	if p.FilterType != nil {
		if strings.TrimSpace(*p.FilterType) == "" {
			return fmt.Errorf("%w: filter_type must not be empty", ErrInvalidChartConfig)
		}
		staged.filterType = *p.FilterType
	}
	if p.FilterGroup1 != nil {
		staged.filterGroup1 = *p.FilterGroup1
	}
	if p.PeriodGrain != nil {
		grain, err := NewPeriodGrain(*p.PeriodGrain)
		if err != nil {
			return err
		}
		staged.periodGrain = grain
	}
	if p.DefaultPeriod != nil {
		dp, err := NewDefaultPeriod(*p.DefaultPeriod)
		if err != nil {
			return err
		}
		staged.defaultPeriod = dp
	}

	chartTypeChanged := false
	if p.ChartType != nil {
		ct, err := NewChartType(*p.ChartType)
		if err != nil {
			return err
		}
		staged.chartType = ct
		chartTypeChanged = true
	}

	if p.ChartConfigRaw != nil {
		cfg, err := ParseChartConfig(staged.chartType, p.ChartConfigRaw)
		if err != nil {
			return err
		}
		staged.chartConfig = cfg
	} else if chartTypeChanged {
		// re-validate existing config against new type
		cfg, err := ParseChartConfig(staged.chartType, staged.chartConfig.MarshalToMap())
		if err != nil {
			return err
		}
		staged.chartConfig = cfg
	}

	if p.LayoutConfigRaw != nil {
		staged.layoutConfig = copyMap(p.LayoutConfigRaw)
	}

	if p.KpiConfigRaw != nil {
		kc, err := ParseKpiConfig(p.KpiConfigRaw)
		if err != nil {
			return err
		}
		staged.kpiConfig = kc
	}

	if p.CompareModes != nil {
		cm, err := NewCompareModes(p.CompareModes)
		if err != nil {
			return err
		}
		staged.compareModes = cm
	}

	if p.DrillEnabled != nil {
		staged.drillEnabled = *p.DrillEnabled
	}
	if p.MaxDrillLevel != nil {
		m, err := NewMaxDrillLevel(*p.MaxDrillLevel)
		if err != nil {
			return err
		}
		staged.maxDrillLevel = m
	}
	if p.CacheTTLSec != nil {
		c, err := NewCacheTTL(*p.CacheTTLSec)
		if err != nil {
			return err
		}
		staged.cacheTTL = c
	}
	if p.RefreshIntervalSec != nil {
		r, err := NewRefreshInterval(*p.RefreshIntervalSec)
		if err != nil {
			return err
		}
		staged.refreshInterval = r
	}
	if p.DisplayOrder != nil {
		staged.displayOrder = *p.DisplayOrder
	}
	if p.GroupID != nil {
		if *p.GroupID == uuid.Nil {
			return fmt.Errorf("%w: group_id must not be empty", ErrInvalidChartConfig)
		}
		staged.groupID = *p.GroupID
	}
	if p.IsActive != nil {
		staged.isActive = *p.IsActive
	}
	if p.AllowedRoleCodes != nil {
		staged.allowedRoleCodes = dedupRoles(p.AllowedRoleCodes)
	}

	staged.updatedAt = time.Now().UTC()
	staged.updatedBy = p.UpdatedBy
	*d = staged
	return nil
}

// SoftDelete marks the dashboard as soft-deleted.
func (d *Dashboard) SoftDelete(by uuid.UUID) {
	now := time.Now().UTC()
	d.deletedAt = now
	d.deletedBy = by
	d.isActive = false
	d.updatedAt = now
	d.updatedBy = by
}

// IsDeleted reports whether SoftDelete has been called.
func (d *Dashboard) IsDeleted() bool { return !d.deletedAt.IsZero() }

// Getters
func (d *Dashboard) ID() uuid.UUID                  { return d.id }
func (d *Dashboard) Code() Code                     { return d.code }
func (d *Dashboard) Title() string                  { return d.title }
func (d *Dashboard) Description() string            { return d.description }
func (d *Dashboard) FilterType() string             { return d.filterType }
func (d *Dashboard) FilterGroup1() string           { return d.filterGroup1 }
func (d *Dashboard) PeriodGrain() PeriodGrain       { return d.periodGrain }
func (d *Dashboard) DefaultPeriod() DefaultPeriod   { return d.defaultPeriod }
func (d *Dashboard) ChartType() ChartType           { return d.chartType }
func (d *Dashboard) ChartConfig() ChartConfig       { return d.chartConfig }
func (d *Dashboard) LayoutConfig() map[string]any   { return copyMap(d.layoutConfig) }
func (d *Dashboard) KpiConfig() KpiConfig           { return d.kpiConfig }
func (d *Dashboard) CompareModes() CompareModes     { return d.compareModes }
func (d *Dashboard) DrillEnabled() bool             { return d.drillEnabled }
func (d *Dashboard) MaxDrillLevel() MaxDrillLevel   { return d.maxDrillLevel }
func (d *Dashboard) CacheTTL() CacheTTL             { return d.cacheTTL }
func (d *Dashboard) RefreshInterval() RefreshInterval { return d.refreshInterval }
func (d *Dashboard) DisplayOrder() int              { return d.displayOrder }
func (d *Dashboard) GroupID() uuid.UUID             { return d.groupID }
func (d *Dashboard) IsActive() bool                 { return d.isActive }
func (d *Dashboard) AllowedRoleCodes() []string {
	out := make([]string, len(d.allowedRoleCodes))
	copy(out, d.allowedRoleCodes)
	return out
}
func (d *Dashboard) CreatedAt() time.Time { return d.createdAt }
func (d *Dashboard) CreatedBy() uuid.UUID { return d.createdBy }
func (d *Dashboard) UpdatedAt() time.Time { return d.updatedAt }
func (d *Dashboard) UpdatedBy() uuid.UUID { return d.updatedBy }
func (d *Dashboard) DeletedAt() time.Time { return d.deletedAt }
func (d *Dashboard) DeletedBy() uuid.UUID { return d.deletedBy }

// SetAuditFromHydration restores audit fields when loading an existing row from the database.
// Repositories use this to bypass the constructor's "set createdAt to now" behaviour.
func (d *Dashboard) SetAuditFromHydration(createdAt, updatedAt, deletedAt time.Time, createdBy, updatedBy, deletedBy uuid.UUID) {
	d.createdAt = createdAt
	d.updatedAt = updatedAt
	d.deletedAt = deletedAt
	d.createdBy = createdBy
	d.updatedBy = updatedBy
	d.deletedBy = deletedBy
}

// SetAllowedRoleCodesFromHydration restores the role mapping loaded from bi_dashboard_role.
func (d *Dashboard) SetAllowedRoleCodesFromHydration(roles []string) {
	d.allowedRoleCodes = dedupRoles(roles)
}

// IsAccessibleBy reports whether a user with the given roles (and super-admin flag) can view this dashboard.
//
// Rules:
//   - if no role codes are mapped → open to anyone with the view permission
//   - if role codes are mapped → user must have at least one matching role OR be super-admin
func (d *Dashboard) IsAccessibleBy(userRoles []string, isSuperAdmin bool) bool {
	if isSuperAdmin {
		return true
	}
	if len(d.allowedRoleCodes) == 0 {
		return true
	}
	for _, ur := range userRoles {
		if slices.Contains(d.allowedRoleCodes, ur) {
			return true
		}
	}
	return false
}

// dedupRoles returns a fresh slice with duplicates removed, preserving order.
func dedupRoles(roles []string) []string {
	if len(roles) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(roles))
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if _, dup := seen[r]; dup {
			continue
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	return out
}

// copyMap returns a shallow copy of a JSON-map; nil input returns nil.
func copyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	maps.Copy(out, m)
	return out
}
