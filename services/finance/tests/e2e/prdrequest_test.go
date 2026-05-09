// Package e2e provides end-to-end tests for the finance service gRPC API.
package e2e

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
)

// PrdRequestE2ESuite covers the full product-request lifecycle via gRPC.
type PrdRequestE2ESuite struct {
	suite.Suite
	conn          *grpc.ClientConn
	reqClient     financev1.ProductRequestServiceClient
	productClient financev1.ProductServiceClient
	ctx           context.Context
	seedRequests  []string // request IDs to clean up
	seedProducts  []string // product IDs to clean up
}

func TestPrdRequestE2ESuite(t *testing.T) {
	if os.Getenv("E2E_TEST") != "true" {
		t.Skip("Skipping E2E test. Set E2E_TEST=true to run.")
	}
	suite.Run(t, new(PrdRequestE2ESuite))
}

func (s *PrdRequestE2ESuite) SetupSuite() {
	addr := getEnv("GRPC_ADDR", "localhost:50051")
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(s.T(), err)
	s.conn = conn
	s.reqClient = financev1.NewProductRequestServiceClient(conn)
	s.productClient = financev1.NewProductServiceClient(conn)

	token := s.generateTestToken()
	md := metadata.Pairs("authorization", "Bearer "+token)
	s.ctx = metadata.NewOutgoingContext(context.Background(), md)

	s.waitForRequestServer()
}

// generateTestToken issues a JWT with product-request and product-master permissions.
// user_id must be a valid UUID — the CreateRequest handler calls uuid.Parse on it.
func (s *PrdRequestE2ESuite) generateTestToken() string {
	secret := getEnv("JWT_ACCESS_SECRET", "dev-access-secret-change-in-production")
	now := time.Now()
	claims := jwt.MapClaims{
		"token_type": "access",
		"user_id":    "00000000-0000-0000-0000-000000000002",
		"username":   "e2e_prdrequest",
		"email":      "e2e_prdrequest@test.local",
		"roles":      []string{"SUPER_ADMIN"},
		"permissions": []string{
			// Product request permissions
			"finance.product.request.view",
			"finance.product.request.create",
			"finance.product.request.update",
			"finance.product.request.delete",
			"finance.product.request.assign",
			"finance.product.request.resolve",
			"finance.product.request.reject",
			// Product master permissions (needed for SearchExistingProducts + seeding)
			"finance.product.master.view",
			"finance.product.master.create",
			"finance.product.master.delete",
		},
		"iss": "goapps-iam",
		"sub": "00000000-0000-0000-0000-000000000002",
		"iat": now.Unix(),
		"exp": now.Add(1 * time.Hour).Unix(),
		"jti": "e2e-prdrequest-token-id",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	require.NoError(s.T(), err)
	return signed
}

func (s *PrdRequestE2ESuite) waitForRequestServer() {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			s.T().Fatal("Server not ready within timeout")
		default:
			_, err := s.reqClient.ListRequests(ctx, &financev1.ListRequestsRequest{
				Pagination: &commonv1.PaginationRequest{Page: 1, PageSize: 1},
			})
			if err == nil {
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func (s *PrdRequestE2ESuite) TearDownSuite() {
	// Delete requests (reject first to put in terminal state, then delete).
	for _, id := range s.seedRequests {
		ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		_, _ = s.reqClient.DeleteRequest(ctx, &financev1.DeleteRequestRequest{RequestId: id})
		cancel()
	}
	// Delete seeded products.
	for _, id := range s.seedProducts {
		ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		_, _ = s.productClient.DeleteProduct(ctx, &financev1.DeleteProductRequest{ProductId: id})
		cancel()
	}
	if s.conn != nil {
		_ = s.conn.Close()
	}
}

func (s *PrdRequestE2ESuite) seedRequest(id string) {
	s.seedRequests = append(s.seedRequests, id)
}

func (s *PrdRequestE2ESuite) seedProduct(id string) {
	s.seedProducts = append(s.seedProducts, id)
}

// createRequest is a helper that creates a request and registers it for cleanup.
func (s *PrdRequestE2ESuite) createRequest(ctx context.Context, title string) *financev1.ProductRequest {
	s.T().Helper()
	resp, err := s.reqClient.CreateRequest(ctx, &financev1.CreateRequestRequest{
		Title:           title,
		Description:     "E2E test request: " + title,
		TargetSpecsJson: `{"color":"red"}`,
	})
	require.NoError(s.T(), err)
	require.True(s.T(), resp.GetBase().GetIsSuccess(), "CreateRequest failed: %s", resp.GetBase().GetMessage())
	s.seedRequest(resp.GetData().GetRequestId())
	return resp.GetData()
}

// =============================================================================
// Tests
// =============================================================================

// TestE2E_PrdRequest_FullLifecycle exercises Create → Get → Update → List.
func (s *PrdRequestE2ESuite) TestE2E_PrdRequest_FullLifecycle() {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	// Create
	createResp, err := s.reqClient.CreateRequest(ctx, &financev1.CreateRequestRequest{
		Title:           "E2E Full Lifecycle Request",
		Description:     "Full lifecycle test",
		TargetSpecsJson: `{"weight":"200g"}`,
	})
	require.NoError(s.T(), err)
	require.True(s.T(), createResp.GetBase().GetIsSuccess(), "Create failed: %s", createResp.GetBase().GetMessage())
	requestID := createResp.GetData().GetRequestId()
	s.seedRequest(requestID)
	assert.NotEmpty(s.T(), requestID)
	assert.Equal(s.T(), "OPEN", createResp.GetData().GetStatus())

	// Get
	getResp, err := s.reqClient.GetRequest(ctx, &financev1.GetRequestRequest{RequestId: requestID})
	require.NoError(s.T(), err)
	assert.True(s.T(), getResp.GetBase().GetIsSuccess())
	assert.Equal(s.T(), "E2E Full Lifecycle Request", getResp.GetData().GetTitle())
	assert.Equal(s.T(), "OPEN", getResp.GetData().GetStatus())

	// Update
	updateResp, err := s.reqClient.UpdateRequest(ctx, &financev1.UpdateRequestRequest{
		RequestId:   requestID,
		Title:       "E2E Full Lifecycle Request Updated",
		Description: "Updated description",
	})
	require.NoError(s.T(), err)
	assert.True(s.T(), updateResp.GetBase().GetIsSuccess(), "Update failed: %s", updateResp.GetBase().GetMessage())
	assert.Equal(s.T(), "E2E Full Lifecycle Request Updated", updateResp.GetData().GetTitle())

	// List — verify request appears in the list
	listResp, err := s.reqClient.ListRequests(ctx, &financev1.ListRequestsRequest{
		Pagination: &commonv1.PaginationRequest{Page: 1, PageSize: 10},
		Search:     "E2E Full Lifecycle",
	})
	require.NoError(s.T(), err)
	assert.True(s.T(), listResp.GetBase().GetIsSuccess())
	assert.GreaterOrEqual(s.T(), len(listResp.GetData()), 1)
}

// TestE2E_PrdRequest_TicketNoFormat verifies ticket_no matches PR-YYYYMM-NNN pattern.
func (s *PrdRequestE2ESuite) TestE2E_PrdRequest_TicketNoFormat() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	req := s.createRequest(ctx, "Ticket Format Test")
	ticketNo := req.GetTicketNo()
	assert.NotEmpty(s.T(), ticketNo)

	pattern := regexp.MustCompile(`^PR-\d{6}-\d{3,}$`)
	assert.True(s.T(), pattern.MatchString(ticketNo),
		"ticket_no %q does not match PR-YYYYMM-NNN format", ticketNo)
}

// TestE2E_PrdRequest_AssignAutoTransitionsOpenToInReview verifies that assigning
// a request in OPEN status transitions it to IN_REVIEW.
func (s *PrdRequestE2ESuite) TestE2E_PrdRequest_AssignAutoTransitionsOpenToInReview() {
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()

	req := s.createRequest(ctx, "Assign Transition Test")
	assert.Equal(s.T(), "OPEN", req.GetStatus())

	assigneeID := uuid.NewString()
	assignResp, err := s.reqClient.AssignRequest(ctx, &financev1.AssignRequestRequest{
		RequestId:  req.GetRequestId(),
		AssigneeId: assigneeID,
	})
	require.NoError(s.T(), err)
	assert.True(s.T(), assignResp.GetBase().GetIsSuccess(), "Assign failed: %s", assignResp.GetBase().GetMessage())
	assert.Equal(s.T(), "IN_REVIEW", assignResp.GetData().GetStatus())
	assert.Equal(s.T(), assigneeID, assignResp.GetData().GetAssignedTo())
}

// TestE2E_PrdRequest_RejectThenResolveFails verifies that resolving a rejected request fails.
func (s *PrdRequestE2ESuite) TestE2E_PrdRequest_RejectThenResolveFails() {
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()

	req := s.createRequest(ctx, "Reject Then Resolve Test")

	// Reject the request
	rejectResp, err := s.reqClient.RejectRequest(ctx, &financev1.RejectRequestRequest{
		RequestId: req.GetRequestId(),
		Reason:    "Not a viable product request",
	})
	require.NoError(s.T(), err)
	assert.True(s.T(), rejectResp.GetBase().GetIsSuccess(), "Reject failed: %s", rejectResp.GetBase().GetMessage())
	assert.Equal(s.T(), "REJECTED", rejectResp.GetData().GetStatus())

	// Attempt to resolve the already-rejected request — must fail
	resolveResp, err := s.reqClient.ResolveRequest(ctx, &financev1.ResolveRequestRequest{
		RequestId:      req.GetRequestId(),
		ProductId:      uuid.NewString(),
		ResolutionNote: "should not work",
	})
	require.NoError(s.T(), err)
	assert.False(s.T(), resolveResp.GetBase().GetIsSuccess(),
		"Expected resolve to fail on a rejected request but it succeeded")
}

// TestE2E_PrdRequest_ResolveWithProductId verifies that resolving with a linked product sets status RESOLVED.
func (s *PrdRequestE2ESuite) TestE2E_PrdRequest_ResolveWithProductId() {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	// Create a real product to link the request to.
	pCode := uniqueProductCode("RSLV")
	pResp, err := s.productClient.CreateProduct(ctx, &financev1.CreateProductRequest{
		ProductCode:     pCode,
		ProductName:     "E2E Resolve Target Product",
		ProductItemCode: "ITM-" + pCode,
		CreatedByDeptId: uuid.NewString(),
		Purpose:         "COMMERCIAL",
	})
	require.NoError(s.T(), err)
	require.True(s.T(), pResp.GetBase().GetIsSuccess(), "Product create failed: %s", pResp.GetBase().GetMessage())
	linkedProductID := pResp.GetData().GetProductId()
	s.seedProduct(linkedProductID)

	// Create request and resolve it.
	req := s.createRequest(ctx, "Resolve With Product Test")

	resolveResp, err := s.reqClient.ResolveRequest(ctx, &financev1.ResolveRequestRequest{
		RequestId:      req.GetRequestId(),
		ProductId:      linkedProductID,
		ResolutionNote: "resolved by e2e test",
	})
	require.NoError(s.T(), err)
	assert.True(s.T(), resolveResp.GetBase().GetIsSuccess(), "Resolve failed: %s", resolveResp.GetBase().GetMessage())
	assert.Equal(s.T(), "RESOLVED", resolveResp.GetData().GetStatus())
	assert.Equal(s.T(), linkedProductID, resolveResp.GetData().GetResolvedProductId())

	// Re-Get to confirm persistent state.
	getResp, err := s.reqClient.GetRequest(ctx, &financev1.GetRequestRequest{
		RequestId: req.GetRequestId(),
	})
	require.NoError(s.T(), err)
	assert.True(s.T(), getResp.GetBase().GetIsSuccess())
	assert.Equal(s.T(), "RESOLVED", getResp.GetData().GetStatus())
	assert.Equal(s.T(), linkedProductID, getResp.GetData().GetResolvedProductId())
}

// TestE2E_PrdRequest_ResolveWithoutProduct_Fails verifies that an empty product_id
// is rejected by the handler before reaching domain logic.
func (s *PrdRequestE2ESuite) TestE2E_PrdRequest_ResolveWithoutProduct_Fails() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	req := s.createRequest(ctx, "Resolve Without Product Test")

	// Send empty product_id — uuid.Parse("") will fail in the handler.
	resolveResp, err := s.reqClient.ResolveRequest(ctx, &financev1.ResolveRequestRequest{
		RequestId:      req.GetRequestId(),
		ProductId:      "", // empty — invalid
		ResolutionNote: "no product provided",
	})
	require.NoError(s.T(), err)
	assert.False(s.T(), resolveResp.GetBase().GetIsSuccess(),
		"Expected failure for empty product_id but got success")
}

// TestE2E_PrdRequest_SearchExistingProducts verifies that a product created via
// ProductService can be found by SearchExistingProducts on the request service.
func (s *PrdRequestE2ESuite) TestE2E_PrdRequest_SearchExistingProducts() {
	ctx, cancel := context.WithTimeout(s.ctx, 20*time.Second)
	defer cancel()

	// Create a product with a distinctive name.
	pCode := uniqueProductCode("SRCH")
	pResp, err := s.productClient.CreateProduct(ctx, &financev1.CreateProductRequest{
		ProductCode:     pCode,
		ProductName:     fmt.Sprintf("Cotton Yarn 30/1 %s", pCode),
		ProductItemCode: "ITM-" + pCode,
		CreatedByDeptId: uuid.NewString(),
		Purpose:         "COMMERCIAL",
	})
	require.NoError(s.T(), err)
	require.True(s.T(), pResp.GetBase().GetIsSuccess(), "Product create failed: %s", pResp.GetBase().GetMessage())
	s.seedProduct(pResp.GetData().GetProductId())

	// Search for it.
	searchResp, err := s.reqClient.SearchExistingProducts(ctx, &financev1.SearchExistingProductsRequest{
		Query: "Cotton",
		Limit: 10,
	})
	require.NoError(s.T(), err)
	assert.True(s.T(), searchResp.GetBase().GetIsSuccess(), "Search failed: %s", searchResp.GetBase().GetMessage())
	assert.GreaterOrEqual(s.T(), len(searchResp.GetData()), 1,
		"Expected at least one result for query 'Cotton'")

	// Verify our product is in the results.
	found := false
	for _, p := range searchResp.GetData() {
		if p.GetProductId() == pResp.GetData().GetProductId() {
			found = true
			break
		}
	}
	assert.True(s.T(), found, "Created product %s not found in SearchExistingProducts results", pCode)
}

// TestE2E_PrdRequest_ListFilters verifies list filtering by status.
func (s *PrdRequestE2ESuite) TestE2E_PrdRequest_ListFilters() {
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()

	// Create a request that we can filter on.
	_ = s.createRequest(ctx, "Filter Test Request")

	// List with OPEN status filter.
	listResp, err := s.reqClient.ListRequests(ctx, &financev1.ListRequestsRequest{
		Pagination: &commonv1.PaginationRequest{Page: 1, PageSize: 50},
		Status:     "OPEN",
	})
	require.NoError(s.T(), err)
	assert.True(s.T(), listResp.GetBase().GetIsSuccess())
	// All returned items should have OPEN status.
	for _, item := range listResp.GetData() {
		assert.Equal(s.T(), "OPEN", item.GetStatus())
	}
}

// TestE2E_PrdRequest_DeleteRequest verifies soft-delete then not-found.
func (s *PrdRequestE2ESuite) TestE2E_PrdRequest_DeleteRequest() {
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()

	req := s.createRequest(ctx, "Delete Test Request")
	requestID := req.GetRequestId()

	// Delete
	deleteResp, err := s.reqClient.DeleteRequest(ctx, &financev1.DeleteRequestRequest{RequestId: requestID})
	require.NoError(s.T(), err)
	assert.True(s.T(), deleteResp.GetBase().GetIsSuccess(), "Delete failed: %s", deleteResp.GetBase().GetMessage())

	// Subsequent Get returns not found
	getResp, err := s.reqClient.GetRequest(ctx, &financev1.GetRequestRequest{RequestId: requestID})
	require.NoError(s.T(), err)
	assert.False(s.T(), getResp.GetBase().GetIsSuccess())
	assert.Equal(s.T(), "404", getResp.GetBase().GetStatusCode())
}
