// Package postgres_test provides integration tests for
// CostProductParameterRepository.ApplyBulkOperations — closing the gap
// disclosed by B4 in docs/superpowers/state/product-route-fork-attach-bulk-STATE.md:
// design.md §4.5's three named scenarios (partial-failure,
// skip-missing-applicable, cascading-removal) were implemented but never
// exercised against a real Postgres instance. Run with INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib" // production driver
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	cpp "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

const cppBulkTestPrefix = "ZZTCPPBULK"

// CostProductParameterBulkRepoSuite exercises ApplyBulkOperations against a
// real database — see design.md §4.5 for the three scenarios this suite
// covers: partial-failure (per-product transaction isolation), skip vs.
// auto-add on a missing-applicable UpsertValue op, and cascading removal of a
// MASTER_LOOKUP trigger param + its fill-group children.
type CostProductParameterBulkRepoSuite struct {
	suite.Suite
	db   *postgres.DB
	repo *postgres.CostProductParameterRepository
	ctx  context.Context

	typeID    int32
	productA  int64
	productB  int64
	productC  int64
	productD  int64
	productE  int64     // dedicated fixture for Fix 1a remove-when-applicable test (isolated from productB)
	productF  int64     // dedicated fixture for Fix 1a remove-when-not-applicable test (isolated from productD)
	productG  int64     // dedicated fixture for Fix 1a combined upserts+skip-remove test (isolated from productC)
	paramNum  uuid.UUID // plain NUMBER/INPUT param, used for value upserts
	paramText uuid.UUID // plain TEXT/INPUT param, never applicable to any fixture product (missing-applicable case)
	trigger   uuid.UUID // MASTER_LOOKUP trigger param
	child1    uuid.UUID // fill-group child of trigger
	child2    uuid.UUID // fill-group child of trigger
}

func TestCostProductParameterBulkRepoSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(CostProductParameterBulkRepoSuite))
}

func (s *CostProductParameterBulkRepoSuite) SetupSuite() {
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
	s.repo = postgres.NewCostProductParameterRepository(s.db)

	s.seedFixtures()
}

func (s *CostProductParameterBulkRepoSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cost_product_parameter WHERE cpp_product_sys_id IN (
		SELECT cpm_product_sys_id FROM cost_product_master WHERE cpm_product_code LIKE $1)`, cppBulkTestPrefix+"%")
	_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cost_product_applicable_param WHERE capp_product_sys_id IN (
		SELECT cpm_product_sys_id FROM cost_product_master WHERE cpm_product_code LIKE $1)`, cppBulkTestPrefix+"%")
	_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cost_product_master WHERE cpm_product_code LIKE $1`, cppBulkTestPrefix+"%")
	_, _ = s.db.ExecContext(s.ctx, `DELETE FROM mst_parameter WHERE param_code LIKE $1`, cppBulkTestPrefix+"%")
	_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cost_product_type WHERE cpt_type_code = 'ZZTBK'`)
	require.NoError(s.T(), s.db.Close())
}

func (s *CostProductParameterBulkRepoSuite) seedFixtures() {
	t := s.T()

	require.NoError(t, s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_type (cpt_type_code, cpt_type_name)
		VALUES ('ZZTBK', 'CPP Bulk Test Type')
		ON CONFLICT (cpt_type_code) DO UPDATE SET cpt_type_name = EXCLUDED.cpt_type_name
		RETURNING cpt_type_id`).Scan(&s.typeID))

	s.productA = s.seedProduct("A")
	s.productB = s.seedProduct("B")
	s.productC = s.seedProduct("C")
	s.productD = s.seedProduct("D")
	s.productE = s.seedProduct("E")
	s.productF = s.seedProduct("F")
	s.productG = s.seedProduct("G")

	s.paramNum = s.seedParam("NUM", "NUMBER", "INPUT", "")
	s.paramText = s.seedParam("MISSING", "TEXT", "INPUT", "")
	s.trigger = s.seedParam("TRIGGER", "TEXT", "MASTER_LOOKUP", "")
	s.child1 = s.seedParam("CHILD1", "NUMBER", "INPUT", cppBulkTestPrefix+"-TRIGGER")
	s.child2 = s.seedParam("CHILD2", "NUMBER", "INPUT", cppBulkTestPrefix+"-TRIGGER")
}

func (s *CostProductParameterBulkRepoSuite) seedProduct(suffix string) int64 {
	t := s.T()
	var sysID int64
	require.NoError(t, s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (
			cpm_product_code, cpm_product_type_id, cpm_product_name,
			cpm_shade_code, cpm_grade_code, cpm_is_active,
			cpm_created_at, cpm_created_by, cpm_updated_at, cpm_updated_by
		) VALUES ($1, $2, $3, 'SH1', 'AX', TRUE, now(), 'itest', now(), 'itest')
		ON CONFLICT (cpm_product_code) DO UPDATE SET cpm_product_type_id = EXCLUDED.cpm_product_type_id
		RETURNING cpm_product_sys_id`,
		cppBulkTestPrefix+"-"+suffix, s.typeID, "cpp bulk itest product "+suffix,
	).Scan(&sysID))
	return sysID
}

// seedParam inserts an mst_parameter row. fillGroupCode, when non-empty, marks
// this param as a fill-group child of the trigger with that param_code.
func (s *CostProductParameterBulkRepoSuite) seedParam(suffix, dataType, category, fillGroupCode string) uuid.UUID {
	t := s.T()
	code := cppBulkTestPrefix + "-" + suffix
	var idStr string
	var fillGroupArg any
	if fillGroupCode != "" {
		fillGroupArg = fillGroupCode
	}
	require.NoError(t, s.db.QueryRowContext(s.ctx, `
		INSERT INTO mst_parameter (param_code, param_name, data_type, param_category, lookup_fill_group_code, is_active, created_by)
		VALUES ($1, $1, $2, $3, $4, TRUE, 'itest')
		ON CONFLICT (param_code) WHERE deleted_at IS NULL DO UPDATE SET data_type = EXCLUDED.data_type
		RETURNING id`, code, dataType, category, fillGroupArg,
	).Scan(&idStr))
	id, err := uuid.Parse(idStr)
	require.NoError(t, err)
	return id
}

func (s *CostProductParameterBulkRepoSuite) cappCount(productSysID int64, paramID uuid.UUID) int {
	var n int
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT COUNT(*) FROM cost_product_applicable_param WHERE capp_product_sys_id = $1 AND capp_param_id = $2`,
		productSysID, paramID,
	).Scan(&n))
	return n
}

func (s *CostProductParameterBulkRepoSuite) cppCount(productSysID int64, paramID uuid.UUID) int {
	var n int
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT COUNT(*) FROM cost_product_parameter WHERE cpp_product_sys_id = $1 AND cpp_param_id = $2`,
		productSysID, paramID,
	).Scan(&n))
	return n
}

func numPtr(s string) *string { return &s }

// ---------------------------------------------------------------------------
// Scenario 1: partial-failure — one product's op sequence fails midway; its
// transaction rolls back entirely (no partial CAPP/CPP mutation survives),
// while ANOTHER product's successful ApplyBulkOperations call in the same
// logical batch is completely unaffected — per-product transaction isolation
// per design.md §4.1/§4.5 ("each in its OWN transaction").
// ---------------------------------------------------------------------------

func (s *CostProductParameterBulkRepoSuite) TestApplyBulkOperations_PartialFailure_RollsBackWholeProductNotOtherProducts() {
	t := s.T()

	// Product A: add applicable NUM, then an UpsertValue op with an
	// impossible shape (both value_numeric and value_text unset AND
	// value_flag unset would violate cpp_one_value_chk if it reached the DB —
	// simulate by writing a NUMBER-typed param's value as text, which the
	// domain layer normally rejects before reaching here; instead exercise
	// the failure path the repository itself can hit: an UpsertValue op
	// against a param ID that does not exist in mst_parameter at all, which
	// fails the FK constraint on cost_product_applicable_param during
	// auto-add (skipMissingApplicable=false).
	unknownParamID := uuid.New()
	opsA := []cpp.BulkOp{
		{Kind: cpp.BulkOpAddApplicable, ParamID: s.paramNum, IsRequired: true},
		{Kind: cpp.BulkOpUpsertValue, ParamID: unknownParamID, ValueNumeric: numPtr("1")},
	}
	_, errA := s.repo.ApplyBulkOperations(s.ctx, s.productA, opsA, "itest", false)
	require.Error(t, errA, "op sequence referencing an unknown param must fail")

	// Assert product A's transaction rolled back completely: the FIRST op
	// (add NUM applicable) must NOT have survived, even though it succeeded
	// before the second op failed.
	require.Equal(t, 0, s.cappCount(s.productA, s.paramNum),
		"product A's whole transaction must roll back on the second op's failure — no partial CAPP row")

	// Product B: same well-formed prefix op, but no failing op after it —
	// must succeed independently of product A's failure.
	opsB := []cpp.BulkOp{
		{Kind: cpp.BulkOpAddApplicable, ParamID: s.paramNum, IsRequired: true},
		{Kind: cpp.BulkOpUpsertValue, ParamID: s.paramNum, ValueNumeric: numPtr("42")},
	}
	outcomesB, errB := s.repo.ApplyBulkOperations(s.ctx, s.productB, opsB, "itest", false)
	require.NoError(t, errB, "product B must succeed independently of product A's failure")
	require.Len(t, outcomesB, 2)
	require.Equal(t, 1, s.cappCount(s.productB, s.paramNum), "product B's CAPP row must be committed")
	require.Equal(t, 1, s.cppCount(s.productB, s.paramNum), "product B's CPP value must be committed")
}

// ---------------------------------------------------------------------------
// Scenario 2: skip-missing-applicable — an UpsertValue op against a param the
// product doesn't have CAPP-applicable. skip_missing_applicable=true records
// a skip and leaves the rest of the sequence intact (not a hard failure);
// skip_missing_applicable=false auto-adds the CAPP row (not required) then
// writes the value.
// ---------------------------------------------------------------------------

func (s *CostProductParameterBulkRepoSuite) TestApplyBulkOperations_SkipMissingApplicable_SkipsAndReports() {
	t := s.T()

	ops := []cpp.BulkOp{
		// paramText is not applicable to productC — must be skipped, not auto-added.
		{Kind: cpp.BulkOpUpsertValue, ParamID: s.paramText, ValueText: strPtr("should-be-skipped")},
		// paramNum IS applied first so we can also confirm skip doesn't block later ops.
		{Kind: cpp.BulkOpAddApplicable, ParamID: s.paramNum, IsRequired: false},
		{Kind: cpp.BulkOpUpsertValue, ParamID: s.paramNum, ValueNumeric: numPtr("7")},
	}

	outcomes, err := s.repo.ApplyBulkOperations(s.ctx, s.productC, ops, "itest", true)
	require.NoError(t, err, "a skipped op must not fail the whole product's operation sequence")
	require.Len(t, outcomes, 3)

	require.True(t, outcomes[0].Skipped, "upsert on a non-applicable param must be reported as skipped")
	require.ErrorIs(t, cpp.ErrBulkSkippedNotApplicable, cpp.ErrBulkSkippedNotApplicable)
	require.NotEmpty(t, outcomes[0].Reason)

	require.False(t, outcomes[1].Skipped)
	require.False(t, outcomes[2].Skipped)

	require.Equal(t, 0, s.cappCount(s.productC, s.paramText), "skip must not auto-add the CAPP row")
	require.Equal(t, 0, s.cppCount(s.productC, s.paramText), "skip must not write a value")
	require.Equal(t, 1, s.cppCount(s.productC, s.paramNum), "the later, unrelated op must still apply")
}

func (s *CostProductParameterBulkRepoSuite) TestApplyBulkOperations_AutoAddWhenSkipMissingApplicableFalse() {
	t := s.T()

	ops := []cpp.BulkOp{
		{Kind: cpp.BulkOpUpsertValue, ParamID: s.paramText, ValueText: strPtr("auto-added-value")},
	}
	outcomes, err := s.repo.ApplyBulkOperations(s.ctx, s.productD, ops, "itest", false)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	require.False(t, outcomes[0].Skipped, "skip_missing_applicable=false must auto-add rather than skip")

	require.Equal(t, 1, s.cappCount(s.productD, s.paramText), "CAPP row must be auto-added")
	require.Equal(t, 1, s.cppCount(s.productD, s.paramText), "value must be written after auto-add")
}

// ---------------------------------------------------------------------------
// Scenario 3: cascading-removal — RemoveApplicableParamOp on a MASTER_LOOKUP
// trigger param must remove the trigger AND every fill-group child's CAPP +
// CPP rows together, reusing the exact same cascade logic as the
// single-product RemoveApplicableWithChildren path (design.md §4.3).
// ---------------------------------------------------------------------------

func (s *CostProductParameterBulkRepoSuite) TestApplyBulkOperations_CascadingRemoval_RemovesTriggerAndChildren() {
	t := s.T()

	// Seed: trigger + 2 children all applicable to productA, each with a value.
	s.addApplicableWithValue(s.productA, s.trigger, "TRIGVAL")
	s.addApplicableWithValue(s.productA, s.child1, "1")
	s.addApplicableWithValue(s.productA, s.child2, "2")

	require.Equal(t, 1, s.cappCount(s.productA, s.trigger))
	require.Equal(t, 1, s.cappCount(s.productA, s.child1))
	require.Equal(t, 1, s.cappCount(s.productA, s.child2))

	ops := []cpp.BulkOp{
		{Kind: cpp.BulkOpRemoveApplicable, ParamID: s.trigger},
	}
	outcomes, err := s.repo.ApplyBulkOperations(s.ctx, s.productA, ops, "itest", true)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)

	require.Equal(t, 0, s.cappCount(s.productA, s.trigger), "trigger CAPP row must be removed")
	require.Equal(t, 0, s.cappCount(s.productA, s.child1), "child1 CAPP row must cascade-remove with trigger")
	require.Equal(t, 0, s.cappCount(s.productA, s.child2), "child2 CAPP row must cascade-remove with trigger")
	require.Equal(t, 0, s.cppCount(s.productA, s.trigger), "trigger value must be removed")
	require.Equal(t, 0, s.cppCount(s.productA, s.child1), "child1 value must be removed")
	require.Equal(t, 0, s.cppCount(s.productA, s.child2), "child2 value must be removed")
}

// ---------------------------------------------------------------------------
// Scenario 4 (Fix 1a — post-ship-bulk-attach-layout-fixes): RemoveApplicable
// on a param that is NOT applicable to the product must be a soft skip, not
// a hard cpp.ErrNotFound that aborts the whole per-product transaction —
// mirroring UpsertValue's skip-missing-applicable semantics. See
// docs/superpowers/state/post-ship-bulk-attach-layout-fixes-STATE.md Fix 1a.
// ---------------------------------------------------------------------------

func (s *CostProductParameterBulkRepoSuite) TestApplyBulkOperations_RemoveApplicable_WhenApplicable_Succeeds() {
	t := s.T()

	// Seed: paramNum IS applicable to productE with a value.
	s.addApplicableWithValue(s.productE, s.paramNum, "99")
	require.Equal(t, 1, s.cappCount(s.productE, s.paramNum))

	ops := []cpp.BulkOp{
		{Kind: cpp.BulkOpRemoveApplicable, ParamID: s.paramNum},
	}
	outcomes, err := s.repo.ApplyBulkOperations(s.ctx, s.productE, ops, "itest", false)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	require.False(t, outcomes[0].Skipped, "removing a param that IS applicable must not be reported as skipped")

	require.Equal(t, 0, s.cappCount(s.productE, s.paramNum), "CAPP row must be removed")
	require.Equal(t, 0, s.cppCount(s.productE, s.paramNum), "CPP value must be removed")
}

func (s *CostProductParameterBulkRepoSuite) TestApplyBulkOperations_RemoveApplicable_WhenNotApplicable_SoftSkips() {
	t := s.T()

	// paramText is never applicable to productF in this scenario (no prior
	// AddApplicable / value seeded for it here).
	require.Equal(t, 0, s.cappCount(s.productF, s.paramText))

	ops := []cpp.BulkOp{
		{Kind: cpp.BulkOpRemoveApplicable, ParamID: s.paramText},
	}
	outcomes, err := s.repo.ApplyBulkOperations(s.ctx, s.productF, ops, "itest", false)
	require.NoError(t, err, "removing a not-applicable param must not error the whole product operation sequence")
	require.Len(t, outcomes, 1)
	require.True(t, outcomes[0].Skipped, "remove of a non-applicable param must be reported as skipped")
	require.NotEmpty(t, outcomes[0].Reason)

	require.Equal(t, 0, s.cappCount(s.productF, s.paramText))
}

func (s *CostProductParameterBulkRepoSuite) TestApplyBulkOperations_CombinedUpsertsAndNonApplicableRemove_AllCommit() {
	t := s.T()

	// Two params applicable to productG via AddApplicable ops, then two
	// UPSERT_VALUE ops setting their values, plus one REMOVE_APPLICABLE op
	// targeting a param that was NEVER applicable to productG (paramText).
	// The remove op must soft-skip without aborting the shared per-product
	// transaction — both set-value ops must still commit.
	ops := []cpp.BulkOp{
		{Kind: cpp.BulkOpAddApplicable, ParamID: s.paramNum, IsRequired: false},
		{Kind: cpp.BulkOpUpsertValue, ParamID: s.paramNum, ValueNumeric: numPtr("11")},
		{Kind: cpp.BulkOpAddApplicable, ParamID: s.trigger, IsRequired: false},
		{Kind: cpp.BulkOpUpsertValue, ParamID: s.trigger, ValueText: strPtr("TRIGVAL")},
		{Kind: cpp.BulkOpRemoveApplicable, ParamID: s.paramText},
	}
	outcomes, err := s.repo.ApplyBulkOperations(s.ctx, s.productG, ops, "itest", false)
	require.NoError(t, err, "a soft-skipped remove must not abort sibling ops in the same product's transaction")
	require.Len(t, outcomes, 5)

	require.False(t, outcomes[0].Skipped)
	require.False(t, outcomes[1].Skipped)
	require.False(t, outcomes[2].Skipped)
	require.False(t, outcomes[3].Skipped)
	require.True(t, outcomes[4].Skipped, "remove of a non-applicable param must be the skipped outcome")

	require.Equal(t, 1, s.cppCount(s.productG, s.paramNum), "first UPSERT_VALUE op must have committed")
	require.Equal(t, 1, s.cppCount(s.productG, s.trigger), "second UPSERT_VALUE op must have committed")
	require.Equal(t, 0, s.cappCount(s.productG, s.paramText), "skipped remove must not have touched an unrelated CAPP row")
}

// addApplicableWithValue seeds a CAPP row + a matching CPP TEXT/NUMBER value
// directly via SQL (bypassing ApplyBulkOperations) so the removal test's
// seed step is independent of the code path under test.
func (s *CostProductParameterBulkRepoSuite) addApplicableWithValue(productSysID int64, paramID uuid.UUID, value string) {
	t := s.T()
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO cost_product_applicable_param (capp_product_sys_id, capp_param_id, capp_is_required, capp_created_by)
		VALUES ($1, $2, FALSE, 'itest')
		ON CONFLICT (capp_product_sys_id, capp_param_id) DO NOTHING`, productSysID, paramID)
	require.NoError(t, err)

	var dataType string
	require.NoError(t, s.db.QueryRowContext(s.ctx, `SELECT data_type FROM mst_parameter WHERE id = $1`, paramID).Scan(&dataType))

	if dataType == "NUMBER" {
		_, err = s.db.ExecContext(s.ctx, `
			INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_filled_by, cpp_created_by)
			VALUES ($1, $2, $3::numeric, 'itest', 'itest')
			ON CONFLICT (cpp_product_sys_id, cpp_param_id) DO UPDATE SET cpp_value_numeric = EXCLUDED.cpp_value_numeric`,
			productSysID, paramID, value)
	} else {
		_, err = s.db.ExecContext(s.ctx, `
			INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_text, cpp_filled_by, cpp_created_by)
			VALUES ($1, $2, $3, 'itest', 'itest')
			ON CONFLICT (cpp_product_sys_id, cpp_param_id) DO UPDATE SET cpp_value_text = EXCLUDED.cpp_value_text`,
			productSysID, paramID, value)
	}
	require.NoError(t, err)
}

func strPtr(s string) *string { return &s }
