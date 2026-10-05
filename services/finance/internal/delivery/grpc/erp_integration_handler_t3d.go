package grpc

import (
	"context"
	"errors"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	cpmapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	erpapp "github.com/mutugading/goapps-backend/services/finance/internal/application/erpintegration"
	cpmdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	erpdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// T3d seams.
type (
	// erpProductLinker is the D-LINK use case plus the product read needed to
	// pass the product's own shade (shade must equal cpm_shade_code).
	erpProductLinker interface {
		Handle(ctx context.Context, cmd cpmapp.LinkErpItemCommand) (cpmapp.LinkErpItemResult, error)
		ShadeOf(ctx context.Context, productSysID int64) (string, error)
	}
	erpMasterSyncTrigger interface {
		Trigger(ctx context.Context, subtype, actor string) (*job.Execution, error)
	}
)

// ErpProductLinker adapts the D-LINK handler and the product repository to
// erpProductLinker.
type ErpProductLinker struct {
	*cpmapp.LinkErpItemHandler
	repo cpmdomain.Repository
}

// NewErpProductLinker builds the adapter.
func NewErpProductLinker(h *cpmapp.LinkErpItemHandler, repo cpmdomain.Repository) *ErpProductLinker {
	return &ErpProductLinker{LinkErpItemHandler: h, repo: repo}
}

// ShadeOf returns the product's own shade code.
func (l *ErpProductLinker) ShadeOf(ctx context.Context, productSysID int64) (string, error) {
	p, err := l.repo.GetBySysID(ctx, productSysID)
	if err != nil {
		return "", err
	}
	return p.ShadeCode(), nil
}

// erpLinkErrBase maps D-LINK errors (V-04/V-06/V-11/V-12) to a BaseResponse.
func erpLinkErrBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, cpmdomain.ErrNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, cpmdomain.ErrLinkInvalidItemCode), errors.Is(err, cpmdomain.ErrLinkInvalidShadeCode):
		return BadRequestResponse(err.Error())
	case errors.Is(err, cpmdomain.ErrLinkNotAxGrade), errors.Is(err, cpmdomain.ErrLinkCmbRequiresMB),
		errors.Is(err, cpmdomain.ErrLinkShadeMismatch), errors.Is(err, cpmdomain.ErrLinkProductHasNoShade),
		errors.Is(err, cpmdomain.ErrLinkStale), errors.Is(err, cpmdomain.ErrInactive):
		return ErrorResponse("412", err.Error())
	case errors.Is(err, cpmdomain.ErrLinkDuplicate):
		return ConflictResponse(err.Error())
	}
	return InternalErrorResponse(err.Error())
}

// LinkErpToProduct links an ERP item to a product via the D-LINK use case.
func (h *ErpIntegrationHandler) LinkErpToProduct(ctx context.Context, req *financev1.LinkErpToProductRequest) (*financev1.LinkErpToProductResponse, error) {
	sysID := req.GetProductSysId()
	if sysID <= 0 {
		return &financev1.LinkErpToProductResponse{Base: BadRequestResponse("invalid product_sys_id")}, nil
	}
	shade, err := h.d.LinkErp.ShadeOf(ctx, sysID)
	if err != nil {
		return &financev1.LinkErpToProductResponse{Base: erpLinkErrBase(err)}, nil
	}
	res, err := h.d.LinkErp.Handle(ctx, cpmapp.LinkErpItemCommand{
		ProductSysID: sysID, ErpItemCode: req.GetErpItemCode(), ErpShadeCode: shade, ActorUserID: erpActor(ctx),
	})
	if err != nil {
		return &financev1.LinkErpToProductResponse{Base: erpLinkErrBase(err)}, nil
	}
	return &financev1.LinkErpToProductResponse{
		Base: erpOK("ERP item linked"), ProductSysId: sysID, ErpItemCode: res.Product.ErpItemCode(),
	}, nil
}

// RunErpMasterSync enqueues the full master replica sync job.
func (h *ErpIntegrationHandler) RunErpMasterSync(ctx context.Context, _ *financev1.RunErpMasterSyncRequest) (*financev1.RunErpMasterSyncResponse, error) {
	ex, err := h.d.MasterSync.Trigger(ctx, erpapp.MasterSubtypeAll, erpActor(ctx))
	if err != nil {
		return &financev1.RunErpMasterSyncResponse{Base: erpErrBase(err)}, nil
	}
	return &financev1.RunErpMasterSyncResponse{Base: erpOK("Master sync queued"), JobId: ex.ID().String()}, nil
}

// ExportErpCoverage renders the batch coverage lines as .xlsx.
func (h *ErpIntegrationHandler) ExportErpCoverage(ctx context.Context, req *financev1.ExportErpCoverageRequest) (*financev1.ExportErpCoverageResponse, error) {
	b, err := h.d.Batches.GetByID(ctx, req.GetBatchId())
	if err != nil {
		return &financev1.ExportErpCoverageResponse{Base: erpErrBase(err)}, nil
	}
	lines, err := h.d.Coverage.List(ctx, req.GetBatchId())
	if err != nil {
		return &financev1.ExportErpCoverageResponse{Base: erpErrBase(err)}, nil
	}
	content, name, err := erpapp.ExportCoverage(b.Period(), req.GetBatchId(), lines)
	if err != nil {
		return &financev1.ExportErpCoverageResponse{Base: erpErrBase(err)}, nil
	}
	return &financev1.ExportErpCoverageResponse{Base: erpOK("Coverage exported"), FileContent: content, FileName: name}, nil
}

// ExportErpManualSample renders a stratified >=30-combo sample (recon
// section 11) of the batch's OK std rows as .xlsx.
func (h *ErpIntegrationHandler) ExportErpManualSample(ctx context.Context, req *financev1.ExportErpManualSampleRequest) (*financev1.ExportErpManualSampleResponse, error) {
	b, err := h.d.Batches.GetByID(ctx, req.GetBatchId())
	if err != nil {
		return &financev1.ExportErpManualSampleResponse{Base: erpErrBase(err)}, nil
	}
	rows, err := h.d.StdCost.List(ctx, req.GetBatchId())
	if err != nil {
		return &financev1.ExportErpManualSampleResponse{Base: erpErrBase(err)}, nil
	}
	sample := erpapp.SelectManualSample(rows, erpapp.ManualSampleMin, erpapp.ManualSampleMax)
	content, name, err := erpapp.ExportManualSample(b.Period(), req.GetBatchId(), sample)
	if err != nil {
		return &financev1.ExportErpManualSampleResponse{Base: erpErrBase(err)}, nil
	}
	return &financev1.ExportErpManualSampleResponse{Base: erpOK("Manual sample exported"), FileContent: content, FileName: name}, nil
}

// erpProductFromDemand is the create-from-coverage use case.
type erpProductFromDemand interface {
	Handle(ctx context.Context, batchID, cecID int64, actor string) (int64, error)
}

// ErpProductCreator adapts the product-master create handler and the D-LINK
// handler to the erpapp create-from-demand ports.
type ErpProductCreator struct {
	create *cpmapp.CreateHandler
	link   *cpmapp.LinkErpItemHandler
}

// NewErpProductCreator builds the adapter.
func NewErpProductCreator(c *cpmapp.CreateHandler, l *cpmapp.LinkErpItemHandler) *ErpProductCreator {
	return &ErpProductCreator{create: c, link: l}
}

// CreateProduct creates the product master and returns its sys id.
func (a *ErpProductCreator) CreateProduct(ctx context.Context, cmd erpapp.NewDemandProduct) (int64, error) {
	p, err := a.create.Handle(ctx, cpmapp.CreateCommand{
		ProductTypeID: cmd.ProductTypeID, ProductName: cmd.ProductName, ShadeCode: cmd.ShadeCode, ActorUserID: cmd.ActorUserID,
	})
	if err != nil {
		return 0, err
	}
	return p.ProductSysID(), nil
}

// Link links the product to the ERP item.
func (a *ErpProductCreator) Link(ctx context.Context, cmd erpapp.DemandProductLink) error {
	_, err := a.link.Handle(ctx, cpmapp.LinkErpItemCommand{
		ProductSysID: cmd.ProductSysID, ErpItemCode: cmd.ErpItemCode, ErpShadeCode: cmd.ErpShadeCode, ActorUserID: cmd.ActorUserID,
	})
	return err
}

// erpCreateFromDemandErrBase maps use-case errors to a BaseResponse.
func erpCreateFromDemandErrBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, erpdomain.ErrCoverageLineNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, erpapp.ErrCoverageAlreadyCovered):
		return ConflictResponse(err.Error())
	case errors.Is(err, erpapp.ErrCoverageNotNoMapping), errors.Is(err, erpapp.ErrCoverageMBLine),
		errors.Is(err, erpapp.ErrCoverageUnknownType), errors.Is(err, cpmdomain.ErrMBProductNotManuallyCreatable):
		return ErrorResponse("412", err.Error())
	}
	return erpLinkErrBase(err)
}

// CreateCostProductFromDemand creates a yarn product from a NO_MAPPING
// coverage line and links it (MB lines are refused). Needs the
// product-master create permission in addition to the RPC's .update.
func (h *ErpIntegrationHandler) CreateCostProductFromDemand(ctx context.Context, req *financev1.CreateCostProductFromDemandRequest) (*financev1.CreateCostProductFromDemandResponse, error) {
	if !HasPermission(ctx, "finance.product.route.create") {
		return &financev1.CreateCostProductFromDemandResponse{Base: ErrorResponse("403", "permission denied: finance.product.route.create")}, nil
	}
	if h.d.CreateProduct == nil {
		return &financev1.CreateCostProductFromDemandResponse{Base: InternalErrorResponse("create product not configured")}, nil
	}
	id, err := h.d.CreateProduct.Handle(ctx, req.GetBatchId(), req.GetCecId(), erpActor(ctx))
	if err != nil {
		return &financev1.CreateCostProductFromDemandResponse{Base: erpCreateFromDemandErrBase(err)}, nil
	}
	return &financev1.CreateCostProductFromDemandResponse{Base: erpOK("Product created and linked"), ProductSysId: id}, nil
}
