package dailyperf

import (
	"context"
	"time"
)

// ShiftLogFilter narrows a machine-shift-log listing.
type ShiftLogFilter struct {
	MachineID *int64
	Area      string // TXT / SPG / TWT; "" = all (matched against machine.machine_area)
	Date      *time.Time
	Shift     string
	Status    string // DRAFT / FINAL; "" = all
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

// MachineShiftLogRepository persists machine shift logs.
type MachineShiftLogRepository interface {
	// Upsert inserts or updates a shift log on its (machine, date, shift) key and
	// assigns the generated id.
	Upsert(ctx context.Context, log *MachineShiftLog) error
	// GetByKey loads a shift log by its natural key, or ErrShiftLogNotFound.
	GetByKey(ctx context.Context, machineID int64, date time.Time, shift string) (*MachineShiftLog, error)
	// GetByID loads a shift log by id, or ErrShiftLogNotFound.
	GetByID(ctx context.Context, id int64) (*MachineShiftLog, error)
	// List returns machine shift logs matching the filter plus the total count.
	List(ctx context.Context, filter ShiftLogFilter) ([]*MachineShiftLog, int64, error)
}

// AreaShiftLogRepository persists area shift logs.
type AreaShiftLogRepository interface {
	// Upsert inserts or updates an area shift log on its (area, date, shift) key.
	Upsert(ctx context.Context, log *AreaShiftLog) error
}

// DowntimeEventRepository persists downtime events.
type DowntimeEventRepository interface {
	// ReplaceForShiftLog deletes all downtime events for a shift log and inserts
	// the supplied set, in one transaction.
	ReplaceForShiftLog(ctx context.Context, shiftLogID int64, events []*DowntimeEvent) error
}

// WasteActualRepository persists waste-actual rows.
type WasteActualRepository interface {
	// ReplaceForShiftLog deletes all waste rows for a shift log and inserts the
	// supplied set, in one transaction.
	ReplaceForShiftLog(ctx context.Context, shiftLogID int64, rows []*WasteActual) error
}

// ShiftLogNoteFilter narrows a shift-log-note listing.
type ShiftLogNoteFilter struct {
	MachineID *int64
	Date      *time.Time
	Shift     string
	NoteType  string
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

// ShiftLogNoteRepository persists and queries shift-log notes.
type ShiftLogNoteRepository interface {
	Create(ctx context.Context, note *ShiftLogNote) error
	Update(ctx context.Context, note *ShiftLogNote) error
	Delete(ctx context.Context, id int64) error
	GetByID(ctx context.Context, id int64) (*ShiftLogNote, error)
	List(ctx context.Context, filter ShiftLogNoteFilter) ([]*ShiftLogNote, int64, error)
}

// EfficiencySnapshotRepository persists efficiency snapshots.
type EfficiencySnapshotRepository interface {
	// Upsert inserts or updates a snapshot on its unique scope key.
	Upsert(ctx context.Context, snap *EfficiencySnapshot) error
	// DeleteScope removes snapshots for an (area, date) optionally scoped to one
	// machine, so a recompute can replace stale rows.
	DeleteScope(ctx context.Context, areaCode string, date time.Time, machineID *int64) error
}

// SnapshotFilter selects and paginates efficiency snapshots for listing/export.
type SnapshotFilter struct {
	Page      int32
	PageSize  int32
	Area      string // "" = all areas
	Scope     string // "" = all scopes
	MachineID *int64
	DateFrom  *time.Time
	DateTo    *time.Time
	SortBy    string
	SortOrder string
}

// EfficiencySnapshotReader reads efficiency snapshots for the dashboard list and
// Excel export.
type EfficiencySnapshotReader interface {
	// ListSnapshots returns snapshots matching the filter plus the total count.
	ListSnapshots(ctx context.Context, filter SnapshotFilter) ([]*EfficiencySnapshot, int64, error)
}

// ── Read ports for the efficiency engine ─────────────────────────────────────

// ProductionActual is a projection of one production-actual row used by the
// efficiency engine.
type ProductionActual struct {
	WoID         int64
	MachineID    int64
	Date         time.Time
	Shift        string
	QtyActual    float64
	Breaks       int
	ProdCategory string // NORMAL / B_TO_B / APQ / TRIAL / SMALL_LOT
	Segment      string // DTY / ACY / ATY / POY
}

// ProductionActualReader returns production-actual rows for an area on a date,
// optionally scoped to one machine and/or shift.
type ProductionActualReader interface {
	ProductionActuals(ctx context.Context, areaCode string, date time.Time, machineID *int64, shift *string) ([]ProductionActual, error)
}

// DowntimeAggregate is the per-(machine,shift) downtime rollup used by the engine.
type DowntimeAggregate struct {
	MachineID           int64
	Shift               string
	DurationMin         int
	LostKg              float64
	ExcludedDurationMin int // downtime whose reason is excluded from efficiency
	ExcludedLostKg      float64
}

// DowntimeReader returns aggregated downtime for an area on a date, optionally
// scoped to one machine and/or shift.
type DowntimeReader interface {
	DowntimeAggregates(ctx context.Context, areaCode string, date time.Time, machineID *int64, shift *string) ([]DowntimeAggregate, error)
}

// WasteAggregate is the per-(machine,shift) waste rollup used by the engine.
type WasteAggregate struct {
	MachineID int64
	Shift     string
	QtyKg     float64
}

// WasteReader returns aggregated waste for an area on a date, optionally scoped
// to one machine and/or shift.
type WasteReader interface {
	WasteAggregates(ctx context.Context, areaCode string, date time.Time, machineID *int64, shift *string) ([]WasteAggregate, error)
}

// ShiftLogReader lists the machine shift logs for an area on a date, providing
// positions + derived running minutes for the theoretical computation.
type ShiftLogReader interface {
	ShiftLogsForArea(ctx context.Context, areaCode string, date time.Time, machineID *int64, shift *string) ([]*MachineShiftLog, error)
}

// WellKnownParams are the efficiency-critical pinned parameters for a WO.
type WellKnownParams struct {
	Denier    float64
	Speed     float64
	Positions float64
	StdWeight float64
}

// WellKnownParamSource resolves the well-known efficiency parameters for a WO on
// a machine. Implementations degrade to zero-value params when unavailable.
type WellKnownParamSource interface {
	WellKnown(ctx context.Context, machineID, woID int64) (WellKnownParams, error)
}

// MachineNoLookup resolves a machine's number for denormalized responses.
type MachineNoLookup interface {
	MachineNo(ctx context.Context, machineID int64) (string, error)
}
