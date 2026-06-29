package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // pgx stdlib driver: required for COPY (staging repo).
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costimportetl"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// CostImportResolveSuite integration-tests the v2 ETL set-based resolve layers
// against a real PostgreSQL instance. Each test streams raw rows into the
// UNLOGGED stg_import_* tables via the staging repository's COPY path, runs the
// resolve layers, and asserts on the resulting costing rows / captured errors.
type CostImportResolveSuite struct {
	suite.Suite
	db        *postgres.DB
	repo      *postgres.CostImportStagingRepository
	ctx       context.Context
	prefix    string
	typeCode  string
	paramCode string
	nextJob   int64
	jobs      []int64
}

// TestCostImportResolveSuite runs the resolve integration suite when
// INTEGRATION_TEST=true; otherwise it is skipped (matches the repo convention).
func TestCostImportResolveSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(CostImportResolveSuite))
}

func (s *CostImportResolveSuite) SetupSuite() {
	s.ctx = context.Background()

	host := getEnvOrDefault("TEST_DB_HOST", "localhost")
	port := getEnvOrDefault("TEST_DB_PORT", "5434")
	user := getEnvOrDefault("TEST_DB_USER", "finance")
	password := getEnvOrDefault("TEST_DB_PASSWORD", "finance123")
	dbname := getEnvOrDefault("TEST_DB_NAME", "finance_db")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	// The staging COPY path reaches the raw pgx connection via (*sql.Conn).Raw,
	// so the pool MUST be opened with the pgx stdlib driver (not lib/pq).
	raw, err := sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), waitForDB(raw, 10*time.Second))

	s.db = postgres.NewDBFromSQL(raw)
	s.repo = postgres.NewCostImportStagingRepository(s.db)

	// Legacy ids land in cost_product_master.cpm_flex_02 (varchar(20)), so keep
	// the per-run prefix short: "IT" + 7-digit seconds window leaves room for a
	// short per-row suffix while staying unique across runs.
	s.prefix = fmt.Sprintf("IT%07d", time.Now().Unix()%10_000_000)
	s.nextJob = time.Now().Unix() * 1000

	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT cpt_type_code FROM cost_product_type WHERE cpt_is_active = TRUE ORDER BY cpt_type_id LIMIT 1`,
	).Scan(&s.typeCode))
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT param_code FROM mst_parameter WHERE deleted_at IS NULL AND is_active = TRUE ORDER BY param_code LIMIT 1`,
	).Scan(&s.paramCode))
}

func (s *CostImportResolveSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	// Cleanup is best-effort — failures here only affect re-runs.
	for _, jobID := range s.jobs {
		_ = s.repo.CleanupStaging(s.ctx, jobID)
	}
	like := s.prefix + "%"
	stmts := []string{
		`DELETE FROM cost_route_rm WHERE crm_seq_id IN (SELECT crs_seq_id FROM cost_route_seq WHERE crs_head_id IN (SELECT crh_head_id FROM cost_route_head WHERE crh_product_sys_id IN (SELECT cpm_product_sys_id FROM cost_product_master WHERE cpm_flex_02 LIKE $1)))`,
		`DELETE FROM cost_route_seq WHERE crs_head_id IN (SELECT crh_head_id FROM cost_route_head WHERE crh_product_sys_id IN (SELECT cpm_product_sys_id FROM cost_product_master WHERE cpm_flex_02 LIKE $1))`,
		`DELETE FROM cost_route_head WHERE crh_product_sys_id IN (SELECT cpm_product_sys_id FROM cost_product_master WHERE cpm_flex_02 LIKE $1)`,
		`DELETE FROM cost_product_parameter WHERE cpp_product_sys_id IN (SELECT cpm_product_sys_id FROM cost_product_master WHERE cpm_flex_02 LIKE $1)`,
		`DELETE FROM cost_product_applicable_param WHERE capp_product_sys_id IN (SELECT cpm_product_sys_id FROM cost_product_master WHERE cpm_flex_02 LIKE $1)`,
		`DELETE FROM cost_product_master WHERE cpm_flex_02 LIKE $1`,
	}
	for _, stmt := range stmts {
		_, _ = s.db.ExecContext(s.ctx, stmt, like)
	}
	_ = s.db.Close()
}

// newJob returns a fresh synthetic job id (staging is scoped by job_id, so no
// real cost_import_job row is needed) and registers it for staging cleanup.
func (s *CostImportResolveSuite) newJob() int64 {
	s.nextJob++
	s.jobs = append(s.jobs, s.nextJob)
	return s.nextJob
}

// legacy builds a per-run-unique legacy product id so tests never collide with
// each other or with pre-existing master data.
func (s *CostImportResolveSuite) legacy(name string) string {
	return s.prefix + name
}

// staticProducer turns an in-memory set of data rows into a RowProducer, the
// transport the staging COPY path pulls rows from.
func staticProducer(rows [][]string) costimportetl.RowProducer {
	return func(emit costimportetl.RowEmitter) error {
		for _, r := range rows {
			if err := emit(r); err != nil {
				return err
			}
		}
		return nil
	}
}

// masterRow assembles one stg_import_product_master data row (column order per
// the staging repository's stgProductMasterColumns, minus job_id + row_num).
func (s *CostImportResolveSuite) masterRow(legacy string) []string {
	return []string{legacy, s.typeCode, "ETL product " + legacy, "", "", "", "", "", "", "", "true"}
}

// countMaster returns how many active master rows carry the given legacy id.
func (s *CostImportResolveSuite) countMaster(legacy string) int {
	var n int
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT count(*) FROM cost_product_master WHERE cpm_flex_02 = $1`, legacy,
	).Scan(&n))
	return n
}

// countParam returns how many parameter values exist for the product identified
// by legacy and the given param code.
func (s *CostImportResolveSuite) countParam(legacy, paramCode string) int {
	var n int
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
SELECT count(*)
FROM cost_product_parameter cpp
JOIN cost_product_master cpm ON cpm.cpm_product_sys_id = cpp.cpp_product_sys_id
JOIN mst_parameter p ON p.id = cpp.cpp_param_id
WHERE cpm.cpm_flex_02 = $1 AND p.param_code = $2`, legacy, paramCode,
	).Scan(&n))
	return n
}

// TestResolveHappyPath stages a valid product + parameter and asserts both
// resolve into the costing tables with no captured errors.
func (s *CostImportResolveSuite) TestResolveHappyPath() {
	jobID := s.newJob()
	legacyA := s.legacy("HA")

	_, err := s.repo.CopyStagingProductMaster(s.ctx, jobID, staticProducer([][]string{s.masterRow(legacyA)}))
	require.NoError(s.T(), err)

	n, err := s.repo.ResolveLayer1Products(s.ctx, jobID, "tester")
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, n)
	require.Equal(s.T(), 1, s.countMaster(legacyA))

	_, err = s.repo.CopyStagingProductParameter(s.ctx, jobID, staticProducer([][]string{
		{legacyA, s.paramCode, "NUMERIC", "12.5", "", ""},
	}))
	require.NoError(s.T(), err)

	n, err = s.repo.ResolveLayer2Params(s.ctx, jobID, "tester")
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, n)
	require.Equal(s.T(), 1, s.countParam(legacyA, s.paramCode))

	errs, err := s.repo.CollectErrors(s.ctx, jobID)
	require.NoError(s.T(), err)
	require.Empty(s.T(), errs, "happy path must capture no row-level errors")
}

// TestResolvePartialFail proves a param row referencing a missing product is
// captured into stg_import_error and skipped, while a valid sibling row inserts.
func (s *CostImportResolveSuite) TestResolvePartialFail() {
	jobID := s.newJob()
	legacyA := s.legacy("PA")
	missing := s.legacy("PMISS")

	_, err := s.repo.CopyStagingProductMaster(s.ctx, jobID, staticProducer([][]string{s.masterRow(legacyA)}))
	require.NoError(s.T(), err)
	_, err = s.repo.ResolveLayer1Products(s.ctx, jobID, "tester")
	require.NoError(s.T(), err)

	_, err = s.repo.CopyStagingProductParameter(s.ctx, jobID, staticProducer([][]string{
		{legacyA, s.paramCode, "NUMERIC", "7", "", ""}, // valid
		{missing, s.paramCode, "NUMERIC", "9", "", ""}, // references a missing product
	}))
	require.NoError(s.T(), err)

	n, err := s.repo.ResolveLayer2Params(s.ctx, jobID, "tester")
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, n, "only the valid row is written")
	require.Equal(s.T(), 1, s.countParam(legacyA, s.paramCode))

	errs, err := s.repo.CollectErrors(s.ctx, jobID)
	require.NoError(s.T(), err)
	require.Len(s.T(), errs, 1)
	require.Equal(s.T(), "product_parameter", errs[0].Sheet)
	require.Equal(s.T(), missing, errs[0].KeyInfo)
	require.Contains(s.T(), errs[0].Message, "produk tidak dikenal")
}

// TestResolveIdempotency runs the master + param resolve twice over the same
// staged data and asserts the second pass is a no-op upsert: same row counts,
// no duplicates.
func (s *CostImportResolveSuite) TestResolveIdempotency() {
	jobID := s.newJob()
	legacyA := s.legacy("ID")

	_, err := s.repo.CopyStagingProductMaster(s.ctx, jobID, staticProducer([][]string{s.masterRow(legacyA)}))
	require.NoError(s.T(), err)
	_, err = s.repo.CopyStagingProductParameter(s.ctx, jobID, staticProducer([][]string{
		{legacyA, s.paramCode, "NUMERIC", "3.25", "", ""},
	}))
	require.NoError(s.T(), err)

	for pass := 1; pass <= 2; pass++ {
		n1, rErr := s.repo.ResolveLayer1Products(s.ctx, jobID, "tester")
		require.NoError(s.T(), rErr)
		require.Equal(s.T(), 1, n1, "master upsert affects exactly one row each pass")

		n2, rErr := s.repo.ResolveLayer2Params(s.ctx, jobID, "tester")
		require.NoError(s.T(), rErr)
		require.Equal(s.T(), 1, n2, "param upsert affects exactly one row each pass")
	}

	require.Equal(s.T(), 1, s.countMaster(legacyA), "re-run must not duplicate the master row")
	require.Equal(s.T(), 1, s.countParam(legacyA, s.paramCode), "re-run must not duplicate the param row")
}

// TestResolveCrossReferenceGlobal is the key case: a routing sequence is staged
// (COPY) BEFORE the product-master rows it references, yet still resolves —
// proving resolution is a global set-based JOIN over the whole staged dataset
// with no chunk / no ordering dependency.
func (s *CostImportResolveSuite) TestResolveCrossReferenceGlobal() {
	jobID := s.newJob()
	head := s.legacy("XH")
	node := s.legacy("XN")

	// Stage the dependent routing rows FIRST, the master rows they reference LAST.
	_, err := s.repo.CopyStagingRouteSeq(s.ctx, jobID, staticProducer([][]string{
		{head, node, "1", "1", "node " + node, "", "", ""},
	}))
	require.NoError(s.T(), err)
	_, err = s.repo.CopyStagingRouteHead(s.ctx, jobID, staticProducer([][]string{
		{head, "DRAFT", ""},
	}))
	require.NoError(s.T(), err)
	_, err = s.repo.CopyStagingProductMaster(s.ctx, jobID, staticProducer([][]string{
		s.masterRow(head),
		s.masterRow(node),
	}))
	require.NoError(s.T(), err)

	// Resolve in dependency order; each layer JOINs the previously resolved tables.
	nMaster, err := s.repo.ResolveLayer1Products(s.ctx, jobID, "tester")
	require.NoError(s.T(), err)
	require.Equal(s.T(), 2, nMaster)

	nHead, err := s.repo.ResolveLayer4RouteHead(s.ctx, jobID, "tester")
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, nHead)

	nSeq, err := s.repo.ResolveLayer5RouteSeq(s.ctx, jobID, "tester")
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, nSeq, "seq referencing a later-staged product must still resolve")

	errs, err := s.repo.CollectErrors(s.ctx, jobID)
	require.NoError(s.T(), err)
	require.Empty(s.T(), errs)

	// The resolved seq must point at the head of `head` and the node product `node`.
	var ok int
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
SELECT count(*)
FROM cost_route_seq crs
JOIN cost_route_head crh ON crh.crh_head_id = crs.crs_head_id
JOIN cost_product_master headp ON headp.cpm_product_sys_id = crh.crh_product_sys_id
JOIN cost_product_master nodep ON nodep.cpm_product_sys_id = crs.crs_product_sys_id
WHERE headp.cpm_flex_02 = $1 AND nodep.cpm_flex_02 = $2`, head, node,
	).Scan(&ok))
	require.Equal(s.T(), 1, ok)
}
