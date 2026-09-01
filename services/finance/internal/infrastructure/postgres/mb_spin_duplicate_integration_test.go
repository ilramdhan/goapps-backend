// Package postgres_test — P8-b: the DuplicateSpin / ListChildren / ExistsByOrionItemCode
// primitives against a real PostgreSQL.
//
// Gated by INTEGRATION_TEST=true; requires migrations 000484 AND 000490 to have been
// applied (mbs_parent_spin_id … mbs_cost_product_id must exist).
//
// SAFETY: mst_mb_spin holds real Oracle-seeded production rows. These tests never
// touch them. Every fixture is named/keyed with ITEST-MBSPINDUP- (a shape no real
// mb_costing, ORION code, or mgt name takes) and the whole prefix range is
// hard-deleted before and after each test. Clones produced by DuplicateSpin inherit
// the prefixed mgt name, so they are swept by the same cleanup.
//
// What is locked in here:
//
//	(a) the A5/D19 column policy (as amended by D31/D32) — what is nulled, set, copied
//	(b) mbs_ldr_is_fixed / mbs_dozing_is_fixed land as FALSE, never NULL (§11 item 95)
//	(c) a self-loop parent is rejected by the app layer even though the DB allows it
//	(d) ListChildren returns R&D children only, one level deep (A6/A7, R13)
//	(e) only a root spin (mbs_parent_spin_id IS NULL) may be duplicated — a
//	    source that is itself a duplicated child is rejected with
//	    ErrAlreadyDuplicated, capping lineage depth at one level
package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

const mbSpinDupPrefix = "ITEST-MBSPINDUP-"

// mbSpinDupCleanupSQL hard-deletes only this suite's fixtures. Children are removed
// before parents so fk_mbs_parent_spin cannot block the delete.
const mbSpinDupCleanupSQL = `DELETE FROM mst_mb_spin
	WHERE mbs_mgt_name LIKE 'ITEST-MBSPINDUP-%'
	   OR mbs_mb_costing LIKE 'ITEST-MBSPINDUP-%'
	   OR mbs_orion_item_code LIKE 'ITEST-MBSPINDUP-%'`

// MBSpinDuplicateSuite exercises the P8 duplicate primitives against a real DB.
type MBSpinDuplicateSuite struct {
	suite.Suite
	db     *postgres.DB
	repo   *postgres.MBSpinRepository
	ctx    context.Context
	headID uuid.UUID
}

func TestMBSpinDuplicateSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(MBSpinDuplicateSuite))
}

func (s *MBSpinDuplicateSuite) SetupSuite() {
	s.ctx = context.Background()

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnvOrDefault("TEST_DB_HOST", "localhost"),
		getEnvOrDefault("TEST_DB_PORT", "5434"),
		getEnvOrDefault("TEST_DB_USER", "finance"),
		getEnvOrDefault("TEST_DB_PASSWORD", "finance123"),
		getEnvOrDefault("TEST_DB_NAME", "finance_db"))

	raw, err := sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), waitForDB(raw, 10*time.Second))

	s.db = postgres.NewDBFromSQL(raw)
	s.repo = postgres.NewMBSpinRepository(s.db)

	// mbs_mbh_id is NOT NULL and FK-constrained, so borrow an existing head. The
	// suite never writes to mst_mb_head.
	require.NoError(s.T(),
		s.db.QueryRowContext(s.ctx,
			`SELECT mbh_id FROM mst_mb_head WHERE deleted_at IS NULL LIMIT 1`,
		).Scan(&s.headID),
		"integration DB needs at least one mst_mb_head row")
}

func (s *MBSpinDuplicateSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanup()
	require.NoError(s.T(), s.db.Close())
}

func (s *MBSpinDuplicateSuite) SetupTest()    { s.cleanup() }
func (s *MBSpinDuplicateSuite) TearDownTest() { s.cleanup() }

func (s *MBSpinDuplicateSuite) cleanup() {
	// Two passes: the first frees children, the second the parents they referenced.
	for i := 0; i < 2; i++ {
		_, err := s.db.ExecContext(s.ctx, mbSpinDupCleanupSQL)
		require.NoError(s.T(), err)
	}
}

// insertSource writes a fully populated source spin so the copy/null/set policy has
// something non-NULL to be checked against.
func (s *MBSpinDuplicateSuite) insertSource(status string) uuid.UUID {
	id := uuid.New()
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO mst_mb_spin (
			mbs_id, mbs_mbh_id, mbs_mgt_name,
			mbs_oracle_sys_id, mbs_orion_item_code, mbs_mb_costing,
			mbs_denier, mbs_filament, mbs_dozing, mbs_cc, mbs_cost_rate_mkt,
			mbs_status, mbs_ldr_prsn, mbs_run_ldr_pct, mbs_final_product, mbs_lesture,
			mbs_ldr_is_fixed, mbs_dozing_is_fixed,
			mbs_is_active, created_at, created_by
		) VALUES (
			$1, $2, $3,
			$4, $5, $6,
			150.5, 48, 2.25, $7, 3.75,
			$8, 1.5, 1.75, $9, 'SD',
			TRUE, TRUE,
			TRUE, NOW(), 'itest'
		)`,
		id, s.headID, mbSpinDupPrefix+"SRC",
		mbSpinDupPrefix+"ORA-"+id.String()[:8],
		mbSpinDupPrefix+"ORION-"+id.String()[:8],
		mbSpinDupPrefix+"MBC-"+id.String()[:8],
		mbSpinDupPrefix+"CC", status, mbSpinDupPrefix+"FP")
	require.NoError(s.T(), err)
	return id
}

// insertChildOf writes a row with mbs_parent_spin_id already set, bypassing
// DuplicateSpin entirely. Used to simulate a grandchild for ListChildren
// fixtures: since ErrAlreadyDuplicated now rejects duplicating a spin that
// already has a parent, DuplicateSpin itself can no longer produce a
// grandchild, but ListChildren's one-level-deep contract must still be proven
// against a row shaped like one (e.g. surviving legacy data).
func (s *MBSpinDuplicateSuite) insertChildOf(parentID uuid.UUID, status string) uuid.UUID {
	id := uuid.New()
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO mst_mb_spin (
			mbs_id, mbs_mbh_id, mbs_mgt_name, mbs_parent_spin_id,
			mbs_status, mbs_is_active, mbs_ldr_is_fixed, mbs_dozing_is_fixed,
			created_at, created_by
		) VALUES ($1, $2, $3, $4, $5, TRUE, TRUE, TRUE, NOW(), 'itest')`,
		id, s.headID, mbSpinDupPrefix+"GRANDCHILD", parentID, status)
	require.NoError(s.T(), err)
	return id
}

// TestDuplicateSpin_ColumnPolicy proves A5/D19: ERP keys nulled, lineage set,
// business values copied.
func (s *MBSpinDuplicateSuite) TestDuplicateSpin_ColumnPolicy() {
	src := s.insertSource("Spinning")

	out, err := s.repo.DuplicateSpin(s.ctx, mbspin.DuplicateInput{
		SourceSpinID: src,
		ActorUserID:  "itest-actor",
	})
	require.NoError(s.T(), err)
	require.NotEqual(s.T(), src, out.NewSpinID)
	require.Equal(s.T(), src, out.ParentSpinID)
	require.Equal(s.T(), mbSpinDupPrefix+"SRC (copy)", out.MgtName)
	require.Equal(s.T(), 0, out.LineageDepth)

	clone, err := s.repo.GetByID(s.ctx, out.NewSpinID)
	require.NoError(s.T(), err)

	// NULLED — ERP / identity keys.
	require.Nil(s.T(), clone.OracleSysID())
	require.Nil(s.T(), clone.OrionItemCode())

	// SET — lineage + status. The clone is always born R&D (D5), regardless of the
	// source's status (here deliberately "Spinning").
	require.NotNil(s.T(), clone.ParentSpinID())
	require.Equal(s.T(), src, *clone.ParentSpinID())
	require.NotNil(s.T(), clone.MBSStatus())
	require.Equal(s.T(), mbspin.StatusRnD, *clone.MBSStatus())
	require.True(s.T(), clone.IsRnD())
	require.True(s.T(), clone.IsActive())
	require.NotNil(s.T(), clone.DuplicatedAt())
	require.NotNil(s.T(), clone.DuplicatedBy())
	require.Equal(s.T(), "itest-actor", *clone.DuplicatedBy())
	require.Nil(s.T(), clone.LastRecalcAt(), "a fresh clone has never been recalculated")

	// COPIED — business values.
	require.NotNil(s.T(), clone.Denier())
	require.InDelta(s.T(), 150.5, *clone.Denier(), 0.0001)
	require.NotNil(s.T(), clone.Filament())
	require.Equal(s.T(), 48, *clone.Filament())
	require.NotNil(s.T(), clone.Dozing())
	require.InDelta(s.T(), 2.25, *clone.Dozing(), 0.0001)
	require.Equal(s.T(), s.headID, clone.HeadID())

	// COPIED — mbs_mb_costing (D31=B: the clone inherits the source's MB
	// Costing instead of being nulled, unlike mbs_oracle_sys_id/orion_item_code
	// above, which stay ERP/identity keys and remain NULLED).
	require.NotNil(s.T(), clone.MBCosting())
	require.Equal(s.T(), mbSpinDupPrefix+"MBC-"+src.String()[:8], *clone.MBCosting())
}

// TestDuplicateSpin_FixedMarkersAreExplicitFalse is the §11-item-95 guard: NULL reads
// as "fixed", which would silently exclude every clone from recalc forever. The source
// here has BOTH markers TRUE, so a naive copy would fail this test too.
func (s *MBSpinDuplicateSuite) TestDuplicateSpin_FixedMarkersAreExplicitFalse() {
	src := s.insertSource(mbspin.StatusRnD)

	out, err := s.repo.DuplicateSpin(s.ctx, mbspin.DuplicateInput{
		SourceSpinID: src, ActorUserID: "itest-actor",
	})
	require.NoError(s.T(), err)

	var ldr, dozing sql.NullBool
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT mbs_ldr_is_fixed, mbs_dozing_is_fixed FROM mst_mb_spin WHERE mbs_id = $1`, out.NewSpinID,
	).Scan(&ldr, &dozing))

	require.True(s.T(), ldr.Valid, "mbs_ldr_is_fixed must not be NULL on a clone")
	require.False(s.T(), ldr.Bool)
	require.True(s.T(), dozing.Valid, "mbs_dozing_is_fixed must not be NULL on a clone")
	require.False(s.T(), dozing.Bool)

	clone, err := s.repo.GetByID(s.ctx, out.NewSpinID)
	require.NoError(s.T(), err)
	require.False(s.T(), clone.IsFixedLDR())
	require.False(s.T(), clone.IsFixedDozing())
}

func (s *MBSpinDuplicateSuite) TestDuplicateSpin_NameAndDimensionOverrides() {
	src := s.insertSource(mbspin.StatusRnD)

	name := mbSpinDupPrefix + "RENAMED"
	denier := 99.25
	filament := 72
	out, err := s.repo.DuplicateSpin(s.ctx, mbspin.DuplicateInput{
		SourceSpinID: src, MgtName: &name, Denier: &denier, Filament: &filament,
		ActorUserID: "itest-actor",
	})
	require.NoError(s.T(), err)
	require.Equal(s.T(), name, out.MgtName)

	clone, err := s.repo.GetByID(s.ctx, out.NewSpinID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), name, clone.MgtName())
	require.InDelta(s.T(), denier, *clone.Denier(), 0.0001)
	require.Equal(s.T(), filament, *clone.Filament())
}

// TestDuplicateSpin_SelfLoopRejected: migration 000484 deliberately ships without
// chk_mbs_parent_not_self, so the DB happily stores A -> A. The application walk-up
// must still refuse to duplicate from it.
func (s *MBSpinDuplicateSuite) TestDuplicateSpin_SelfLoopRejected() {
	src := s.insertSource(mbspin.StatusRnD)
	_, err := s.db.ExecContext(s.ctx,
		`UPDATE mst_mb_spin SET mbs_parent_spin_id = mbs_id WHERE mbs_id = $1`, src)
	require.NoError(s.T(), err, "the DB is expected to ACCEPT a self-loop — that is why the app check exists")

	_, err = s.repo.DuplicateSpin(s.ctx, mbspin.DuplicateInput{
		SourceSpinID: src, ActorUserID: "itest-actor",
	})
	require.ErrorIs(s.T(), err, mbspin.ErrParentCycle)
}

func (s *MBSpinDuplicateSuite) TestDuplicateSpin_MissingSourceIsNotFound() {
	_, err := s.repo.DuplicateSpin(s.ctx, mbspin.DuplicateInput{
		SourceSpinID: uuid.New(), ActorUserID: "itest-actor",
	})
	require.ErrorIs(s.T(), err, mbspin.ErrNotFound)
}

// TestDuplicateSpin_AlreadyDuplicatedSourceRejected supersedes the old
// TestDuplicateSpin_LineageDepthCountsAncestors, which asserted that
// duplicating a clone (producing a grandchild) succeeded with LineageDepth==1.
// That is now deliberately disallowed: only a root spin (mbs_parent_spin_id IS
// NULL) may be duplicated, so re-duplicating a spin that is itself already a
// duplicated child must fail with ErrAlreadyDuplicated instead, and must not
// insert anything.
func (s *MBSpinDuplicateSuite) TestDuplicateSpin_AlreadyDuplicatedSourceRejected() {
	root := s.insertSource(mbspin.StatusRnD)
	child, err := s.repo.DuplicateSpin(s.ctx, mbspin.DuplicateInput{
		SourceSpinID: root, ActorUserID: "itest-actor",
	})
	require.NoError(s.T(), err)

	_, err = s.repo.DuplicateSpin(s.ctx, mbspin.DuplicateInput{
		SourceSpinID: child.NewSpinID, ActorUserID: "itest-actor",
	})
	require.ErrorIs(s.T(), err, mbspin.ErrAlreadyDuplicated)

	children, err := s.repo.ListChildren(s.ctx, child.NewSpinID)
	require.NoError(s.T(), err)
	require.Empty(s.T(), children, "the rejected duplicate must not have inserted a grandchild")
}

// TestListChildren_RnDOnlyAndOneLevelDeep covers A6, A7 and R13 together.
func (s *MBSpinDuplicateSuite) TestListChildren_RnDOnlyAndOneLevelDeep() {
	root := s.insertSource(mbspin.StatusRnD)

	candidate, err := s.repo.DuplicateSpin(s.ctx, mbspin.DuplicateInput{
		SourceSpinID: root, ActorUserID: "itest-actor",
	})
	require.NoError(s.T(), err)

	// A7: a child promoted out of R&D drops out of the candidate set.
	nonCandidate, err := s.repo.DuplicateSpin(s.ctx, mbspin.DuplicateInput{
		SourceSpinID: root, ActorUserID: "itest-actor",
	})
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx,
		`UPDATE mst_mb_spin SET mbs_status = 'Spinning' WHERE mbs_id = $1`, nonCandidate.NewSpinID)
	require.NoError(s.T(), err)

	// A soft-deleted child is excluded too.
	deleted, err := s.repo.DuplicateSpin(s.ctx, mbspin.DuplicateInput{
		SourceSpinID: root, ActorUserID: "itest-actor",
	})
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.repo.SoftDelete(s.ctx, deleted.NewSpinID, "itest"))

	// R13: a grandchild must NOT appear under root. Inserted directly rather
	// than via DuplicateSpin, since ErrAlreadyDuplicated now prevents
	// DuplicateSpin from ever producing one from a live source.
	s.insertChildOf(candidate.NewSpinID, mbspin.StatusRnD)

	children, err := s.repo.ListChildren(s.ctx, root)
	require.NoError(s.T(), err)
	require.Len(s.T(), children, 1, "only the live R&D direct child is a candidate")
	require.Equal(s.T(), candidate.NewSpinID, children[0].ID())
}

func (s *MBSpinDuplicateSuite) TestExistsByOrionItemCode() {
	src := s.insertSource(mbspin.StatusRnD)

	var code string
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT mbs_orion_item_code FROM mst_mb_spin WHERE mbs_id = $1`, src).Scan(&code))

	exists, err := s.repo.ExistsByOrionItemCode(s.ctx, code)
	require.NoError(s.T(), err)
	require.True(s.T(), exists)

	exists, err = s.repo.ExistsByOrionItemCode(s.ctx, mbSpinDupPrefix+"NEVER-USED")
	require.NoError(s.T(), err)
	require.False(s.T(), exists)

	// An empty code is never a collision — a spin with no ORION code is legal.
	exists, err = s.repo.ExistsByOrionItemCode(s.ctx, "")
	require.NoError(s.T(), err)
	require.False(s.T(), exists)

	// A clone has its ORION code nulled, so it can never re-trip the check.
	out, err := s.repo.DuplicateSpin(s.ctx, mbspin.DuplicateInput{
		SourceSpinID: src, ActorUserID: "itest-actor",
	})
	require.NoError(s.T(), err)
	clone, err := s.repo.GetByID(s.ctx, out.NewSpinID)
	require.NoError(s.T(), err)
	require.Nil(s.T(), clone.OrionItemCode())
}
