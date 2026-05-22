package costcalc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	calcdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// =============================================================================
// Pure unit tests — no DB required.
// =============================================================================

func TestTopoSortFormulas_OrdersByDependency(t *testing.T) {
	t.Parallel()
	// F1 depends on F2's result. F2 must come first.
	f1 := Formula{
		FormulaCode:     "F1",
		ResultParamCode: "PROD_COST",
		InputParamCodes: []string{"RM_COST"},
	}
	f2 := Formula{
		FormulaCode:     "F2",
		ResultParamCode: "RM_COST",
		InputParamCodes: []string{"RAW_RATE"},
	}
	sorted, err := topoSortFormulas([]Formula{f1, f2})
	require.NoError(t, err)
	require.Len(t, sorted, 2)
	require.Equal(t, "F2", sorted[0].FormulaCode, "F2 (dependency) must precede F1 (dependent)")
	require.Equal(t, "F1", sorted[1].FormulaCode)
	require.Equal(t, 0, sorted[0].SortOrder)
	require.Equal(t, 1, sorted[1].SortOrder)
}

func TestTopoSortFormulas_NoExternalEdges(t *testing.T) {
	t.Parallel()
	// All inputs are external (CAPP / RM) — every formula has in-degree 0.
	fs := []Formula{
		{FormulaCode: "A", ResultParamCode: "X", InputParamCodes: []string{"capp1"}},
		{FormulaCode: "B", ResultParamCode: "Y", InputParamCodes: []string{"capp2"}},
	}
	sorted, err := topoSortFormulas(fs)
	require.NoError(t, err)
	require.Len(t, sorted, 2)
}

func TestTopoSortFormulas_CycleError(t *testing.T) {
	t.Parallel()
	// F1 produces A, consumes B; F2 produces B, consumes A. → cycle.
	f1 := Formula{FormulaCode: "F1", ResultParamCode: "A", InputParamCodes: []string{"B"}}
	f2 := Formula{FormulaCode: "F2", ResultParamCode: "B", InputParamCodes: []string{"A"}}
	_, err := topoSortFormulas([]Formula{f1, f2})
	require.Error(t, err)
	require.True(t, errors.Is(err, calcdomain.ErrCycleDetected), "want ErrCycleDetected, got %v", err)
}

func TestTopoSortFormulas_Empty(t *testing.T) {
	t.Parallel()
	sorted, err := topoSortFormulas(nil)
	require.NoError(t, err)
	require.Nil(t, sorted)
}

// =============================================================================
// Integration tests — INTEGRATION_TEST=true required.
// =============================================================================

type LoaderSuite struct {
	suite.Suite
	ctx        context.Context
	db         *sql.DB
	loader     ProductLoader
	productIDs []int64 // 3 products
	upstreamID int64   // additional product used as an upstream reference
	headIDs    []int64 // route heads, 1 per product
	formulaIDs []string
	paramIDs   map[string]string // paramCode → uuid
	period     string
	calcType   string
	actor      string
	// fixture tagging for cleanup
	codePrefix string
}

func TestLoaderSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(LoaderSuite))
}

func (s *LoaderSuite) SetupSuite() {
	s.ctx = context.Background()
	s.period = "999999"
	s.calcType = "ACTUAL"
	s.actor = "loader-test"
	s.codePrefix = fmt.Sprintf("LD%d", time.Now().UnixNano()%10000)
	s.paramIDs = map[string]string{}

	host := envOr("TEST_DB_HOST", "localhost")
	port := envOr("TEST_DB_PORT", "5434")
	user := envOr("TEST_DB_USER", "finance")
	password := envOr("TEST_DB_PASSWORD", "finance123")
	dbname := envOr("TEST_DB_NAME", "finance_db")
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	raw, err := sql.Open("postgres", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), waitDB(raw, 10*time.Second))
	s.db = raw
	s.loader = NewProductLoader(raw)

	s.seedAll()
}

func (s *LoaderSuite) TearDownSuite() {
	// Best-effort cleanup; FK ON DELETE CASCADE handles route_seq/rm + CPP/CAPP.
	for _, fid := range s.formulaIDs {
		_, _ = s.db.ExecContext(s.ctx, `DELETE FROM formula_param WHERE formula_id = $1`, fid)
		_, _ = s.db.ExecContext(s.ctx, `DELETE FROM mst_formula WHERE id = $1`, fid)
	}
	for _, paramID := range s.paramIDs {
		_, _ = s.db.ExecContext(s.ctx, `DELETE FROM mst_parameter WHERE id = $1`, paramID)
	}
	_, _ = s.db.ExecContext(s.ctx,
		`DELETE FROM cst_product_cost WHERE cpc_product_sys_id = ANY($1)`,
		int64ToArray(append(s.productIDs, s.upstreamID)))
	_, _ = s.db.ExecContext(s.ctx,
		`DELETE FROM cst_rm_cost WHERE period = $1`, s.period)
	_, _ = s.db.ExecContext(s.ctx,
		`DELETE FROM cost_route_head WHERE crh_head_id = ANY($1)`,
		int64ToArray(s.headIDs))
	_, _ = s.db.ExecContext(s.ctx,
		`DELETE FROM cost_product_master WHERE cpm_product_sys_id = ANY($1)`,
		int64ToArray(append(s.productIDs, s.upstreamID)))
	_ = s.db.Close()
}

func (s *LoaderSuite) seedAll() {
	s.seedProducts()
	s.seedRoutes()
	s.seedParameters()
	s.seedCAPPAndCPP()
	s.seedFormulas()
	s.seedRMCosts()
	s.seedUpstreamCost()
}

func (s *LoaderSuite) seedProducts() {
	var typeID int
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT cpt_type_id FROM cost_product_type ORDER BY cpt_type_id LIMIT 1`,
	).Scan(&typeID))

	insert := func(suffix string) int64 {
		var id int64
		require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
			INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_created_by, cpm_updated_by)
			VALUES ($1, $2, 'loader test', $3, $3) RETURNING cpm_product_sys_id`,
			s.codePrefix+suffix, typeID, s.actor,
		).Scan(&id))
		return id
	}
	s.productIDs = []int64{insert("-A"), insert("-B"), insert("-C")}
	s.upstreamID = insert("-UP")
}

func (s *LoaderSuite) seedRoutes() {
	for _, pid := range s.productIDs {
		var headID int64
		require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
			INSERT INTO cost_route_head (crh_product_sys_id, crh_routing_status, crh_version, crh_created_by, crh_updated_by)
			VALUES ($1, 'COMPLETE', 1, $2, $2) RETURNING crh_head_id`,
			pid, s.actor,
		).Scan(&headID))
		s.headIDs = append(s.headIDs, headID)

		// 1 seq + 2 RMs per product.
		var seqID int64
		require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
			INSERT INTO cost_route_seq (crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq, crs_created_by, crs_updated_by)
			VALUES ($1, $2, 1, 1, $3, $3) RETURNING crs_seq_id`,
			headID, pid, s.actor,
		).Scan(&seqID))
		for i, sub := range []string{"X", "Y"} {
			_, err := s.db.ExecContext(s.ctx, `
				INSERT INTO cost_route_rm (crm_seq_id, crm_parent_product_sys_id, crm_rm_type, crm_rm_item_code, crm_route_rm_ratio, crm_sub_type, crm_created_by, crm_updated_by)
				VALUES ($1, $2, 'ITEM', $3, $4, $5, $6, $6)`,
				seqID, pid, fmt.Sprintf("RAW-%s-%d", sub, i), 1.0+float64(i)*0.5, sub, s.actor)
			require.NoError(s.T(), err)
		}
	}
}

func (s *LoaderSuite) seedParameters() {
	// Three params used in CAPP/CPP + formula chain.
	codes := []string{"L_RAW_RATE", "L_RM_COST", "L_PROD_COST"}
	for _, code := range codes {
		paramCode := s.codePrefix + "-" + code
		var id string
		require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
			INSERT INTO mst_parameter (param_code, param_name, data_type, param_category, is_active, created_by)
			VALUES ($1, $1, 'NUMBER', 'INPUT', TRUE, $2)
			RETURNING id`, paramCode, s.actor,
		).Scan(&id))
		s.paramIDs[code] = id
	}
}

func (s *LoaderSuite) seedCAPPAndCPP() {
	// Make L_RAW_RATE applicable + value-set for product A and B; product C has it applicable but no value.
	rawRateID := s.paramIDs["L_RAW_RATE"]
	for i, pid := range s.productIDs {
		_, err := s.db.ExecContext(s.ctx, `
			INSERT INTO cost_product_applicable_param (capp_product_sys_id, capp_param_id, capp_is_required, capp_created_by)
			VALUES ($1, $2, FALSE, $3)`, pid, rawRateID, s.actor)
		require.NoError(s.T(), err)
		if i < 2 {
			_, err := s.db.ExecContext(s.ctx, `
				INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_filled_by, cpp_created_by)
				VALUES ($1, $2, $3, $4, $4)`, pid, rawRateID, 100.0+float64(i)*5, s.actor)
			require.NoError(s.T(), err)
		}
	}
}

func (s *LoaderSuite) seedFormulas() {
	insertFormula := func(code, name, expr, resultParamCode string, inputCodes []string) string {
		var id string
		require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
			INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, is_active, created_by)
			VALUES ($1, $2, 'CALCULATION', $3, $4, TRUE, $5) RETURNING id`,
			s.codePrefix+"-"+code, name, expr, s.paramIDs[resultParamCode], s.actor,
		).Scan(&id))
		for ord, inputCode := range inputCodes {
			_, err := s.db.ExecContext(s.ctx, `
				INSERT INTO formula_param (formula_id, param_id, sort_order)
				VALUES ($1, $2, $3)`, id, s.paramIDs[inputCode], ord)
			require.NoError(s.T(), err)
		}
		return id
	}

	// F2 (RM_COST = RAW_RATE * 1.1)  must precede  F1 (PROD_COST = RM_COST + 5).
	f1 := insertFormula("F1", "prod cost", "RM_COST + 5", "L_PROD_COST", []string{"L_RM_COST"})
	f2 := insertFormula("F2", "rm cost", "RAW_RATE * 1.1", "L_RM_COST", []string{"L_RAW_RATE"})
	s.formulaIDs = []string{f1, f2}
}

func (s *LoaderSuite) seedRMCosts() {
	// Five rm_code rows. Two with item_code populated.
	rows := []struct {
		rmCode   string
		rmType   string
		itemCode sql.NullString
		val      float64
	}{
		{"GRP-AA", "GROUP", sql.NullString{}, 12.50},
		{"GRP-BB", "GROUP", sql.NullString{}, 18.00},
		{"ITM-XX", "ITEM", sql.NullString{String: "ITM-XX", Valid: true}, 7.25},
		{"ITM-YY", "ITEM", sql.NullString{String: "ITM-YY", Valid: true}, 3.10},
		{"ITM-ZZ", "ITEM", sql.NullString{String: "ITM-ZZ", Valid: true}, 99.00},
	}
	for _, r := range rows {
		_, err := s.db.ExecContext(s.ctx, `
			INSERT INTO cst_rm_cost (
				period, rm_code, rm_type, item_code, cost_val,
				flag_valuation, flag_marketing, flag_simulation,
				flag_valuation_used, flag_marketing_used, flag_simulation_used,
				created_by
			) VALUES ($1, $2, $3, $4, $5,
				'CONS','CONS','CONS','CONS','CONS','CONS',
				$6)`,
			s.period, r.rmCode, r.rmType, r.itemCode, r.val, s.actor)
		require.NoError(s.T(), err)
	}
}

func (s *LoaderSuite) seedUpstreamCost() {
	// One CALCULATED row + one SUPERSEDED row for the upstream product. Only the
	// non-SUPERSEDED row must be returned.
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO cst_product_cost (cpc_product_sys_id, cpc_period, cpc_calculation_type, cpc_route_head_id, cpc_cost_per_unit, cpc_status, cpc_calculated_by)
		VALUES ($1, $2, $3, $4, $5, 'CALCULATED', $6)`,
		s.upstreamID, s.period, s.calcType, s.headIDs[0], 42.42, s.actor)
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `
		INSERT INTO cst_product_cost (cpc_product_sys_id, cpc_period, cpc_calculation_type, cpc_route_head_id, cpc_cost_per_unit, cpc_status, cpc_calculated_by, cpc_version)
		VALUES ($1, $2, $3, $4, $5, 'SUPERSEDED', $6, 0)`,
		s.upstreamID, s.period, s.calcType, s.headIDs[0], 1.0, s.actor)
	require.NoError(s.T(), err)
}

// ---------- Tests ----------

func (s *LoaderSuite) TestLoader_LoadProducts_HappyPath() {
	got, err := s.loader.LoadProducts(s.ctx, s.productIDs)
	require.NoError(s.T(), err)
	require.Len(s.T(), got, 3)
	for _, pid := range s.productIDs {
		p, ok := got[pid]
		require.True(s.T(), ok, "missing product %d", pid)
		require.Equal(s.T(), pid, p.ProductSysID())
	}
}

func (s *LoaderSuite) TestLoader_LoadProducts_EmptyInput() {
	got, err := s.loader.LoadProducts(s.ctx, nil)
	require.NoError(s.T(), err)
	require.Empty(s.T(), got)
}

func (s *LoaderSuite) TestLoader_LoadRoutesByProducts_AggregatesHeadSeqRM() {
	got, err := s.loader.LoadRoutesByProducts(s.ctx, s.productIDs)
	require.NoError(s.T(), err)
	require.Len(s.T(), got, 3)
	for _, pid := range s.productIDs {
		g, ok := got[pid]
		require.True(s.T(), ok, "missing route for product %d", pid)
		require.NotNil(s.T(), g.Head)
		require.Equal(s.T(), pid, g.Head.ProductSysID)
		require.Len(s.T(), g.Seqs, 1)
		require.Len(s.T(), g.Seqs[0].Rms, 2)
	}
}

func (s *LoaderSuite) TestLoader_LoadCAPP_Batch() {
	got, err := s.loader.LoadCAPP(s.ctx, s.productIDs)
	require.NoError(s.T(), err)
	// Products A and B have values; C has CAPP but no CPP value → not in map.
	require.Len(s.T(), got, 2)
	pidA := s.productIDs[0]
	pidB := s.productIDs[1]
	rawRateCode := s.codePrefix + "-L_RAW_RATE"
	require.InDelta(s.T(), 100.0, got[pidA][rawRateCode], 0.01)
	require.InDelta(s.T(), 105.0, got[pidB][rawRateCode], 0.01)
}

func (s *LoaderSuite) TestLoader_LoadFormulas_TopoSort() {
	got, err := s.loader.LoadFormulas(s.ctx, s.productIDs)
	require.NoError(s.T(), err)
	require.Len(s.T(), got, 3)
	formulas := got[s.productIDs[0]]
	require.GreaterOrEqual(s.T(), len(formulas), 2)

	// Find F1 and F2 (our test fixtures) and verify F2 precedes F1.
	posF1, posF2 := -1, -1
	rmCostCode := s.codePrefix + "-L_RM_COST"
	prodCostCode := s.codePrefix + "-L_PROD_COST"
	for i, f := range formulas {
		switch f.ResultParamCode {
		case prodCostCode:
			posF1 = i
		case rmCostCode:
			posF2 = i
		}
	}
	require.NotEqual(s.T(), -1, posF1, "F1 (PROD_COST) not in result")
	require.NotEqual(s.T(), -1, posF2, "F2 (RM_COST) not in result")
	require.Less(s.T(), posF2, posF1, "F2 must precede F1 in topo order")
}

func (s *LoaderSuite) TestLoader_LoadRMCosts_KeyFormat() {
	got, err := s.loader.LoadRMCosts(s.ctx, []string{"GRP-AA", "ITM-XX", "ITM-NOPE"}, s.period)
	require.NoError(s.T(), err)
	// GROUP has empty item_code → trailing pipe.
	require.InDelta(s.T(), 12.50, got["GRP-AA|"], 0.01)
	// ITEM has item_code populated.
	require.InDelta(s.T(), 7.25, got["ITM-XX|ITM-XX"], 0.01)
	// Missing rm_code simply absent.
	_, has := got["ITM-NOPE|"]
	require.False(s.T(), has)
}

func (s *LoaderSuite) TestLoader_LoadUpstreamCosts_RespectStatus() {
	got, err := s.loader.LoadUpstreamCosts(s.ctx, []int64{s.upstreamID}, s.period, s.calcType)
	require.NoError(s.T(), err)
	require.Len(s.T(), got, 1)
	require.InDelta(s.T(), 42.42, got[s.upstreamID], 0.01)
}

// ---------- helpers ----------

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func waitDB(db *sql.DB, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := db.Ping(); err == nil {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("database not ready within %v", timeout)
}

// int64ToArray wraps pq.Array for terse cleanup statements.
func int64ToArray(ids []int64) interface{} {
	return pq.Array(ids)
}
