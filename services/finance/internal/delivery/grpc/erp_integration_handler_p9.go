package grpc

import (
	"context"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"

	erpapp "github.com/mutugading/goapps-backend/services/finance/internal/application/erpintegration"
)

// erpReconLister reads the persisted recon outcome of a batch.
type erpReconLister interface {
	ListRecon(ctx context.Context, batchID int64) ([]domain.ReconExportRow, error)
}

// ExportErpRecon renders the batch recon outcome as .xlsx.
func (h *ErpIntegrationHandler) ExportErpRecon(ctx context.Context, req *financev1.ExportErpReconRequest) (*financev1.ExportErpReconResponse, error) {
	b, err := h.d.Batches.GetByID(ctx, req.GetBatchId())
	if err != nil {
		return &financev1.ExportErpReconResponse{Base: erpErrBase(err)}, nil
	}
	rows, err := h.d.Recon.ListRecon(ctx, req.GetBatchId())
	if err != nil {
		return &financev1.ExportErpReconResponse{Base: erpErrBase(err)}, nil
	}
	content, name, err := erpapp.ExportRecon(b.Period(), req.GetBatchId(), rows)
	if err != nil {
		return &financev1.ExportErpReconResponse{Base: erpErrBase(err)}, nil
	}
	return &financev1.ExportErpReconResponse{Base: erpOK("Recon exported"), FileContent: content, FileName: name}, nil
}
