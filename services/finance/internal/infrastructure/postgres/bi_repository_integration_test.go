// Package postgres — BI repository integration tests.
//
// Gated by INTEGRATION_TEST=true; requires a reachable PostgreSQL (defaults match the
// docker-compose finance DB). The suite creates the bi_* schema + materialized views +
// the sign-convention function inline, seeds DETERMINISTIC fact rows (no random()), and
// asserts Upsert, GetDistincts, QueryAggregate (via MV), and the dashboard CRUD roundtrip
// including role mapping + ListAccessible visibility rules.
package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

type BIRepositorySuite struct {
	suite.Suite
	db        *postgres.DB
	ctx       context.Context
	dashRepo  *postgres.BiDashboardRepository
	groupRepo *postgres.BiDashboardGroupRepository
	factRepo  *postgres.BiFactMetricRepository
	sourceID  uuid.UUID
	groupID   uuid.UUID
}

func TestBIRepositorySuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(BIRepositorySuite))
}

func (s *BIRepositorySuite) SetupSuite() {
	s.ctx = context.Background()
	host := getEnvOrDefault("TEST_DB_HOST", "localhost")
	port := getEnvOrDefault("TEST_DB_PORT", "5434")
	user := getEnvOrDefault("TEST_DB_USER", "finance")
	password := getEnvOrDefault("TEST_DB_PASSWORD", "finance123")
	dbname := getEnvOrDefault("TEST_DB_NAME", "finance_db")
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", host, port, user, password, dbname)

	db, err := sql.Open("postgres", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), waitForDB(db, 30*time.Second))
	s.db = &postgres.DB{DB: db}
	s.dashRepo = postgres.NewBiDashboardRepository(s.db)
	s.groupRepo = postgres.NewBiDashboardGroupRepository(s.db)
	s.factRepo = postgres.NewBiFactMetricRepository(s.db)
	s.setupSchema()
}

func (s *BIRepositorySuite) TearDownSuite() {
	if s.db != nil {
		_, _ = s.db.ExecContext(s.ctx, "DELETE FROM bi_fact_metric WHERE dimension_key = '__ITEST__'")
		_, _ = s.db.ExecContext(s.ctx, "DELETE FROM bi_dashboard WHERE dashboard_code LIKE 'ITEST%'")
		_, _ = s.db.ExecContext(s.ctx, "DELETE FROM bi_dashboard_group WHERE group_code LIKE 'ITEST%'")
		_, _ = s.db.ExecContext(s.ctx, "DELETE FROM bi_data_source WHERE source_code = 'ITEST_SRC'")
		s.db.Close()
	}
}

func (s *BIRepositorySuite) setupSchema() {
	// The migrations create these in production; for an isolated test DB we create-if-not-exists.
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS bi_data_source (
			source_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_code VARCHAR(40) UNIQUE NOT NULL, source_name VARCHAR(120) NOT NULL,
			source_type VARCHAR(20) NOT NULL, connection_info JSONB, description TEXT,
			is_active BOOLEAN NOT NULL DEFAULT TRUE, created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			created_by UUID, updated_at TIMESTAMP, updated_by UUID)`,
		`CREATE TABLE IF NOT EXISTS bi_dashboard_group (
			group_id UUID PRIMARY KEY DEFAULT gen_random_uuid(), group_code VARCHAR(40) UNIQUE NOT NULL,
			group_name VARCHAR(120) NOT NULL, description TEXT, icon VARCHAR(40),
			display_order INT NOT NULL DEFAULT 0, is_active BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(), created_by UUID, updated_at TIMESTAMP, updated_by UUID)`,
		`CREATE TABLE IF NOT EXISTS bi_dashboard (
			dashboard_id UUID PRIMARY KEY DEFAULT gen_random_uuid(), dashboard_code VARCHAR(60) UNIQUE NOT NULL,
			dashboard_title VARCHAR(200) NOT NULL, description TEXT, filter_type VARCHAR(40) NOT NULL,
			filter_group_1 VARCHAR(100), periode_grain VARCHAR(10) NOT NULL, default_period VARCHAR(20) NOT NULL DEFAULT 'L12M',
			chart_type VARCHAR(40) NOT NULL, chart_config JSONB NOT NULL DEFAULT '{}'::jsonb, layout_config JSONB,
			compare_modes JSONB NOT NULL DEFAULT '[]'::jsonb, kpi_config JSONB NOT NULL DEFAULT '[]'::jsonb,
			drill_enabled BOOLEAN NOT NULL DEFAULT TRUE, max_drill_level INT NOT NULL DEFAULT 3,
			cache_ttl_sec INT NOT NULL DEFAULT 1800, refresh_interval_sec INT NOT NULL DEFAULT 0,
			display_order INT NOT NULL DEFAULT 0, group_id UUID REFERENCES bi_dashboard_group(group_id),
			is_active BOOLEAN NOT NULL DEFAULT TRUE, created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			created_by UUID, updated_at TIMESTAMP, updated_by UUID, deleted_at TIMESTAMP, deleted_by UUID)`,
		`CREATE TABLE IF NOT EXISTS bi_dashboard_role (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			dashboard_id UUID NOT NULL REFERENCES bi_dashboard(dashboard_id) ON DELETE CASCADE,
			role_code VARCHAR(60) NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT NOW(), created_by UUID,
			CONSTRAINT uq_bi_dr_itest UNIQUE (dashboard_id, role_code))`,
		// Drop + recreate the fact table so the test always gets the current constraint
		// (NULLS NOT DISTINCT). bi_fact_metric in a test DB only holds test data, so this is safe.
		`DROP MATERIALIZED VIEW IF EXISTS mv_bi_metric_g2`,
		`DROP MATERIALIZED VIEW IF EXISTS mv_bi_metric_g1`,
		`DROP TABLE IF EXISTS bi_fact_metric`,
		`CREATE TABLE bi_fact_metric (
			metric_id BIGSERIAL PRIMARY KEY, type VARCHAR(40) NOT NULL, group_1 VARCHAR(120) NOT NULL,
			group_2 VARCHAR(120), group_3 VARCHAR(120), group_1_order INT, group_2_order INT, group_3_order INT,
			periode_grain VARCHAR(10) NOT NULL, periode_date DATE NOT NULL, periode_label VARCHAR(20) NOT NULL,
			value NUMERIC(20,4) NOT NULL, display_value NUMERIC(20,4) NOT NULL, uom VARCHAR(20),
			scenario VARCHAR(20) NOT NULL DEFAULT 'ACTUAL', source_id UUID NOT NULL REFERENCES bi_data_source(source_id), metric_name VARCHAR(50) NOT NULL DEFAULT 'VALUE', metric_category VARCHAR(20) NOT NULL DEFAULT 'VALUE', agg_method VARCHAR(20) NOT NULL DEFAULT 'SUM',
			dimension_key VARCHAR(200) NOT NULL DEFAULT '', uploaded_by UUID, loaded_at TIMESTAMP NOT NULL DEFAULT NOW(),
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			CONSTRAINT uq_bi_fm_bk_itest UNIQUE NULLS NOT DISTINCT (type, group_1, group_2, group_3, periode_grain, periode_date, metric_name, scenario, dimension_key))`,
		`CREATE MATERIALIZED VIEW IF NOT EXISTS mv_bi_metric_g1 AS
			SELECT type, group_1, periode_grain, periode_date, scenario, SUM(display_value) AS value,
			       MAX(group_1_order) AS group_1_order
			FROM bi_fact_metric WHERE is_active GROUP BY type, group_1, periode_grain, periode_date, scenario`,
		`CREATE MATERIALIZED VIEW IF NOT EXISTS mv_bi_metric_g2 AS
			SELECT type, group_1, group_2, periode_grain, periode_date, scenario, SUM(display_value) AS value,
			       MAX(group_2_order) AS group_2_order
			FROM bi_fact_metric WHERE is_active AND group_2 IS NOT NULL
			GROUP BY type, group_1, group_2, periode_grain, periode_date, scenario`,
	}
	for _, stmt := range stmts {
		_, err := s.db.ExecContext(s.ctx, stmt)
		n := min(len(stmt), 50)
		require.NoError(s.T(), err, "schema stmt: %s", stmt[:n])
	}

	// Seed a data source + group for FK references.
	s.sourceID = uuid.New()
	_, err := s.db.ExecContext(s.ctx,
		`INSERT INTO bi_data_source (source_id, source_code, source_name, source_type) VALUES ($1,'ITEST_SRC','itest','MANUAL')
		 ON CONFLICT (source_code) DO UPDATE SET source_name='itest' RETURNING source_id`,
		s.sourceID)
	require.NoError(s.T(), err)
	_ = s.db.QueryRowContext(s.ctx, "SELECT source_id FROM bi_data_source WHERE source_code='ITEST_SRC'").Scan(&s.sourceID)

	s.groupID = uuid.New()
	_, err = s.db.ExecContext(s.ctx,
		`INSERT INTO bi_dashboard_group (group_id, group_code, group_name) VALUES ($1,'ITEST_GRP','itest')
		 ON CONFLICT (group_code) DO NOTHING`, s.groupID)
	require.NoError(s.T(), err)
	_ = s.db.QueryRowContext(s.ctx, "SELECT group_id FROM bi_dashboard_group WHERE group_code='ITEST_GRP'").Scan(&s.groupID)

	// Seed deterministic fact rows here so every test (regardless of testify's alphabetical
	// method order) sees the same data. Tests that exercise Upsert re-write a subset.
	s.seedFacts()
}

// seedFacts inserts the canonical __ITEST__ fact rows and refreshes the MVs.
func (s *BIRepositorySuite) seedFacts() {
	d1 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	rows := []factmetric.FactMetric{
		{Type: "MIS", Group1: "EBITDA", Group2: "INCOME", Group2Order: 10, PeriodGrain: "MONTHLY", PeriodDate: d1, PeriodLabel: "202604", Value: -5000, DisplayValue: 5000, Scenario: "ACTUAL", SourceID: s.sourceID, DimensionKey: "__ITEST__", IsActive: true},
		{Type: "MIS", Group1: "EBITDA", Group2: "INCOME", Group2Order: 10, PeriodGrain: "MONTHLY", PeriodDate: d2, PeriodLabel: "202605", Value: -6000, DisplayValue: 6000, Scenario: "ACTUAL", SourceID: s.sourceID, DimensionKey: "__ITEST__", IsActive: true},
		{Type: "MIS", Group1: "EBITDA", Group2: "COST", Group2Order: 20, PeriodGrain: "MONTHLY", PeriodDate: d2, PeriodLabel: "202605", Value: 2000, DisplayValue: -2000, Scenario: "ACTUAL", SourceID: s.sourceID, DimensionKey: "__ITEST__", IsActive: true},
	}
	require.NoError(s.T(), s.factRepo.Upsert(s.ctx, rows))
	_, err := s.db.ExecContext(s.ctx, "REFRESH MATERIALIZED VIEW mv_bi_metric_g1")
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, "REFRESH MATERIALIZED VIEW mv_bi_metric_g2")
	require.NoError(s.T(), err)
}

func (s *BIRepositorySuite) TestUpsertAndQueryAggregate() {
	d2 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	// Idempotent re-upsert with a changed value updates, not duplicates (business-key conflict).
	updated := factmetric.FactMetric{
		Type: "MIS", Group1: "EBITDA", Group2: "COST", Group2Order: 20, PeriodGrain: "MONTHLY",
		PeriodDate: d2, PeriodLabel: "202605", Value: 2000, DisplayValue: -2500, Scenario: "ACTUAL",
		SourceID: s.sourceID, DimensionKey: "__ITEST__", IsActive: true,
	}
	require.NoError(s.T(), s.factRepo.Upsert(s.ctx, []factmetric.FactMetric{updated}))
	_, err := s.db.ExecContext(s.ctx, "REFRESH MATERIALIZED VIEW mv_bi_metric_g2")
	require.NoError(s.T(), err)

	// Query group_2 breakdown for May via MV — INCOME (6000) and COST (updated to -2500).
	plan := factmetric.PlannedQuery{
		SQL: `SELECT COALESCE(group_2,'') AS category, NULL::date, ''::text, SUM(value) AS value, 0::numeric, COALESCE(MAX(group_2_order),0) AS ord
		      FROM mv_bi_metric_g2 WHERE type=$1 AND group_1=$2 AND periode_grain='MONTHLY' AND periode_date=$3 AND scenario='ACTUAL'
		      GROUP BY group_2 ORDER BY ord`,
		Args: []any{"MIS", "EBITDA", d2},
	}
	agg, err := s.factRepo.QueryAggregate(s.ctx, plan)
	require.NoError(s.T(), err)
	require.Len(s.T(), agg, 2, "idempotent upsert must not duplicate rows")
	require.Equal(s.T(), "INCOME", agg[0].Category)
	require.InDelta(s.T(), 6000, agg[0].Value, 0.01)
	require.Equal(s.T(), "COST", agg[1].Category)
	require.InDelta(s.T(), -2500, agg[1].Value, 0.01, "re-upsert should update display_value")
}

func (s *BIRepositorySuite) TestGetDistincts() {
	d := factmetric.DistinctScope{Type: "MIS"}
	got, err := s.factRepo.GetDistincts(s.ctx, d)
	require.NoError(s.T(), err)
	require.Contains(s.T(), got.Types, "MIS")
	require.Contains(s.T(), got.Group1s, "EBITDA")
	require.Contains(s.T(), got.Group2s, "INCOME")
}

func (s *BIRepositorySuite) TestDashboardCRUDRoundtrip() {
	d, err := dashboard.NewDashboard(dashboard.NewDashboardParams{
		Code: "ITEST_DASH", Title: "ITest", FilterType: "MIS", FilterGroup1: "EBITDA",
		PeriodGrain: "MONTHLY", DefaultPeriod: "L12M", ChartType: "waterfall",
		ChartConfigRaw: map[string]any{"x_axis_field": "group_2", "y_axis_field": "display_value"},
		CompareModes:   []string{"YoY"}, MaxDrillLevel: 3, CacheTTLSec: 1800,
		GroupID: s.groupID, IsActive: true, AllowedRoleCodes: []string{"CFO"}, CreatedBy: uuid.New(),
	})
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.dashRepo.Create(s.ctx, d))

	// GetByCode hydrates incl. roles.
	got, err := s.dashRepo.GetByCode(s.ctx, "ITEST_DASH")
	require.NoError(s.T(), err)
	require.Equal(s.T(), "ITest", got.Title())
	require.Equal(s.T(), []string{"CFO"}, got.AllowedRoleCodes())

	// ListAccessible: CFO sees it, INTERN doesn't, super-admin always.
	cfoList, err := s.dashRepo.ListAccessible(s.ctx, []string{"CFO"}, false)
	require.NoError(s.T(), err)
	require.True(s.T(), containsCode(cfoList, "ITEST_DASH"))

	internList, err := s.dashRepo.ListAccessible(s.ctx, []string{"INTERN"}, false)
	require.NoError(s.T(), err)
	require.False(s.T(), containsCode(internList, "ITEST_DASH"))

	adminList, err := s.dashRepo.ListAccessible(s.ctx, []string{"INTERN"}, true)
	require.NoError(s.T(), err)
	require.True(s.T(), containsCode(adminList, "ITEST_DASH"))

	// SetRoles to empty → visible to everyone now.
	require.NoError(s.T(), s.dashRepo.SetRoles(s.ctx, got.ID(), nil, uuid.New()))
	openList, err := s.dashRepo.ListAccessible(s.ctx, []string{"INTERN"}, false)
	require.NoError(s.T(), err)
	require.True(s.T(), containsCode(openList, "ITEST_DASH"), "empty role whitelist = open to all")

	// SoftDelete hides it.
	require.NoError(s.T(), s.dashRepo.SoftDelete(s.ctx, got.ID(), uuid.New()))
	_, err = s.dashRepo.GetByCode(s.ctx, "ITEST_DASH")
	require.ErrorIs(s.T(), err, dashboard.ErrNotFound)
}

func (s *BIRepositorySuite) TestDuplicateCode_Conflict() {
	mk := func() *dashboard.Dashboard {
		d, err := dashboard.NewDashboard(dashboard.NewDashboardParams{
			Code: "ITEST_DUP", Title: "Dup", FilterType: "MIS", PeriodGrain: "MONTHLY",
			DefaultPeriod: "L12M", ChartType: "bar",
			ChartConfigRaw: map[string]any{"x_axis_field": "group_1", "y_axis_field": "value"},
			MaxDrillLevel:  1, CacheTTLSec: 60, GroupID: s.groupID, IsActive: true, CreatedBy: uuid.New(),
		})
		require.NoError(s.T(), err)
		return d
	}
	require.NoError(s.T(), s.dashRepo.Create(s.ctx, mk()))
	err := s.dashRepo.Create(s.ctx, mk())
	require.ErrorIs(s.T(), err, dashboard.ErrAlreadyExists)
	_, _ = s.db.ExecContext(s.ctx, "DELETE FROM bi_dashboard WHERE dashboard_code='ITEST_DUP'")
}

func containsCode(list []*dashboard.Dashboard, code string) bool {
	for _, d := range list {
		if d.Code().String() == code {
			return true
		}
	}
	return false
}
