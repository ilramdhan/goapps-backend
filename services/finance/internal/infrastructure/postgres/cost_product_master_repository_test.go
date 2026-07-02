// Package postgres_test provides integration tests for CostProductMasterRepository.List
// covering the multi-type filter, oracle-sys-id search, and the new sort keys.
package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // production driver — validates array binding through pgx
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

const cpmTestSearchPrefix = "ZZTCPM"

// CostProductMasterRepoSuite exercises List against a real database using the
// same pgx stdlib driver as production (important for the pq.Array binding in
// the multi-type ANY predicate).
type CostProductMasterRepoSuite struct {
	suite.Suite
	db   *postgres.DB
	repo *postgres.CostProductMasterRepository
	ctx  context.Context

	typeID1 int32
	typeID2 int32
	sysIDs  []int64
}

func TestCostProductMasterRepoSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(CostProductMasterRepoSuite))
}

func (s *CostProductMasterRepoSuite) SetupSuite() {
	s.ctx = context.Background()

	host := getEnvOrDefault("TEST_DB_HOST", "localhost")
	port := getEnvOrDefault("TEST_DB_PORT", "5434")
	user := getEnvOrDefault("TEST_DB_USER", "finance")
	password := getEnvOrDefault("TEST_DB_PASSWORD", "finance123")
	dbname := getEnvOrDefault("TEST_DB_NAME", "finance_db")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	raw, err := sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), waitForDB(raw, 10*time.Second))

	s.db = postgres.NewDBFromSQL(raw)
	s.repo = postgres.NewCostProductMasterRepository(s.db)

	s.seedFixtures()
}

func (s *CostProductMasterRepoSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	_, err := s.db.ExecContext(s.ctx, `DELETE FROM cost_product_master WHERE cpm_product_code LIKE $1`, cpmTestSearchPrefix+"%")
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM cost_product_type WHERE cpt_type_code IN ('ZZT1','ZZT2')`)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.db.Close())
}

func (s *CostProductMasterRepoSuite) seedFixtures() {
	t := s.T()

	upsertType := `
		INSERT INTO cost_product_type (cpt_type_code, cpt_type_name)
		VALUES ($1, $2)
		ON CONFLICT (cpt_type_code) DO UPDATE SET cpt_type_name = EXCLUDED.cpt_type_name
		RETURNING cpt_type_id`
	require.NoError(t, s.db.QueryRowContext(s.ctx, upsertType, "ZZT1", "CPM List Test Type 1").Scan(&s.typeID1))
	require.NoError(t, s.db.QueryRowContext(s.ctx, upsertType, "ZZT2", "CPM List Test Type 2").Scan(&s.typeID2))

	upsertProduct := `
		INSERT INTO cost_product_master (
			cpm_product_code, cpm_product_type_id, cpm_product_name,
			cpm_shade_code, cpm_grade_code, cpm_flex_01, cpm_flex_02, cpm_flex_03,
			cpm_is_active, cpm_created_at, cpm_created_by, cpm_updated_at, cpm_updated_by
		) VALUES ($1, $2, $3, $4, 'AX', $5, $6, $7, TRUE, now(), 'itest', now(), 'itest')
		ON CONFLICT (cpm_product_code) DO UPDATE SET
			cpm_product_type_id = EXCLUDED.cpm_product_type_id,
			cpm_flex_01 = EXCLUDED.cpm_flex_01,
			cpm_flex_02 = EXCLUDED.cpm_flex_02,
			cpm_flex_03 = EXCLUDED.cpm_flex_03
		RETURNING cpm_product_sys_id`

	fixtures := []struct {
		code, name, shade, flex01, flex02, flex03 string
		typeID                                    int32
	}{
		{cpmTestSearchPrefix + "-001", "cpm itest product bravo", "SH2", "CK-B", "ZZORA9002", "LBL-B", s.typeID1},
		{cpmTestSearchPrefix + "-002", "cpm itest product alpha", "SH1", "CK-A", "ZZORA9001", "LBL-A", s.typeID1},
		{cpmTestSearchPrefix + "-003", "cpm itest product charlie", "SH3", "CK-C", "ZZORA9003", "LBL-C", s.typeID2},
	}
	s.sysIDs = s.sysIDs[:0]
	for _, fx := range fixtures {
		var sysID int64
		require.NoError(t, s.db.QueryRowContext(s.ctx, upsertProduct,
			fx.code, fx.typeID, fx.name, fx.shade, fx.flex01, fx.flex02, fx.flex03,
		).Scan(&sysID))
		s.sysIDs = append(s.sysIDs, sysID)
	}
}

func (s *CostProductMasterRepoSuite) codesOf(items []*costproductmaster.CostProductMaster) []string {
	codes := make([]string, 0, len(items))
	for _, it := range items {
		codes = append(codes, it.ProductCode())
	}
	return codes
}

func (s *CostProductMasterRepoSuite) TestList_SearchMatchesOracleSysID() {
	items, total, err := s.repo.List(s.ctx, costproductmaster.Filter{Search: "zzora9002"})
	s.Require().NoError(err)
	s.Require().Equal(int64(1), total)
	s.Require().Len(items, 1)
	s.Equal(cpmTestSearchPrefix+"-001", items[0].ProductCode())
	s.Equal("ZZORA9002", items[0].Flex02())
}

func (s *CostProductMasterRepoSuite) TestList_MultiTypeFilter() {
	base := costproductmaster.Filter{Search: cpmTestSearchPrefix}

	// Slice with both types → all three fixtures (exercises = ANY through pgx).
	f := base
	f.ProductTypeIDs = []int32{s.typeID1, s.typeID2}
	items, total, err := s.repo.List(s.ctx, f)
	s.Require().NoError(err)
	s.Equal(int64(3), total)
	s.Len(items, 3)

	// Slice with a single type → single-value equality path.
	f = base
	f.ProductTypeIDs = []int32{s.typeID2}
	items, total, err = s.repo.List(s.ctx, f)
	s.Require().NoError(err)
	s.Equal(int64(1), total)
	s.Require().Len(items, 1)
	s.Equal(cpmTestSearchPrefix+"-003", items[0].ProductCode())

	// Legacy single id unioned with the slice.
	f = base
	f.ProductTypeID = s.typeID1
	f.ProductTypeIDs = []int32{s.typeID2}
	_, total, err = s.repo.List(s.ctx, f)
	s.Require().NoError(err)
	s.Equal(int64(3), total)

	// Legacy single id alone still works.
	f = base
	f.ProductTypeID = s.typeID1
	_, total, err = s.repo.List(s.ctx, f)
	s.Require().NoError(err)
	s.Equal(int64(2), total)
}

func (s *CostProductMasterRepoSuite) TestList_NewSortKeys() {
	base := costproductmaster.Filter{Search: cpmTestSearchPrefix}

	// oracle_sys_id ascending → ZZORA9001 (product -002) first.
	f := base
	f.SortBy = "oracle_sys_id"
	items, _, err := s.repo.List(s.ctx, f)
	s.Require().NoError(err)
	s.Require().Len(items, 3)
	s.Equal([]string{
		cpmTestSearchPrefix + "-002",
		cpmTestSearchPrefix + "-001",
		cpmTestSearchPrefix + "-003",
	}, s.codesOf(items))

	// product_type_code descending → ZZT2 product first (scalar subquery sort),
	// ZZT1 products tie and fall back to product_code ASC (stable secondary).
	f = base
	f.SortBy = "product_type_code"
	f.SortOrder = "desc"
	items, _, err = s.repo.List(s.ctx, f)
	s.Require().NoError(err)
	s.Require().Len(items, 3)
	s.Equal([]string{
		cpmTestSearchPrefix + "-003",
		cpmTestSearchPrefix + "-001",
		cpmTestSearchPrefix + "-002",
	}, s.codesOf(items))

	// Remaining new keys must at least execute without SQL errors.
	for _, key := range []string{"updated_at", "shade_code", "grade_code", "erp_compound_key", "type_label", "status"} {
		f = base
		f.SortBy = key
		_, _, err = s.repo.List(s.ctx, f)
		s.Require().NoError(err, "sort by %q must not fail", key)
	}
}
