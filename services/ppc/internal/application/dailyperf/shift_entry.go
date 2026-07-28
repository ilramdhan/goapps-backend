package dailyperf

import (
	"context"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/dailyperf"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// DowntimeInput is one downtime line supplied with a shift entry.
type DowntimeInput struct {
	ReasonID    int64
	PositionNo  *string
	WoID        *int64
	CeID        *int64
	StartAt     *time.Time
	EndAt       *time.Time
	DurationMin *int32
	LostKg      *float64
	Notes       *string
}

// WasteInput is one waste line supplied with a shift entry.
type WasteInput struct {
	CategoryID int64
	WoID       *int64
	QtyKg      float64
	IsUpset    bool
	Notes      *string
}

// SubmitShiftEntryCommand carries the inputs for recording one machine shift.
type SubmitShiftEntryCommand struct {
	MachineID        int64
	Date             time.Time
	Shift            string
	PositionsTotal   int32
	PositionsRunning float64
	Status           string
	Downtime         []DowntimeInput
	Waste            []WasteInput
	InputBy          int64
}

// SubmitShiftEntry records a machine shift log with its downtime and waste. Per
// v1.2 the running minutes are DERIVED from downtime (480 − Σ downtime minutes,
// floored at 0), never typed by the operator. Downtime lost_kg is auto-computed
// from the well-known theoretical rate when not supplied. The shift-log upsert
// and its downtime/waste replacements are individually transactional at the repo.
func (s *Service) SubmitShiftEntry(ctx context.Context, cmd SubmitShiftEntryCommand) (*dailyperf.MachineShiftLog, error) {
	runningMinutes := deriveRunningMinutes(cmd.Downtime)

	log, err := dailyperf.NewMachineShiftLog(dailyperf.NewShiftLogParams{
		MachineID:        cmd.MachineID,
		Date:             cmd.Date,
		Shift:            cmd.Shift,
		PositionsTotal:   cmd.PositionsTotal,
		PositionsRunning: cmd.PositionsRunning,
		RunningMinutes:   runningMinutes,
		Status:           cmd.Status,
		InputBy:          cmd.InputBy,
	})
	if err != nil {
		return nil, err
	}

	if err := s.shiftLogs.Upsert(ctx, log); err != nil {
		return nil, err
	}

	params := s.wellKnownFor(ctx, cmd.MachineID, cmd.Downtime, cmd.Waste)

	if s.downtime != nil {
		events := s.buildDowntimeEvents(log, cmd, params)
		if err := s.downtime.ReplaceForShiftLog(ctx, log.ID(), events); err != nil {
			return nil, err
		}
	}

	if s.waste != nil {
		rows := buildWasteRows(log, cmd)
		if err := s.waste.ReplaceForShiftLog(ctx, log.ID(), rows); err != nil {
			return nil, err
		}
	}

	log.SetMachineNo(s.resolveMachineNo(ctx, cmd.MachineID))
	return log, nil
}

// deriveRunningMinutes computes running minutes from downtime: full shift minus
// the sum of downtime durations, floored at zero.
func deriveRunningMinutes(downtime []DowntimeInput) int32 {
	total := 0
	for _, d := range downtime {
		if d.DurationMin != nil && *d.DurationMin > 0 {
			total += int(*d.DurationMin)
		}
	}
	running := dailyperf.FullShiftMinutes - total
	if running < 0 {
		running = 0
	}
	return safeconv.IntToInt32(running)
}

// wellKnownFor resolves the shift's well-known params from the first downtime or
// waste line that carries a WO id, so downtime lost_kg can be auto-rated. Returns
// zero-value params when no WO context or source is available (degraded).
func (s *Service) wellKnownFor(ctx context.Context, machineID int64, downtime []DowntimeInput, waste []WasteInput) dailyperf.WellKnownParams {
	if s.wellKnown == nil {
		return dailyperf.WellKnownParams{}
	}
	woID := firstWoID(downtime, waste)
	if woID == 0 {
		return dailyperf.WellKnownParams{}
	}
	params, err := s.wellKnown.WellKnown(ctx, machineID, woID)
	if err != nil {
		return dailyperf.WellKnownParams{}
	}
	return params
}

func firstWoID(downtime []DowntimeInput, waste []WasteInput) int64 {
	for _, d := range downtime {
		if d.WoID != nil && *d.WoID > 0 {
			return *d.WoID
		}
	}
	for _, w := range waste {
		if w.WoID != nil && *w.WoID > 0 {
			return *w.WoID
		}
	}
	return 0
}

func (s *Service) buildDowntimeEvents(log *dailyperf.MachineShiftLog, cmd SubmitShiftEntryCommand, params dailyperf.WellKnownParams) []*dailyperf.DowntimeEvent {
	shiftLogID := log.ID()
	events := make([]*dailyperf.DowntimeEvent, 0, len(cmd.Downtime))
	for _, d := range cmd.Downtime {
		events = append(events, &dailyperf.DowntimeEvent{
			MachineID:   cmd.MachineID,
			WoID:        d.WoID,
			ShiftLogID:  &shiftLogID,
			CeID:        d.CeID,
			Date:        cmd.Date,
			Shift:       cmd.Shift,
			PositionNo:  d.PositionNo,
			ReasonID:    d.ReasonID,
			StartAt:     d.StartAt,
			EndAt:       d.EndAt,
			DurationMin: d.DurationMin,
			LostKg:      s.resolveLostKg(d, params),
			Notes:       d.Notes,
			InputBy:     cmd.InputBy,
			InputAt:     time.Now(),
		})
	}
	return events
}

// resolveLostKg keeps an operator-supplied lost_kg, else auto-computes it from
// the well-known theoretical rate and the downtime duration. Returns nil when
// neither a value nor a computable rate is available.
func (s *Service) resolveLostKg(d DowntimeInput, params dailyperf.WellKnownParams) *float64 {
	if d.LostKg != nil {
		return d.LostKg
	}
	if d.DurationMin == nil || *d.DurationMin <= 0 {
		return nil
	}
	if params.Positions <= 0 || params.Speed <= 0 || params.Denier <= 0 {
		return nil
	}
	lost := s.calc.LossKg(int(*d.DurationMin), params.Positions, params.Speed, params.Denier)
	return &lost
}

func buildWasteRows(log *dailyperf.MachineShiftLog, cmd SubmitShiftEntryCommand) []*dailyperf.WasteActual {
	shiftLogID := log.ID()
	machineID := cmd.MachineID
	rows := make([]*dailyperf.WasteActual, 0, len(cmd.Waste))
	for _, w := range cmd.Waste {
		rows = append(rows, &dailyperf.WasteActual{
			MachineID:  &machineID,
			WoID:       w.WoID,
			ShiftLogID: &shiftLogID,
			Date:       cmd.Date,
			Shift:      cmd.Shift,
			CategoryID: w.CategoryID,
			QtyKg:      w.QtyKg,
			IsUpset:    w.IsUpset,
			Notes:      w.Notes,
			InputBy:    cmd.InputBy,
			InputAt:    time.Now(),
		})
	}
	return rows
}

// ListShiftLogsQuery carries inputs for listing machine shift logs.
type ListShiftLogsQuery struct {
	Page      int
	PageSize  int
	MachineID *int64
	Area      string
	Date      *time.Time
	Shift     string
	Status    string
	SortBy    string
	SortOrder string
}

// ListShiftLogsResult holds a page of machine shift logs plus pagination metadata.
type ListShiftLogsResult struct {
	Items       []*dailyperf.MachineShiftLog
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListShiftLogs retrieves a filtered, paginated page of machine shift logs.
func (s *Service) ListShiftLogs(ctx context.Context, query ListShiftLogsQuery) (*ListShiftLogsResult, error) {
	page := query.Page
	if page <= 0 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	items, total, err := s.shiftLogs.List(ctx, dailyperf.ShiftLogFilter{
		MachineID: query.MachineID,
		Area:      query.Area,
		Date:      query.Date,
		Shift:     query.Shift,
		Status:    query.Status,
		Page:      page,
		PageSize:  pageSize,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
	})
	if err != nil {
		return nil, err
	}

	var totalPages int32
	if total > 0 {
		totalPages = safeconv.Int64ToInt32((total + int64(pageSize) - 1) / int64(pageSize))
	}

	return &ListShiftLogsResult{
		Items:       items,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(page),
		PageSize:    safeconv.IntToInt32(pageSize),
	}, nil
}
