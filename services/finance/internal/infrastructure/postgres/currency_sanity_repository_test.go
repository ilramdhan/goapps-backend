// Package postgres_test — T-CUR currency sanity repository (P0-T10a).
//
// Gated by INTEGRATION_TEST=true; LOCAL PostgreSQL only (TEST_DB_* as in
// cost_calc_repos_test.go). Rows are seeded under a synthetic 2099xx period
// range and removed in TearDown.
package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	erpapp "github.com/mutugading/goapps-backend/services/finance/internal/application/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

type CurrencySanitySuite struct {
	suite.Suite
	ctx      context.Context
	db       *postgres.DB
	repo     *postgres.CurrencySanityRepository
	period   string
	products []int64
	routes   []int64
}

func TestCurrencySanitySuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(CurrencySanitySuite))
}

func (s *CurrencySanitySuite) SetupSuite() {
	s.ctx = context.Background()
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnvOrDefault("TEST_DB_HOST", "localhost"), getEnvOrDefault("TEST_DB_PORT", "5434"),
		getEnvOrDefault("TEST_DB_USER", "finance"), getEnvOrDefault("TEST_DB_PASSWORD", "finance123"),
		getEnvOrDefault("TEST_DB_NAME", "finance_db"))
	raw, err := sql.Open("postgres", dsn)
	s.Require().NoError(err)
	s.Require().NoError(waitForDB(raw, 10*time.Second))
	s.db = postgres.NewDBFromSQL(raw)
	s.repo = postgres.NewCurrencySanityRepository(s.db)
	s.period = fmt.Sprintf("2099%02d", time.Now().Nanosecond()%12+1)

	// product erp item | cost | label | status
	s.seed("POY0000001", "19.5", "IDR", "APPROVED")    // guarded, under 20: ok
	s.seed("POY0000002", "25", "IDR", "CALCULATED")    // guarded, > 20: outlier
	s.seed("CMB0000001", "92", "IDR", "APPROVED")      // unguarded, < 100: ok
	s.seed("CMB0000002", "19271.5", "IDR", "VERIFIED") // > 100: outlier + guard count
	s.seed("", "150", "USD", "CALCULATED")             // unlinked, > 100
	s.seed("TTY0000001", "500", "IDR", "SUPERSEDED")   // superseded: ignored
}

func (s *CurrencySanitySuite) seed(erpItem, cost, currency, status string) {
	var typeID int
	s.Require().NoError(s.db.QueryRowContext(s.ctx,
		`SELECT cpt_type_id FROM cost_product_type ORDER BY cpt_type_id LIMIT 1`).Scan(&typeID))
	code := fmt.Sprintf("CUR-%d", time.Now().UnixNano()%100000000)
	var pid int64
	s.Require().NoError(s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name,
			cpm_erp_item_code, cpm_created_by, cpm_updated_by)
		VALUES ($1, $2, 'currency sanity test', NULLIF($3, ''), 'integ-test', 'integ-test')
		RETURNING cpm_product_sys_id`, code, typeID, erpItem).Scan(&pid))
	var head int64
	s.Require().NoError(s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_route_head (crh_product_sys_id, crh_routing_status, crh_version, crh_created_by, crh_updated_by)
		VALUES ($1, 'DRAFT', 1, 'integ-test', 'integ-test') RETURNING crh_head_id`, pid).Scan(&head))
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO cst_product_cost (cpc_product_sys_id, cpc_period, cpc_calculation_type, cpc_route_head_id,
			cpc_version, cpc_cost_per_unit, cpc_currency_code, cpc_status, cpc_calculated_by)
		VALUES ($1, $2, 'ACTUAL', $3, 1, $4::numeric, $5, $6, 'integ-test')`,
		pid, s.period, head, cost, currency, status)
	s.Require().NoError(err)
	s.products = append(s.products, pid)
	s.routes = append(s.routes, head)
}

func (s *CurrencySanitySuite) TearDownSuite() {
	for i, pid := range s.products {
		_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cst_product_cost WHERE cpc_product_sys_id = $1`, pid)
		_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cost_route_head WHERE crh_head_id = $1`, s.routes[i])
		_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cost_product_master WHERE cpm_product_sys_id = $1`, pid)
	}
	_ = s.db.Close()
}

func (s *CurrencySanitySuite) filter(linked bool) erpapp.CurrencySanityFilter {
	f, err := erpapp.CurrencySanityFilter{Period: s.period, LinkedOnly: linked}.Normalize()
	s.Require().NoError(err)
	return f
}

func (s *CurrencySanitySuite) TestLabelDistribution() {
	got, err := s.repo.LabelDistribution(s.ctx, s.filter(false))
	s.Require().NoError(err)
	m := map[string]int64{}
	for _, c := range got {
		m[c.Currency] = c.Rows
	}
	s.Equal(map[string]int64{"IDR": 4, "USD": 1}, m)

	got, err = s.repo.LabelDistribution(s.ctx, s.filter(true))
	s.Require().NoError(err)
	s.Require().Len(got, 1)
	s.Equal(int64(4), got[0].Rows)
}

func (s *CurrencySanitySuite) TestPercentiles() {
	p, err := s.repo.Percentiles(s.ctx, s.filter(true))
	s.Require().NoError(err)
	s.Equal(int64(4), p.Rows)
	s.True(p.Min.Decimal.Equal(decimal.RequireFromString("19.5")))
	s.True(p.Max.Decimal.Equal(decimal.RequireFromString("19271.5")))
	s.True(p.P50.Valid)
}

func (s *CurrencySanitySuite) TestListOutliers() {
	rows, total, err := s.repo.ListOutliers(s.ctx, s.filter(false))
	s.Require().NoError(err)
	s.Equal(int64(3), total)
	s.Require().Len(rows, 3)
	for _, o := range rows {
		s.True(erpapp.IsCurrencyOutlier(o.ErpItemCode, o.CostPerUnit), "SQL and Go outlier rule must agree: %+v", o)
		s.NotEqual("SUPERSEDED", o.Status)
	}
	byItem := map[string]erpapp.CurrencyOutlier{}
	for _, o := range rows {
		byItem[o.ErpItemCode] = o
	}
	s.True(byItem["POY0000002"].Threshold.Equal(erpapp.GuardedPrefixThreshold))
	s.True(byItem["CMB0000002"].Threshold.Equal(erpapp.OverallThreshold))

	f := s.filter(true)
	f.OutlierLimit = 1
	rows, total, err = s.repo.ListOutliers(s.ctx, f)
	s.Require().NoError(err)
	s.Equal(int64(2), total, "linked-only excludes the unlinked row")
	s.Len(rows, 1)
	s.True(rows[0].CostPerUnit.Equal(decimal.RequireFromString("19271.5")), "highest first")
}

func (s *CurrencySanitySuite) TestPeriodSummaries() {
	got, err := s.repo.PeriodSummaries(s.ctx, s.period)
	s.Require().NoError(err)
	require.Len(s.T(), got, 1)
	p := got[0]
	s.Equal(int64(5), p.Rows)
	s.Equal(int64(2), p.ApprovedRows)
	s.Equal(int64(4), p.IDRRows)
	s.Equal(int64(1), p.USDRows)
	s.Equal(int64(2), p.OverOverall)
	s.Equal(int64(3), p.OutlierRows)
	s.True(p.Max.Equal(decimal.RequireFromString("19271.5")))

	rep, err := erpapp.NewCurrencySanityService(s.repo).Report(s.ctx, erpapp.CurrencySanityFilter{Period: s.period})
	s.Require().NoError(err)
	s.False(rep.Periods[0].RelabelSafe)
}
