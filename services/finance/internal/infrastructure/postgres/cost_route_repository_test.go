// Package postgres_test provides integration tests for CostRouteRepository
// covering the list sort keys (L1 regression), the level/RM aggregates, the
// rm_group_name join, and the RM position round-trip.
package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

const (
	crTestPrefix    = "ZZTCRT"
	crTestTypeCode  = "ZZTCR" // cpt_type_code is VARCHAR(5); must stay <= 5 chars.
	crTestGroupCode = "ZZTCRTGRP1"
)

// CostRouteRepoSuite exercises ListHeads + graph read/write against a real DB.
type CostRouteRepoSuite struct {
	suite.Suite
	db   *postgres.DB
	repo *postgres.CostRouteRepository
	ctx  context.Context

	productSysID int64
	headID       int64
}

func TestCostRouteRepoSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(CostRouteRepoSuite))
}

func (s *CostRouteRepoSuite) SetupSuite() {
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
	s.repo = postgres.NewCostRouteRepository(s.db)

	s.seedFixtures()
}

func (s *CostRouteRepoSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	// cascade: deleting the head removes seqs + rms via FK ON DELETE CASCADE.
	_, err := s.db.ExecContext(s.ctx, `DELETE FROM cost_route_head WHERE crh_head_id = $1`, s.headID)
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM cost_product_master WHERE cpm_product_code LIKE $1`, crTestPrefix+"%")
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM cost_product_type WHERE cpt_type_code = $1`, crTestTypeCode)
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM cst_rm_group_head WHERE group_code = $1`, crTestGroupCode)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.db.Close())
}

func (s *CostRouteRepoSuite) seedFixtures() {
	t := s.T()

	var typeID int32
	require.NoError(t, s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_type (cpt_type_code, cpt_type_name)
		VALUES ($1, $2)
		ON CONFLICT (cpt_type_code) DO UPDATE SET cpt_type_name = EXCLUDED.cpt_type_name
		RETURNING cpt_type_id`, crTestTypeCode, "CR Route Test Type").Scan(&typeID))

	require.NoError(t, s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_is_active, cpm_created_by, cpm_updated_by)
		VALUES ($1, $2, $3, TRUE, 'itest', 'itest')
		ON CONFLICT (cpm_product_code) DO UPDATE SET cpm_product_name = EXCLUDED.cpm_product_name
		RETURNING cpm_product_sys_id`, crTestPrefix+"-FG", typeID, "cr itest fg").Scan(&s.productSysID))

	// RM group master so the rm_group_name join resolves.
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO cst_rm_group_head (group_code, group_name, created_by)
		VALUES ($1, $2, 'itest')
		ON CONFLICT DO NOTHING`, crTestGroupCode, "CR Itest Group")
	require.NoError(t, err)

	// Head.
	require.NoError(t, s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_route_head (crh_product_sys_id, crh_routing_status, crh_version, crh_created_by, crh_updated_by)
		VALUES ($1, 'DRAFT', 1, 'itest', 'itest')
		RETURNING crh_head_id`, s.productSysID).Scan(&s.headID))

	// Two levels: L1 seq with a GROUP rm, L2 seq with a PRODUCT rm.
	var seqL1, seqL2 int64
	require.NoError(t, s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_route_seq (crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq, crs_created_by, crs_updated_by)
		VALUES ($1, $2, 1, 1, 'itest', 'itest') RETURNING crs_seq_id`, s.headID, s.productSysID).Scan(&seqL1))
	require.NoError(t, s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_route_seq (crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq, crs_created_by, crs_updated_by)
		VALUES ($1, $2, 2, 1, 'itest', 'itest') RETURNING crs_seq_id`, s.headID, s.productSysID).Scan(&seqL2))

	_, err = s.db.ExecContext(s.ctx, `
		INSERT INTO cost_route_rm (crm_seq_id, crm_parent_product_sys_id, crm_rm_group_code, crm_rm_type,
			crm_route_rm_ratio, crm_created_by, crm_updated_by)
		VALUES ($1, $2, $3, 'GROUP', 1.0, 'itest', 'itest')`, seqL1, s.productSysID, crTestGroupCode)
	require.NoError(t, err)
	_, err = s.db.ExecContext(s.ctx, `
		INSERT INTO cost_route_rm (crm_seq_id, crm_parent_product_sys_id, crm_rm_product_sys_id, crm_rm_type,
			crm_route_rm_ratio, crm_created_by, crm_updated_by)
		VALUES ($1, $2, $3, 'PRODUCT', 1.0, 'itest', 'itest')`, seqL2, s.productSysID, s.productSysID)
	require.NoError(t, err)
}

// TestListHeads_SortKeysReturnRows is the L1 regression guard: every sort key
// the UI exposes must return the seeded head, not an empty list.
func (s *CostRouteRepoSuite) TestListHeads_SortKeysReturnRows() {
	keys := []string{"", "created_at", "product_code", "status", "head_id", "version"}
	orders := []string{"", "asc", "desc"}
	for _, k := range keys {
		for _, o := range orders {
			rows, total, err := s.repo.ListHeads(s.ctx, costroute.Filter{
				Search: crTestPrefix, SortBy: k, SortOrder: o, Page: 1, PageSize: 50,
			})
			s.Require().NoErrorf(err, "sort_by=%q sort_order=%q", k, o)
			s.Require().GreaterOrEqualf(total, int64(1), "sort_by=%q sort_order=%q returned empty", k, o)
			s.Require().NotEmptyf(rows, "sort_by=%q sort_order=%q returned no rows", k, o)
		}
	}
}

// TestListHeads_Aggregates verifies level_count/rm_count are computed per head.
func (s *CostRouteRepoSuite) TestListHeads_Aggregates() {
	rows, _, err := s.repo.ListHeads(s.ctx, costroute.Filter{Search: crTestPrefix, Page: 1, PageSize: 50})
	s.Require().NoError(err)
	var found *costroute.Head
	for _, h := range rows {
		if h.HeadID == s.headID {
			found = h
			break
		}
	}
	s.Require().NotNil(found, "seeded head not present in list")
	s.Equal(int32(2), found.LevelCount, "distinct levels")
	s.Equal(int32(2), found.RmCount, "total rm rows")
}

// TestGetActiveByProduct_ProductNameJoin is the P6-T1 regression guard: the
// routing "Name" column must not silently come back empty. Verifies that
// GetActiveByProduct's SELECT joins cost_product_master and populates both
// ProductCode and ProductName on the returned head.
func (s *CostRouteRepoSuite) TestGetActiveByProduct_ProductNameJoin() {
	h, err := s.repo.GetActiveByProduct(s.ctx, s.productSysID)
	s.Require().NoError(err)
	s.Require().NotNil(h)
	s.Equal(crTestPrefix+"-FG", h.ProductCode, "product code should be populated via join")
	s.Equal("cr itest fg", h.ProductName, "product name should be populated via join")
}

// TestGetGraph_RmGroupNameJoin verifies the GROUP rm resolves its display name.
func (s *CostRouteRepoSuite) TestGetGraph_RmGroupNameJoin() {
	g, err := s.repo.GetGraph(s.ctx, s.headID)
	s.Require().NoError(err)
	var groupRM *costroute.Rm
	for _, seq := range g.Seqs {
		for _, rm := range seq.Rms {
			if rm.RmType == costroute.RmTypeGroup {
				groupRM = rm
			}
		}
	}
	s.Require().NotNil(groupRM, "no GROUP rm found")
	s.Equal("CR Itest Group", groupRM.RmGroupName)
}

// TestSaveGraph_PositionRoundTrip verifies RM position persists through save+read.
func (s *CostRouteRepoSuite) TestSaveGraph_PositionRoundTrip() {
	g, err := s.repo.GetGraph(s.ctx, s.headID)
	s.Require().NoError(err)
	s.Require().NotEmpty(g.Seqs)

	// Set a distinct position on the first RM we find.
	var target *costroute.Rm
	for _, seq := range g.Seqs {
		for _, rm := range seq.Rms {
			target = rm
			break
		}
		if target != nil {
			break
		}
	}
	s.Require().NotNil(target)
	target.PositionX = 321.75
	target.PositionY = 654.5

	saved, err := s.repo.SaveGraph(s.ctx, s.headID, g, "itest")
	s.Require().NoError(err)

	// Re-read and confirm persistence.
	reread, err := s.repo.GetGraph(s.ctx, s.headID)
	s.Require().NoError(err)
	_ = saved
	var got *costroute.Rm
	for _, seq := range reread.Seqs {
		for _, rm := range seq.Rms {
			if rm.RmID == target.RmID {
				got = rm
			}
		}
	}
	s.Require().NotNil(got)
	s.InDelta(321.75, got.PositionX, 1e-6)
	s.InDelta(654.5, got.PositionY, 1e-6)
}

// seedDraftHeadWithGraph creates an isolated product + DRAFT head + 2-level
// graph (mirrors seedFixtures' shape) so DuplicateRoute tests don't disturb
// the shared s.productSysID/s.headID fixtures other tests depend on.
// codeSuffix must be unique per call within a test run.
func (s *CostRouteRepoSuite) seedDraftHeadWithGraph(codeSuffix string) (productSysID, headID int64) {
	t := s.T()

	var typeID int32
	require.NoError(t, s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_type (cpt_type_code, cpt_type_name)
		VALUES ($1, $2)
		ON CONFLICT (cpt_type_code) DO UPDATE SET cpt_type_name = EXCLUDED.cpt_type_name
		RETURNING cpt_type_id`, crTestTypeCode, "CR Route Test Type").Scan(&typeID))

	require.NoError(t, s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_is_active, cpm_created_by, cpm_updated_by)
		VALUES ($1, $2, $3, TRUE, 'itest', 'itest')
		RETURNING cpm_product_sys_id`, crTestPrefix+"-"+codeSuffix, typeID, "cr itest "+codeSuffix).Scan(&productSysID))

	require.NoError(t, s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_route_head (crh_product_sys_id, crh_routing_status, crh_version, crh_created_by, crh_updated_by)
		VALUES ($1, 'DRAFT', 1, 'itest', 'itest')
		RETURNING crh_head_id`, productSysID).Scan(&headID))

	var seqL1, seqL2 int64
	require.NoError(t, s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_route_seq (crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq, crs_created_by, crs_updated_by)
		VALUES ($1, $2, 1, 1, 'itest', 'itest') RETURNING crs_seq_id`, headID, productSysID).Scan(&seqL1))
	require.NoError(t, s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_route_seq (crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq, crs_created_by, crs_updated_by)
		VALUES ($1, $2, 2, 1, 'itest', 'itest') RETURNING crs_seq_id`, headID, productSysID).Scan(&seqL2))

	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO cost_route_rm (crm_seq_id, crm_parent_product_sys_id, crm_rm_group_code, crm_rm_type,
			crm_route_rm_ratio, crm_created_by, crm_updated_by)
		VALUES ($1, $2, $3, 'GROUP', 1.0, 'itest', 'itest')`, seqL1, productSysID, crTestGroupCode)
	require.NoError(t, err)
	_, err = s.db.ExecContext(s.ctx, `
		INSERT INTO cost_route_rm (crm_seq_id, crm_parent_product_sys_id, crm_rm_product_sys_id, crm_rm_type,
			crm_route_rm_ratio, crm_created_by, crm_updated_by)
		VALUES ($1, $2, $3, 'PRODUCT', 1.0, 'itest', 'itest')`, seqL2, productSysID, productSysID)
	require.NoError(t, err)

	return productSysID, headID
}

// TestDuplicateRoute_SameProduct_ForksFromDraftAndLocksSource is design.md
// §1.5's primary integration scenario: forking a DRAFT source in SAME_PRODUCT
// mode must (a) lock the source, (b) create a new DRAFT head for the SAME
// product with version incremented, and (c) copy the graph 1:1 (no product
// remap, same seq/RM counts).
func (s *CostRouteRepoSuite) TestDuplicateRoute_SameProduct_ForksFromDraftAndLocksSource() {
	productSysID, headID := s.seedDraftHeadWithGraph("F1DRAFT")

	out, err := s.repo.DuplicateRoute(s.ctx, costroute.DuplicateInput{
		SourceHeadID:   headID,
		IncludeRouting: true,
		TargetMode:     costroute.DuplicateTargetModeSameProduct,
		ActorUserID:    "itest",
	})
	s.Require().NoError(err)
	s.Equal(productSysID, out.NewProductSysID, "SAME_PRODUCT mode must target the same product")
	s.NotEqual(headID, out.NewHeadID, "must create a brand-new head")

	// Source head must now be LOCKED.
	srcHead, err := s.repo.GetHead(s.ctx, headID)
	s.Require().NoError(err)
	s.Equal(costroute.StatusLocked, srcHead.RoutingStatus, "source head must be auto-locked")

	// New head must be DRAFT, same product, version incremented, forked lineage.
	newHead, err := s.repo.GetHead(s.ctx, out.NewHeadID)
	s.Require().NoError(err)
	s.Equal(costroute.StatusDraft, newHead.RoutingStatus)
	s.Equal(productSysID, newHead.ProductSysID)
	s.Equal(int32(2), newHead.Version, "version must be incremented from source's 1")

	// Graph must be copied 1:1 -- same seq/RM counts, same product references
	// (identity copy: no upstream product cloning happens for SAME_PRODUCT).
	srcGraph, err := s.repo.GetGraph(s.ctx, headID)
	s.Require().NoError(err)
	newGraph, err := s.repo.GetGraph(s.ctx, out.NewHeadID)
	s.Require().NoError(err)
	s.Equal(len(srcGraph.Seqs), len(newGraph.Seqs), "seq count must match")
	var srcRmCount, newRmCount int
	for _, seq := range srcGraph.Seqs {
		srcRmCount += len(seq.Rms)
	}
	for _, seq := range newGraph.Seqs {
		newRmCount += len(seq.Rms)
		s.Equal(productSysID, seq.ProductSysID, "every copied seq must reference the SAME product")
		for _, rm := range seq.Rms {
			if rm.RmType == costroute.RmTypeProduct {
				s.Equal(productSysID, rm.RmProductSysID, "PRODUCT rm must reference the SAME product, not a clone")
			}
		}
	}
	s.Equal(srcRmCount, newRmCount, "rm count must match")

	// Cleanup (soft-deletes are fine to leave, but hard-delete to keep the DB tidy).
	_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cost_route_head WHERE crh_head_id IN ($1,$2)`, headID, out.NewHeadID)
	_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cost_product_master WHERE cpm_product_sys_id = $1`, productSysID)
}

// TestDuplicateRoute_SameProduct_AlreadyLockedSourceIsIdempotent covers
// design.md §1.5's second scenario: forking an already-LOCKED source must not
// error and must not double-lock -- the source simply stays LOCKED while a
// new DRAFT head is still created.
func (s *CostRouteRepoSuite) TestDuplicateRoute_SameProduct_AlreadyLockedSourceIsIdempotent() {
	productSysID, headID := s.seedDraftHeadWithGraph("F1LOCKED")

	// Pre-lock the source directly (bypassing the COMPLETE-gated domain Lock()
	// since this test only cares about the already-LOCKED starting state).
	_, err := s.db.ExecContext(s.ctx, `
		UPDATE cost_route_head SET crh_routing_status = 'LOCKED', crh_locked_by = 'itest', crh_locked_at = now()
		WHERE crh_head_id = $1`, headID)
	s.Require().NoError(err)

	out, err := s.repo.DuplicateRoute(s.ctx, costroute.DuplicateInput{
		SourceHeadID:   headID,
		IncludeRouting: true,
		TargetMode:     costroute.DuplicateTargetModeSameProduct,
		ActorUserID:    "itest",
	})
	s.Require().NoError(err, "forking an already-LOCKED source must not error")

	srcHead, err := s.repo.GetHead(s.ctx, headID)
	s.Require().NoError(err)
	s.Equal(costroute.StatusLocked, srcHead.RoutingStatus, "source must remain LOCKED")

	newHead, err := s.repo.GetHead(s.ctx, out.NewHeadID)
	s.Require().NoError(err)
	s.Equal(costroute.StatusDraft, newHead.RoutingStatus, "new head must still be created as DRAFT")

	_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cost_route_head WHERE crh_head_id IN ($1,$2)`, headID, out.NewHeadID)
	_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cost_product_master WHERE cpm_product_sys_id = $1`, productSysID)
}

// TestDuplicateRoute_SameProduct_ConcurrentForksOnlyOneSucceeds is design.md
// §1.5's concurrency backstop: two concurrent SAME_PRODUCT fork calls on the
// same source must not both succeed in creating two live (non-LOCKED) heads
// for the product -- the DB's uk_cost_route_head_active_per_product partial
// unique index is the final guard, and the loser must get a clean domain
// error (ErrAlreadyExists), not a raw constraint-violation 500.
func (s *CostRouteRepoSuite) TestDuplicateRoute_SameProduct_ConcurrentForksOnlyOneSucceeds() {
	productSysID, headID := s.seedDraftHeadWithGraph("F1RACE")

	type result struct {
		out costroute.DuplicateOutput
		err error
	}
	results := make(chan result, 2)
	for i := 0; i < 2; i++ {
		go func() {
			out, err := s.repo.DuplicateRoute(s.ctx, costroute.DuplicateInput{
				SourceHeadID:   headID,
				IncludeRouting: true,
				TargetMode:     costroute.DuplicateTargetModeSameProduct,
				ActorUserID:    "itest",
			})
			results <- result{out, err}
		}()
	}

	var successes, failures int
	var newHeadIDs []int64
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err == nil {
			successes++
			newHeadIDs = append(newHeadIDs, r.out.NewHeadID)
		} else {
			failures++
			s.Require().ErrorIsf(r.err, costroute.ErrAlreadyExists, "loser must get a clean domain error, got: %v", r.err)
		}
	}
	s.Equal(1, successes, "exactly one concurrent fork must succeed")
	s.Equal(1, failures, "exactly one concurrent fork must fail cleanly")

	cleanupIDs := append([]int64{headID}, newHeadIDs...)
	for _, id := range cleanupIDs {
		_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cost_route_head WHERE crh_head_id = $1`, id)
	}
	_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cost_product_master WHERE cpm_product_sys_id = $1`, productSysID)
}

// TestDuplicateRoute_NewProductMode_StillClonesUpstream is the regression
// guard for the pre-existing NEW_PRODUCT path (default TargetMode, unset by
// most existing callers): DuplicateRoute must still mint a brand-new FG
// product and clone the graph onto it exactly as before B1 introduced the
// SAME_PRODUCT branch at the top of the function.
func (s *CostRouteRepoSuite) TestDuplicateRoute_NewProductMode_StillClonesUpstream() {
	productSysID, headID := s.seedDraftHeadWithGraph("F1NEWPROD")

	out, err := s.repo.DuplicateRoute(s.ctx, costroute.DuplicateInput{
		SourceHeadID:   headID,
		IncludeRouting: true,
		NewCodePrefix:  crTestPrefix + "-F1NEWPROD-FORK",
		ActorUserID:    "itest",
		// TargetMode left unset (UNSPECIFIED = 0) -- must behave exactly like
		// the pre-B1 code path (implicit NEW_PRODUCT), not SAME_PRODUCT.
	})
	s.Require().NoError(err)
	s.NotEqual(productSysID, out.NewProductSysID, "NEW_PRODUCT mode must mint a brand-new product")
	s.NotEqual(headID, out.NewHeadID)

	// Source head must be untouched (still DRAFT, NOT auto-locked -- that only
	// happens in SAME_PRODUCT mode).
	srcHead, err := s.repo.GetHead(s.ctx, headID)
	s.Require().NoError(err)
	s.Equal(costroute.StatusDraft, srcHead.RoutingStatus, "NEW_PRODUCT mode must never lock the source")

	newGraph, err := s.repo.GetGraph(s.ctx, out.NewHeadID)
	s.Require().NoError(err)
	s.NotEmpty(newGraph.Seqs)
	for _, seq := range newGraph.Seqs {
		s.Equal(out.NewProductSysID, seq.ProductSysID, "cloned seq must reference the NEW product")
	}

	_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cost_route_head WHERE crh_head_id IN ($1,$2)`, headID, out.NewHeadID)
	_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cost_product_master WHERE cpm_product_sys_id IN ($1,$2)`, productSysID, out.NewProductSysID)
}

// seedMultiLevelRoute builds design.md §3.5's critical scenario fixture:
// masterbatch [ITEM] -> POY [PRODUCT-type intermediate, embedded seq] ->
// FG product, all inside ONE self-contained head (matching the real seed-data
// norm: intermediate levels are embedded seqs in the FG's own head, not a
// pointer into POY's independently-owned head). Returns the FG product/head
// plus the POY product sys id and the masterbatch item code, so the caller
// can run the cost-calc engine against both the source and (post-attach)
// target graphs with identical inputs.
func (s *CostRouteRepoSuite) seedMultiLevelRoute(codeSuffix string) (fgProductSysID, headID, poyProductSysID int64, mbItemCode string) {
	t := s.T()
	mbItemCode = "ZZTCRT_MB_" + codeSuffix

	var typeID int32
	require.NoError(t, s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_type (cpt_type_code, cpt_type_name)
		VALUES ($1, $2)
		ON CONFLICT (cpt_type_code) DO UPDATE SET cpt_type_name = EXCLUDED.cpt_type_name
		RETURNING cpt_type_id`, crTestTypeCode, "CR Route Test Type").Scan(&typeID))

	// POY intermediate product (own cost_product_master row, no route of its
	// own -- per the real seed-data norm, its own levels are embedded seqs
	// inside the FG's head, not a standalone active head).
	require.NoError(t, s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_is_active, cpm_created_by, cpm_updated_by)
		VALUES ($1, $2, $3, TRUE, 'itest', 'itest')
		RETURNING cpm_product_sys_id`, crTestPrefix+"-POY-"+codeSuffix, typeID, "cr itest poy "+codeSuffix).Scan(&poyProductSysID))

	// FG product (Product A).
	require.NoError(t, s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_is_active, cpm_created_by, cpm_updated_by)
		VALUES ($1, $2, $3, TRUE, 'itest', 'itest')
		RETURNING cpm_product_sys_id`, crTestPrefix+"-FG-"+codeSuffix, typeID, "cr itest fg "+codeSuffix).Scan(&fgProductSysID))

	require.NoError(t, s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_route_head (crh_product_sys_id, crh_routing_status, crh_version, crh_created_by, crh_updated_by)
		VALUES ($1, 'DRAFT', 1, 'itest', 'itest')
		RETURNING crh_head_id`, fgProductSysID).Scan(&headID))

	// Level 1: owned by the FG, consumes POY as a PRODUCT-type rm.
	var seqL1, seqL2 int64
	require.NoError(t, s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_route_seq (crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq, crs_created_by, crs_updated_by)
		VALUES ($1, $2, 1, 1, 'itest', 'itest') RETURNING crs_seq_id`, headID, fgProductSysID).Scan(&seqL1))
	// Level 2: owned by POY (embedded intermediate level), consumes masterbatch as an ITEM-type rm.
	require.NoError(t, s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_route_seq (crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq, crs_created_by, crs_updated_by)
		VALUES ($1, $2, 2, 1, 'itest', 'itest') RETURNING crs_seq_id`, headID, poyProductSysID).Scan(&seqL2))

	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO cost_route_rm (crm_seq_id, crm_parent_product_sys_id, crm_rm_product_sys_id, crm_rm_type,
			crm_route_rm_ratio, crm_created_by, crm_updated_by)
		VALUES ($1, $2, $3, 'PRODUCT', 2.0, 'itest', 'itest')`, seqL1, fgProductSysID, poyProductSysID)
	require.NoError(t, err)
	_, err = s.db.ExecContext(s.ctx, `
		INSERT INTO cost_route_rm (crm_seq_id, crm_parent_product_sys_id, crm_rm_item_code, crm_rm_type,
			crm_route_rm_ratio, crm_created_by, crm_updated_by)
		VALUES ($1, $2, $3, 'ITEM', 1.05, 'itest', 'itest')`, seqL2, poyProductSysID, mbItemCode)
	require.NoError(t, err)

	return fgProductSysID, headID, poyProductSysID, mbItemCode
}

// computeCost runs the real costcalc engine (ComputeProduct) against a graph
// for the given product, resolving the masterbatch ITEM cost from rmUnitCost
// and any PRODUCT-type upstream references from upstreamCosts. Fails the test
// on any compute error (e.g. ErrMissingUpstreamCost / ErrMissingRMCost).
func (s *CostRouteRepoSuite) computeCost(graph *costroute.Graph, productSysID int64, rmUnitCost float64, mbItemCode string, upstreamCosts map[int64]float64) float64 {
	t := s.T()
	in := costcalc.ComputeInput{
		ProductSysID: productSysID,
		Period:       "202609",
		CalcType:     costcalcdom.CalcTypeActual,
		Route:        graph,
		CAPP:         map[string]float64{},
		// No formulas: per resolveFinalCost's documented fallback (c), a product
		// with zero formulas resolves its final cost as pure COST_RM_TOTAL --
		// exactly what this test wants to compare, with no evaluator involved.
		RMCosts:       map[string]float64{mbItemCode + "|": rmUnitCost},
		UpstreamCosts: upstreamCosts,
		EvalCache:     evaluator.NewCache(),
	}
	out, err := costcalc.ComputeProduct(s.ctx, in)
	require.NoErrorf(t, err, "ComputeProduct(product=%d) must succeed against the real cost-calc engine", productSysID)
	require.NotNil(t, out)
	return out.CostPerUnit
}

// TestAttachRoute_MultiLevel_PreservesGraphAndComputesIdenticalCost is
// design.md §3.5's CRITICAL scenario: masterbatch [ITEM] -> POY [embedded
// PRODUCT seq] -> Product A [FG] attached onto target Product B. Verifies
// both the copied graph shape AND that the real cost-calc engine
// (costcalc.ComputeProduct) resolves B's route to the EXACT SAME cost as A's,
// proving the shared upstream (POY) reference survived the attach unbroken.
func (s *CostRouteRepoSuite) TestAttachRoute_MultiLevel_PreservesGraphAndComputesIdenticalCost() {
	fgProductSysID, headID, poyProductSysID, mbItemCode := s.seedMultiLevelRoute("ATT1")

	// Target product B: exists, has no route yet.
	var typeID int32
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = $1`, crTestTypeCode).Scan(&typeID))
	var targetProductSysID int64
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_is_active, cpm_created_by, cpm_updated_by)
		VALUES ($1, $2, $3, TRUE, 'itest', 'itest')
		RETURNING cpm_product_sys_id`, crTestPrefix+"-TARGETB-ATT1", typeID, "cr itest target b").Scan(&targetProductSysID))

	out, err := s.repo.AttachRoute(s.ctx, costroute.AttachInput{
		SourceHeadID:       headID,
		TargetProductSysID: targetProductSysID,
		ActorUserID:        "itest",
	})
	s.Require().NoError(err)
	s.NotEqual(headID, out.NewHeadID, "must create a brand-new head")

	// New head must be owned by the target product.
	newHead, err := s.repo.GetHead(s.ctx, out.NewHeadID)
	s.Require().NoError(err)
	s.Equal(costroute.StatusDraft, newHead.RoutingStatus)
	s.Equal(targetProductSysID, newHead.ProductSysID)

	// Source head/product must be untouched -- AttachRoute never mutates the source.
	srcHead, err := s.repo.GetHead(s.ctx, headID)
	s.Require().NoError(err)
	s.Equal(costroute.StatusDraft, srcHead.RoutingStatus, "attach must never lock or mutate the source head")

	srcGraph, err := s.repo.GetGraph(s.ctx, headID)
	s.Require().NoError(err)
	newGraph, err := s.repo.GetGraph(s.ctx, out.NewHeadID)
	s.Require().NoError(err)

	s.Equal(len(srcGraph.Seqs), len(newGraph.Seqs), "seq count must match (2: FG level + POY level)")
	s.Require().Len(newGraph.Seqs, 2)

	var topSeq, poySeq *costroute.Seq
	for _, seq := range newGraph.Seqs {
		switch seq.RouteLevel {
		case 1:
			topSeq = seq
		case 2:
			poySeq = seq
		}
	}
	s.Require().NotNil(topSeq, "top-level (FG) seq must exist in the copied graph")
	s.Require().NotNil(poySeq, "POY-level seq must exist in the copied graph")

	// Only the top-level FG seq's product is remapped to the target.
	s.Equal(targetProductSysID, topSeq.ProductSysID, "top-level seq must now be owned by the target product")
	// The POY-level seq keeps referencing the SAME existing POY product -- not a clone.
	s.Equal(poyProductSysID, poySeq.ProductSysID, "POY-level seq must be UNCHANGED, still the original POY product")

	// The top-level RM (PRODUCT -> POY) must still reference the SAME POY product.
	s.Require().Len(topSeq.Rms, 1)
	s.Equal(costroute.RmTypeProduct, topSeq.Rms[0].RmType)
	s.Equal(poyProductSysID, topSeq.Rms[0].RmProductSysID, "the copied PRODUCT rm must reference the SAME POY, not a clone")
	s.InDelta(2.0, topSeq.Rms[0].RouteRmRatio, 1e-9, "ratio must be preserved exactly")

	// The POY-level RM (ITEM -> masterbatch) must be an unchanged copy.
	s.Require().Len(poySeq.Rms, 1)
	s.Equal(costroute.RmTypeItem, poySeq.Rms[0].RmType)
	s.Equal(mbItemCode, poySeq.Rms[0].RmItemCode)
	s.InDelta(1.05, poySeq.Rms[0].RouteRmRatio, 1e-9, "ratio must be preserved exactly")

	// --- Run the REAL cost-calc engine against both graphs with identical inputs. ---
	const mbUnitCost = 40.0 // masterbatch price per kg, arbitrary but fixed for the assertion.

	// POY's own per-unit cost is computed identically whether read from the
	// source graph or the (unchanged) copy inside the new graph -- proving the
	// embedded upstream level survived the attach with no broken reference.
	poyCostFromSource := s.computeCost(srcGraph, poyProductSysID, mbUnitCost, mbItemCode, nil)
	poyCostFromAttached := s.computeCost(newGraph, poyProductSysID, mbUnitCost, mbItemCode, nil)
	s.InDelta(poyCostFromSource, poyCostFromAttached, 1e-9, "POY's cost must compute identically from the attached copy")
	s.InDelta(mbUnitCost*1.05, poyCostFromSource, 1e-9, "sanity: POY cost = masterbatch unit cost * ratio")

	upstream := map[int64]float64{poyProductSysID: poyCostFromSource}

	// Product A's (source FG) cost, computed via ComputeProduct -- no
	// ErrMissingUpstreamCost since POY's cost is supplied.
	fgCostA := s.computeCost(srcGraph, fgProductSysID, mbUnitCost, mbItemCode, upstream)
	// Product B's (attached target FG) cost, computed against the NEW graph
	// with ProductSysID=targetProductSysID (the remapped top-level seq).
	fgCostB := s.computeCost(newGraph, targetProductSysID, mbUnitCost, mbItemCode, upstream)

	s.InDelta(fgCostA, fgCostB, 1e-9, "B's attached route must compute the EXACT SAME cost as A's source route given identical inputs")
	s.InDelta(mbUnitCost*1.05*2.0, fgCostA, 1e-9, "sanity: FG cost = POY unit cost * FG ratio = (mb*1.05) * 2.0")

	s.T().Logf("REAL cost-calc engine output — POY unit cost: %.4f, Product A (source) cost: %.4f, Product B (attached) cost: %.4f",
		poyCostFromSource, fgCostA, fgCostB)

	_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cost_route_head WHERE crh_head_id IN ($1,$2)`, headID, out.NewHeadID)
	_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cost_product_master WHERE cpm_product_sys_id IN ($1,$2,$3)`, fgProductSysID, poyProductSysID, targetProductSysID)
}

// TestAttachRoute_TargetAlreadyHasActiveHead_RejectsWithFriendlyError covers
// design.md §3.5's reject-path scenario: attaching onto a target that already
// has a live (non-LOCKED) head must return the friendly domain sentinel, not
// a raw DB constraint-violation error.
func (s *CostRouteRepoSuite) TestAttachRoute_TargetAlreadyHasActiveHead_RejectsWithFriendlyError() {
	_, sourceHeadID, _, _ := s.seedMultiLevelRoute("ATT2SRC")
	targetProductSysID, targetHeadID := s.seedDraftHeadWithGraph("ATT2TGT")

	_, err := s.repo.AttachRoute(s.ctx, costroute.AttachInput{
		SourceHeadID:       sourceHeadID,
		TargetProductSysID: targetProductSysID,
		ActorUserID:        "itest",
	})
	s.Require().ErrorIs(err, costroute.ErrTargetProductHasActiveRoute, "must return the friendly sentinel, not a raw constraint error")

	// Cleanup: only the seeded fixtures (nothing new was created by the failed attach).
	fgID, err2 := s.repo.GetHead(s.ctx, sourceHeadID)
	s.Require().NoError(err2)
	_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cost_route_head WHERE crh_head_id IN ($1,$2)`, sourceHeadID, targetHeadID)
	_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cost_product_master WHERE cpm_product_sys_id IN ($1,$2)`, fgID.ProductSysID, targetProductSysID)
	// POY product from seedMultiLevelRoute is cleaned up via the FG's route seq FK cascade
	// on head delete only for seq/rm rows; the POY product master row itself needs explicit cleanup.
	_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cost_product_master WHERE cpm_product_code = $1`, crTestPrefix+"-POY-ATT2SRC")
}
