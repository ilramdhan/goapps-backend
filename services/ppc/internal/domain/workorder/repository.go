package workorder

import (
	"context"
	"time"
)

// Repository defines persistence operations for work orders and their children.
type Repository interface {
	// Create persists a new WO header and assigns its ID.
	Create(ctx context.Context, entity *WorkOrder) error
	// GetByID retrieves a WO header with its parameters + RM allocations.
	GetByID(ctx context.Context, id int64) (*WorkOrder, error)
	// List retrieves WOs with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*WorkOrder, int64, error)
	// Update persists changes to a WO header (incl. snapshots + approval audit).
	Update(ctx context.Context, entity *WorkOrder) error
	// Delete removes a WO by its ID.
	Delete(ctx context.Context, id int64) error

	// ReplacePlanItemLinks rewrites the set of plan items a WO covers.
	ReplacePlanItemLinks(ctx context.Context, woID int64, links []PlanItemLink) error
	// ListPlanItemLinks lists the plan items a WO covers, anchor included.
	ListPlanItemLinks(ctx context.Context, woID int64) ([]PlanItemLink, error)

	// ReplaceParameters replaces all planned parameter rows for a WO.
	ReplaceParameters(ctx context.Context, woID int64, params []*Parameter) error
	// SetParameterPPCValue upserts the PPC value of one parameter.
	SetParameterPPCValue(ctx context.Context, woID int64, p *Parameter) error
	// SetParameterPCValue upserts the PC value of one parameter.
	SetParameterPCValue(ctx context.Context, woID int64, p *Parameter) error
	// ListParameters lists a WO's planned parameters.
	ListParameters(ctx context.Context, woID int64) ([]*Parameter, error)

	// UpsertExecution upserts one actual parameter value (per date+shift+param).
	UpsertExecution(ctx context.Context, exec *Execution) error
	// ListExecutions lists a WO's actual parameter values.
	ListExecutions(ctx context.Context, woID int64) ([]*Execution, error)

	// ReplaceRmAllocations replaces all RM allocation lines for a WO.
	ReplaceRmAllocations(ctx context.Context, woID int64, allocs []*RmAllocation) error
	// ListRmAllocations lists a WO's RM allocations.
	ListRmAllocations(ctx context.Context, woID int64) ([]*RmAllocation, error)

	// GetProductionActuals lists production-actual rows for a WO, optionally
	// scoped to a date/shift.
	GetProductionActuals(ctx context.Context, woID int64, date *time.Time, shift string) ([]*ProductionActual, error)
	// AdjustActual sets qty_actual (source=ADJUSTED) for a (wo,date,shift) row and
	// appends a wo_actual_log entry in one transaction. Returns the updated row.
	AdjustActual(ctx context.Context, woID int64, date time.Time, shift string, qtyActual float64, reason string, editedBy int64) (*ProductionActual, error)

	// ListPendingApprovals lists submitted/PC-approved WOs whose approval is still
	// pending and were updated at or before cutoff (auto-approve candidates).
	ListPendingApprovals(ctx context.Context, cutoff time.Time) ([]*WorkOrder, error)

	// MaxRevisionNo returns the highest revision number in a WO's revision chain.
	MaxRevisionNo(ctx context.Context, refID int64) (int32, error)
}

// ListFilter contains filtering and pagination for listing WOs.
type ListFilter struct {
	Search     string
	Area       string
	Status     string
	MachineID  *int64
	PlanItemID *int64
	LotNo      string
	Page       int
	PageSize   int
	SortBy     string
	SortOrder  string
}

// Validate normalizes pagination and sort defaults.
func (f *ListFilter) Validate() {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 10
	}
	if f.PageSize > 100 {
		f.PageSize = 100
	}
	if f.SortBy == "" {
		f.SortBy = "created_at"
	}
	if f.SortOrder == "" {
		f.SortOrder = "desc"
	}
}

// Offset returns the SQL offset for pagination.
func (f *ListFilter) Offset() int { return (f.Page - 1) * f.PageSize }

// ProductionActual is the ETL-fed, editable per-date+shift production actual for
// a WO (two-axis v1.2). The full column set lives in migration 000014; this read
// model surfaces the fields consumed by the WO delivery layer. Nullable numeric
// columns are coalesced to zero.
type ProductionActual struct {
	ID    int64
	WOID  int64
	Date  time.Time
	Shift string
	Area  string

	TotalBobbins  int32
	FullBobbins   int32
	UnfullBobbins int32
	NormalBobs    int32
	DowngradeBobs int32
	PendingBobs   int32
	PackCekBobs   int32

	GrossBobbins     int32
	TransferredBobs  int32
	CutBobbins       int32
	NotTransfer      int32
	NormalBobsSpg    int32
	DowngradeBobsSpg int32
	NotCheckedBobs   int32
	WeightPerBob     float64

	QtyBobbin        float64
	QtyActual        float64
	QtySource        string // BOBBIN / ADJUSTED
	AdjustReason     string
	QtyDoffedKg      float64
	QtyTransferredKg float64

	BreaksShift1    int32
	BreaksShift2    int32
	BreaksShift3    int32
	DoffFullCount   int32
	DoffManualCount int32
	CoFailureCount  int32

	SyncStatus   string
	SyncedAt     *time.Time
	LastEditedBy *int64
	LastEditedAt *time.Time
}
