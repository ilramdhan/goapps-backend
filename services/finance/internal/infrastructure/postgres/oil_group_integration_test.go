// Integration coverage for oil-cost-rm-group P4 repositories: OilGroupPolicyRepository,
// the RM_GROUP_OIL restriction in ListMasterOptionsInCodes, the ETL
// RejectDisallowedOilGroups staging SQL, the product-type oil config repo and the RM
// group is_oil_group column. Gated by INTEGRATION_TEST=true.
//
// SAFETY: every fixture row uses the ITOIL prefix (group codes ITOIL-*, type codes
// derived from a random suffix, product codes ITOIL-*) and is hard-deleted in
// TearDownTest. Real oil groups/types are never modified.
package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand/v2"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	cptdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

type OilGroupIntegrationSuite struct {
	suite.Suite
	ctx context.Context
	raw *sql.DB
	db  *postgres.DB

	suffix     string
	oilTypeID  int32 // oil class PTY, mapped to groupA (default)
	noOilType  int32 // no oil class
	groupA     uuid.UUID
	groupACode string
	groupB     uuid.UUID // oil group, not mapped to oilTypeID
	groupBCode string
	nonOil     uuid.UUID
	nonOilCode string
	oilProduct int64
	noOilProd  int64
	legacyID   string
}

func TestOilGroupIntegrationSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(OilGroupIntegrationSuite))
}

func (s *OilGroupIntegrationSuite) SetupSuite() {
	s.ctx = context.Background()
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnvOrDefault("TEST_DB_HOST", "localhost"), getEnvOrDefault("TEST_DB_PORT", "5434"),
		getEnvOrDefault("TEST_DB_USER", "finance"), getEnvOrDefault("TEST_DB_PASSWORD", "finance123"),
		getEnvOrDefault("TEST_DB_NAME", "finance_db"))
	raw, err := sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), waitForDB(raw, 10*time.Second))
	s.raw = raw
	s.db = postgres.NewDBFromSQL(raw)
}

func (s *OilGroupIntegrationSuite) TearDownSuite() {
	if s.raw != nil {
		require.NoError(s.T(), s.raw.Close())
	}
}

func (s *OilGroupIntegrationSuite) exec(q string, args ...any) {
	_, err := s.raw.ExecContext(s.ctx, q, args...)
	require.NoError(s.T(), err, q)
}

func (s *OilGroupIntegrationSuite) insertGroup(code string, oil bool) uuid.UUID {
	id := uuid.New()
	s.exec(`INSERT INTO cst_rm_group_head (group_head_id, group_code, group_name, created_by, is_oil_group)
	        VALUES ($1, $2, $3, 'itest-oil', $4)`, id, code, code+" NAME", oil)
	return id
}

func (s *OilGroupIntegrationSuite) insertType(code string, oilClass *string) int32 {
	var id int32
	require.NoError(s.T(), s.raw.QueryRowContext(s.ctx,
		`INSERT INTO cost_product_type (cpt_type_code, cpt_type_name, cpt_oil_class)
		 VALUES ($1, 'itest oil type', $2) RETURNING cpt_type_id`, code, oilClass).Scan(&id))
	return id
}

func (s *OilGroupIntegrationSuite) insertProduct(code string, typeID int32, legacy *string) int64 {
	var id int64
	require.NoError(s.T(), s.raw.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name,
		                                 cpm_flex_02, cpm_created_by, cpm_updated_by)
		VALUES ($1, $2, 'itest oil product', $3, 'itest-oil', 'itest-oil')
		RETURNING cpm_product_sys_id`, code, typeID, legacy).Scan(&id))
	return id
}

func (s *OilGroupIntegrationSuite) SetupTest() {
	n := rand.IntN(900) + 100 //nolint:gosec // test fixture suffix, not security relevant
	s.suffix = fmt.Sprintf("%d", n)
	pty := "PTY"
	s.oilTypeID = s.insertType("Q"+s.suffix, &pty)
	s.noOilType = s.insertType("R"+s.suffix, nil)
	s.groupACode, s.groupBCode, s.nonOilCode = "ITOIL-A"+s.suffix, "ITOIL-B"+s.suffix, "ITOIL-N"+s.suffix
	s.groupA = s.insertGroup(s.groupACode, true)
	s.groupB = s.insertGroup(s.groupBCode, true)
	s.nonOil = s.insertGroup(s.nonOilCode, false)
	s.exec(`INSERT INTO cost_product_type_oil_group (cptog_type_id, cptog_group_head_id, cptog_is_default, cptog_created_by)
	        VALUES ($1, $2, TRUE, 'itest-oil')`, s.oilTypeID, s.groupA)
	s.legacyID = "ITOIL-L" + s.suffix
	s.oilProduct = s.insertProduct("ITOIL-P"+s.suffix, s.oilTypeID, &s.legacyID)
	s.noOilProd = s.insertProduct("ITOIL-Q"+s.suffix, s.noOilType, nil)
}

func (s *OilGroupIntegrationSuite) TearDownTest() {
	s.exec(`DELETE FROM stg_import_error WHERE job_id = -424242`)
	s.exec(`DELETE FROM stg_import_product_parameter WHERE job_id = -424242`)
	s.exec(`DELETE FROM stg_import_product_master WHERE job_id = -424242`)
	s.exec(`DELETE FROM cost_product_master WHERE cpm_product_code LIKE 'ITOIL-%'`)
	s.exec(`DELETE FROM cost_product_type_oil_group WHERE cptog_type_id IN ($1, $2)`, s.oilTypeID, s.noOilType)
	s.exec(`DELETE FROM aud_rm_group WHERE group_code LIKE 'ITOIL-%'`)
	s.exec(`DELETE FROM cst_rm_group_head WHERE group_code LIKE 'ITOIL-%'`)
	s.exec(`DELETE FROM cost_product_type WHERE cpt_type_id IN ($1, $2)`, s.oilTypeID, s.noOilType)
}

func (s *OilGroupIntegrationSuite) TestPolicy_RulesForProducts() {
	repo := postgres.NewOilGroupPolicyRepository(s.db)
	rules, err := repo.RulesForProducts(s.ctx, []int64{s.oilProduct, s.noOilProd, -1})
	require.NoError(s.T(), err)
	require.Len(s.T(), rules, 1)
	r := rules[s.oilProduct]
	require.NotNil(s.T(), r)
	s.Equal("Q"+s.suffix, r.TypeCode)
	s.Equal("PTY", r.OilClass)
	s.Equal([]string{s.groupACode}, r.Allowed)
	s.Equal(s.groupACode, r.Default)

	single, err := repo.RuleForProduct(s.ctx, s.noOilProd)
	require.NoError(s.T(), err)
	s.Nil(single)

	// An un-flagged group drops out of the allowed set (view v_rm_group_oil).
	s.exec(`UPDATE cst_rm_group_head SET is_oil_group = FALSE WHERE group_head_id = $1`, s.groupA)
	r, err = repo.RuleForProduct(s.ctx, s.oilProduct)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), r)
	s.Empty(r.Allowed)
	s.Empty(r.Default)
}

func (s *OilGroupIntegrationSuite) TestLookup_ListMasterOptionsInCodes() {
	repo := postgres.NewLookupMasterRepository(s.db)
	opts, err := repo.ListMasterOptionsInCodes(s.ctx, "RM_GROUP_OIL", "ITOIL-", -1, []string{s.groupACode})
	require.NoError(s.T(), err)
	require.Len(s.T(), opts, 1)
	s.Equal(s.groupACode, opts[0].Value)

	all, err := repo.ListMasterOptionsInCodes(s.ctx, "RM_GROUP_OIL", "ITOIL-", -1, nil)
	require.NoError(s.T(), err)
	var codes []string
	for _, o := range all {
		codes = append(codes, o.Value)
	}
	s.ElementsMatch([]string{s.groupACode, s.groupBCode}, codes, "non-oil group must never be an option")

	none, err := repo.ListMasterOptionsInCodes(s.ctx, "RM_GROUP_OIL", "ITOIL-", -1, []string{})
	require.NoError(s.T(), err)
	s.Empty(none)
}

func (s *OilGroupIntegrationSuite) TestETL_RejectDisallowedOilGroups() {
	const job = int64(-424242)
	ins := `INSERT INTO stg_import_product_parameter (job_id, row_num, legacy_oracle_sys_id, param_code, value_text)
	        VALUES ($1, $2, $3, $4, $5)`
	s.exec(ins, job, 2, s.legacyID, "OIL_NAME", s.groupACode)      // allowed
	s.exec(ins, job, 3, s.legacyID, "OIL_NAME", s.groupBCode)      // oil group, not mapped → rejected
	s.exec(ins, job, 4, s.legacyID, "OIL_NAME", "")                // blank → left alone
	s.exec(ins, job, 5, s.legacyID, "COLOR", s.groupBCode)         // other param → left alone
	s.exec(ins, job, 6, "ITOIL-UNKNOWN", "OIL_NAME", s.groupBCode) // unknown product → left for Layer 2

	repo := postgres.NewCostImportStagingRepository(s.db)
	removed, err := repo.RejectDisallowedOilGroups(s.ctx, job)
	require.NoError(s.T(), err)
	s.Equal(1, removed)

	var left int
	require.NoError(s.T(), s.raw.QueryRowContext(s.ctx,
		`SELECT COUNT(*) FROM stg_import_product_parameter WHERE job_id = $1`, job).Scan(&left))
	s.Equal(4, left)

	var rowNum int
	var msg string
	require.NoError(s.T(), s.raw.QueryRowContext(s.ctx,
		`SELECT row_num, error_message FROM stg_import_error WHERE job_id = $1`, job).Scan(&rowNum, &msg))
	s.Equal(3, rowNum)
	s.Equal(fmt.Sprintf(`OIL_NAME "%s" is not allowed for product type Q%s; allowed: %s`, s.groupBCode, s.suffix, s.groupACode), msg)

	// A staged product master row re-typing the product to a non-oil type wins.
	s.exec(`INSERT INTO stg_import_product_master (job_id, row_num, legacy_oracle_sys_id, product_type_code)
	        VALUES ($1, 2, $2, $3)`, job, s.legacyID, "R"+s.suffix)
	s.exec(ins, job, 7, s.legacyID, "OIL_NAME", s.groupBCode)
	removed, err = repo.RejectDisallowedOilGroups(s.ctx, job)
	require.NoError(s.T(), err)
	s.Equal(0, removed)
}

func (s *OilGroupIntegrationSuite) TestProductTypeOilConfig() {
	repo := postgres.NewCostProductTypeRepository(s.db)
	cfg, err := repo.GetOilConfig(s.ctx, s.oilTypeID)
	require.NoError(s.T(), err)
	s.Equal("PTY", cfg.OilClass)
	require.Len(s.T(), cfg.Groups, 1)
	s.True(cfg.Groups[0].IsDefault)

	t, err := repo.GetByID(s.ctx, s.oilTypeID)
	require.NoError(s.T(), err)
	s.Equal("PTY", t.OilClass())

	resolved, err := repo.ResolveOilGroups(s.ctx, []string{s.groupACode, s.groupBCode, s.nonOilCode})
	require.NoError(s.T(), err)
	s.Len(resolved, 2)
	s.NotContains(resolved, s.nonOilCode)

	require.NoError(s.T(), repo.ReplaceOilConfig(s.ctx, s.oilTypeID, "POY", []cptdomain.ReplaceOilGroup{
		{GroupHeadID: s.groupA.String(), IsDefault: false},
		{GroupHeadID: s.groupB.String(), IsDefault: true},
	}, "itest-oil"))
	cfg, err = repo.GetOilConfig(s.ctx, s.oilTypeID)
	require.NoError(s.T(), err)
	s.Equal("POY", cfg.OilClass)
	require.Len(s.T(), cfg.Groups, 2)
	s.Equal(s.groupBCode, cfg.Groups[0].GroupCode, "default listed first")

	require.NoError(s.T(), repo.ReplaceOilConfig(s.ctx, s.oilTypeID, "", nil, "itest-oil"))
	cfg, err = repo.GetOilConfig(s.ctx, s.oilTypeID)
	require.NoError(s.T(), err)
	s.Empty(cfg.OilClass)
	s.Empty(cfg.Groups)

	_, err = repo.GetOilConfig(s.ctx, -1)
	s.ErrorIs(err, cptdomain.ErrNotFound)
}

func (s *OilGroupIntegrationSuite) TestRMGroup_OilFlag() {
	repo := postgres.NewRMGroupRepository(s.db)
	head, err := repo.GetHeadByID(s.ctx, s.groupA)
	require.NoError(s.T(), err)
	s.True(head.IsOilGroup())

	inUse, err := repo.IsOilGroupInUse(s.ctx, s.groupA)
	require.NoError(s.T(), err)
	s.True(inUse)
	inUse, err = repo.IsOilGroupInUse(s.ctx, s.groupB)
	require.NoError(s.T(), err)
	s.False(inUse)

	nonOil, err := repo.GetHeadByID(s.ctx, s.nonOil)
	require.NoError(s.T(), err)
	s.False(nonOil.IsOilGroup())
	nonOil.SetOilGroup(true)
	require.NoError(s.T(), nonOil.Update(rmgroup.UpdateInput{}, "itest-oil"))
	require.NoError(s.T(), repo.UpdateHead(s.ctx, nonOil))
	reloaded, err := repo.GetHeadByID(s.ctx, s.nonOil)
	require.NoError(s.T(), err)
	s.True(reloaded.IsOilGroup())

	yes := true
	f := rmgroup.NewListFilter()
	f.Search = "ITOIL-"
	f.IsOilGroup = &yes
	heads, total, err := repo.ListHeads(s.ctx, f)
	require.NoError(s.T(), err)
	s.Equal(int64(3), total)
	s.Len(heads, 3)

	code, err := rmgroup.NewCode("ITOIL-C" + s.suffix)
	require.NoError(s.T(), err)
	created, err := rmgroup.NewHead(code, "created oil", "", 0, 0, "itest-oil")
	require.NoError(s.T(), err)
	created.SetOilGroup(true)
	require.NoError(s.T(), repo.CreateHead(s.ctx, created))
	got, err := repo.GetHeadByID(s.ctx, created.ID())
	require.NoError(s.T(), err)
	s.True(got.IsOilGroup())
}
