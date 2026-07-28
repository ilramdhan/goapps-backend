// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	dpapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/dailyperf"
	dpdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/dailyperf"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// nilServiceCode is the status code returned when a use case is not wired.
const nilServiceCode = "500"

// dailyPerfHandler implements the daily-performance RPCs of PPCService.
type dailyPerfHandler struct {
	svc *dpapp.Service
}

func newDailyPerfHandler(svc *dpapp.Service) *dailyPerfHandler {
	return &dailyPerfHandler{svc: svc}
}

// SubmitShiftEntry records a machine shift log with downtime + waste. The proto
// running_minutes is ignored — it is derived from downtime (v1.2).
func (h *dailyPerfHandler) SubmitShiftEntry(ctx context.Context, req *ppcv1.SubmitShiftEntryRequest) (*ppcv1.SubmitShiftEntryResponse, error) {
	if h.svc == nil {
		return &ppcv1.SubmitShiftEntryResponse{Base: errorResponse(nilServiceCode, "daily performance service unavailable")}, nil
	}
	date, errResp := dateField("date", req.GetDate())
	if errResp != nil {
		return &ppcv1.SubmitShiftEntryResponse{Base: errResp}, nil
	}
	positionsRunning, errResp := decimalField("positions_running", req.GetPositionsRunning())
	if errResp != nil {
		return &ppcv1.SubmitShiftEntryResponse{Base: errResp}, nil
	}
	downtime, errResp := mapDowntimeInputs(req.GetDowntimeEntries())
	if errResp != nil {
		return &ppcv1.SubmitShiftEntryResponse{Base: errResp}, nil
	}
	waste, errResp := mapWasteInputs(req.GetWasteEntries())
	if errResp != nil {
		return &ppcv1.SubmitShiftEntryResponse{Base: errResp}, nil
	}

	log, err := h.svc.SubmitShiftEntry(ctx, dpapp.SubmitShiftEntryCommand{
		MachineID:        req.GetMachineId(),
		Date:             date,
		Shift:            req.GetShift(),
		PositionsTotal:   req.GetPositionsTotal(),
		PositionsRunning: positionsRunning,
		Status:           req.GetStatus(),
		Downtime:         downtime,
		Waste:            waste,
		InputBy:          actorID(ctx),
	})
	if err != nil {
		return &ppcv1.SubmitShiftEntryResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.SubmitShiftEntryResponse{
		Base: successResponse("Shift entry submitted successfully"),
		Data: shiftLogToProto(log),
	}, nil
}

// SubmitAreaShiftLog upserts an area-level overtime + notes record.
func (h *dailyPerfHandler) SubmitAreaShiftLog(ctx context.Context, req *ppcv1.SubmitAreaShiftLogRequest) (*ppcv1.SubmitAreaShiftLogResponse, error) {
	if h.svc == nil {
		return &ppcv1.SubmitAreaShiftLogResponse{Base: errorResponse(nilServiceCode, "daily performance service unavailable")}, nil
	}
	date, errResp := dateField("date", req.GetDate())
	if errResp != nil {
		return &ppcv1.SubmitAreaShiftLogResponse{Base: errResp}, nil
	}
	otHours, errResp := optionalDecimalField("ot_hours", req.GetOtHours())
	if errResp != nil {
		return &ppcv1.SubmitAreaShiftLogResponse{Base: errResp}, nil
	}

	log, err := h.svc.SubmitAreaShiftLog(ctx, dpapp.SubmitAreaShiftLogCommand{
		Area:    areaCodeToString(req.GetArea()),
		Date:    date,
		Shift:   optionalStringField(req.GetShift()),
		OtHours: otHours,
		Notes:   req.GetNotes(),
		InputBy: actorID(ctx),
	})
	if err != nil {
		return &ppcv1.SubmitAreaShiftLogResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.SubmitAreaShiftLogResponse{
		Base: successResponse("Area shift log submitted successfully"),
		Data: areaShiftLogToProto(log),
	}, nil
}

// RecalcEfficiency recomputes efficiency snapshots for an area on a date.
func (h *dailyPerfHandler) RecalcEfficiency(ctx context.Context, req *ppcv1.RecalcEfficiencyRequest) (*ppcv1.RecalcEfficiencyResponse, error) {
	if h.svc == nil {
		return &ppcv1.RecalcEfficiencyResponse{Base: errorResponse(nilServiceCode, "daily performance service unavailable")}, nil
	}
	date, errResp := dateField("date", req.GetDate())
	if errResp != nil {
		return &ppcv1.RecalcEfficiencyResponse{Base: errResp}, nil
	}

	written, err := h.svc.Recalc(ctx, areaCodeToString(req.GetArea()), date, req.MachineId, req.Shift)
	if err != nil {
		return &ppcv1.RecalcEfficiencyResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.RecalcEfficiencyResponse{
		Base:             successResponse("Efficiency recomputed successfully"),
		SnapshotsWritten: safeconv.IntToInt32(written),
	}, nil
}

// CreateShiftLogNote creates a shift-log book entry.
func (h *dailyPerfHandler) CreateShiftLogNote(ctx context.Context, req *ppcv1.CreateShiftLogNoteRequest) (*ppcv1.CreateShiftLogNoteResponse, error) {
	if h.svc == nil {
		return &ppcv1.CreateShiftLogNoteResponse{Base: errorResponse(nilServiceCode, "daily performance service unavailable")}, nil
	}
	date, errResp := dateField("date", req.GetDate())
	if errResp != nil {
		return &ppcv1.CreateShiftLogNoteResponse{Base: errResp}, nil
	}

	note, err := h.svc.CreateNote(ctx, dpapp.CreateNoteCommand{
		MachineID: req.GetMachineId(),
		Date:      date,
		Shift:     req.GetShift(),
		NoteType:  noteTypeToString(req.GetNoteType()),
		Note:      req.GetNote(),
		InputBy:   actorID(ctx),
	})
	if err != nil {
		return &ppcv1.CreateShiftLogNoteResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.CreateShiftLogNoteResponse{
		Base: successResponse("Shift log note created successfully"),
		Data: shiftLogNoteToProto(note),
	}, nil
}

// UpdateShiftLogNote updates a shift-log book entry.
func (h *dailyPerfHandler) UpdateShiftLogNote(ctx context.Context, req *ppcv1.UpdateShiftLogNoteRequest) (*ppcv1.UpdateShiftLogNoteResponse, error) {
	if h.svc == nil {
		return &ppcv1.UpdateShiftLogNoteResponse{Base: errorResponse(nilServiceCode, "daily performance service unavailable")}, nil
	}
	note, err := h.svc.UpdateNote(ctx, dpapp.UpdateNoteCommand{
		ID:       req.GetNoteId(),
		NoteType: optionalNoteType(req.NoteType),
		Note:     req.Note,
	})
	if err != nil {
		return &ppcv1.UpdateShiftLogNoteResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.UpdateShiftLogNoteResponse{
		Base: successResponse("Shift log note updated successfully"),
		Data: shiftLogNoteToProto(note),
	}, nil
}

// DeleteShiftLogNote removes a shift-log book entry.
func (h *dailyPerfHandler) DeleteShiftLogNote(ctx context.Context, req *ppcv1.DeleteShiftLogNoteRequest) (*ppcv1.DeleteShiftLogNoteResponse, error) {
	if h.svc == nil {
		return &ppcv1.DeleteShiftLogNoteResponse{Base: errorResponse(nilServiceCode, "daily performance service unavailable")}, nil
	}
	if err := h.svc.DeleteNote(ctx, req.GetNoteId()); err != nil {
		return &ppcv1.DeleteShiftLogNoteResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.DeleteShiftLogNoteResponse{Base: successResponse("Shift log note deleted successfully")}, nil
}

// ListShiftLogNotes lists shift-log book entries with filtering and pagination.
func (h *dailyPerfHandler) ListShiftLogNotes(ctx context.Context, req *ppcv1.ListShiftLogNotesRequest) (*ppcv1.ListShiftLogNotesResponse, error) {
	if h.svc == nil {
		return &ppcv1.ListShiftLogNotesResponse{Base: errorResponse(nilServiceCode, "daily performance service unavailable")}, nil
	}
	date, errResp := optionalDateField("date", req.GetDate())
	if errResp != nil {
		return &ppcv1.ListShiftLogNotesResponse{Base: errResp}, nil
	}

	result, err := h.svc.ListNotes(ctx, dpapp.ListNotesQuery{
		Page:      int(req.GetPage()),
		PageSize:  int(req.GetPageSize()),
		MachineID: req.MachineId,
		Date:      date,
		Shift:     req.GetShift(),
		NoteType:  noteTypeToString(req.GetNoteType()),
		SortBy:    req.GetSortBy(),
		SortOrder: req.GetSortOrder(),
	})
	if err != nil {
		return &ppcv1.ListShiftLogNotesResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	items := make([]*ppcv1.ShiftLogNote, len(result.Items))
	for i, note := range result.Items {
		items[i] = shiftLogNoteToProto(note)
	}
	return &ppcv1.ListShiftLogNotesResponse{
		Base:       successResponse("Shift log notes retrieved successfully"),
		Data:       items,
		Pagination: paginationProto(result.CurrentPage, result.PageSize, result.TotalItems, result.TotalPages),
	}, nil
}

// ListMachineShiftLogs lists machine shift logs with filtering and pagination.
func (h *dailyPerfHandler) ListMachineShiftLogs(ctx context.Context, req *ppcv1.ListMachineShiftLogsRequest) (*ppcv1.ListMachineShiftLogsResponse, error) {
	if h.svc == nil {
		return &ppcv1.ListMachineShiftLogsResponse{Base: errorResponse(nilServiceCode, "daily performance service unavailable")}, nil
	}
	date, errResp := optionalDateField("date", req.GetDate())
	if errResp != nil {
		return &ppcv1.ListMachineShiftLogsResponse{Base: errResp}, nil
	}

	result, err := h.svc.ListShiftLogs(ctx, dpapp.ListShiftLogsQuery{
		Page:      int(req.GetPage()),
		PageSize:  int(req.GetPageSize()),
		MachineID: req.MachineId,
		Area:      areaCodeToString(req.GetArea()),
		Date:      date,
		Shift:     req.GetShift(),
		Status:    req.GetStatus(),
		SortBy:    req.GetSortBy(),
		SortOrder: req.GetSortOrder(),
	})
	if err != nil {
		return &ppcv1.ListMachineShiftLogsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	items := make([]*ppcv1.MachineShiftLog, len(result.Items))
	for i, log := range result.Items {
		items[i] = shiftLogToProto(log)
	}
	return &ppcv1.ListMachineShiftLogsResponse{
		Base:       successResponse("Machine shift logs retrieved successfully"),
		Data:       items,
		Pagination: paginationProto(result.CurrentPage, result.PageSize, result.TotalItems, result.TotalPages),
	}, nil
}

// ── proto mappers ────────────────────────────────────────────────────────────

func mapDowntimeInputs(entries []*ppcv1.DowntimeEntry) ([]dpapp.DowntimeInput, *commonv1.BaseResponse) {
	out := make([]dpapp.DowntimeInput, 0, len(entries))
	for _, e := range entries {
		startAt, errResp := optionalTimeField("start_at", e.StartAt)
		if errResp != nil {
			return nil, errResp
		}
		endAt, errResp := optionalTimeField("end_at", e.EndAt)
		if errResp != nil {
			return nil, errResp
		}
		lostKg, errResp := optionalDecimalPtr("lost_kg", e.LostKg)
		if errResp != nil {
			return nil, errResp
		}
		out = append(out, dpapp.DowntimeInput{
			ReasonID:    e.GetReasonId(),
			PositionNo:  e.PositionNo,
			WoID:        e.WoId,
			StartAt:     startAt,
			EndAt:       endAt,
			DurationMin: e.DurationMin,
			LostKg:      lostKg,
			Notes:       e.Notes,
		})
	}
	return out, nil
}

func mapWasteInputs(entries []*ppcv1.WasteEntry) ([]dpapp.WasteInput, *commonv1.BaseResponse) {
	out := make([]dpapp.WasteInput, 0, len(entries))
	for _, e := range entries {
		qtyKg, errResp := decimalField("qty_kg", e.GetQtyKg())
		if errResp != nil {
			return nil, errResp
		}
		out = append(out, dpapp.WasteInput{
			CategoryID: e.GetCategoryId(),
			WoID:       e.WoId,
			QtyKg:      qtyKg,
			IsUpset:    e.GetIsUpset(),
			Notes:      e.Notes,
		})
	}
	return out, nil
}

func shiftLogToProto(m *dpdomain.MachineShiftLog) *ppcv1.MachineShiftLog {
	return &ppcv1.MachineShiftLog{
		LogId:            m.ID(),
		MachineId:        m.MachineID(),
		MachineNo:        m.MachineNo(),
		Date:             formatDate(m.Date()),
		Shift:            m.Shift(),
		PositionsTotal:   m.PositionsTotal(),
		PositionsRunning: formatDecimal(m.PositionsRunning()),
		RunningMinutes:   m.RunningMinutes(),
		Status:           m.Status(),
		InputBy:          m.InputBy(),
		InputAt:          m.InputAt().Format(time.RFC3339),
		UpdatedAt:        m.UpdatedAt().Format(time.RFC3339),
	}
}

func areaShiftLogToProto(a *dpdomain.AreaShiftLog) *ppcv1.AreaShiftLog {
	proto := &ppcv1.AreaShiftLog{
		LogId:   a.ID(),
		Area:    stringToAreaCode(a.AreaCode()),
		Date:    formatDate(a.Date()),
		OtHours: formatOptionalDecimal(a.OtHours()),
		Notes:   a.Notes(),
		InputBy: a.InputBy(),
		InputAt: a.InputAt().Format(time.RFC3339),
	}
	if a.Shift() != nil {
		proto.Shift = *a.Shift()
	}
	return proto
}

func shiftLogNoteToProto(n *dpdomain.ShiftLogNote) *ppcv1.ShiftLogNote {
	return &ppcv1.ShiftLogNote{
		NoteId:    n.ID(),
		MachineId: n.MachineID(),
		MachineNo: n.MachineNo(),
		Date:      formatDate(n.Date()),
		Shift:     n.Shift(),
		NoteType:  stringToNoteType(n.NoteType()),
		Note:      n.Note(),
		InputBy:   n.InputBy(),
		InputAt:   n.InputAt().Format(time.RFC3339),
	}
}

// noteTypeToString maps a proto ShiftLogNoteType to its domain string.
func noteTypeToString(t ppcv1.ShiftLogNoteType) string {
	switch t {
	case ppcv1.ShiftLogNoteType_SHIFT_LOG_NOTE_TYPE_INSTRUKSI:
		return dpdomain.NoteInstruksi
	case ppcv1.ShiftLogNoteType_SHIFT_LOG_NOTE_TYPE_ACTIVITY:
		return dpdomain.NoteActivity
	case ppcv1.ShiftLogNoteType_SHIFT_LOG_NOTE_TYPE_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

// stringToNoteType maps a domain note-type string to its proto ShiftLogNoteType.
func stringToNoteType(s string) ppcv1.ShiftLogNoteType {
	switch s {
	case dpdomain.NoteInstruksi:
		return ppcv1.ShiftLogNoteType_SHIFT_LOG_NOTE_TYPE_INSTRUKSI
	case dpdomain.NoteActivity:
		return ppcv1.ShiftLogNoteType_SHIFT_LOG_NOTE_TYPE_ACTIVITY
	default:
		return ppcv1.ShiftLogNoteType_SHIFT_LOG_NOTE_TYPE_UNSPECIFIED
	}
}

// optionalNoteType maps an optional proto note type to an optional domain string.
// A nil or UNSPECIFIED type yields nil (no change).
func optionalNoteType(t *ppcv1.ShiftLogNoteType) *string {
	if t == nil || *t == ppcv1.ShiftLogNoteType_SHIFT_LOG_NOTE_TYPE_UNSPECIFIED {
		return nil
	}
	s := noteTypeToString(*t)
	return &s
}

// optionalTimeField parses an optional RFC3339 timestamp field. Empty/nil yields
// nil. On parse failure it returns a 400 BaseResponse.
func optionalTimeField(field string, value *string) (*time.Time, *commonv1.BaseResponse) {
	if value == nil || *value == "" {
		return nil, nil //nolint:nilnil // empty timestamp legitimately maps to no value
	}
	t, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return nil, errorResponse("400", "invalid "+field)
	}
	return &t, nil
}

// optionalDecimalPtr parses an optional decimal-as-string pointer. Nil/empty
// yields nil. On parse failure it returns a 400 BaseResponse.
func optionalDecimalPtr(field string, value *string) (*float64, *commonv1.BaseResponse) {
	if value == nil {
		return nil, nil //nolint:nilnil // absent decimal legitimately maps to no value
	}
	return optionalDecimalField(field, *value)
}
