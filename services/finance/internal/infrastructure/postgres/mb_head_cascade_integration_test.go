// Package postgres — integration coverage for MBHeadRepository.UpdateWithCascade (MB Recipe
// cascade design §3.2): one head edit reaches every live spin, the guarded root LDR, the
// MB-recipe cost product and the frozen CPP params in a single transaction.
//
// Gated by INTEGRATION_TEST=true; requires a reachable PostgreSQL (defaults match the
// docker-compose finance DB). Reuses MBAutoGenRegenSuite's connection helpers.
//
// SAFETY: every fixture row is prefixed with ITEST-MBCASC- and hard-deleted before and after
// each test.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

const mbCascFixturePrefix = "ITEST-MBCASC-"

// MBHeadCascadeSuite proves UpdateWithCascade's contract end to end.
type MBHeadCascadeSuite struct {
	suite.Suite
	db           *DB
	headRepo     *MBHeadRepository
	ctx          context.Context
	typeID       int32
	mbhID        uuid.UUID
	productSysID int64
	machineA     uuid.UUID
	machineB     uuid.UUID
	rootSpin     uuid.UUID
	childSpin    uuid.UUID
	deletedSpin  uuid.UUID
}

func TestMBHeadCascadeSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(MBHeadCascadeSuite))
}

func (s *MBHeadCascadeSuite) SetupSuite() {
	s.ctx = context.Background()
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		mbRegenEnvOrDefault("TEST_DB_HOST", "localhost"),
		mbRegenEnvOrDefault("TEST_DB_PORT", "5434"),
		mbRegenEnvOrDefault("TEST_DB_USER", "finance"),
		mbRegenEnvOrDefault("TEST_DB_PASSWORD", "finance123"),
		mbRegenEnvOrDefault("TEST_DB_NAME", "finance_db"))
	raw, err := sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), mbRegenWaitForDB(raw, 10*time.Second))

	s.db = NewDBFromSQL(raw)
	s.headRepo = NewMBHeadRepository(s.db, NewMBCompositionRepository(s.db))
	require.NoError(s.T(),
		s.db.QueryRowContext(s.ctx, `SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = 'MB' AND cpt_is_active = TRUE`).
			Scan(&s.typeID))
}

func (s *MBHeadCascadeSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanupFixtures()
	require.NoError(s.T(), s.db.Close())
}

func (s *MBHeadCascadeSuite) SetupTest() {
	s.cleanupFixtures()
	s.seedFixtures()
}

func (s *MBHeadCascadeSuite) TearDownTest() { s.cleanupFixtures() }

func (s *MBHeadCascadeSuite) exec(q string, args ...any) {
	_, err := s.db.ExecContext(s.ctx, q, args...)
	require.NoError(s.T(), err)
}

func (s *MBHeadCascadeSuite) cleanupFixtures() {
	prefix := mbCascFixturePrefix + "%"
	// Children first (self-FK mbs_parent_spin_id), then roots, head, product, machines.
	s.exec(`DELETE FROM mst_mb_spin WHERE mbs_parent_spin_id IS NOT NULL AND mbs_mbh_id IN (SELECT mbh_id FROM mst_mb_head WHERE mbh_mb_costing LIKE $1)`, prefix)
	s.exec(`DELETE FROM mst_mb_spin WHERE mbs_mbh_id IN (SELECT mbh_id FROM mst_mb_head WHERE mbh_mb_costing LIKE $1)`, prefix)
	s.exec(`DELETE FROM mst_mb_head WHERE mbh_mb_costing LIKE $1`, prefix)
	s.exec(`DELETE FROM cost_product_master WHERE cpm_product_code LIKE $1`, prefix) // CAPP/CPP cascade
	s.exec(`DELETE FROM mst_machine WHERE mc_code LIKE $1`, prefix)
}

// seedFixtures builds a validated head (cost product linked) on machine A, with a root spin,
// a child spin and a soft-deleted spin, all carrying stale values plus manual-only columns.
func (s *MBHeadCascadeSuite) seedFixtures() {
	s.machineA, s.machineB = uuid.New(), uuid.New()
	s.exec(`INSERT INTO mst_machine (mc_id, mc_code, mc_name, mc_tot_fxd_cst, created_by) VALUES
		($1, $3, 'itest machine A', 1000, 'itest'), ($2, $4, 'itest machine B', 2500, 'itest')`,
		s.machineA, s.machineB, mbCascFixturePrefix+"MA", mbCascFixturePrefix+"MB")

	code := mbCascFixturePrefix + uuid.NewString()[:7] // cpm_product_code is VARCHAR(20)
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_source,
		                                 cpm_shade_code, cpm_shade_name, cpm_created_by, cpm_updated_by)
		VALUES ($1, $2, 'old name', 'MB_RECIPE', 'OLDSH', 'Old Shade', 'itest', 'itest')
		RETURNING cpm_product_sys_id`, code, s.typeID).Scan(&s.productSysID))

	s.mbhID = uuid.New()
	s.exec(`INSERT INTO mst_mb_head (mbh_id, mbh_mb_costing, mbh_cost_product_id, mbh_current_version, mbh_entry_status,
	                                 mbh_machine_id, mbh_param_throughput_per_hour, mbh_param_no_of_process, created_by)
		VALUES ($1, $2, $3, 1, 'VALIDATED', $4, 'B', 'D', 'itest')`,
		s.mbhID, mbCascFixturePrefix+"OLD", s.productSysID, s.machineA)

	s.rootSpin, s.childSpin, s.deletedSpin = uuid.New(), uuid.New(), uuid.New()
	const spinInsert = `
		INSERT INTO mst_mb_spin (mbs_id, mbs_mbh_id, mbs_parent_spin_id, mbs_mgt_name, mbs_mb_costing, mbs_denier,
		    mbs_filament, mbs_dozing, mbs_shade_code, mbs_shade_name, mbs_cross_section, mbs_cc,
		    mbs_orion_item_code, mbs_status, mbs_is_active, mbs_cost_rate_mkt, mbs_run_ldr_pct, mbs_ldr_prsn,
		    mbs_ldr_is_fixed, mbs_dozing_is_fixed, mbs_ldr_calculated_pct, mbs_ldr_type, mbs_ldr_is_actual,
		    deleted_at, created_by)
		VALUES ($1, $2, $3, 'old mgt', $4, 50, 24, 1.5, 'KEEPSH', 'Keep Shade', 'KEEPCS', 'KEEPSH',
		    $5, 'Spinning', TRUE, 3.25, 11, 12, TRUE, TRUE, 10, 'ACTUAL', $6, $7, 'itest')`
	old := mbCascFixturePrefix + "OLD"
	s.exec(spinInsert, s.rootSpin, s.mbhID, nil, old, "ORI-ROOT", false, nil)
	s.exec(spinInsert, s.childSpin, s.mbhID, s.rootSpin, "manually edited", "ORI-CHILD", false, nil)
	s.exec(spinInsert, s.deletedSpin, s.mbhID, nil, old, "ORI-DEL", false, time.Now())
}

// editedHead loads the head and applies a realistic edit: new costing, spec fields, shade
// and machine B, then freezes a snapshot exactly like the handler does.
func (s *MBHeadCascadeSuite) editedHead(mutate func(*mbhead.UpdateInput)) (*mbhead.Entity, *mbhead.ParamSnapshot) {
	e, err := s.headRepo.GetByID(s.ctx, s.mbhID)
	require.NoError(s.T(), err)
	costing, mgt, final, vs := mbCascFixturePrefix+"NEW", "new mgt", "FP-NEW", "VS-9"
	denier, dozing, filament := 75.0, 2.25, 36
	shade, shadeName, cs, lus := "NEWSH", "New Shade", "TRILOBAL", "SD"
	ldr := 42.5
	in := mbhead.UpdateInput{
		MBCosting: &costing, MgtName: &mgt, Denier: &denier, Filament: &filament, Dozing: &dozing,
		MBHFinalProduct: &final, VSNumber: &vs, ShadeCode: &shade, ShadeName: &shadeName,
		CrossSection: &cs, LustureCode: &lus, MachineID: &s.machineB, MBHRunLdrPct: &ldr,
	}
	if mutate != nil {
		mutate(&in)
	}
	require.NoError(s.T(), e.Update(in, "editor"))
	waste, ql, eff, dev, pack, perDay := "2", "0.6", "94", "1", "3", "16"
	p := &mbhead.ParamSnapshot{
		Waste: &waste, QualityLoss: &ql, Efficiency: &eff, DevExpense: &dev, Packing: &pack, MBProdPerDay: &perDay,
		ThroughputPerHour: "C", NoOfProcess: "T",
	}
	e.FreezeParams(p.Waste, p.QualityLoss, p.Efficiency, p.DevExpense, p.Packing, p.MBProdPerDay, p.ThroughputPerHour, p.NoOfProcess)
	return e, p
}

type cascSpinRow struct {
	costing, mgt, shade, shadeName, cs, cc, final, vs, lus sql.NullString
	orion, status                                          sql.NullString
	denier, dozing, rateMkt, runLdr, ldrPrsn, ldrCalc      sql.NullFloat64
	filament                                               sql.NullInt64
	isActive, ldrFixed, dozFixed, ldrActual                bool
	ldrType                                                sql.NullString
	updatedBy                                              sql.NullString
}

func (s *MBHeadCascadeSuite) spin(id uuid.UUID) cascSpinRow {
	var r cascSpinRow
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		SELECT mbs_mb_costing, mbs_mgt_name, mbs_shade_code, mbs_shade_name, mbs_cross_section, mbs_cc,
		       mbs_final_product, mbs_vs_number, mbs_lusture_code, mbs_orion_item_code, mbs_status,
		       mbs_denier, mbs_dozing, mbs_cost_rate_mkt, mbs_run_ldr_pct, mbs_ldr_prsn, mbs_ldr_calculated_pct,
		       mbs_filament, mbs_is_active, mbs_ldr_is_fixed, mbs_dozing_is_fixed, mbs_ldr_is_actual,
		       mbs_ldr_type, updated_by
		FROM mst_mb_spin WHERE mbs_id = $1`, id).Scan(
		&r.costing, &r.mgt, &r.shade, &r.shadeName, &r.cs, &r.cc, &r.final, &r.vs, &r.lus, &r.orion, &r.status,
		&r.denier, &r.dozing, &r.rateMkt, &r.runLdr, &r.ldrPrsn, &r.ldrCalc,
		&r.filament, &r.isActive, &r.ldrFixed, &r.dozFixed, &r.ldrActual, &r.ldrType, &r.updatedBy))
	return r
}

func (s *MBHeadCascadeSuite) cppNumeric(paramCode string) sql.NullFloat64 {
	var v sql.NullFloat64
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		SELECT cpp.cpp_value_numeric FROM cost_product_parameter cpp
		JOIN mst_parameter mp ON mp.id = cpp.cpp_param_id
		WHERE cpp.cpp_product_sys_id = $1 AND mp.param_code = $2`, s.productSysID, paramCode).Scan(&v))
	return v
}

func (s *MBHeadCascadeSuite) TestCascade_AllLiveSpinsGetHeadFields_ForbiddenColumnsUntouched() {
	e, p := s.editedHead(nil)
	ids, err := s.headRepo.UpdateWithCascade(s.ctx, e, mbCascFixturePrefix+"OLD", p)
	require.NoError(s.T(), err)
	require.ElementsMatch(s.T(), []uuid.UUID{s.rootSpin, s.childSpin}, ids, "root + child, never the soft-deleted spin")

	for _, id := range []uuid.UUID{s.rootSpin, s.childSpin} {
		r := s.spin(id)
		require.Equal(s.T(), mbCascFixturePrefix+"NEW", r.costing.String, "costing overwritten even on a manually edited child")
		require.Equal(s.T(), "new mgt", r.mgt.String)
		require.InDelta(s.T(), 75, r.denier.Float64, 1e-9)
		require.InDelta(s.T(), 2.25, r.dozing.Float64, 1e-9)
		require.Equal(s.T(), int64(36), r.filament.Int64)
		require.Equal(s.T(), "FP-NEW", r.final.String)
		require.Equal(s.T(), "VS-9", r.vs.String)
		require.Equal(s.T(), "SD", r.lus.String)
		require.Equal(s.T(), "NEWSH", r.shade.String)
		require.Equal(s.T(), "New Shade", r.shadeName.String)
		require.Equal(s.T(), "TRILOBAL", r.cs.String)
		require.Equal(s.T(), "NEWSH", r.cc.String, "mbs_cc mirrors the shade code")
		require.Equal(s.T(), "editor", r.updatedBy.String)

		// ⛔ Columns the cascade must never write.
		require.Equal(s.T(), "Spinning", r.status.String)
		require.True(s.T(), r.isActive)
		require.InDelta(s.T(), 3.25, r.rateMkt.Float64, 1e-9)
		require.InDelta(s.T(), 11, r.runLdr.Float64, 1e-9)
		require.InDelta(s.T(), 12, r.ldrPrsn.Float64, 1e-9)
		require.True(s.T(), r.ldrFixed)
		require.True(s.T(), r.dozFixed)
	}
	require.Equal(s.T(), "ORI-ROOT", s.spin(s.rootSpin).orion.String)
	require.Equal(s.T(), "ORI-CHILD", s.spin(s.childSpin).orion.String)

	del := s.spin(s.deletedSpin)
	require.Equal(s.T(), mbCascFixturePrefix+"OLD", del.costing.String, "soft-deleted spin untouched")
}

func (s *MBHeadCascadeSuite) TestCascade_RootLDR_RespectsActualLock() {
	// Root unlocked, child unlocked: only the ROOT gets the head LDR (syncRootSpinLDRFromHead).
	s.exec(`UPDATE mst_mb_spin SET mbs_ldr_is_actual = FALSE, mbs_ldr_type = 'CALCULATED' WHERE mbs_id IN ($1, $2)`, s.rootSpin, s.childSpin)
	e, p := s.editedHead(nil)
	_, err := s.headRepo.UpdateWithCascade(s.ctx, e, mbCascFixturePrefix+"OLD", p)
	require.NoError(s.T(), err)
	require.InDelta(s.T(), 42.5, s.spin(s.rootSpin).ldrCalc.Float64, 1e-9)
	require.InDelta(s.T(), 10, s.spin(s.childSpin).ldrCalc.Float64, 1e-9, "child LDR is never synced from the head")

	// Root locked ACTUAL: a second edit must leave its LDR alone.
	s.exec(`UPDATE mst_mb_spin SET mbs_ldr_is_actual = TRUE, mbs_ldr_calculated_pct = 7, mbs_ldr_type = 'ACTUAL' WHERE mbs_id = $1`, s.rootSpin)
	e, p = s.editedHead(func(in *mbhead.UpdateInput) { v := 99.0; in.MBHRunLdrPct = &v })
	_, err = s.headRepo.UpdateWithCascade(s.ctx, e, mbCascFixturePrefix+"NEW", p)
	require.NoError(s.T(), err)
	root := s.spin(s.rootSpin)
	require.InDelta(s.T(), 7, root.ldrCalc.Float64, 1e-9)
	require.Equal(s.T(), "ACTUAL", root.ldrType.String)
}

func (s *MBHeadCascadeSuite) TestCascade_EmptyHeadShadeKeepsSpinAndProductShade() {
	e, p := s.editedHead(func(in *mbhead.UpdateInput) {
		empty := ""
		in.ShadeCode, in.ShadeName, in.CrossSection = &empty, &empty, &empty
	})
	_, err := s.headRepo.UpdateWithCascade(s.ctx, e, mbCascFixturePrefix+"OLD", p)
	require.NoError(s.T(), err)
	r := s.spin(s.rootSpin)
	require.Equal(s.T(), "KEEPSH", r.shade.String)
	require.Equal(s.T(), "Keep Shade", r.shadeName.String)
	require.Equal(s.T(), "KEEPCS", r.cs.String)
	require.Equal(s.T(), "KEEPSH", r.cc.String)

	var code, name, prodName string
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT cpm_shade_code, cpm_shade_name, cpm_product_name FROM cost_product_master WHERE cpm_product_sys_id = $1`,
		s.productSysID).Scan(&code, &name, &prodName))
	require.Equal(s.T(), "OLDSH", code)
	require.Equal(s.T(), "Old Shade", name)
	require.Equal(s.T(), mbCascFixturePrefix+"NEW", prodName)
}

func (s *MBHeadCascadeSuite) TestCascade_CostProductNameShade_AndCPPRefrozen() {
	e, p := s.editedHead(nil)
	_, err := s.headRepo.UpdateWithCascade(s.ctx, e, mbCascFixturePrefix+"OLD", p)
	require.NoError(s.T(), err)

	var code, name, prodName string
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT cpm_shade_code, cpm_shade_name, cpm_product_name FROM cost_product_master WHERE cpm_product_sys_id = $1`,
		s.productSysID).Scan(&code, &name, &prodName))
	require.Equal(s.T(), mbCascFixturePrefix+"NEW", prodName)
	require.Equal(s.T(), "NEWSH", code)
	require.Equal(s.T(), "New Shade", name)

	require.InDelta(s.T(), 2500, s.cppNumeric(paramMachineFixedTotal).Float64, 1e-9, "fixed total follows the new machine")
	require.InDelta(s.T(), 50, s.cppNumeric(paramMBThroughput).Float64, 1e-9, "throughput option C = 50")
	require.InDelta(s.T(), 3, s.cppNumeric(paramMBNoProcess).Float64, 1e-9, "no_of_process option T = 3")

	var tp, np string
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT mbh_param_throughput_per_hour, mbh_param_no_of_process FROM mst_mb_head WHERE mbh_id = $1`, s.mbhID).Scan(&tp, &np))
	require.Equal(s.T(), "C", tp)
	require.Equal(s.T(), "T", np)
}

func (s *MBHeadCascadeSuite) TestCascade_HandMadeProductNotRenamed() {
	s.exec(`UPDATE cost_product_master SET cpm_source = 'MANUAL' WHERE cpm_product_sys_id = $1`, s.productSysID)
	e, _ := s.editedHead(nil)
	_, err := s.headRepo.UpdateWithCascade(s.ctx, e, mbCascFixturePrefix+"OLD", nil)
	require.NoError(s.T(), err)
	var prodName, shade string
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT cpm_product_name, cpm_shade_code FROM cost_product_master WHERE cpm_product_sys_id = $1`, s.productSysID).Scan(&prodName, &shade))
	require.Equal(s.T(), "old name", prodName)
	require.Equal(s.T(), "OLDSH", shade)
}

func (s *MBHeadCascadeSuite) TestCascade_FreezeFailureRollsBackEverything() {
	e, p := s.editedHead(nil)
	// An option code with no mst_mb_param_option row makes mbFreezeCostParams fail.
	p.ThroughputPerHour = "ZZ"
	e.FreezeParams(p.Waste, p.QualityLoss, p.Efficiency, p.DevExpense, p.Packing, p.MBProdPerDay, p.ThroughputPerHour, p.NoOfProcess)

	_, err := s.headRepo.UpdateWithCascade(s.ctx, e, mbCascFixturePrefix+"OLD", p)
	require.Error(s.T(), err)

	var costing string
	var tp sql.NullString
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT mbh_mb_costing, mbh_param_throughput_per_hour FROM mst_mb_head WHERE mbh_id = $1`, s.mbhID).Scan(&costing, &tp))
	require.Equal(s.T(), mbCascFixturePrefix+"OLD", costing, "head update rolled back")
	require.Equal(s.T(), "B", tp.String)
	require.Equal(s.T(), mbCascFixturePrefix+"OLD", s.spin(s.rootSpin).costing.String, "spin cascade rolled back")
	require.InDelta(s.T(), 50, s.spin(s.rootSpin).denier.Float64, 1e-9)
	var prodName string
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT cpm_product_name FROM cost_product_master WHERE cpm_product_sys_id = $1`, s.productSysID).Scan(&prodName))
	require.Equal(s.T(), "old name", prodName, "product rename rolled back")
}

func (s *MBHeadCascadeSuite) TestNoProduct_OnlyRenamesSpinsCarryingOldCosting() {
	s.exec(`UPDATE mst_mb_head SET mbh_cost_product_id = NULL WHERE mbh_id = $1`, s.mbhID)
	e, _ := s.editedHead(nil)
	ids, err := s.headRepo.UpdateWithCascade(s.ctx, e, mbCascFixturePrefix+"OLD", nil)
	require.NoError(s.T(), err)
	require.Empty(s.T(), ids)

	root, child := s.spin(s.rootSpin), s.spin(s.childSpin)
	require.Equal(s.T(), mbCascFixturePrefix+"NEW", root.costing.String, "legacy rename still applies")
	require.InDelta(s.T(), 50, root.denier.Float64, 1e-9, "no full cascade without a product")
	require.Equal(s.T(), "manually edited", child.costing.String, "only spins still carrying the old costing")
}

// Shade code + name are written as a pair (old syncRootSpinShadeFromHead /
// syncCostProductMasterShadeFromHead semantics): a head carrying only a code blanks the stale
// name instead of mixing the new code with the old shade's name.
func (s *MBHeadCascadeSuite) TestCascade_ShadeWrittenAsPair_NeverMixed() {
	e, _ := s.editedHead(func(in *mbhead.UpdateInput) {
		code, empty := "PAIRSH", ""
		in.ShadeCode, in.ShadeName, in.CrossSection = &code, &empty, &empty
	})
	_, err := s.headRepo.UpdateWithCascade(s.ctx, e, mbCascFixturePrefix+"OLD", nil)
	require.NoError(s.T(), err)
	for _, id := range []uuid.UUID{s.rootSpin, s.childSpin} {
		r := s.spin(id)
		require.Equal(s.T(), "PAIRSH", r.shade.String)
		require.False(s.T(), r.shadeName.Valid, "stale shade name must not survive next to a new code")
		require.False(s.T(), r.cs.Valid, "cross-section is written with the shade set")
		require.Equal(s.T(), "PAIRSH", r.cc.String, "mbs_cc follows the shade code")
	}
	var code string
	var name sql.NullString
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT cpm_shade_code, cpm_shade_name FROM cost_product_master WHERE cpm_product_sys_id = $1`,
		s.productSysID).Scan(&code, &name))
	require.Equal(s.T(), "PAIRSH", code)
	require.False(s.T(), name.Valid, "product shade name written with its code")
}

// A cosmetic edit (nil params) must leave the frozen snapshot and every CPP row alone even
// when the master values it would re-read have since changed.
func (s *MBHeadCascadeSuite) TestCascade_NilParams_CPPAndFrozenSnapshotUntouched() {
	e, p := s.editedHead(nil)
	_, err := s.headRepo.UpdateWithCascade(s.ctx, e, mbCascFixturePrefix+"OLD", p)
	require.NoError(s.T(), err)
	require.InDelta(s.T(), 2500, s.cppNumeric(paramMachineFixedTotal).Float64, 1e-9)

	// Master drift after validation: machine B's fixed cost changes.
	s.exec(`UPDATE mst_machine SET mc_tot_fxd_cst = 9999 WHERE mc_id = $1`, s.machineB)

	e, err = s.headRepo.GetByID(s.ctx, s.mbhID)
	require.NoError(s.T(), err)
	mgt := "typo fixed"
	require.NoError(s.T(), e.Update(mbhead.UpdateInput{MgtName: &mgt}, "editor"))
	_, err = s.headRepo.UpdateWithCascade(s.ctx, e, mbCascFixturePrefix+"NEW", nil)
	require.NoError(s.T(), err)

	require.InDelta(s.T(), 2500, s.cppNumeric(paramMachineFixedTotal).Float64, 1e-9, "CPP not re-frozen")
	require.InDelta(s.T(), 50, s.cppNumeric(paramMBThroughput).Float64, 1e-9)
	require.InDelta(s.T(), 3, s.cppNumeric(paramMBNoProcess).Float64, 1e-9)
	var tp, np string
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT mbh_param_throughput_per_hour, mbh_param_no_of_process FROM mst_mb_head WHERE mbh_id = $1`, s.mbhID).Scan(&tp, &np))
	require.Equal(s.T(), "C", tp)
	require.Equal(s.T(), "T", np)
	require.Equal(s.T(), "typo fixed", s.spin(s.rootSpin).mgt.String, "spins still cascade")
}
