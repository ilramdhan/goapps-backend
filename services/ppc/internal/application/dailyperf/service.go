// Package dailyperf provides application-layer use cases for the daily-performance
// domain: shift entries, area shift logs, shift-log notes, and the efficiency
// snapshot engine (PRD v1.2).
package dailyperf

import (
	"context"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/dailyperf"
)

// Deps carries the repositories, read ports, calculator, and lookups needed by
// the daily-performance service. Every field is optional where the corresponding
// use case degrades gracefully; the constructor tolerates nil.
type Deps struct {
	ShiftLogs     dailyperf.MachineShiftLogRepository
	AreaShiftLogs dailyperf.AreaShiftLogRepository
	Downtime      dailyperf.DowntimeEventRepository
	Waste         dailyperf.WasteActualRepository
	Notes         dailyperf.ShiftLogNoteRepository
	Snapshots     dailyperf.EfficiencySnapshotRepository

	ProductionReader dailyperf.ProductionActualReader
	DowntimeReader   dailyperf.DowntimeReader
	WasteReader      dailyperf.WasteReader
	ShiftLogReader   dailyperf.ShiftLogReader

	WellKnown dailyperf.WellKnownParamSource
	MachineNo dailyperf.MachineNoLookup
}

// Service bundles the daily-performance use cases over its dependencies.
type Service struct {
	shiftLogs     dailyperf.MachineShiftLogRepository
	areaShiftLogs dailyperf.AreaShiftLogRepository
	downtime      dailyperf.DowntimeEventRepository
	waste         dailyperf.WasteActualRepository
	notes         dailyperf.ShiftLogNoteRepository
	snapshots     dailyperf.EfficiencySnapshotRepository

	productionReader dailyperf.ProductionActualReader
	downtimeReader   dailyperf.DowntimeReader
	wasteReader      dailyperf.WasteReader
	shiftLogReader   dailyperf.ShiftLogReader

	wellKnown dailyperf.WellKnownParamSource
	machineNo dailyperf.MachineNoLookup

	calc dailyperf.EfficiencyCalculator
}

// NewService builds a daily-performance service from its dependencies.
func NewService(deps Deps) *Service {
	return &Service{
		shiftLogs:        deps.ShiftLogs,
		areaShiftLogs:    deps.AreaShiftLogs,
		downtime:         deps.Downtime,
		waste:            deps.Waste,
		notes:            deps.Notes,
		snapshots:        deps.Snapshots,
		productionReader: deps.ProductionReader,
		downtimeReader:   deps.DowntimeReader,
		wasteReader:      deps.WasteReader,
		shiftLogReader:   deps.ShiftLogReader,
		wellKnown:        deps.WellKnown,
		machineNo:        deps.MachineNo,
		calc:             dailyperf.NewEfficiencyCalculator(),
	}
}

// resolveMachineNo returns the denormalized machine number, or "" when the lookup
// is unavailable or fails (denormalization is best-effort).
func (s *Service) resolveMachineNo(ctx context.Context, machineID int64) string {
	if s.machineNo == nil {
		return ""
	}
	no, err := s.machineNo.MachineNo(ctx, machineID)
	if err != nil {
		return ""
	}
	return no
}
