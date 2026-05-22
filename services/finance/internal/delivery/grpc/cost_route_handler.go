package grpc

import (
	"context"
	"errors"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costroute"
	costroute "github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func actorFromCtx(ctx context.Context) string {
	id, _ := GetUserIDFromCtx(ctx)
	if id == "" {
		return "system"
	}
	return id
}

// CostRouteHandler implements financev1.CostRouteServiceServer.
type CostRouteHandler struct {
	financev1.UnimplementedCostRouteServiceServer
	getByProduct *app.GetByProductHandler
	getGraph     *app.GetGraphHandler
	saveGraph    *app.SaveGraphHandler
	markComplete *app.MarkCompleteHandler
	lock         *app.LockHandler
	unlock       *app.UnlockHandler
	del          *app.DeleteHandler
	list         *app.ListHandler
	duplicate          *app.DuplicateHandler
	listLinkedRequests *app.ListLinkedRequestsHandler
}

// NewCostRouteHandler constructs a handler.
func NewCostRouteHandler(repo costroute.Repository) (*CostRouteHandler, error) {
	return &CostRouteHandler{
		getByProduct: app.NewGetByProductHandler(repo),
		getGraph:     app.NewGetGraphHandler(repo),
		saveGraph:    app.NewSaveGraphHandler(repo),
		markComplete: app.NewMarkCompleteHandler(repo),
		lock:         app.NewLockHandler(repo),
		unlock:       app.NewUnlockHandler(repo),
		del:          app.NewDeleteHandler(repo),
		list:         app.NewListHandler(repo),
		duplicate:          app.NewDuplicateHandler(repo),
		listLinkedRequests: app.NewListLinkedRequestsHandler(repo),
	}, nil
}

// GetRouteByProduct returns the active head for a product.
func (h *CostRouteHandler) GetRouteByProduct(ctx context.Context, req *financev1.GetRouteByProductRequest) (*financev1.GetRouteByProductResponse, error) {
	head, err := h.getByProduct.Handle(ctx, req.GetProductSysId())
	if err != nil {
		return &financev1.GetRouteByProductResponse{Base: routeErrToBase(err)}, nil
	}
	return &financev1.GetRouteByProductResponse{Base: successResponse("OK"), Data: routeHeadToProto(head)}, nil
}

// GetRouteGraph returns the full graph (head + seqs + rms inline).
func (h *CostRouteHandler) GetRouteGraph(ctx context.Context, req *financev1.GetRouteGraphRequest) (*financev1.GetRouteGraphResponse, error) {
	g, err := h.getGraph.Handle(ctx, req.GetHeadId())
	if err != nil {
		return &financev1.GetRouteGraphResponse{Base: routeErrToBase(err)}, nil
	}
	return &financev1.GetRouteGraphResponse{Base: successResponse("OK"), Data: routeGraphToProto(g)}, nil
}

// SaveRouteGraph diffs+upserts the entire graph.
func (h *CostRouteHandler) SaveRouteGraph(ctx context.Context, req *financev1.SaveRouteGraphRequest) (*financev1.SaveRouteGraphResponse, error) {
	actor := actorFromCtx(ctx)
	in := routeGraphFromProto(req.GetGraph())
	out, err := h.saveGraph.Handle(ctx, req.GetHeadId(), in, actor)
	if err != nil {
		return &financev1.SaveRouteGraphResponse{Base: routeErrToBase(err)}, nil
	}
	return &financev1.SaveRouteGraphResponse{Base: successResponse("Graph saved"), Data: routeGraphToProto(out)}, nil
}

// CompleteRoute marks the head COMPLETE.
func (h *CostRouteHandler) CompleteRoute(ctx context.Context, req *financev1.CompleteRouteRequest) (*financev1.CompleteRouteResponse, error) {
	head, err := h.markComplete.Handle(ctx, req.GetHeadId(), actorFromCtx(ctx))
	if err != nil {
		return &financev1.CompleteRouteResponse{Base: routeErrToBase(err)}, nil
	}
	return &financev1.CompleteRouteResponse{Base: successResponse("Route marked COMPLETE"), Data: routeHeadToProto(head)}, nil
}

// LockRoute marks the head LOCKED.
func (h *CostRouteHandler) LockRoute(ctx context.Context, req *financev1.LockRouteRequest) (*financev1.LockRouteResponse, error) {
	head, err := h.lock.Handle(ctx, req.GetHeadId(), actorFromCtx(ctx))
	if err != nil {
		return &financev1.LockRouteResponse{Base: routeErrToBase(err)}, nil
	}
	return &financev1.LockRouteResponse{Base: successResponse("Route locked"), Data: routeHeadToProto(head)}, nil
}

// UnlockRoute reverts LOCKED -> COMPLETE.
func (h *CostRouteHandler) UnlockRoute(ctx context.Context, req *financev1.UnlockRouteRequest) (*financev1.UnlockRouteResponse, error) {
	head, err := h.unlock.Handle(ctx, req.GetHeadId(), actorFromCtx(ctx))
	if err != nil {
		return &financev1.UnlockRouteResponse{Base: routeErrToBase(err)}, nil
	}
	return &financev1.UnlockRouteResponse{Base: successResponse("Route unlocked"), Data: routeHeadToProto(head)}, nil
}

// DeleteRoute soft-deletes the head (refuses if LOCKED).
func (h *CostRouteHandler) DeleteRoute(ctx context.Context, req *financev1.DeleteRouteRequest) (*financev1.DeleteRouteResponse, error) {
	if err := h.del.Handle(ctx, req.GetHeadId(), actorFromCtx(ctx)); err != nil {
		return &financev1.DeleteRouteResponse{Base: routeErrToBase(err)}, nil
	}
	return &financev1.DeleteRouteResponse{Base: successResponse("Route deleted")}, nil
}

// ListRoutes returns paginated heads.
func (h *CostRouteHandler) ListRoutes(ctx context.Context, req *financev1.ListRoutesRequest) (*financev1.ListRoutesResponse, error) {
	rows, total, err := h.list.Handle(ctx, costroute.Filter{
		Search:    req.GetSearch(),
		Status:    req.GetStatus(),
		Page:      req.GetPage(),
		PageSize:  req.GetPageSize(),
		SortBy:    req.GetSortBy(),
		SortOrder: req.GetSortOrder(),
	})
	if err != nil {
		return &financev1.ListRoutesResponse{Base: routeErrToBase(err)}, nil
	}
	data := make([]*financev1.CostRouteHead, 0, len(rows))
	for _, h := range rows {
		data = append(data, routeHeadToProto(h))
	}
	page := req.GetPage()
	if page < 1 {
		page = 1
	}
	pageSize := req.GetPageSize()
	if pageSize < 1 {
		pageSize = 20
	}
	totalPages := int32(0)
	if pageSize > 0 {
		totalPages = int32((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return &financev1.ListRoutesResponse{
		Base: successResponse("OK"),
		Data: data,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: page,
			PageSize:    pageSize,
			TotalItems:  total,
			TotalPages:  totalPages,
		},
	}, nil
}

// DuplicateRoute deep-copies the route per the requested toggles.
func (h *CostRouteHandler) DuplicateRoute(ctx context.Context, req *financev1.DuplicateRouteRequest) (*financev1.DuplicateRouteResponse, error) {
	out, err := h.duplicate.Handle(ctx, costroute.DuplicateInput{
		SourceHeadID:         req.GetHeadId(),
		IncludeRouting:       req.GetIncludeRouting(),
		IncludeUpstream:      req.GetIncludeUpstream(),
		IncludeApplicability: req.GetIncludeApplicability(),
		IncludeValues:        req.GetIncludeValues(),
		NewCodePrefix:        req.GetNewCodePrefix(),
		LinkedRequestID:      req.GetLinkedRequestId(),
		ActorUserID:          actorFromCtx(ctx),
	})
	if err != nil {
		return &financev1.DuplicateRouteResponse{Base: routeErrToBase(err)}, nil
	}
	return &financev1.DuplicateRouteResponse{
		Base:            successResponse("Route duplicated"),
		NewHeadId:       out.NewHeadID,
		NewProductSysId: out.NewProductSysID,
		NewProductCode:  out.NewProductCode,
	}, nil
}

// ListLinkedRequests returns requests linking to this route head.
func (h *CostRouteHandler) ListLinkedRequests(ctx context.Context, req *financev1.ListLinkedRequestsRequest) (*financev1.ListLinkedRequestsResponse, error) {
	rows, err := h.listLinkedRequests.Handle(ctx, req.GetHeadId())
	if err != nil {
		return &financev1.ListLinkedRequestsResponse{Base: routeErrToBase(err)}, nil
	}
	data := make([]*financev1.LinkedRequest, 0, len(rows))
	for _, lr := range rows {
		data = append(data, &financev1.LinkedRequest{
			RequestId:    lr.RequestID,
			RequestNo:    lr.RequestNo,
			Status:       lr.Status,
			ProductTop_2: lr.ProductTop2,
			CreatedBy:    lr.CreatedBy,
			CreatedAt:    lr.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	return &financev1.ListLinkedRequestsResponse{
		Base: successResponse("OK"),
		Data: data,
	}, nil
}

// =============================================================================
// proto <-> domain mappers
// =============================================================================

func routeHeadToProto(h *costroute.Head) *financev1.CostRouteHead {
	if h == nil {
		return nil
	}
	return &financev1.CostRouteHead{
		HeadId:              h.HeadID,
		ProductSysId:        h.ProductSysID,
		ProductCode:         h.ProductCode,
		ProductName:         h.ProductName,
		RoutingStatus:       h.RoutingStatus,
		Version:             h.Version,
		PromotedFromDraftId: h.PromotedFromDraftID,
		CylTypeId:           h.CylTypeID,
		Notes:               h.Notes,
	}
}

func routeGraphToProto(g *costroute.Graph) *financev1.RouteGraph {
	if g == nil {
		return nil
	}
	seqs := make([]*financev1.CostRouteSeq, 0, len(g.Seqs))
	for _, s := range g.Seqs {
		if s == nil {
			continue
		}
		rms := make([]*financev1.CostRouteRm, 0, len(s.Rms))
		for _, rm := range s.Rms {
			if rm == nil {
				continue
			}
			rms = append(rms, &financev1.CostRouteRm{
				RmId:               rm.RmID,
				SeqId:              rm.SeqID,
				ParentProductSysId: rm.ParentProductSysID,
				RmType:             rm.RmType,
				RmProductSysId:     rm.RmProductSysID,
				RmItemCode:         rm.RmItemCode,
				RmGroupCode:        rm.RmGroupCode,
				RouteRmName:        rm.RouteRmName,
				RouteRmItemCode:    rm.RouteRmItemCode,
				RouteRmShadeCode:   rm.RouteRmShadeCode,
				RouteRmShadeName:   rm.RouteRmShadeName,
				RouteRmRatio:       rm.RouteRmRatio,
				UomId:              rm.UomID,
				SubType:            rm.SubType,
				Notes:              rm.Notes,
			})
		}
		seqs = append(seqs, &financev1.CostRouteSeq{
			SeqId:          s.SeqID,
			HeadId:         s.HeadID,
			ProductSysId:   s.ProductSysID,
			ProductCode:    s.ProductCode,
			ProductName:    s.ProductName,
			RouteLevel:     s.RouteLevel,
			RouteSeq:       s.RouteSeq,
			RouteName:      s.RouteName,
			RouteItemCode:  s.RouteItemCode,
			RouteShadeCode: s.RouteShadeCode,
			RouteShadeName: s.RouteShadeName,
			PositionX:      s.PositionX,
			PositionY:      s.PositionY,
			Rms:            rms,
		})
	}
	return &financev1.RouteGraph{Head: routeHeadToProto(g.Head), Seqs: seqs}
}

func routeGraphFromProto(p *financev1.RouteGraph) *costroute.Graph {
	if p == nil {
		return &costroute.Graph{}
	}
	out := &costroute.Graph{Seqs: make([]*costroute.Seq, 0, len(p.GetSeqs()))}
	if p.GetHead() != nil {
		out.Head = &costroute.Head{
			HeadID:              p.GetHead().GetHeadId(),
			ProductSysID:        p.GetHead().GetProductSysId(),
			RoutingStatus:       p.GetHead().GetRoutingStatus(),
			Version:             p.GetHead().GetVersion(),
			PromotedFromDraftID: p.GetHead().GetPromotedFromDraftId(),
			CylTypeID:           p.GetHead().GetCylTypeId(),
			Notes:               p.GetHead().GetNotes(),
		}
	}
	for _, s := range p.GetSeqs() {
		if s == nil {
			continue
		}
		seq := &costroute.Seq{
			SeqID:          s.GetSeqId(),
			HeadID:         s.GetHeadId(),
			ProductSysID:   s.GetProductSysId(),
			RouteLevel:     s.GetRouteLevel(),
			RouteSeq:       s.GetRouteSeq(),
			RouteName:      s.GetRouteName(),
			RouteItemCode:  s.GetRouteItemCode(),
			RouteShadeCode: s.GetRouteShadeCode(),
			RouteShadeName: s.GetRouteShadeName(),
			PositionX:      s.GetPositionX(),
			PositionY:      s.GetPositionY(),
		}
		for _, rm := range s.GetRms() {
			if rm == nil {
				continue
			}
			seq.Rms = append(seq.Rms, &costroute.Rm{
				RmID:               rm.GetRmId(),
				SeqID:              rm.GetSeqId(),
				ParentProductSysID: rm.GetParentProductSysId(),
				RmType:             rm.GetRmType(),
				RmProductSysID:     rm.GetRmProductSysId(),
				RmItemCode:         rm.GetRmItemCode(),
				RmGroupCode:        rm.GetRmGroupCode(),
				RouteRmName:        rm.GetRouteRmName(),
				RouteRmItemCode:    rm.GetRouteRmItemCode(),
				RouteRmShadeCode:   rm.GetRouteRmShadeCode(),
				RouteRmShadeName:   rm.GetRouteRmShadeName(),
				RouteRmRatio:       rm.GetRouteRmRatio(),
				UomID:              rm.GetUomId(),
				SubType:            rm.GetSubType(),
				Notes:              rm.GetNotes(),
			})
		}
		out.Seqs = append(out.Seqs, seq)
	}
	return out
}

// =============================================================================
// error mapping
// =============================================================================

func routeErrToBase(err error) *commonv1.BaseResponse {
	// Use the same shape the other handlers use; delegate to status.Code where helpful.
	switch {
	case errors.Is(err, costroute.ErrNotFound):
		return ErrorResponse("404", "route not found")
	case errors.Is(err, costroute.ErrAlreadyExists):
		return ErrorResponse("409", "route already exists for product")
	case errors.Is(err, costroute.ErrLocked):
		return ErrorResponse("409", "route is locked")
	case errors.Is(err, costroute.ErrInvalidStatusTransition):
		return ErrorResponse("400", "invalid status transition")
	case errors.Is(err, costroute.ErrLevelOneMismatch),
		errors.Is(err, costroute.ErrLevelOneMissing),
		errors.Is(err, costroute.ErrUpstreamMissing),
		errors.Is(err, costroute.ErrUpstreamNotHigherLevel),
		errors.Is(err, costroute.ErrInvalidRmType),
		errors.Is(err, costroute.ErrMultipleRmRefs),
		errors.Is(err, costroute.ErrRmRefTypeMismatch),
		errors.Is(err, costroute.ErrNonPositiveRatio):
		return ErrorResponse("400", err.Error())
	}
	if s, ok := status.FromError(err); ok && s != nil {
		return ErrorResponse(grpcCodeToStatusCode(s.Code()), s.Message())
	}
	return ErrorResponse("500", err.Error())
}

func grpcCodeToStatusCode(c codes.Code) string {
	switch c {
	case codes.InvalidArgument:
		return "400"
	case codes.NotFound:
		return "404"
	case codes.AlreadyExists:
		return "409"
	case codes.PermissionDenied:
		return "403"
	case codes.Unauthenticated:
		return "401"
	case codes.Unavailable:
		return "503"
	case codes.Unimplemented:
		return "501"
	}
	return "500"
}
