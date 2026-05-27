package costcalc

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// ProcessChunkSuite spins up real product / route / formula / CAPP / RM-cost
// fixtures and exercises the inline ProcessChunk path end-to-end.
type ProcessChunkSuite struct {
	suite.Suite
	ctx               context.Context
	raw               *sql.DB
	db                *postgres.DB
	svc               *Service
	productID         int64
	headID            int64
	formulaIDs        []string
	deactivatedFmlIDs []string // formulas we temporarily set inactive to bypass the result_param unique index
	paramIDs          map[string]string
	insertedParamIDs  []string // only the params WE created — others (e.g. pre-existing COST_STAGE_OUT) are left alone
	period            string
	calcType          costcalcdom.CalculationType
	actor             string
	codePrefix        string
	cappValue         bool // when true the seed inserts a CPP row for L_RAW_RATE
}

func TestProcessChunkSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(ProcessChunkSuite))
}

func (s *ProcessChunkSuite) SetupSuite() {
	s.ctx = context.Background()
	s.period = "999990"
	s.calcType = costcalcdom.CalcTypeActual
	s.actor = "process-chunk-test"
	s.codePrefix = fmt.Sprintf("PC%d", time.Now().UnixNano()%10000)
	// Use underscore for the formula-expression-safe code: identifiers can't
	// contain '-' (expr-lang would parse it as subtraction).
	s.paramIDs = map[string]string{}
	s.cappValue = true

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
	s.raw = raw
	s.db = postgres.NewDBFromSQL(raw)

	s.svc = NewService(
		postgres.NewCostCalcJobRepository(s.db),
		postgres.NewCostCalcChunkRepository(s.db),
		postgres.NewCostCalcJobProductRepository(s.db),
		postgres.NewCostResultRepository(s.db),
		postgres.NewCostAuditHistoryRepository(s.db),
		NewProductLoader(raw),
		evaluator.NewCache(),
		nil,
		nil,
	)

	s.seedAll()
}

func (s *ProcessChunkSuite) TearDownSuite() {
	if s.raw == nil {
		return
	}
	for _, fid := range s.formulaIDs {
		_, _ = s.raw.ExecContext(s.ctx, `DELETE FROM formula_param WHERE formula_id = $1`, fid)
		_, _ = s.raw.ExecContext(s.ctx, `DELETE FROM mst_formula WHERE id = $1`, fid)
	}
	// Restore pre-existing formulas we temporarily deactivated. Order matters
	// only if there's more than one — the unique index forbids concurrent active
	// rows, so we re-activate after our own formula has been deleted above.
	for _, fid := range s.deactivatedFmlIDs {
		_, _ = s.raw.ExecContext(s.ctx,
			`UPDATE mst_formula SET deleted_at = NULL, is_active = TRUE WHERE id = $1`, fid)
	}
	for _, paramID := range s.insertedParamIDs {
		_, _ = s.raw.ExecContext(s.ctx, `DELETE FROM cost_product_parameter WHERE cpp_param_id = $1`, paramID)
		_, _ = s.raw.ExecContext(s.ctx, `DELETE FROM cost_product_applicable_param WHERE capp_param_id = $1`, paramID)
		_, _ = s.raw.ExecContext(s.ctx, `DELETE FROM mst_parameter WHERE id = $1`, paramID)
	}
	_, _ = s.raw.ExecContext(s.ctx, `DELETE FROM aud_cost_history WHERE ach_product_sys_id = $1`, s.productID)
	_, _ = s.raw.ExecContext(s.ctx, `DELETE FROM cst_product_cost WHERE cpc_product_sys_id = $1`, s.productID)
	// Catch-all: recompute tests + missing-rm-cost test use non-s.period periods
	// (999991, 999992, etc.). Delete by created_by to scoop up everything this
	// suite inserted across all periods.
	_, _ = s.raw.ExecContext(s.ctx, `DELETE FROM cst_rm_cost WHERE created_by = $1`, s.actor)
	_, _ = s.raw.ExecContext(s.ctx, `DELETE FROM cst_rm_cost WHERE period = $1`, s.period)
	_, _ = s.raw.ExecContext(s.ctx, `DELETE FROM cost_route_head WHERE crh_head_id = $1`, s.headID)
	_, _ = s.raw.ExecContext(s.ctx, `DELETE FROM cost_product_master WHERE cpm_product_sys_id = $1`, s.productID)
	_ = s.raw.Close()
}

func (s *ProcessChunkSuite) seedAll() {
	s.seedProduct()
	s.seedRoute()
	s.seedParameters()
	s.seedCAPPAndCPP()
	s.seedFormulas()
	s.seedRMCosts()
}

func (s *ProcessChunkSuite) seedProduct() {
	var typeID int
	require.NoError(s.T(), s.raw.QueryRowContext(s.ctx,
		`SELECT cpt_type_id FROM cost_product_type ORDER BY cpt_type_id LIMIT 1`,
	).Scan(&typeID))
	require.NoError(s.T(), s.raw.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_created_by, cpm_updated_by)
		VALUES ($1, $2, 'process chunk test', $3, $3) RETURNING cpm_product_sys_id`,
		s.codePrefix+"-PROD", typeID, s.actor,
	).Scan(&s.productID))
}

func (s *ProcessChunkSuite) seedRoute() {
	require.NoError(s.T(), s.raw.QueryRowContext(s.ctx, `
		INSERT INTO cost_route_head (crh_product_sys_id, crh_routing_status, crh_version, crh_created_by, crh_updated_by)
		VALUES ($1, 'COMPLETE', 1, $2, $2) RETURNING crh_head_id`,
		s.productID, s.actor,
	).Scan(&s.headID))

	var seqID int64
	require.NoError(s.T(), s.raw.QueryRowContext(s.ctx, `
		INSERT INTO cost_route_seq (crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq, crs_created_by, crs_updated_by)
		VALUES ($1, $2, 1, 1, $3, $3) RETURNING crs_seq_id`,
		s.headID, s.productID, s.actor,
	).Scan(&seqID))

	// Single ITEM-type RM at ratio 1.0 → contribution = unit cost.
	_, err := s.raw.ExecContext(s.ctx, `
		INSERT INTO cost_route_rm (crm_seq_id, crm_parent_product_sys_id, crm_rm_type, crm_rm_item_code, crm_route_rm_ratio, crm_sub_type, crm_created_by, crm_updated_by)
		VALUES ($1, $2, 'ITEM', $3, $4, $5, $6, $6)`,
		seqID, s.productID, s.codePrefix+"-ITM", 1.0, "X", s.actor)
	require.NoError(s.T(), err)
}

func (s *ProcessChunkSuite) seedParameters() {
	codes := []string{"L_RAW_RATE", "COST_STAGE_OUT"}
	for _, code := range codes {
		// Use underscore separator: param_code is referenced verbatim as an
		// identifier inside formula expressions and '-' would be parsed as
		// subtraction by expr-lang.
		paramCode := s.codePrefix + "_" + code
		if code == "COST_STAGE_OUT" {
			// ComputeProduct looks up the reserved key "COST_STAGE_OUT" verbatim,
			// so we use that exact code (no prefix) for the result param.
			paramCode = "COST_STAGE_OUT"
		}
		var id string
		// Reuse existing row if present (the reserved COST_STAGE_OUT param_code
		// may already exist from a prior run or migration). Only track in
		// insertedParamIDs the rows we actually inserted, so teardown leaves
		// shared rows untouched.
		err := s.raw.QueryRowContext(s.ctx,
			`SELECT id FROM mst_parameter WHERE param_code = $1 AND deleted_at IS NULL LIMIT 1`,
			paramCode,
		).Scan(&id)
		if err != nil {
			require.NoError(s.T(), s.raw.QueryRowContext(s.ctx, `
				INSERT INTO mst_parameter (param_code, param_name, data_type, param_category, is_active, created_by)
				VALUES ($1, $1, 'NUMBER', 'INPUT', TRUE, $2)
				RETURNING id`, paramCode, s.actor,
			).Scan(&id))
			s.insertedParamIDs = append(s.insertedParamIDs, id)
		}
		s.paramIDs[code] = id
	}
}

func (s *ProcessChunkSuite) seedCAPPAndCPP() {
	rawRateID := s.paramIDs["L_RAW_RATE"]
	_, err := s.raw.ExecContext(s.ctx, `
		INSERT INTO cost_product_applicable_param (capp_product_sys_id, capp_param_id, capp_is_required, capp_created_by)
		VALUES ($1, $2, FALSE, $3)`, s.productID, rawRateID, s.actor)
	require.NoError(s.T(), err)
	if s.cappValue {
		_, err := s.raw.ExecContext(s.ctx, `
			INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_filled_by, cpp_created_by)
			VALUES ($1, $2, $3, $4, $4)`, s.productID, rawRateID, 7.0, s.actor)
		require.NoError(s.T(), err)
	}
}

func (s *ProcessChunkSuite) seedFormulas() {
	// The mst_formula table has a unique partial index on result_param_id for
	// active rows. Any pre-existing active formula pointing at COST_STAGE_OUT
	// must be deactivated for our test fixture to slot in. Restored in teardown.
	// Deactivate ALL active formulas: the loader returns every active formula
	// and feeds them into the eval scope. Any pre-existing formula referencing
	// a CAPP we haven't seeded would surface as ErrFormulaEval → BLOCKED.
	rows, err := s.raw.QueryContext(s.ctx,
		`SELECT id FROM mst_formula WHERE deleted_at IS NULL AND is_active = TRUE`)
	require.NoError(s.T(), err)
	for rows.Next() {
		var id string
		require.NoError(s.T(), rows.Scan(&id))
		s.deactivatedFmlIDs = append(s.deactivatedFmlIDs, id)
	}
	require.NoError(s.T(), rows.Close())
	// Soft-delete to escape the partial unique index (deleted_at IS NULL).
	for _, id := range s.deactivatedFmlIDs {
		_, derr := s.raw.ExecContext(s.ctx,
			`UPDATE mst_formula SET deleted_at = now(), is_active = FALSE WHERE id = $1`, id)
		require.NoError(s.T(), derr)
	}

	// COST_STAGE_OUT = COST_RM_TOTAL + L_RAW_RATE
	// L_RAW_RATE comes from CAPP (=7.0) and COST_RM_TOTAL from the route RM
	// (rate 25.0 × ratio 1.0 = 25.0) → expected 32.0.
	rawRateCode := s.codePrefix + "_L_RAW_RATE"
	expr := fmt.Sprintf("COST_RM_TOTAL + %s", rawRateCode)
	var id string
	require.NoError(s.T(), s.raw.QueryRowContext(s.ctx, `
		INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, is_active, created_by)
		VALUES ($1, $2, 'CALCULATION', $3, $4, TRUE, $5) RETURNING id`,
		s.codePrefix+"-FOUT", "compute stage out", expr, s.paramIDs["COST_STAGE_OUT"], s.actor,
	).Scan(&id))
	s.formulaIDs = []string{id}

	_, err = s.raw.ExecContext(s.ctx, `
		INSERT INTO formula_param (formula_id, param_id, sort_order)
		VALUES ($1, $2, 0)`, id, s.paramIDs["L_RAW_RATE"])
	require.NoError(s.T(), err)
}

func (s *ProcessChunkSuite) seedRMCosts() {
	_, err := s.raw.ExecContext(s.ctx, `
		INSERT INTO cst_rm_cost (
			period, rm_code, rm_type, item_code, cost_val,
			flag_valuation, flag_marketing, flag_simulation,
			flag_valuation_used, flag_marketing_used, flag_simulation_used,
			created_by
		) VALUES ($1, $2, 'ITEM', NULL, 25.0,
			'CONS','CONS','CONS','CONS','CONS','CONS',
			$3)`, s.period, s.codePrefix+"-ITM", s.actor)
	require.NoError(s.T(), err)
}

// createJob is a convenience helper for tests that need to seed a fresh job.
func (s *ProcessChunkSuite) createJob() *costcalcdom.Job {
	job, err := costcalcdom.NewJob(s.period, s.calcType, costcalcdom.ScopeSingleProduct, nil, "TEST", s.actor)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.svc.jobRepo.Create(s.ctx, job))
	return job
}

// seedJobProduct creates the cal_job_product row required by ProcessChunk to
// call MarkSuccess/MarkBlocked on.
func (s *ProcessChunkSuite) seedJobProduct(job *costcalcdom.Job) {
	jp := costcalcdom.NewJobProduct(job.ID(), s.productID, s.headID, 0)
	require.NoError(s.T(), s.svc.productRepo.BulkCreate(s.ctx, []*costcalcdom.JobProduct{jp}))
}

// ---------- tests ----------

func (s *ProcessChunkSuite) TestProcessChunk_SingleProduct_HappyPath() {
	job := s.createJob()
	s.seedJobProduct(job)

	out, err := s.svc.ProcessChunk(s.ctx, ProcessChunkInput{
		JobID:    job.ID(),
		ChunkID:  0,
		Period:   s.period,
		CalcType: s.calcType,
		Products: []int64{s.productID},
		Actor:    s.actor,
	})
	require.NoError(s.T(), err)
	if out.Success != 1 {
		jp, _ := s.svc.productRepo.GetByJobAndProduct(s.ctx, job.ID(), s.productID)
		s.T().Fatalf("expected Success=1 got out=%+v jp.status=%s jp.blockReason=%q jp.err=%q",
			out, jp.Status(), jp.BlockReason(), jp.ErrorMessage())
	}

	active, err := s.svc.resultRepo.GetActive(s.ctx, s.productID, s.period, s.calcType)
	require.NoError(s.T(), err)
	require.InDelta(s.T(), 32.0, active.CostPerUnit(), 0.001)
	require.Equal(s.T(), costcalcdom.ResultStatusCalculated, active.Status())

	jp, err := s.svc.productRepo.GetByJobAndProduct(s.ctx, job.ID(), s.productID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), costcalcdom.JobProductStatusSuccess, jp.Status())
	require.Equal(s.T(), active.ID(), jp.CostID())
}

func (s *ProcessChunkSuite) TestProcessChunk_MissingRMCost_Blocked() {
	// Switch periods so cst_rm_cost lookup misses — engine returns
	// ErrMissingRMCost which maps to BLOCKED.
	job := s.createJob()
	s.seedJobProduct(job)

	out, err := s.svc.ProcessChunk(s.ctx, ProcessChunkInput{
		JobID:    job.ID(),
		ChunkID:  0,
		Period:   "888888",
		CalcType: s.calcType,
		Products: []int64{s.productID},
		Actor:    s.actor,
	})
	require.NoError(s.T(), err)
	require.Equal(s.T(), 0, out.Success)
	require.Equal(s.T(), 1, out.Blocked)

	jp, err := s.svc.productRepo.GetByJobAndProduct(s.ctx, job.ID(), s.productID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), costcalcdom.JobProductStatusBlocked, jp.Status())
	require.Equal(s.T(), blockReasonMissingRMCost, jp.BlockReason())
}

func (s *ProcessChunkSuite) TestProcessChunk_Recompute_SupersedesAndAudits() {
	// Use a distinct period so this test doesn't collide with TestProcessChunk_SingleProduct_HappyPath
	// (suite-shared fixtures + suite-shared DB state).
	recomputePeriod := "999991"
	// Seed an RM cost row for the recompute period (the seedRMCosts helper only
	// covers s.period).
	_, err := s.raw.ExecContext(s.ctx, `
		INSERT INTO cst_rm_cost (
			period, rm_code, rm_type, item_code, cost_val,
			flag_valuation, flag_marketing, flag_simulation,
			flag_valuation_used, flag_marketing_used, flag_simulation_used,
			created_by
		) VALUES ($1, $2, 'ITEM', NULL, 25.0,
			'CONS','CONS','CONS','CONS','CONS','CONS',
			$3)`, recomputePeriod, s.codePrefix+"-ITM", s.actor)
	require.NoError(s.T(), err)

	jobA, err := costcalcdom.NewJob(recomputePeriod, s.calcType, costcalcdom.ScopeSingleProduct, nil, "TEST", s.actor)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.svc.jobRepo.Create(s.ctx, jobA))
	s.seedJobProduct(jobA)
	outA, err := s.svc.ProcessChunk(s.ctx, ProcessChunkInput{
		JobID: jobA.ID(), Period: recomputePeriod, CalcType: s.calcType,
		Products: []int64{s.productID}, Actor: s.actor,
	})
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, outA.Success)

	firstID, err := getActiveID(s.ctx, s.raw, s.productID, recomputePeriod)
	require.NoError(s.T(), err)

	jobB, err := costcalcdom.NewJob(recomputePeriod, s.calcType, costcalcdom.ScopeSingleProduct, nil, "TEST", s.actor)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.svc.jobRepo.Create(s.ctx, jobB))
	s.seedJobProduct(jobB)
	outB, err := s.svc.ProcessChunk(s.ctx, ProcessChunkInput{
		JobID: jobB.ID(), Period: recomputePeriod, CalcType: s.calcType,
		Products: []int64{s.productID}, Actor: s.actor,
	})
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, outB.Success)

	active, err := s.svc.resultRepo.GetActive(s.ctx, s.productID, recomputePeriod, s.calcType)
	require.NoError(s.T(), err)
	require.NotEqual(s.T(), firstID, active.ID(), "second pass should produce a new row")
	require.Equal(s.T(), 2, active.Version())

	var historyCount int
	require.NoError(s.T(), s.raw.QueryRowContext(s.ctx,
		`SELECT count(*) FROM aud_cost_history WHERE ach_product_sys_id = $1 AND ach_new_job_id = $2`,
		s.productID, jobB.ID(),
	).Scan(&historyCount))
	require.GreaterOrEqual(s.T(), historyCount, 1, "supersede should write a history row")
}

// getActiveID returns the non-SUPERSEDED cost_id for the tuple (helper for tests).
func getActiveID(ctx context.Context, db *sql.DB, productID int64, period string) (int64, error) {
	var id int64
	err := db.QueryRowContext(ctx,
		`SELECT cpc_cost_id FROM cst_product_cost
		   WHERE cpc_product_sys_id = $1 AND cpc_period = $2 AND cpc_status != 'SUPERSEDED'
		   ORDER BY cpc_calculated_at DESC LIMIT 1`,
		productID, period).Scan(&id)
	return id, err
}
