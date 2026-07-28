// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	changeoverapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/changeover"
	changeoverdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/changeover"
)

// changeoverHandler serves the changeover read RPCs (Get/List). Create/Update/
// Detect usecases exist on the service but have no proto RPCs yet (plan-07b).
type changeoverHandler struct {
	svc *changeoverapp.Service
}

// newChangeoverHandler builds the changeover sub-handler.
func newChangeoverHandler(svc *changeoverapp.Service) *changeoverHandler {
	return &changeoverHandler{svc: svc}
}

// GetChangeoverEvent returns a changeover event with its component breakdown.
func (h *changeoverHandler) GetChangeoverEvent(ctx context.Context, req *ppcv1.GetChangeoverEventRequest) (*ppcv1.GetChangeoverEventResponse, error) {
	event, err := h.svc.Get(ctx, req.GetEventId())
	if err != nil {
		return &ppcv1.GetChangeoverEventResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.GetChangeoverEventResponse{
		Base: successResponse("Changeover event retrieved successfully"),
		Data: changeoverToProto(event),
	}, nil
}

// ListChangeoverEvents returns changeover events matching the filter.
func (h *changeoverHandler) ListChangeoverEvents(ctx context.Context, req *ppcv1.ListChangeoverEventsRequest) (*ppcv1.ListChangeoverEventsResponse, error) {
	from, errResp := optionalDateField("date_from", req.GetDateFrom())
	if errResp != nil {
		return &ppcv1.ListChangeoverEventsResponse{Base: errResp}, nil
	}
	to, errResp := optionalDateField("date_to", req.GetDateTo())
	if errResp != nil {
		return &ppcv1.ListChangeoverEventsResponse{Base: errResp}, nil
	}

	result, err := h.svc.List(ctx, changeoverdomain.Filter{
		Page:      req.GetPage(),
		PageSize:  req.GetPageSize(),
		MachineID: req.MachineId,
		DateFrom:  from,
		DateTo:    to,
		Status:    req.GetStatus(),
	})
	if err != nil {
		return &ppcv1.ListChangeoverEventsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	data := make([]*ppcv1.ChangeoverEvent, len(result.Items))
	for i, e := range result.Items {
		data[i] = changeoverToProto(e)
	}
	return &ppcv1.ListChangeoverEventsResponse{
		Base:       successResponse("Changeover events retrieved successfully"),
		Data:       data,
		Pagination: paginationProto(result.CurrentPage, result.PageSize, result.TotalItems, result.TotalPages),
	}, nil
}

// DetectChangeover previews the active components + duration/waste/group without persisting.
func (h *changeoverHandler) DetectChangeover(ctx context.Context, req *ppcv1.DetectChangeoverRequest) (*ppcv1.DetectChangeoverResponse, error) {
	res, err := h.svc.Detect(ctx, req.GetFromWoId(), req.GetToWoId(), req.GetDeepClean())
	if err != nil {
		return &ppcv1.DetectChangeoverResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	comps := make([]*ppcv1.ChangeoverComponent, len(res.Components))
	for i := range res.Components {
		comps[i] = changeoverComponentToProto(res.Components[i])
	}
	return &ppcv1.DetectChangeoverResponse{
		Base:              successResponse("Changeover detected successfully"),
		Components:        comps,
		DurationEstimated: res.DurationEstimated,
		WasteEstimated:    formatDecimal(res.WasteEstimated),
		Group:             res.Group,
	}, nil
}

// CreateChangeoverEvent persists a PLANNED changeover event.
func (h *changeoverHandler) CreateChangeoverEvent(ctx context.Context, req *ppcv1.CreateChangeoverEventRequest) (*ppcv1.CreateChangeoverEventResponse, error) {
	comps, errResp := changeoverComponentInputs(req.GetComponents())
	if errResp != nil {
		return &ppcv1.CreateChangeoverEventResponse{Base: errResp}, nil
	}
	event, err := h.svc.Create(ctx, changeoverapp.CreateCommand{
		FromWOID:   req.GetFromWoId(),
		ToWOID:     req.GetToWoId(),
		MachineID:  req.GetMachineId(),
		DeepClean:  req.GetDeepClean(),
		Notes:      req.GetNotes(),
		Components: comps,
	})
	if err != nil {
		return &ppcv1.CreateChangeoverEventResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.CreateChangeoverEventResponse{
		Base: successResponse("Changeover event created successfully"),
		Data: changeoverToProto(event),
	}, nil
}

// StartChangeover transitions a PLANNED event to IN_PROGRESS.
func (h *changeoverHandler) StartChangeover(ctx context.Context, req *ppcv1.StartChangeoverRequest) (*ppcv1.StartChangeoverResponse, error) {
	event, err := h.svc.Start(ctx, req.GetEventId())
	if err != nil {
		return &ppcv1.StartChangeoverResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.StartChangeoverResponse{
		Base: successResponse("Changeover started successfully"),
		Data: changeoverToProto(event),
	}, nil
}

// UpdateChangeoverActual records actual duration/waste and completes the event.
func (h *changeoverHandler) UpdateChangeoverActual(ctx context.Context, req *ppcv1.UpdateChangeoverActualRequest) (*ppcv1.UpdateChangeoverActualResponse, error) {
	waste, errResp := decimalField("waste_actual", req.GetWasteActual())
	if errResp != nil {
		return &ppcv1.UpdateChangeoverActualResponse{Base: errResp}, nil
	}
	event, err := h.svc.UpdateActual(ctx, changeoverapp.UpdateActualCommand{
		EventID:        req.GetEventId(),
		DurationActual: req.GetDurationActual(),
		WasteActual:    waste,
		Notes:          req.GetNotes(),
	})
	if err != nil {
		return &ppcv1.UpdateChangeoverActualResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.UpdateChangeoverActualResponse{
		Base: successResponse("Changeover actual recorded successfully"),
		Data: changeoverToProto(event),
	}, nil
}

// changeoverComponentInputs maps proto override lines to domain components. An
// empty input list yields nil so the service auto-detects.
func changeoverComponentInputs(in []*ppcv1.ChangeoverComponentInput) ([]changeoverdomain.Component, *commonv1.BaseResponse) {
	if len(in) == 0 {
		return nil, nil
	}
	comps := make([]changeoverdomain.Component, 0, len(in))
	for _, c := range in {
		waste, errResp := decimalField("waste_applied", c.GetWasteApplied())
		if errResp != nil {
			return nil, errResp
		}
		comps = append(comps, changeoverdomain.NewComponent(c.GetComponentCode(), c.GetDurationApplied(), waste))
	}
	return comps, nil
}

// changeoverToProto maps a domain changeover event to its proto message.
func changeoverToProto(e *changeoverdomain.Event) *ppcv1.ChangeoverEvent {
	out := &ppcv1.ChangeoverEvent{
		EventId:           e.ID(),
		FromWoId:          e.FromWOID(),
		ToWoId:            e.ToWOID(),
		MachineId:         e.MachineID(),
		DurationEstimated: e.DurationEstimated(),
		WasteEstimated:    formatDecimal(e.WasteEstimated()),
		Group:             e.Group(),
		Status:            e.Status(),
		StartedAt:         formatTimePtr(e.StartedAt()),
		CompletedAt:       formatTimePtr(e.CompletedAt()),
		Notes:             e.Notes(),
	}
	if v := e.DurationActual(); v != nil {
		out.DurationActual = *v
	}
	if v := e.WasteActual(); v != nil {
		out.WasteActual = formatDecimal(*v)
	}
	comps := e.Components()
	out.Components = make([]*ppcv1.ChangeoverComponent, len(comps))
	for i := range comps {
		out.Components[i] = changeoverComponentToProto(comps[i])
	}
	return out
}

// changeoverComponentToProto maps a domain component to its proto message.
func changeoverComponentToProto(c changeoverdomain.Component) *ppcv1.ChangeoverComponent {
	out := &ppcv1.ChangeoverComponent{
		ComponentId:     c.ID(),
		EventId:         c.EventID(),
		ComponentCode:   c.Code(),
		DurationApplied: c.DurationMin(),
		WasteApplied:    formatDecimal(c.WasteKg()),
		IsAutoDetected:  c.IsAutoDetected(),
		OverrideAt:      formatTimePtr(c.OverrideAt()),
	}
	if v := c.OverrideBy(); v != nil {
		out.OverrideBy = *v
	}
	return out
}

// compile-time interface guard.
var _ interface {
	GetChangeoverEvent(context.Context, *ppcv1.GetChangeoverEventRequest) (*ppcv1.GetChangeoverEventResponse, error)
	ListChangeoverEvents(context.Context, *ppcv1.ListChangeoverEventsRequest) (*ppcv1.ListChangeoverEventsResponse, error)
	DetectChangeover(context.Context, *ppcv1.DetectChangeoverRequest) (*ppcv1.DetectChangeoverResponse, error)
	CreateChangeoverEvent(context.Context, *ppcv1.CreateChangeoverEventRequest) (*ppcv1.CreateChangeoverEventResponse, error)
	StartChangeover(context.Context, *ppcv1.StartChangeoverRequest) (*ppcv1.StartChangeoverResponse, error)
	UpdateChangeoverActual(context.Context, *ppcv1.UpdateChangeoverActualRequest) (*ppcv1.UpdateChangeoverActualResponse, error)
} = (*changeoverHandler)(nil)
