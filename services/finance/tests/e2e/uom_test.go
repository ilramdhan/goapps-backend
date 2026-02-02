// Package e2e provides end-to-end tests for the finance service gRPC API.
package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
)

// E2ETestSuite is the end-to-end test suite
type E2ETestSuite struct {
	suite.Suite
	conn   *grpc.ClientConn
	client financev1.UOMServiceClient
	ctx    context.Context
}

func TestE2ESuite(t *testing.T) {
	// Skip if not in e2e test mode
	if os.Getenv("E2E_TEST") != "true" {
		t.Skip("Skipping E2E test. Set E2E_TEST=true to run.")
	}
	suite.Run(t, new(E2ETestSuite))
}

func (s *E2ETestSuite) SetupSuite() {
	s.ctx = context.Background()

	// Get gRPC server address
	addr := getEnv("GRPC_ADDR", "localhost:50051")

	// Connect to gRPC server
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(s.T(), err)

	s.conn = conn
	s.client = financev1.NewUOMServiceClient(conn)

	// Wait for server to be ready
	s.waitForServer()
}

func (s *E2ETestSuite) TearDownSuite() {
	if s.conn != nil {
		s.conn.Close()
	}
}

func (s *E2ETestSuite) waitForServer() {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			s.T().Fatal("Server not ready within timeout")
		default:
			_, err := s.client.ListUOMs(ctx, &financev1.ListUOMsRequest{Page: 1, PageSize: 1})
			if err == nil {
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func (s *E2ETestSuite) TestListUOMs() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	resp, err := s.client.ListUOMs(ctx, &financev1.ListUOMsRequest{
		Page:     1,
		PageSize: 10,
	})

	require.NoError(s.T(), err)
	assert.NotNil(s.T(), resp.Base)
	assert.True(s.T(), resp.Base.IsSuccess)
	assert.Equal(s.T(), "200", resp.Base.StatusCode)
	assert.NotNil(s.T(), resp.Pagination)
}

func (s *E2ETestSuite) TestCreateUOM_Success() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	// Generate unique code for test
	code := "E2E_" + time.Now().Format("150405")

	resp, err := s.client.CreateUOM(ctx, &financev1.CreateUOMRequest{
		UomCode:     code,
		UomName:     "E2E Test Unit",
		UomCategory: financev1.UOMCategory_UOM_CATEGORY_QUANTITY,
		Description: "Created by E2E test",
	})

	require.NoError(s.T(), err)
	assert.True(s.T(), resp.Base.IsSuccess, "Expected success but got: %s", resp.Base.Message)
	if resp.Base.IsSuccess {
		assert.NotEmpty(s.T(), resp.Data.UomId)
		assert.Equal(s.T(), code, resp.Data.UomCode)
		assert.Equal(s.T(), "E2E Test Unit", resp.Data.UomName)
	}
}

func (s *E2ETestSuite) TestCreateUOM_ValidationError() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	resp, err := s.client.CreateUOM(ctx, &financev1.CreateUOMRequest{
		UomCode:     "invalid_lowercase", // Invalid: lowercase
		UomName:     "",                  // Invalid: empty
		UomCategory: financev1.UOMCategory_UOM_CATEGORY_UNSPECIFIED,
	})

	// Should NOT return error - validation errors in BaseResponse
	require.NoError(s.T(), err)
	assert.False(s.T(), resp.Base.IsSuccess)
	assert.Equal(s.T(), "400", resp.Base.StatusCode)
	assert.NotEmpty(s.T(), resp.Base.ValidationErrors)
	assert.Equal(s.T(), "Validation failed", resp.Base.Message)

	// Check validation errors contain expected fields
	fields := make(map[string]bool)
	for _, ve := range resp.Base.ValidationErrors {
		fields[ve.Field] = true
	}
	assert.True(s.T(), fields["uom_code"], "Expected validation error for uom_code")
	assert.True(s.T(), fields["uom_name"], "Expected validation error for uom_name")
}

func (s *E2ETestSuite) TestGetUOM_NotFound() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	resp, err := s.client.GetUOM(ctx, &financev1.GetUOMRequest{
		UomId: "00000000-0000-0000-0000-000000000000",
	})

	require.NoError(s.T(), err)
	assert.False(s.T(), resp.Base.IsSuccess)
	assert.Equal(s.T(), "404", resp.Base.StatusCode)
}

func (s *E2ETestSuite) TestCRUDFlow() {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	code := "CRUD_" + time.Now().Format("150405")

	// 1. Create
	createResp, err := s.client.CreateUOM(ctx, &financev1.CreateUOMRequest{
		UomCode:     code,
		UomName:     "CRUD Test",
		UomCategory: financev1.UOMCategory_UOM_CATEGORY_WEIGHT,
		Description: "CRUD flow test",
	})
	require.NoError(s.T(), err)
	require.True(s.T(), createResp.Base.IsSuccess, "Create failed: %s", createResp.Base.Message)
	uomID := createResp.Data.UomId

	// 2. Get
	getResp, err := s.client.GetUOM(ctx, &financev1.GetUOMRequest{UomId: uomID})
	require.NoError(s.T(), err)
	assert.True(s.T(), getResp.Base.IsSuccess)
	assert.Equal(s.T(), code, getResp.Data.UomCode)

	// 3. Update
	updatedName := "CRUD Updated"
	updatedDesc := "Updated description"
	updateResp, err := s.client.UpdateUOM(ctx, &financev1.UpdateUOMRequest{
		UomId:       uomID,
		UomName:     &updatedName,
		Description: &updatedDesc,
	})
	require.NoError(s.T(), err)
	assert.True(s.T(), updateResp.Base.IsSuccess)
	assert.Equal(s.T(), "CRUD Updated", updateResp.Data.UomName)

	// 4. Delete
	deleteResp, err := s.client.DeleteUOM(ctx, &financev1.DeleteUOMRequest{UomId: uomID})
	require.NoError(s.T(), err)
	assert.True(s.T(), deleteResp.Base.IsSuccess)

	// 5. Verify deleted
	getDeletedResp, err := s.client.GetUOM(ctx, &financev1.GetUOMRequest{UomId: uomID})
	require.NoError(s.T(), err)
	assert.False(s.T(), getDeletedResp.Base.IsSuccess)
	assert.Equal(s.T(), "404", getDeletedResp.Base.StatusCode)
}

func (s *E2ETestSuite) TestListWithFilters() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	// List with category filter
	resp, err := s.client.ListUOMs(ctx, &financev1.ListUOMsRequest{
		Page:     1,
		PageSize: 50,
		Category: financev1.UOMCategory_UOM_CATEGORY_WEIGHT,
	})

	require.NoError(s.T(), err)
	assert.True(s.T(), resp.Base.IsSuccess)

	// All returned items should be WEIGHT category
	for _, item := range resp.Data {
		assert.Equal(s.T(), financev1.UOMCategory_UOM_CATEGORY_WEIGHT, item.UomCategory)
	}
}

func (s *E2ETestSuite) TestDownloadTemplate() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	resp, err := s.client.DownloadTemplate(ctx, &financev1.DownloadTemplateRequest{})

	require.NoError(s.T(), err)
	assert.True(s.T(), resp.Base.IsSuccess)
	assert.NotEmpty(s.T(), resp.FileContent)
	assert.Contains(s.T(), resp.FileName, ".xlsx")
}

func (s *E2ETestSuite) TestExportUOMs() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	resp, err := s.client.ExportUOMs(ctx, &financev1.ExportUOMsRequest{})

	require.NoError(s.T(), err)
	assert.True(s.T(), resp.Base.IsSuccess)
	assert.NotEmpty(s.T(), resp.FileContent)
	assert.Contains(s.T(), resp.FileName, ".xlsx")
}

// Helper function
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
