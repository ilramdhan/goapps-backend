// Package postgres — integration tests for the supersede param repoint
// (repointSupersededParamsTx) and MBSourceAutoFillStore.ApplyForce (mb-spin-fg-poy-link P3-T3).
//
// Gated by INTEGRATION_TEST=true. Fixtures are prefixed ITEST-MBRPT- and hard-deleted
// before/after each test. The MB_SP_CODE group mst_parameter rows are only read.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbsourceautofill"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

const mbRptPrefix = "ITEST-MBRPT-"

type MBSupersedeRepointSuite struct {
	suite.Suite
	db      *DB
	ctx     context.Context
	poyType int64
	ps      mbsourceautofill.ParamSet
}

func TestMBSupersedeRepointSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(MBSupersedeRepointSuite))
}

func (s *MBSupersedeRepointSuite) SetupSuite() {
	s.ctx = context.Background()
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		mbSpinAutoGenEnvOrDefault("TEST_DB_HOST", "localhost"), mbSpinAutoGenEnvOrDefault("TEST_DB_PORT", "5434"),
		mbSpinAutoGenEnvOrDefault("TEST_DB_USER", "finance"), mbSpinAutoGenEnvOrDefault("TEST_DB_PASSWORD", "finance123"),
		mbSpinAutoGenEnvOrDefault("TEST_DB_NAME", "finance_db"))
	raw, err := sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), mbSpinAutoGenWaitForDB(raw, 10*time.Second))
	s.db = NewDBFromSQL(raw)
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = 'POY' LIMIT 1`).Scan(&s.poyType))
	s.ps, _, err = loadMBSourceParamsOn(s.ctx, s.db)
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), s.ps.Children, "MB_SP_CODE fill group must exist")
}

func (s *MBSupersedeRepointSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanup()
	require.NoError(s.T(), s.db.Close())
}

func (s *MBSupersedeRepointSuite) SetupTest()    { s.cleanup() }
func (s *MBSupersedeRepointSuite) TearDownTest() { s.cleanup() }

func (s *MBSupersedeRepointSuite) exec(q string, args ...any) {
	_, err := s.db.ExecContext(s.ctx, q, args...)
	require.NoError(s.T(), err)
}

func (s *MBSupersedeRepointSuite) cleanup() {
	like := mbRptPrefix + "%"
	s.exec(`DELETE FROM mst_mb_spin_supersede_item WHERE mssi_old_spin_id IN (SELECT mbs_id FROM mst_mb_spin WHERE mbs_mgt_name LIKE $1)`, like)
	s.exec(`DELETE FROM cost_product_applicable_param WHERE capp_product_sys_id IN (SELECT cpm_product_sys_id FROM cost_product_master WHERE cpm_product_code LIKE $1)`, like)
	s.exec(`DELETE FROM cost_product_parameter WHERE cpp_product_sys_id IN (SELECT cpm_product_sys_id FROM cost_product_master WHERE cpm_product_code LIKE $1)`, like)
	s.exec(`UPDATE mst_mb_spin SET mbs_superseded_by_spin_id = NULL WHERE mbs_mgt_name LIKE $1`, like)
	s.exec(`DELETE FROM mst_mb_spin WHERE mbs_mgt_name LIKE $1`, like)
	s.exec(`DELETE FROM mst_mb_head WHERE mbh_mb_costing LIKE $1`, like)
	s.exec(`DELETE FROM cost_product_master WHERE cpm_product_code LIKE $1`, like)
}

func (s *MBSupersedeRepointSuite) newProduct(code string, locked bool) int64 {
	var id int64
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_is_active, cpm_is_locked, cpm_created_by, cpm_updated_by)
		VALUES ($1, $2, $3, TRUE, $4, 'itest', 'itest') RETURNING cpm_product_sys_id`,
		mbRptPrefix+code, s.poyType, "Prod "+code, locked).Scan(&id))
	return id
}

func (s *MBSupersedeRepointSuite) newHead(code string) uuid.UUID {
	id := uuid.New()
	s.exec(`INSERT INTO mst_mb_head (mbh_id, mbh_mb_costing, mbh_current_version, mbh_entry_status, created_by)
		VALUES ($1, $2, 1, 'VALIDATED', 'itest')`, id, mbRptPrefix+code)
	return id
}

// newSpin inserts a spin; mgt name = prefix+tag (MB_SP_DYE source), denier set.
func (s *MBSupersedeRepointSuite) newSpin(head uuid.UUID, tag, shade string, fg *int64, orion *string, denier float64) uuid.UUID {
	id := uuid.New()
	s.exec(`INSERT INTO mst_mb_spin (mbs_id, mbs_mbh_id, mbs_mgt_name, mbs_mb_costing, mbs_orion_item_code, mbs_shade_code,
			mbs_fg_product_id, mbs_denier, mbs_is_active, created_at, created_by)
		VALUES ($1,$2,$3,$3,$4,$5,$6,$7,TRUE,NOW(),'itest')`,
		id, head, mbRptPrefix+tag, orion, shade, fg, denier)
	return id
}

func (s *MBSupersedeRepointSuite) childID(code string) uuid.UUID {
	id, ok := s.ps.Children[code]
	require.True(s.T(), ok, "child %s", code)
	return id
}

func (s *MBSupersedeRepointSuite) newTextRow(product int64, param uuid.UUID, text string, spin *uuid.UUID, filledBy string) {
	s.exec(`INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_text, cpp_value_mb_spin_id, cpp_filled_by, cpp_created_by)
		VALUES ($1, $2, $3, $4, $5, 'itest')`, product, param, text, spin, filledBy)
}

func (s *MBSupersedeRepointSuite) newNumRow(product int64, param uuid.UUID, v float64, filledBy string) {
	s.exec(`INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_filled_by, cpp_created_by)
		VALUES ($1, $2, $3, $4, 'itest')`, product, param, v, filledBy)
}

type rptRow struct {
	text     sql.NullString
	num      sql.NullFloat64
	spin     sql.NullString
	filledBy string
}

func (s *MBSupersedeRepointSuite) row(product int64, param uuid.UUID) (rptRow, bool) {
	var r rptRow
	err := s.db.QueryRowContext(s.ctx, `SELECT cpp_value_text, cpp_value_numeric, cpp_value_mb_spin_id, cpp_filled_by
		FROM cost_product_parameter WHERE cpp_product_sys_id = $1 AND cpp_param_id = $2`, product, param).
		Scan(&r.text, &r.num, &r.spin, &r.filledBy)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false
	}
	require.NoError(s.T(), err)
	return r, true
}

// fixture: old head/spin and new head/spin sharing shade + FG.
type rptFixture struct {
	oldSpin, newSpin uuid.UUID
	oldOrion         string
	newOrion         string
	fg               int64
	newHead          uuid.UUID
	shade            string
}

func (s *MBSupersedeRepointSuite) fixture(tag string) rptFixture {
	f := rptFixture{fg: s.newProduct("FG"+tag, false), shade: mbRptPrefix + "SH" + tag}
	oldHead, newHead := s.newHead("OLD"+tag), s.newHead("NEW"+tag)
	f.newHead = newHead
	f.oldOrion, f.newOrion = mbRptPrefix+"O-OLD"+tag, mbRptPrefix+"O-NEW"+tag
	f.oldSpin = s.newSpin(oldHead, "old"+tag, f.shade, &f.fg, &f.oldOrion, 50)
	// New spin is created without FG so the live (shade, FG) unique index does not trip;
	// the repoint does not depend on the new spin's FG.
	f.newSpin = s.newSpin(newHead, "new"+tag, f.shade, nil, &f.newOrion, 75)
	return f
}

// repoint runs repointSupersededParamsTx for f.oldSpin in one committed tx.
func (s *MBSupersedeRepointSuite) repoint(f rptFixture, batch uuid.UUID) SupersedeRepointResult {
	var out SupersedeRepointResult
	require.NoError(s.T(), s.db.Transaction(s.ctx, func(tx *sql.Tx) error {
		cands, err := findSupersedeCandidatesOn(s.ctx, tx, f.newHead, f.shade, f.fg, true)
		if err != nil {
			return err
		}
		require.Len(s.T(), cands, 1)
		out, err = repointSupersededParamsTx(s.ctx, tx, batch, cands[0], f.newSpin, "tester")
		return err
	}))
	return out
}

func (s *MBSupersedeRepointSuite) TestPointerRowsRepointedRegardlessOfFilledBy_ChildrenOverwritten() {
	f := s.fixture("A")
	pAuto, pManual := s.newProduct("PA", false), s.newProduct("PM", false)
	dye, den := s.childID(mbsource.ParamSPDye), s.childID("MB_SP_DENIER")
	s.newTextRow(pAuto, s.ps.TriggerID, f.oldOrion, &f.oldSpin, mbsource.FilledBy)
	s.newTextRow(pAuto, dye, mbRptPrefix+"oldA", nil, mbsource.FilledBy)
	s.newNumRow(pAuto, den, 50, mbsource.FilledBy)
	s.newTextRow(pManual, s.ps.TriggerID, "MANUAL-TEXT", &f.oldSpin, "manual")

	batch := uuid.New()
	res := s.repoint(f, batch)
	require.ElementsMatch(s.T(), []int64{pAuto, pManual}, res.Products)

	for _, p := range []int64{pAuto, pManual} {
		r, ok := s.row(p, s.ps.TriggerID)
		require.True(s.T(), ok)
		require.Equal(s.T(), f.newOrion, r.text.String)
		require.Equal(s.T(), f.newSpin.String(), r.spin.String)
		require.Equal(s.T(), mbsource.FilledBySupersede, r.filledBy)
		d, ok := s.row(p, dye)
		require.True(s.T(), ok, "dye written/inserted")
		require.Equal(s.T(), mbRptPrefix+"newA", d.text.String)
		n, ok := s.row(p, den)
		require.True(s.T(), ok)
		require.InDelta(s.T(), 75.0, n.num.Float64, 1e-9)
	}

	// Audit: trigger rows keep old values; existing children keep old values; inserted children have NULL old_*.
	var trig, child, inserted int
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `SELECT
		COUNT(*) FILTER (WHERE mssi_repoint_kind = 'POINTER' AND mssi_old_mb_spin_id = $2 AND mssi_new_value_text = $3),
		COUNT(*) FILTER (WHERE mssi_repoint_kind = 'CHILD'),
		COUNT(*) FILTER (WHERE mssi_repoint_kind = 'CHILD' AND mssi_old_filled_by IS NULL AND mssi_old_value_text IS NULL AND mssi_old_value_numeric IS NULL)
		FROM mst_mb_spin_supersede_item WHERE mssi_batch_id = $1`, batch, f.oldSpin, f.newOrion).Scan(&trig, &child, &inserted))
	require.Equal(s.T(), 2, trig)
	require.Positive(s.T(), child)
	require.Equal(s.T(), res.Rows, trig+child)

	var oldText, oldFilled string
	var oldNum float64
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `SELECT mssi_old_value_text, mssi_old_filled_by FROM mst_mb_spin_supersede_item
		WHERE mssi_batch_id = $1 AND mssi_product_sys_id = $2 AND mssi_param_code = 'MB_SP_CODE'`, batch, pManual).Scan(&oldText, &oldFilled))
	require.Equal(s.T(), "MANUAL-TEXT", oldText)
	require.Equal(s.T(), "manual", oldFilled)
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `SELECT mssi_old_value_numeric FROM mst_mb_spin_supersede_item
		WHERE mssi_batch_id = $1 AND mssi_product_sys_id = $2 AND mssi_param_code = 'MB_SP_DENIER'`, batch, pAuto).Scan(&oldNum))
	require.InDelta(s.T(), 50.0, oldNum, 1e-9)
	require.Positive(s.T(), inserted, "pManual had no children: inserted rows have NULL old values")
}

func (s *MBSupersedeRepointSuite) TestTextOnlyUniqueRepointed() {
	f := s.fixture("B")
	p := s.newProduct("PT", false)
	s.newTextRow(p, s.ps.TriggerID, " "+f.oldOrion+" ", nil, "import")

	res := s.repoint(f, uuid.New())
	require.Equal(s.T(), []int64{p}, res.Products)
	r, _ := s.row(p, s.ps.TriggerID)
	require.Equal(s.T(), f.newOrion, r.text.String)
	require.Equal(s.T(), f.newSpin.String(), r.spin.String)

	var kind string
	var oldSpin sql.NullString
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `SELECT mssi_repoint_kind, mssi_old_mb_spin_id FROM mst_mb_spin_supersede_item
		WHERE mssi_product_sys_id = $1 AND mssi_param_code = 'MB_SP_CODE'`, p).Scan(&kind, &oldSpin))
	require.Equal(s.T(), mbspin.SupersedeRepointTextOnly, kind)
	require.False(s.T(), oldSpin.Valid)
}

func (s *MBSupersedeRepointSuite) TestTextOnlySharedOrionNotRepointed() {
	f := s.fixture("C")
	other := s.newHead("OTHC")
	s.newSpin(other, "dupC", mbRptPrefix+"SHX", nil, &f.oldOrion, 10) // same ORION code
	p := s.newProduct("PS", false)
	s.newTextRow(p, s.ps.TriggerID, f.oldOrion, nil, "import")

	res := s.repoint(f, uuid.New())
	require.Empty(s.T(), res.Products)
	require.Zero(s.T(), res.Rows)
	require.Len(s.T(), res.Warnings, 1)
	require.Contains(s.T(), res.Warnings[0], "shared")
	r, _ := s.row(p, s.ps.TriggerID)
	require.Equal(s.T(), f.oldOrion, r.text.String)
	require.False(s.T(), r.spin.Valid)
}

func (s *MBSupersedeRepointSuite) TestLockedProductSkipped() {
	f := s.fixture("D")
	p := s.newProduct("PL", true)
	s.newTextRow(p, s.ps.TriggerID, f.oldOrion, &f.oldSpin, mbsource.FilledBy)

	res := s.repoint(f, uuid.New())
	require.Empty(s.T(), res.Products)
	require.Len(s.T(), res.Warnings, 1)
	require.Contains(s.T(), res.Warnings[0], "locked")
	r, _ := s.row(p, s.ps.TriggerID)
	require.Equal(s.T(), f.oldSpin.String(), r.spin.String)
	var n int
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT COUNT(*) FROM mst_mb_spin_supersede_item WHERE mssi_product_sys_id = $1`, p).Scan(&n))
	require.Zero(s.T(), n)
}

func (s *MBSupersedeRepointSuite) TestApplyForceOverwritesAndRejectsLocked() {
	f := s.fixture("E")
	p, locked := s.newProduct("PF", false), s.newProduct("PFL", true)
	s.newTextRow(p, s.ps.TriggerID, "OLD-VALUE", nil, "manual")
	store := NewMBSourceAutoFillStore(s.db)
	spin, err := loadSpinForRepointOn(s.ctx, s.db, f.newSpin)
	require.NoError(s.T(), err)
	_, children, err := loadMBSourceParamsOn(s.ctx, s.db)
	require.NoError(s.T(), err)
	plan := mbsourceautofill.Plan{ProductSysID: p, Resolution: mbsource.SpinResolution(spin, children)}

	require.NoError(s.T(), store.ApplyForce(s.ctx, s.ps, plan, "tester"))
	r, _ := s.row(p, s.ps.TriggerID)
	require.Equal(s.T(), f.newOrion, r.text.String)
	require.Equal(s.T(), f.newSpin.String(), r.spin.String)
	require.Equal(s.T(), mbsource.FilledBy, r.filledBy)

	plan.ProductSysID = locked
	require.ErrorIs(s.T(), store.ApplyForce(s.ctx, s.ps, plan, "tester"), costproductparameter.ErrProductLocked)
	_, has := s.row(locked, s.ps.TriggerID)
	require.False(s.T(), has)
}

// K5 (mb-active-sync P2f-1): lookup by old spin id; legacy spin with NULL FG scans as 0.
func (s *MBSupersedeRepointSuite) TestFindSupersedeCandidatesBySpin_NullFG() {
	head := s.newHead("K5H")
	orion := mbRptPrefix + "K5ORION"
	spin := s.newSpin(head, "K5S", "", nil, &orion, 1)
	tx, err := s.db.BeginTx(s.ctx, nil)
	require.NoError(s.T(), err)
	defer func() { _ = tx.Rollback() }()

	got, err := findSupersedeCandidatesBySpinOn(s.ctx, tx, spin)
	require.NoError(s.T(), err)
	require.Len(s.T(), got, 1)
	require.Equal(s.T(), spin, got[0].SpinID)
	require.Equal(s.T(), int64(0), got[0].FgProductID)
	require.Equal(s.T(), head, got[0].HeadID)
	require.Equal(s.T(), orion, got[0].SpinCode)

	// inactive spin -> empty
	_, err = tx.ExecContext(s.ctx, `UPDATE mst_mb_spin SET mbs_is_active = FALSE WHERE mbs_id = $1`, spin)
	require.NoError(s.T(), err)
	got, err = findSupersedeCandidatesBySpinOn(s.ctx, tx, spin)
	require.NoError(s.T(), err)
	require.Empty(s.T(), got)
}
