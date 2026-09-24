package costcalc

// Integration coverage for P6-T3 of the oil-cost-rm-group plan: exercises the
// REAL engine path for OIL_RATE / OIL_COST / OIL_GAIN against a fully migrated
// (>= 000526) throwaway database — not mocks/fakes:
//   - LoadOilContext / LoadRMCosts: the actual productLoader SQL (loader_oil.go,
//     loader.go), run against real fixture rows.
//   - resolveOilRate / injectProductClassFlags: the actual unexported engine
//     functions (oil_rate.go), called directly (white-box, same package).
//   - F_YARN_OIL_COST / F_YARN_OIL_GAIN / F_YARN_OIL_GAIN_POY_DEFAULT: the
//     ACTUAL expression strings read live from mst_formula (migration 000524),
//     compiled and run through the real evaluator package — not hand-copied
//     literals in the test.
//
// This deliberately does not build full cost-route graphs and drive the
// whole-product ProcessChunk pipeline (see process_chunk_test.go's
// ProcessChunkSuite for that, which explicitly sidesteps oil-class types to
// avoid needing oil RM cost fixtures). The oil resolution/formula logic is
// self-contained enough that exercising it directly against the real DB and
// real formula text gives equivalent confidence for spec §6 E1-E4 without the
// unrelated route/CAPP scaffolding.
//
// SAFETY: every fixture uses a unique OFE- prefixed code (see uniqueCodePrefix)
// and is hard-deleted in TearDownSuite. The two real oil RM groups
// (202006101/202006077) are never touched — this suite creates its OWN
// throwaway oil groups and its own product-type oil-group mapping rows,
// mirroring exactly what a fresh/CI database needs (see the "manual oil
// config" note in the deploy runbook): migrations 000520/000521 only seed
// is_oil_group / cost_product_type_oil_group for group codes 202006101 and
// 202006077 IF those rows already exist at migration time. A throwaway DB
// with no such rows gets neither seed (guarded no-op), so this suite recreates
// that mapping by hand for its own throwaway codes.
import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

type OilFormulaE2ESuite struct {
	suite.Suite
	ctx    context.Context
	raw    *sql.DB
	loader ProductLoader
	cache  *evaluator.Cache

	period     string
	prefix     string
	groupOil1  uuid.UUID // 202006101-equivalent: PTY's default oil group
	groupOil1C string
	groupOil2  uuid.UUID // 202006077-equivalent: POY + Superba's default oil group
	groupOil2C string

	ptyType, poyType, tcsType, dtyType int32
	ptyProduct, poyProduct, tcsProduct, dtyProduct int64
}

func TestOilFormulaE2ESuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(OilFormulaE2ESuite))
}

func (s *OilFormulaE2ESuite) SetupSuite() {
	s.ctx = context.Background()
	s.period = "999970"
	s.prefix = uniqueCodePrefix(s.T(), "OF")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		envOr("TEST_DB_HOST", "localhost"), envOr("TEST_DB_PORT", "5434"),
		envOr("TEST_DB_USER", "finance"), envOr("TEST_DB_PASSWORD", "finance123"),
		envOr("TEST_DB_NAME", "finance_db"))
	raw, err := sql.Open("postgres", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), waitDB(raw, 10*time.Second))
	s.raw = raw
	s.loader = NewProductLoader(raw)
	s.cache = evaluator.NewCache()

	s.seed()
}

func (s *OilFormulaE2ESuite) TearDownSuite() {
	if s.raw == nil {
		return
	}
	_, _ = s.raw.ExecContext(s.ctx, `DELETE FROM cst_rm_cost WHERE rm_code LIKE $1`, s.prefix+"-%")
	_, _ = s.raw.ExecContext(s.ctx, `DELETE FROM cost_product_master WHERE cpm_product_code LIKE $1`, s.prefix+"-%")
	_, _ = s.raw.ExecContext(s.ctx, `DELETE FROM cost_product_type_oil_group WHERE cptog_type_id IN ($1, $2, $3)`,
		s.ptyType, s.poyType, s.tcsType)
	_, _ = s.raw.ExecContext(s.ctx, `DELETE FROM cst_rm_group_head WHERE group_code LIKE $1`, strings.ToUpper(s.prefix)+"-%")
	_, _ = s.raw.ExecContext(s.ctx, `DELETE FROM cost_product_type WHERE cpt_type_id IN ($1, $2, $3, $4)`,
		s.ptyType, s.poyType, s.tcsType, s.dtyType)
	_ = s.raw.Close()
}

// SetupTest clears any cst_rm_cost rows this suite's tests may have left for
// its two throwaway oil groups at s.period, so each test starts from a clean
// slate regardless of testify's (lexicographic-by-method-name) run order —
// e.g. TestMissingOilRow_PTY_Blocked must never see a row TestE1_PTY inserted.
func (s *OilFormulaE2ESuite) SetupTest() {
	_, err := s.raw.ExecContext(s.ctx,
		`DELETE FROM cst_rm_cost WHERE rm_code IN ($1, $2) AND period = $3`,
		s.groupOil1C, s.groupOil2C, s.period)
	require.NoError(s.T(), err)
}

// seed builds: two throwaway oil RM groups (playing the role of 202006101 /
// 202006077), three oil-class product TYPES (PTY/POY/SUPERBA, mirroring the
// real PTY/POY/TCS mapping) each with its own default-group mapping, and one
// non-oil type (DTY stand-in). Real cst_rm_group_head rows and real
// cost_product_type_oil_group rows — this is exactly the manual step an
// operator does on a DB where 202006101/202006077 don't pre-exist.
func (s *OilFormulaE2ESuite) seed() {
	// cst_rm_group_head.group_code must match chk_rm_group_code_format
	// (^[A-Z0-9][A-Z0-9 \-]{0,29}$) -- uppercase only, no lowercase hex.
	upperPrefix := strings.ToUpper(s.prefix)
	s.groupOil1C = upperPrefix + "-CONING"
	s.groupOil2C = upperPrefix + "-SPINFIN"
	s.groupOil1 = s.insertOilGroup(s.groupOil1C)
	s.groupOil2 = s.insertOilGroup(s.groupOil2C)

	// cost_product_type.cpt_type_code is varchar(5) -- too short for the
	// full uniqueCodePrefix; use a short single-letter + 3-digit suffix
	// instead (same scheme as oil_group_integration_test.go's ITOIL suite).
	n := rand.IntN(900) + 100 //nolint:gosec // test fixture suffix, not security relevant
	typeSuffix := fmt.Sprintf("%d", n)
	s.ptyType = s.insertType("P"+typeSuffix, "PTY")
	s.poyType = s.insertType("O"+typeSuffix, "POY")
	s.tcsType = s.insertType("S"+typeSuffix, "SUPERBA")
	s.dtyType = s.insertType("D"+typeSuffix, "") // no oil class, DTY stand-in

	s.insertOilGroupMapping(s.ptyType, s.groupOil1, true)
	s.insertOilGroupMapping(s.poyType, s.groupOil2, true)
	s.insertOilGroupMapping(s.tcsType, s.groupOil2, true)

	s.ptyProduct = s.insertProduct("PTYP", s.ptyType)
	s.poyProduct = s.insertProduct("POYP", s.poyType)
	s.tcsProduct = s.insertProduct("TCSP", s.tcsType)
	s.dtyProduct = s.insertProduct("DTYP", s.dtyType)
}

func (s *OilFormulaE2ESuite) insertOilGroup(code string) uuid.UUID {
	id := uuid.New()
	_, err := s.raw.ExecContext(s.ctx, `
		INSERT INTO cst_rm_group_head (group_head_id, group_code, group_name, created_by, is_oil_group)
		VALUES ($1, $2, $3, 'oil-formula-e2e', TRUE)`, id, code, code+" NAME")
	require.NoError(s.T(), err)
	return id
}

func (s *OilFormulaE2ESuite) insertType(code, oilClass string) int32 {
	var id int32
	var cls any
	if oilClass != "" {
		cls = oilClass
	}
	require.NoError(s.T(), s.raw.QueryRowContext(s.ctx,
		`INSERT INTO cost_product_type (cpt_type_code, cpt_type_name, cpt_oil_class)
		 VALUES ($1, 'oil formula e2e type', $2) RETURNING cpt_type_id`, code, cls).Scan(&id))
	return id
}

func (s *OilFormulaE2ESuite) insertOilGroupMapping(typeID int32, groupID uuid.UUID, isDefault bool) {
	_, err := s.raw.ExecContext(s.ctx, `
		INSERT INTO cost_product_type_oil_group (cptog_type_id, cptog_group_head_id, cptog_is_default, cptog_created_by)
		VALUES ($1, $2, $3, 'oil-formula-e2e')`, typeID, groupID, isDefault)
	require.NoError(s.T(), err)
}

func (s *OilFormulaE2ESuite) insertProduct(suffix string, typeID int32) int64 {
	var id int64
	require.NoError(s.T(), s.raw.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_created_by, cpm_updated_by)
		VALUES ($1, $2, 'oil formula e2e product', 'oil-formula-e2e', 'oil-formula-e2e')
		RETURNING cpm_product_sys_id`, s.prefix+"-"+suffix, typeID).Scan(&id))
	return id
}

// insertRMCost inserts a GROUP-level (item_code NULL) cst_rm_cost row for the
// given group code at s.period with the given CR/SR/PR rates.
func (s *OilFormulaE2ESuite) insertRMCost(groupCode string, cr, sr, pr float64) {
	_, err := s.raw.ExecContext(s.ctx, `
		INSERT INTO cst_rm_cost (period, rm_code, rm_type, cr_rate, sr_rate, pr_rate,
		                          flag_valuation, flag_marketing, flag_simulation,
		                          flag_valuation_used, flag_marketing_used, flag_simulation_used,
		                          created_by)
		VALUES ($1, $2, 'GROUP', $3, $4, $5, 'CONS', 'CONS', 'CONS', 'CONS', 'CONS', 'CONS', 'oil-formula-e2e')`,
		s.period, groupCode, cr, sr, pr)
	require.NoError(s.T(), err)
}

// loadFormula fetches the REAL, currently-migrated expression for a formula
// code and compiles it via the real evaluator — the crux of why this is an
// engine-path test and not a re-typed-expression unit test.
func (s *OilFormulaE2ESuite) loadFormula(code string) *evaluator.Evaluator {
	var expr string
	require.NoError(s.T(), s.raw.QueryRowContext(s.ctx,
		`SELECT expression FROM mst_formula WHERE formula_code = $1 AND deleted_at IS NULL AND is_active`, code,
	).Scan(&expr))
	ev, err := s.cache.GetOrCompile(code, expr)
	require.NoError(s.T(), err)
	return ev
}

// resolveAndCompute runs the real resolveOilRate + injectProductClassFlags
// against DB-loaded oil context/RM costs, then evaluates the real
// F_YARN_OIL_COST / F_YARN_OIL_GAIN formulas (loaded live from mst_formula)
// for the given product/opu/waste. Returns (oilRate, oilCost, oilGain).
func (s *OilFormulaE2ESuite) resolveAndCompute(productID int64, opu, wastePerc float64) (float64, float64, float64) {
	oilCtx, err := s.loader.LoadOilContext(s.ctx, []int64{productID})
	require.NoError(s.T(), err)
	oil := oilCtx[productID] // may be nil for non-oil-class products (DTY)

	var codes []string
	if oil != nil {
		codes = oilGroupCodes(map[int64]*OilInput{productID: oil}, nil)
	}
	rmCosts, err := s.loader.LoadRMCosts(s.ctx, codes, s.period, "ACTUAL")
	require.NoError(s.T(), err)

	in := ComputeInput{ProductSysID: productID, Period: s.period, Oil: oil, RMCosts: rmCosts}
	rate, _, applied, rateErr := resolveOilRate(in)
	require.NoError(s.T(), rateErr)
	if !applied {
		rate = 0
	}

	scope := map[string]any{"OPU": opu, "WASTE_PERC": wastePerc, "OIL_RATE": rate}
	zeroFilled := map[string]bool{}
	injectProductClassFlags(scope, zeroFilled, oil)

	gainDefault, err := s.loadFormula("F_YARN_OIL_GAIN_POY_DEFAULT").Run(scope)
	require.NoError(s.T(), err)
	scope["OIL_GAIN_POY_DEFAULT"] = gainDefault

	cost, err := s.loadFormula("F_YARN_OIL_COST").Run(scope)
	require.NoError(s.T(), err)
	gain, err := s.loadFormula("F_YARN_OIL_GAIN").Run(scope)
	require.NoError(s.T(), err)

	return rate, cost, gain
}

// resolveOilRateErr runs only oil-context load + resolveOilRate, for the
// BLOCKED-path assertions (E6/E9 style) where no formula evaluation happens.
func (s *OilFormulaE2ESuite) resolveOilRateErr(productID int64) error {
	oilCtx, err := s.loader.LoadOilContext(s.ctx, []int64{productID})
	require.NoError(s.T(), err)
	oil := oilCtx[productID]
	var codes []string
	if oil != nil {
		codes = oilGroupCodes(map[int64]*OilInput{productID: oil}, nil)
	}
	rmCosts, err := s.loader.LoadRMCosts(s.ctx, codes, s.period, "ACTUAL")
	require.NoError(s.T(), err)
	in := ComputeInput{ProductSysID: productID, Period: s.period, Oil: oil, RMCosts: rmCosts}
	_, _, _, rateErr := resolveOilRate(in)
	return rateErr
}

// TestE1_PTY matches spec §6 E1: OPU=2.2, WASTE_PERC=0.7, rates CR=0/SR=2.2869/PR=3.
// CR is zero so the CR->SR->PR cascade picks SR=2.2869. Expected OIL_COST =
// 0.0569286126, OIL_GAIN = -0.0503118.
func (s *OilFormulaE2ESuite) TestE1_PTY() {
	s.insertRMCost(s.groupOil1C, 0, 2.2869, 3)
	rate, cost, gain := s.resolveAndCompute(s.ptyProduct, 2.2, 0.7)
	s.InDelta(2.2869, rate, 1e-9, "OIL_RATE must be SR (CR is zero)")
	s.InDelta(0.0569286126, cost, 1e-9)
	s.InDelta(-0.0503118, gain, 1e-9)
}

// TestE2_POY matches spec §6 E2: OPU=0.6, WASTE_PERC=0.7, CR=1.5. OIL_GAIN
// comes from F_YARN_OIL_GAIN_POY_DEFAULT (-0.002), not from OPU/OIL_RATE.
func (s *OilFormulaE2ESuite) TestE2_POY() {
	s.insertRMCost(s.groupOil2C, 1.5, 0, 0)
	rate, cost, gain := s.resolveAndCompute(s.poyProduct, 0.6, 0.7)
	s.InDelta(1.5, rate, 1e-9, "OIL_RATE must be CR (first non-zero)")
	s.InDelta(0.0101836451, cost, 1e-9)
	s.InDelta(-0.002, gain, 1e-9)
}

// TestE3_TCS matches spec §6 E3 (Superba): OPU=0.6, CR=1.5. OIL_COST uses the
// Superba arm (OIL_RATE*OPU)/1000; OIL_GAIN is always 0 for Superba.
func (s *OilFormulaE2ESuite) TestE3_TCS() {
	s.insertRMCost(s.groupOil2C, 1.5, 0, 0)
	rate, cost, gain := s.resolveAndCompute(s.tcsProduct, 0.6, 0)
	s.InDelta(1.5, rate, 1e-9)
	s.InDelta(0.0009, cost, 1e-9)
	s.InDelta(0, gain, 1e-9)
}

// TestE4_DTY_NoOilClass matches spec §6 E4: a non-oil-class type is never
// blocked and OIL_RATE is left untouched by the engine (applied=false). No
// oil context row is even returned by the real LoadOilContext query.
func (s *OilFormulaE2ESuite) TestE4_DTY_NoOilClass() {
	oilCtx, err := s.loader.LoadOilContext(s.ctx, []int64{s.dtyProduct})
	s.Require().NoError(err)
	s.Nil(oilCtx[s.dtyProduct], "non-oil-class product must be absent/nil from LoadOilContext")

	in := ComputeInput{ProductSysID: s.dtyProduct, Period: s.period, Oil: nil, RMCosts: nil}
	rate, _, applied, err := resolveOilRate(in)
	s.NoError(err)
	s.False(applied, "DTY must not be blocked and OIL_RATE must be left untouched")
	s.Equal(float64(0), rate)
}

// TestMissingOilRow_PTY_Blocked matches spec §6 E6 / plan P6-T3: a PTY
// product whose oil group has NO cst_rm_cost row for the period resolves to
// ErrMissingRMCost, which process_chunk.go turns into cjp_status=BLOCKED,
// cjp_block_reason='MISSING_RM_COST'.
func (s *OilFormulaE2ESuite) TestMissingOilRow_PTY_Blocked() {
	// Deliberately do NOT insert a cst_rm_cost row for s.groupOil1C this period.
	err := s.resolveOilRateErr(s.ptyProduct)
	s.Require().Error(err)
	s.True(errors.Is(err, costcalcdom.ErrMissingRMCost), "want ErrMissingRMCost, got %v", err)
}

// TestAllZeroRates_Blocked matches spec §6 E9 / decision D17: a rate row that
// exists but has CR=SR=PR=0 is treated as BLOCKED, stricter than the ordinary
// RM-unit-cost cascade (which would accept all-zero as "no rate yet").
func (s *OilFormulaE2ESuite) TestAllZeroRates_Blocked() {
	s.insertRMCost(s.groupOil2C, 0, 0, 0)
	err := s.resolveOilRateErr(s.poyProduct)
	s.Require().Error(err)
	s.True(errors.Is(err, costcalcdom.ErrMissingRMCost))
	s.Contains(err.Error(), "all rates zero")
}
