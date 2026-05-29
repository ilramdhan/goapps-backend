package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
)

// BiDashboardRepository implements dashboard.Repository using PostgreSQL.
type BiDashboardRepository struct {
	db *DB
}

// NewBiDashboardRepository constructs a BiDashboardRepository.
func NewBiDashboardRepository(db *DB) *BiDashboardRepository {
	return &BiDashboardRepository{db: db}
}

// Verify interface implementation at compile time.
var _ dashboard.Repository = (*BiDashboardRepository)(nil)

// colDisplayOrder is the SQL column name for dashboard display ordering.
const colDisplayOrder = "display_order"

// Create inserts a new dashboard row and its role mapping in a single transaction.
func (r *BiDashboardRepository) Create(ctx context.Context, d *dashboard.Dashboard) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			_ = err
		}
	}()

	if err := r.insertDashboardTx(ctx, tx, d); err != nil {
		return err
	}
	if err := r.replaceRolesTx(ctx, tx, d.ID(), d.AllowedRoleCodes(), d.CreatedBy()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// insertDashboardTx writes the bi_dashboard row inside an open transaction.
func (r *BiDashboardRepository) insertDashboardTx(ctx context.Context, tx *sql.Tx, d *dashboard.Dashboard) error {
	chartConfig, err := json.Marshal(d.ChartConfig().MarshalToMap())
	if err != nil {
		return fmt.Errorf("marshal chart_config: %w", err)
	}
	var layoutConfig []byte
	if lc := d.LayoutConfig(); lc != nil {
		layoutConfig, err = json.Marshal(lc)
		if err != nil {
			return fmt.Errorf("marshal layout_config: %w", err)
		}
	}
	kpiConfig, err := json.Marshal(d.KpiConfig().MarshalToList())
	if err != nil {
		return fmt.Errorf("marshal kpi_config: %w", err)
	}
	compareModes, err := json.Marshal(d.CompareModes().Strings())
	if err != nil {
		return fmt.Errorf("marshal compare_modes: %w", err)
	}

	const q = `
INSERT INTO bi_dashboard (
    dashboard_id, dashboard_code, dashboard_title, description,
    filter_type, filter_group_1, periode_grain, default_period,
    chart_type, chart_config, layout_config, compare_modes, kpi_config,
    drill_enabled, max_drill_level, cache_ttl_sec, refresh_interval_sec,
    display_order, group_id, is_active, is_featured, feature_order,
    created_at, created_by
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)`
	_, err = tx.ExecContext(ctx, q,
		d.ID(), d.Code().String(), d.Title(), d.Description(),
		d.FilterType(), biNullableString(d.FilterGroup1()), d.PeriodGrain().String(), d.DefaultPeriod().String(),
		d.ChartType().String(), chartConfig, nullableBytes(layoutConfig), compareModes, kpiConfig,
		d.DrillEnabled(), d.MaxDrillLevel().Int(), d.CacheTTL().Seconds(), d.RefreshInterval().Seconds(),
		d.DisplayOrder(), d.GroupID(), d.IsActive(), d.IsFeatured(), d.FeatureOrder(),
		d.CreatedAt(), nullableUUID(d.CreatedBy()),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return dashboard.ErrAlreadyExists
		}
		return fmt.Errorf("insert bi_dashboard: %w", err)
	}
	return nil
}

// replaceRolesTx wipes and re-inserts the role mapping for a dashboard inside a transaction.
func (r *BiDashboardRepository) replaceRolesTx(ctx context.Context, tx *sql.Tx, dashboardID uuid.UUID, roles []string, by uuid.UUID) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM bi_dashboard_role WHERE dashboard_id = $1", dashboardID); err != nil {
		return fmt.Errorf("delete roles: %w", err)
	}
	if len(roles) == 0 {
		return nil
	}
	const q = `INSERT INTO bi_dashboard_role (dashboard_id, role_code, created_by) VALUES ($1, $2, $3)`
	stmt, err := tx.PrepareContext(ctx, q)
	if err != nil {
		return fmt.Errorf("prepare roles insert: %w", err)
	}
	defer func() {
		if err := stmt.Close(); err != nil {
			_ = err
		}
	}()
	for _, code := range roles {
		if _, err := stmt.ExecContext(ctx, dashboardID, code, nullableUUID(by)); err != nil {
			return fmt.Errorf("insert role %q: %w", code, err)
		}
	}
	return nil
}

// GetByID returns the dashboard (and its roles) by primary key.
func (r *BiDashboardRepository) GetByID(ctx context.Context, id uuid.UUID) (*dashboard.Dashboard, error) {
	row := r.db.QueryRowContext(ctx, selectDashboardByID, id)
	d, err := r.scanDashboard(ctx, row.Scan)
	if err != nil {
		return nil, err
	}
	roles, err := r.GetRoles(ctx, d.ID())
	if err != nil {
		return nil, err
	}
	d.SetAllowedRoleCodesFromHydration(roles)
	return d, nil
}

// GetByCode returns the dashboard by its business code.
func (r *BiDashboardRepository) GetByCode(ctx context.Context, code string) (*dashboard.Dashboard, error) {
	row := r.db.QueryRowContext(ctx, selectDashboardByCode, code)
	d, err := r.scanDashboard(ctx, row.Scan)
	if err != nil {
		return nil, err
	}
	roles, err := r.GetRoles(ctx, d.ID())
	if err != nil {
		return nil, err
	}
	d.SetAllowedRoleCodesFromHydration(roles)
	return d, nil
}

// List paginated dashboards with filter + sort.
//
//nolint:gocyclo // dynamic filter/sort query builder; each filter condition adds one branch
func (r *BiDashboardRepository) List(ctx context.Context, f dashboard.ListFilter) ([]*dashboard.Dashboard, int64, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}

	var conditions []string
	var args []any
	idx := 1

	conditions = append(conditions, "deleted_at IS NULL")
	if !f.IncludeInactive {
		conditions = append(conditions, "is_active = TRUE")
	}
	if f.Search != "" {
		conditions = append(conditions, fmt.Sprintf("(dashboard_code ILIKE $%d OR dashboard_title ILIKE $%d)", idx, idx))
		args = append(args, "%"+f.Search+"%")
		idx++
	}
	if f.GroupID != nil {
		conditions = append(conditions, fmt.Sprintf("group_id = $%d", idx))
		args = append(args, *f.GroupID)
		idx++
	}
	if f.FilterType != "" {
		conditions = append(conditions, fmt.Sprintf("filter_type = $%d", idx))
		args = append(args, f.FilterType)
		idx++
	}

	where := "WHERE " + strings.Join(conditions, " AND ")

	// Count
	var total int64
	countQ := "SELECT COUNT(*) FROM bi_dashboard " + where
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count: %w", err)
	}

	// Sort & paginate
	sortCol := mapDashboardSortField(f.SortField)
	sortDir := sortASC
	if strings.EqualFold(f.SortDir, "desc") {
		sortDir = sortDESC
	}
	limit := f.PageSize
	offset := (f.Page - 1) * f.PageSize

	q := selectDashboardBase + " " + where +
		fmt.Sprintf(" ORDER BY %s %s, display_order ASC, dashboard_code ASC LIMIT $%d OFFSET $%d",
			sortCol, sortDir, idx, idx+1)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query list: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			_ = err
		}
	}()

	var out []*dashboard.Dashboard
	for rows.Next() {
		d, err := r.scanDashboard(ctx, rows.Scan)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows err: %w", err)
	}

	// Hydrate roles for the page in one round trip
	if err := r.hydrateRolesBatch(ctx, out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// Update mutates the dashboard row + replaces role mapping.
func (r *BiDashboardRepository) Update(ctx context.Context, d *dashboard.Dashboard) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			_ = err
		}
	}()

	chartConfig, err := json.Marshal(d.ChartConfig().MarshalToMap())
	if err != nil {
		return fmt.Errorf("marshal chart_config: %w", err)
	}
	var layoutConfig []byte
	if lc := d.LayoutConfig(); lc != nil {
		layoutConfig, err = json.Marshal(lc)
		if err != nil {
			return fmt.Errorf("marshal layout_config: %w", err)
		}
	}
	kpiConfig, err := json.Marshal(d.KpiConfig().MarshalToList())
	if err != nil {
		return fmt.Errorf("marshal kpi_config: %w", err)
	}
	compareModes, err := json.Marshal(d.CompareModes().Strings())
	if err != nil {
		return fmt.Errorf("marshal compare_modes: %w", err)
	}

	const q = `
UPDATE bi_dashboard SET
    dashboard_title = $2,
    description = $3,
    filter_type = $4,
    filter_group_1 = $5,
    periode_grain = $6,
    default_period = $7,
    chart_type = $8,
    chart_config = $9,
    layout_config = $10,
    compare_modes = $11,
    kpi_config = $12,
    drill_enabled = $13,
    max_drill_level = $14,
    cache_ttl_sec = $15,
    refresh_interval_sec = $16,
    display_order = $17,
    group_id = $18,
    is_active = $19,
    is_featured = $20,
    feature_order = $21,
    updated_at = $22,
    updated_by = $23
WHERE dashboard_id = $1 AND deleted_at IS NULL`
	res, err := tx.ExecContext(ctx, q,
		d.ID(), d.Title(), d.Description(),
		d.FilterType(), biNullableString(d.FilterGroup1()), d.PeriodGrain().String(), d.DefaultPeriod().String(),
		d.ChartType().String(), chartConfig, nullableBytes(layoutConfig), compareModes, kpiConfig,
		d.DrillEnabled(), d.MaxDrillLevel().Int(), d.CacheTTL().Seconds(), d.RefreshInterval().Seconds(),
		d.DisplayOrder(), d.GroupID(), d.IsActive(), d.IsFeatured(), d.FeatureOrder(),
		d.UpdatedAt(), nullableUUID(d.UpdatedBy()),
	)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return dashboard.ErrNotFound
	}

	if err := r.replaceRolesTx(ctx, tx, d.ID(), d.AllowedRoleCodes(), d.UpdatedBy()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// SoftDelete sets deleted_at, deleted_by, and is_active=false.
func (r *BiDashboardRepository) SoftDelete(ctx context.Context, id uuid.UUID, by uuid.UUID) error {
	const q = `
UPDATE bi_dashboard
SET deleted_at = NOW(), deleted_by = $2, is_active = FALSE, updated_at = NOW(), updated_by = $2
WHERE dashboard_id = $1 AND deleted_at IS NULL`
	res, err := r.db.ExecContext(ctx, q, id, nullableUUID(by))
	if err != nil {
		return fmt.Errorf("soft delete: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return dashboard.ErrNotFound
	}
	return nil
}

// Duplicate clones a dashboard with a new code/title + fresh role mapping.
func (r *BiDashboardRepository) Duplicate(ctx context.Context, sourceID uuid.UUID, newCode, newTitle string, by uuid.UUID) (*dashboard.Dashboard, error) {
	src, err := r.GetByID(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	clone, err := dashboard.NewDashboard(dashboard.NewDashboardParams{
		Code:               newCode,
		Title:              newTitle,
		Description:        src.Description(),
		FilterType:         src.FilterType(),
		FilterGroup1:       src.FilterGroup1(),
		PeriodGrain:        src.PeriodGrain().String(),
		DefaultPeriod:      src.DefaultPeriod().String(),
		ChartType:          src.ChartType().String(),
		ChartConfigRaw:     src.ChartConfig().MarshalToMap(),
		LayoutConfigRaw:    src.LayoutConfig(),
		KpiConfigRaw:       src.KpiConfig().MarshalToList(),
		CompareModes:       src.CompareModes().Strings(),
		DrillEnabled:       src.DrillEnabled(),
		MaxDrillLevel:      src.MaxDrillLevel().Int(),
		CacheTTLSec:        src.CacheTTL().Seconds(),
		RefreshIntervalSec: src.RefreshInterval().Seconds(),
		DisplayOrder:       src.DisplayOrder() + 1,
		GroupID:            src.GroupID(),
		IsActive:           src.IsActive(),
		AllowedRoleCodes:   src.AllowedRoleCodes(),
		CreatedBy:          by,
	})
	if err != nil {
		return nil, err
	}
	if err := r.Create(ctx, clone); err != nil {
		return nil, err
	}
	return clone, nil
}

// SetRoles overwrites the dashboard's role whitelist.
func (r *BiDashboardRepository) SetRoles(ctx context.Context, dashboardID uuid.UUID, roleCodes []string, by uuid.UUID) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			_ = err
		}
	}()
	if err := r.replaceRolesTx(ctx, tx, dashboardID, roleCodes, by); err != nil {
		return err
	}
	return tx.Commit()
}

// GetRoles returns the current whitelist for a dashboard.
func (r *BiDashboardRepository) GetRoles(ctx context.Context, dashboardID uuid.UUID) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT role_code FROM bi_dashboard_role WHERE dashboard_id = $1 ORDER BY role_code", dashboardID)
	if err != nil {
		return nil, fmt.Errorf("query roles: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			_ = err
		}
	}()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListAccessible returns active dashboards the user can see.
//
// Rule: (no role rows exist for dashboard OR user has at least one matching role
// OR isSuperAdmin) AND is_active AND deleted_at IS NULL.
func (r *BiDashboardRepository) ListAccessible(ctx context.Context, userRoles []string, isSuperAdmin bool) ([]*dashboard.Dashboard, error) {
	const baseWhere = "deleted_at IS NULL AND is_active = TRUE"
	var q string
	var args []any
	if isSuperAdmin {
		q = selectDashboardBase + " WHERE " + baseWhere + " ORDER BY display_order ASC, dashboard_code ASC"
	} else {
		q = selectDashboardBase + " WHERE " + baseWhere + ` AND (
            NOT EXISTS (SELECT 1 FROM bi_dashboard_role WHERE dashboard_id = bi_dashboard.dashboard_id)
            OR EXISTS (SELECT 1 FROM bi_dashboard_role WHERE dashboard_id = bi_dashboard.dashboard_id AND role_code = ANY($1))
        ) ORDER BY display_order ASC, dashboard_code ASC`
		args = append(args, pq.Array(userRoles))
	}

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query accessible: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			_ = err
		}
	}()

	var out []*dashboard.Dashboard
	for rows.Next() {
		d, err := r.scanDashboard(ctx, rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows err: %w", err)
	}
	if err := r.hydrateRolesBatch(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListFeatured returns active dashboards pinned to the Executive Dashboard landing page.
func (r *BiDashboardRepository) ListFeatured(ctx context.Context) ([]*dashboard.Dashboard, error) {
	const q = selectDashboardBase + ` WHERE deleted_at IS NULL AND is_active = TRUE AND is_featured = TRUE
ORDER BY feature_order ASC, dashboard_code ASC`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query featured: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			_ = err
		}
	}()

	var out []*dashboard.Dashboard
	for rows.Next() {
		d, err := r.scanDashboard(ctx, rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows err: %w", err)
	}
	if err := r.hydrateRolesBatch(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ---- helpers ----

const selectDashboardBase = `
SELECT dashboard_id, dashboard_code, dashboard_title, description,
       filter_type, filter_group_1, periode_grain, default_period,
       chart_type, chart_config, layout_config, compare_modes, kpi_config,
       drill_enabled, max_drill_level, cache_ttl_sec, refresh_interval_sec,
       display_order, group_id, is_active, is_featured, feature_order,
       created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
FROM bi_dashboard`

const selectDashboardByID = selectDashboardBase + " WHERE dashboard_id = $1 AND deleted_at IS NULL"
const selectDashboardByCode = selectDashboardBase + " WHERE dashboard_code = $1 AND deleted_at IS NULL"

type scanFunc func(dest ...any) error

// dashboardRow is the row buffer used by scanDashboard.
type dashboardRow struct {
	ID                 uuid.UUID
	Code               string
	Title              string
	Description        sql.NullString
	FilterType         string
	FilterGroup1       sql.NullString
	PeriodGrain        string
	DefaultPeriod      string
	ChartType          string
	ChartConfig        []byte
	LayoutConfig       []byte
	CompareModes       []byte
	KpiConfig          []byte
	DrillEnabled       bool
	MaxDrillLevel      int
	CacheTTLSec        int
	RefreshIntervalSec int
	DisplayOrder       int
	GroupID            uuid.UUID
	IsActive           bool
	IsFeatured         bool
	FeatureOrder       int
	CreatedAt          time.Time
	CreatedBy          uuid.NullUUID
	UpdatedAt          sql.NullTime
	UpdatedBy          uuid.NullUUID
	DeletedAt          sql.NullTime
	DeletedBy          uuid.NullUUID
}

// scanDashboard scans a single row into a domain entity. Roles are hydrated separately.
func (r *BiDashboardRepository) scanDashboard(_ context.Context, scan scanFunc) (*dashboard.Dashboard, error) {
	var row dashboardRow
	err := scan(
		&row.ID, &row.Code, &row.Title, &row.Description,
		&row.FilterType, &row.FilterGroup1, &row.PeriodGrain, &row.DefaultPeriod,
		&row.ChartType, &row.ChartConfig, &row.LayoutConfig, &row.CompareModes, &row.KpiConfig,
		&row.DrillEnabled, &row.MaxDrillLevel, &row.CacheTTLSec, &row.RefreshIntervalSec,
		&row.DisplayOrder, &row.GroupID, &row.IsActive, &row.IsFeatured, &row.FeatureOrder,
		&row.CreatedAt, &row.CreatedBy, &row.UpdatedAt, &row.UpdatedBy, &row.DeletedAt, &row.DeletedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, dashboard.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	chartConfigMap, err := bytesToMap(row.ChartConfig)
	if err != nil {
		return nil, fmt.Errorf("decode chart_config: %w", err)
	}
	layoutConfigMap, err := bytesToMap(row.LayoutConfig)
	if err != nil {
		return nil, fmt.Errorf("decode layout_config: %w", err)
	}
	compareModes, err := bytesToStringSlice(row.CompareModes)
	if err != nil {
		return nil, fmt.Errorf("decode compare_modes: %w", err)
	}
	kpiConfigList, err := bytesToMapList(row.KpiConfig)
	if err != nil {
		return nil, fmt.Errorf("decode kpi_config: %w", err)
	}

	d, err := dashboard.NewDashboard(dashboard.NewDashboardParams{
		ID:                 row.ID,
		Code:               row.Code,
		Title:              row.Title,
		Description:        nullToString(row.Description),
		FilterType:         row.FilterType,
		FilterGroup1:       nullToString(row.FilterGroup1),
		PeriodGrain:        row.PeriodGrain,
		DefaultPeriod:      row.DefaultPeriod,
		ChartType:          row.ChartType,
		ChartConfigRaw:     chartConfigMap,
		LayoutConfigRaw:    layoutConfigMap,
		KpiConfigRaw:       kpiConfigList,
		CompareModes:       compareModes,
		DrillEnabled:       row.DrillEnabled,
		MaxDrillLevel:      row.MaxDrillLevel,
		CacheTTLSec:        row.CacheTTLSec,
		RefreshIntervalSec: row.RefreshIntervalSec,
		DisplayOrder:       row.DisplayOrder,
		GroupID:            row.GroupID,
		IsActive:           row.IsActive,
		IsFeatured:         row.IsFeatured,
		FeatureOrder:       row.FeatureOrder,
		CreatedBy:          uuidOrNil(row.CreatedBy),
	})
	if err != nil {
		return nil, fmt.Errorf("reconstruct dashboard from db row: %w", err)
	}

	d.SetAuditFromHydration(
		row.CreatedAt,
		nullTimeOrZero(row.UpdatedAt),
		nullTimeOrZero(row.DeletedAt),
		uuidOrNil(row.CreatedBy),
		uuidOrNil(row.UpdatedBy),
		uuidOrNil(row.DeletedBy),
	)
	return d, nil
}

// hydrateRolesBatch fetches role codes for a page of dashboards in a single query.
func (r *BiDashboardRepository) hydrateRolesBatch(ctx context.Context, dashboards []*dashboard.Dashboard) error {
	if len(dashboards) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(dashboards))
	idx := make(map[uuid.UUID][]string, len(dashboards))
	for i, d := range dashboards {
		ids[i] = d.ID()
		idx[d.ID()] = nil
	}
	rows, err := r.db.QueryContext(ctx,
		"SELECT dashboard_id, role_code FROM bi_dashboard_role WHERE dashboard_id = ANY($1) ORDER BY role_code",
		pq.Array(ids))
	if err != nil {
		return fmt.Errorf("query roles batch: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			_ = err
		}
	}()
	for rows.Next() {
		var did uuid.UUID
		var code string
		if err := rows.Scan(&did, &code); err != nil {
			return fmt.Errorf("scan role row: %w", err)
		}
		idx[did] = append(idx[did], code)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows err: %w", err)
	}
	for _, d := range dashboards {
		d.SetAllowedRoleCodesFromHydration(idx[d.ID()])
	}
	return nil
}

// mapDashboardSortField translates a frontend sort field to a SQL column.
func mapDashboardSortField(field string) string {
	switch field {
	case "code":
		return "dashboard_code"
	case "title":
		return "dashboard_title"
	case "created_at":
		return "created_at"
	case colDisplayOrder, "":
		return colDisplayOrder
	}
	return colDisplayOrder
}

// ---- shared utility helpers (used by other bi_* repos as well) ----

func biNullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

func nullableUUID(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}

func nullToString(s sql.NullString) string {
	if s.Valid {
		return s.String
	}
	return ""
}

func nullTimeOrZero(t sql.NullTime) time.Time {
	if t.Valid {
		return t.Time
	}
	return time.Time{}
}

func uuidOrNil(n uuid.NullUUID) uuid.UUID {
	if n.Valid {
		return n.UUID
	}
	return uuid.Nil
}

// bytesToMap decodes a JSONB column into a Go map.
// Returns an empty map (not nil) for empty input to avoid nilnil.
func bytesToMap(b []byte) (map[string]any, error) {
	if len(b) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// bytesToStringSlice decodes a JSONB array of strings.
func bytesToStringSlice(b []byte) ([]string, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var s []string
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return s, nil
}

// bytesToMapList decodes a JSONB array of objects.
func bytesToMapList(b []byte) ([]map[string]any, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var l []map[string]any
	if err := json.Unmarshal(b, &l); err != nil {
		return nil, err
	}
	return l, nil
}
