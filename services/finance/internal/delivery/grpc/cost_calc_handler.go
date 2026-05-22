package grpc

import (
	"context"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CostCalcHandler implements financev1.CostCalcServiceServer.
// S8a foundation: all RPCs return Unimplemented; real handlers land in S8b.
type CostCalcHandler struct {
	financev1.UnimplementedCostCalcServiceServer
}

// NewCostCalcHandler constructs a stub handler.
func NewCostCalcHandler() *CostCalcHandler {
	return &CostCalcHandler{}
}

func costCalcUnimplemented(name string) error {
	return status.Errorf(codes.Unimplemented, "%s not yet implemented (S8a foundation)", name)
}

// TriggerCalcJob is a stub.
func (h *CostCalcHandler) TriggerCalcJob(_ context.Context, _ *financev1.TriggerCalcJobRequest) (*financev1.TriggerCalcJobResponse, error) {
	return nil, costCalcUnimplemented("TriggerCalcJob")
}

// GetCalcJob is a stub.
func (h *CostCalcHandler) GetCalcJob(_ context.Context, _ *financev1.GetCalcJobRequest) (*financev1.GetCalcJobResponse, error) {
	return nil, costCalcUnimplemented("GetCalcJob")
}

// ListCalcJobs is a stub.
func (h *CostCalcHandler) ListCalcJobs(_ context.Context, _ *financev1.ListCalcJobsRequest) (*financev1.ListCalcJobsResponse, error) {
	return nil, costCalcUnimplemented("ListCalcJobs")
}

// ListCalcJobChunks is a stub.
func (h *CostCalcHandler) ListCalcJobChunks(_ context.Context, _ *financev1.ListCalcJobChunksRequest) (*financev1.ListCalcJobChunksResponse, error) {
	return nil, costCalcUnimplemented("ListCalcJobChunks")
}

// ListCalcJobProducts is a stub.
func (h *CostCalcHandler) ListCalcJobProducts(_ context.Context, _ *financev1.ListCalcJobProductsRequest) (*financev1.ListCalcJobProductsResponse, error) {
	return nil, costCalcUnimplemented("ListCalcJobProducts")
}

// CancelCalcJob is a stub.
func (h *CostCalcHandler) CancelCalcJob(_ context.Context, _ *financev1.CancelCalcJobRequest) (*financev1.CancelCalcJobResponse, error) {
	return nil, costCalcUnimplemented("CancelCalcJob")
}

// GetCostResult is a stub.
func (h *CostCalcHandler) GetCostResult(_ context.Context, _ *financev1.GetCostResultRequest) (*financev1.GetCostResultResponse, error) {
	return nil, costCalcUnimplemented("GetCostResult")
}

// GetCostBreakdown is a stub.
func (h *CostCalcHandler) GetCostBreakdown(_ context.Context, _ *financev1.GetCostBreakdownRequest) (*financev1.GetCostBreakdownResponse, error) {
	return nil, costCalcUnimplemented("GetCostBreakdown")
}

// ListCostHistory is a stub.
func (h *CostCalcHandler) ListCostHistory(_ context.Context, _ *financev1.ListCostHistoryRequest) (*financev1.ListCostHistoryResponse, error) {
	return nil, costCalcUnimplemented("ListCostHistory")
}

// VerifyCostResult is a stub.
func (h *CostCalcHandler) VerifyCostResult(_ context.Context, _ *financev1.VerifyCostResultRequest) (*financev1.VerifyCostResultResponse, error) {
	return nil, costCalcUnimplemented("VerifyCostResult")
}

// ApproveCostResult is a stub.
func (h *CostCalcHandler) ApproveCostResult(_ context.Context, _ *financev1.ApproveCostResultRequest) (*financev1.ApproveCostResultResponse, error) {
	return nil, costCalcUnimplemented("ApproveCostResult")
}
