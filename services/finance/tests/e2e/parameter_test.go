// Package e2e provides end-to-end tests for the finance service gRPC API.
package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
)

// ParameterE2ETestSuite is the end-to-end test suite for Parameter service.
type ParameterE2ETestSuite struct {
	suite.Suite
	conn   *grpc.ClientConn
	client financev1.ParameterServiceClient
	ctx    context.Context // authenticated context with JWT
}

func TestParameterE2ESuite(t *testing.T) {
	if os.Getenv("E2E_TEST") != "true" {
		t.Skip("Skipping E2E test. Set E2E_TEST=true to run.")
	}
	suite.Run(t, new(ParameterE2ETestSuite))
}

func (s *ParameterE2ETestSuite) SetupSuite() {
	addr := getEnv("GRPC_ADDR", "localhost:50051")

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(s.T(), err)

	s.conn = conn
	s.client = financev1.NewParameterServiceClient(conn)

	// Generate test JWT and create authenticated context
	token := s.generateTestToken()
	md := metadata.Pairs("authorization", "Bearer "+token)
	s.ctx = metadata.NewOutgoingContext(context.Background(), md)

	// Wait for server to be ready
	s.waitForServer()
}

func (s *ParameterE2ETestSuite) generateTestToken() string {
	secret := getEnv("JWT_ACCESS_SECRET", "dev-access-secret-change-in-production")

	now := time.Now()
	claims := jwt.MapClaims{
		"token_type": "access",
		"user_id":    "e2e-test-user-id",
		"username":   "e2e_tester",
		"email":      "e2e@test.local",
		"roles":      []string{"SUPER_ADMIN"},
		"permissions": []string{
			"finance.master.parameter.create",
			"finance.master.parameter.view",
			"finance.master.parameter.update",
			"finance.master.parameter.delete",
			"finance.master.parameter.export",
			"finance.master.parameter.import",
		},
		"iss": "goapps-iam",
		"sub": "e2e-test-user-id",
		"iat": now.Unix(),
		"exp": now.Add(1 * time.Hour).Unix(),
		"jti": "e2e-param-test-token-id",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	require.NoError(s.T(), err)

	return signed
}

func (s *ParameterE2ETestSuite) TearDownSuite() {
	if s.conn != nil {
		s.conn.Close()
	}
}

func (s *ParameterE2ETestSuite) waitForServer() {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			s.T().Fatal("Server not ready within timeout")
		default:
			_, err := s.client.ListParameters(ctx, &financev1.ListParametersRequest{Page: 1, PageSize: 1})
			if err == nil {
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// =============================================================================
// Tests
// =============================================================================

func (s *ParameterE2ETestSuite) TestListParameters() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	resp, err := s.client.ListParameters(ctx, &financev1.ListParametersRequest{
		Page:     1,
		PageSize: 10,
	})

	require.NoError(s.T(), err)
	assert.NotNil(s.T(), resp.Base)
	assert.True(s.T(), resp.Base.IsSuccess)
	assert.Equal(s.T(), "200", resp.Base.StatusCode)
	assert.NotNil(s.T(), resp.Pagination)
}

func (s *ParameterE2ETestSuite) TestCreateParameter_Success() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	code := "E2E_" + time.Now().Format("150405")

	resp, err := s.client.CreateParameter(ctx, &financev1.CreateParameterRequest{
		ParamCode:      code,
		ParamName:      "E2E Test Parameter",
		ParamShortName: "E2E",
		DataType:       financev1.DataType_DATA_TYPE_NUMBER,
		ParamCategory:  financev1.ParamCategory_PARAM_CATEGORY_INPUT,
		DefaultValue:   "100",
		MinValue:       "0",
		MaxValue:       "9999",
	})

	require.NoError(s.T(), err)
	assert.True(s.T(), resp.Base.IsSuccess, "Expected success but got: %s", resp.Base.Message)
	if resp.Base.IsSuccess {
		assert.NotEmpty(s.T(), resp.Data.ParamId)
		assert.Equal(s.T(), code, resp.Data.ParamCode)
		assert.Equal(s.T(), "E2E Test Parameter", resp.Data.ParamName)
		assert.Equal(s.T(), financev1.DataType_DATA_TYPE_NUMBER, resp.Data.DataType)
		assert.Equal(s.T(), financev1.ParamCategory_PARAM_CATEGORY_INPUT, resp.Data.ParamCategory)
	}
}

func (s *ParameterE2ETestSuite) TestCreateParameter_ValidationError() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	resp, err := s.client.CreateParameter(ctx, &financev1.CreateParameterRequest{
		ParamCode:     "invalid_lowercase",
		ParamName:     "",
		DataType:      financev1.DataType_DATA_TYPE_UNSPECIFIED,
		ParamCategory: financev1.ParamCategory_PARAM_CATEGORY_UNSPECIFIED,
	})

	require.NoError(s.T(), err)
	assert.False(s.T(), resp.Base.IsSuccess)
	assert.Equal(s.T(), "400", resp.Base.StatusCode)
	assert.NotEmpty(s.T(), resp.Base.ValidationErrors)
}

func (s *ParameterE2ETestSuite) TestGetParameter_NotFound() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	resp, err := s.client.GetParameter(ctx, &financev1.GetParameterRequest{
		ParamId: "00000000-0000-0000-0000-000000000000",
	})

	require.NoError(s.T(), err)
	assert.False(s.T(), resp.Base.IsSuccess)
	assert.Equal(s.T(), "404", resp.Base.StatusCode)
}

func (s *ParameterE2ETestSuite) TestCRUDFlow() {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	code := "CRUD_" + time.Now().Format("150405")

	// 1. Create
	createResp, err := s.client.CreateParameter(ctx, &financev1.CreateParameterRequest{
		ParamCode:      code,
		ParamName:      "CRUD Test Param",
		ParamShortName: "CTP",
		DataType:       financev1.DataType_DATA_TYPE_NUMBER,
		ParamCategory:  financev1.ParamCategory_PARAM_CATEGORY_INPUT,
		DefaultValue:   "50.5",
		MinValue:       "0",
		MaxValue:       "1000",
	})
	require.NoError(s.T(), err)
	require.True(s.T(), createResp.Base.IsSuccess, "Create failed: %s", createResp.Base.Message)
	paramID := createResp.Data.ParamId

	// 2. Get
	getResp, err := s.client.GetParameter(ctx, &financev1.GetParameterRequest{ParamId: paramID})
	require.NoError(s.T(), err)
	assert.True(s.T(), getResp.Base.IsSuccess)
	assert.Equal(s.T(), code, getResp.Data.ParamCode)
	assert.Equal(s.T(), "CRUD Test Param", getResp.Data.ParamName)
	assert.Equal(s.T(), "50.5", getResp.Data.DefaultValue)

	// 3. Update
	updatedName := "CRUD Updated Param"
	updatedShortName := "CUP"
	newDataType := financev1.DataType_DATA_TYPE_TEXT
	updateResp, err := s.client.UpdateParameter(ctx, &financev1.UpdateParameterRequest{
		ParamId:        paramID,
		ParamName:      &updatedName,
		ParamShortName: &updatedShortName,
		DataType:       &newDataType,
	})
	require.NoError(s.T(), err)
	assert.True(s.T(), updateResp.Base.IsSuccess)
	assert.Equal(s.T(), "CRUD Updated Param", updateResp.Data.ParamName)
	assert.Equal(s.T(), "CUP", updateResp.Data.ParamShortName)
	assert.Equal(s.T(), financev1.DataType_DATA_TYPE_TEXT, updateResp.Data.DataType)

	// 4. Delete
	deleteResp, err := s.client.DeleteParameter(ctx, &financev1.DeleteParameterRequest{ParamId: paramID})
	require.NoError(s.T(), err)
	assert.True(s.T(), deleteResp.Base.IsSuccess)

	// 5. Verify deleted
	getDeletedResp, err := s.client.GetParameter(ctx, &financev1.GetParameterRequest{ParamId: paramID})
	require.NoError(s.T(), err)
	assert.False(s.T(), getDeletedResp.Base.IsSuccess)
	assert.Equal(s.T(), "404", getDeletedResp.Base.StatusCode)
}

func (s *ParameterE2ETestSuite) TestListWithFilters() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	// List with data type filter
	resp, err := s.client.ListParameters(ctx, &financev1.ListParametersRequest{
		Page:     1,
		PageSize: 50,
		DataType: financev1.DataType_DATA_TYPE_NUMBER,
	})

	require.NoError(s.T(), err)
	assert.True(s.T(), resp.Base.IsSuccess)

	for _, item := range resp.Data {
		assert.Equal(s.T(), financev1.DataType_DATA_TYPE_NUMBER, item.DataType)
	}

	// List with category filter
	resp2, err := s.client.ListParameters(ctx, &financev1.ListParametersRequest{
		Page:          1,
		PageSize:      50,
		ParamCategory: financev1.ParamCategory_PARAM_CATEGORY_RATE,
	})

	require.NoError(s.T(), err)
	assert.True(s.T(), resp2.Base.IsSuccess)

	for _, item := range resp2.Data {
		assert.Equal(s.T(), financev1.ParamCategory_PARAM_CATEGORY_RATE, item.ParamCategory)
	}
}

func (s *ParameterE2ETestSuite) TestDownloadParameterTemplate() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	resp, err := s.client.DownloadParameterTemplate(ctx, &financev1.DownloadParameterTemplateRequest{})

	require.NoError(s.T(), err)
	assert.True(s.T(), resp.Base.IsSuccess)
	assert.NotEmpty(s.T(), resp.FileContent)
	assert.Contains(s.T(), resp.FileName, ".xlsx")
}

func (s *ParameterE2ETestSuite) TestExportParameters() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	resp, err := s.client.ExportParameters(ctx, &financev1.ExportParametersRequest{})

	require.NoError(s.T(), err)
	assert.True(s.T(), resp.Base.IsSuccess)
	assert.NotEmpty(s.T(), resp.FileContent)
	assert.Contains(s.T(), resp.FileName, ".xlsx")
}
