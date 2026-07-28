package grpc

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib" // production driver
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// CostMasterLookupIntegrationSuite exercises the handler + repository against a
// real finance_db, mirroring the live projection PPC consumes.
type CostMasterLookupIntegrationSuite struct {
	suite.Suite
	db      *postgres.DB
	handler *CostMasterLookupHandler
	ctx     context.Context
}

func TestCostMasterLookupIntegrationSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(CostMasterLookupIntegrationSuite))
}

func (s *CostMasterLookupIntegrationSuite) SetupSuite() {
	s.ctx = context.Background()
	envOr := func(k, def string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return def
	}
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		envOr("TEST_DB_HOST", "localhost"), envOr("TEST_DB_PORT", "5434"),
		envOr("TEST_DB_USER", "finance"), envOr("TEST_DB_PASSWORD", "finance123"),
		envOr("TEST_DB_NAME", "finance_db"))
	raw, err := sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), raw.PingContext(s.ctx))
	s.db = postgres.NewDBFromSQL(raw)
	h, err := NewCostMasterLookupHandler(postgres.NewCostMasterLookupRepository(s.db))
	require.NoError(s.T(), err)
	s.handler = h
}

func (s *CostMasterLookupIntegrationSuite) TearDownSuite() {
	if s.db != nil {
		require.NoError(s.T(), s.db.Close())
	}
}

// firstProductSysID returns a real product sys id from the live DB, or 0.
func (s *CostMasterLookupIntegrationSuite) firstProductSysID() int64 {
	var id int64
	err := s.db.QueryRowContext(s.ctx, `SELECT cpm_product_sys_id FROM cost_product_master ORDER BY cpm_product_sys_id LIMIT 1`).Scan(&id)
	require.NoError(s.T(), err)
	return id
}

func (s *CostMasterLookupIntegrationSuite) TestGetProduct_ReturnsProduct() {
	id := s.firstProductSysID()
	resp, err := s.handler.GetCostProductMasterForPPC(s.ctx, &financev1.GetCostProductMasterForPPCRequest{ProductSysId: id})
	require.NoError(s.T(), err)
	require.True(s.T(), resp.GetBase().GetIsSuccess())
	require.NotNil(s.T(), resp.GetData())
	require.Equal(s.T(), id, resp.GetData().GetProductSysId())
	require.Empty(s.T(), resp.GetData().GetProductDenier(), "denier must be empty per G-005")
}

func (s *CostMasterLookupIntegrationSuite) TestGetProduct_MissingReturnsNotFound() {
	resp, err := s.handler.GetCostProductMasterForPPC(s.ctx, &financev1.GetCostProductMasterForPPCRequest{ProductSysId: 999999999})
	require.NoError(s.T(), err)
	require.False(s.T(), resp.GetBase().GetIsSuccess())
	require.Equal(s.T(), "404", resp.GetBase().GetStatusCode())
	require.Nil(s.T(), resp.GetData())
}

func (s *CostMasterLookupIntegrationSuite) TestBatchGetProducts_HandlesMissingID() {
	real := s.firstProductSysID()
	resp, err := s.handler.BatchGetCostProductMaster(s.ctx, &financev1.BatchGetCostProductMasterRequest{
		ProductSysIds: []int64{real, 999999999},
	})
	require.NoError(s.T(), err)
	require.True(s.T(), resp.GetBase().GetIsSuccess())
	// Only the real id resolves; the missing one is simply absent.
	require.Len(s.T(), resp.GetData(), 1)
	require.Equal(s.T(), real, resp.GetData()[0].GetProductSysId())
}

func (s *CostMasterLookupIntegrationSuite) TestListProducts_Paginated() {
	resp, err := s.handler.ListCostProductMasterForPPC(s.ctx, &financev1.ListCostProductMasterForPPCRequest{
		Page: 1, PageSize: 5,
	})
	require.NoError(s.T(), err)
	require.True(s.T(), resp.GetBase().GetIsSuccess())
	require.LessOrEqual(s.T(), len(resp.GetData()), 5)
	require.Greater(s.T(), resp.GetPagination().GetTotalItems(), int64(0))
}

func (s *CostMasterLookupIntegrationSuite) TestListGrades_NonEmpty() {
	resp, err := s.handler.ListProductGradesForPPC(s.ctx, &financev1.ListProductGradesForPPCRequest{
		Page: 1, PageSize: 50,
	})
	require.NoError(s.T(), err)
	require.True(s.T(), resp.GetBase().GetIsSuccess())
	require.NotEmpty(s.T(), resp.GetData(), "finance_db seeds product grades")
}

func (s *CostMasterLookupIntegrationSuite) TestListParameters_NonEmpty() {
	resp, err := s.handler.ListProductParametersForPPC(s.ctx, &financev1.ListProductParametersForPPCRequest{
		Page: 1, PageSize: 50,
	})
	require.NoError(s.T(), err)
	require.True(s.T(), resp.GetBase().GetIsSuccess())
	require.NotEmpty(s.T(), resp.GetData())
}

func (s *CostMasterLookupIntegrationSuite) TestBatchGetParameterValues_Typed() {
	// Find a product that actually has parameter values.
	var pid int64
	err := s.db.QueryRowContext(s.ctx, `SELECT cpp_product_sys_id FROM cost_product_parameter LIMIT 1`).Scan(&pid)
	require.NoError(s.T(), err)
	resp, err := s.handler.BatchGetProductParameterValues(s.ctx, &financev1.BatchGetProductParameterValuesRequest{
		ProductSysIds: []int64{pid},
	})
	require.NoError(s.T(), err)
	require.True(s.T(), resp.GetBase().GetIsSuccess())
	require.NotEmpty(s.T(), resp.GetData())
	for _, v := range resp.GetData() {
		require.Equal(s.T(), pid, v.GetProductSysId())
		require.NotEmpty(s.T(), v.GetParamId())
	}
}

func (s *CostMasterLookupIntegrationSuite) TestGetProductRoute_WhenReleasedRouteExists() {
	// Pick a product that has a COMPLETE/LOCKED route head.
	var pid int64
	err := s.db.QueryRowContext(s.ctx,
		`SELECT crh_product_sys_id FROM cost_route_head
		 WHERE crh_routing_status IN ('COMPLETE','LOCKED') AND crh_deleted_at IS NULL LIMIT 1`).Scan(&pid)
	if err != nil {
		s.T().Skip("no released route in finance_db")
		return
	}
	resp, err := s.handler.GetProductRouteForPPC(s.ctx, &financev1.GetProductRouteForPPCRequest{ProductSysId: pid})
	require.NoError(s.T(), err)
	require.True(s.T(), resp.GetBase().GetIsSuccess())
	require.NotNil(s.T(), resp.GetData())
	require.Equal(s.T(), pid, resp.GetData().GetProductSysId())
	require.Contains(s.T(), []string{"COMPLETE", "LOCKED"}, resp.GetData().GetRoutingStatus())
}
