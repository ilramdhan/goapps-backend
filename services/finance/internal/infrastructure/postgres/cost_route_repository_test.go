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
