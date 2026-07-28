package planitem

import (
	"regexp"
	"time"
)

var monthPattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}$`)

// GanttRow projects a PlanItem plus Gantt-view-only fields sourced from
// machine_group (area) and the linked work_order (machine/WO/lot/changeover).
// Kept separate from PlanItem so the core entity/DTO used by List/Get/Update
// stays free of Gantt-specific joins.
type GanttRow struct {
	Item         *PlanItem
	Area         string
	MachineNo    string
	WoID         int64
	LotNo        string
	IsChangeover bool
}

// PlanItem is the Layer-2 aggregate root — one row of the living monthly plan.
type PlanItem struct {
	id                 int64
	cpmProductSysID    int64
	itemType           string
	demandID           *int64
	parentItemID       *int64
	qtyTarget          float64
	deadline           time.Time
	rmSource           string
	shadeCode          string
	shadeName          string
	sequence           int32
	status             string
	machineGroupID     int64
	preferredMachineID *int64
	month              string
	plannedStartDate   *time.Time
	plannedDuration    *int32
	durationSource     string
	notes              string
	createdBy          int64
	createdAt          time.Time
	updatedAt          time.Time
	// pendingParentIndex is a transient, never-persisted back-reference used by
	// the cascade: it names the position, within the batch being written, of the
	// item that will become this one's parent. It exists because a chain must be
	// constructed before any of its rows have IDs. Nil for ordinary items.
	pendingParentIndex *int
}

// NewParams carries the inputs for constructing a new PlanItem.
type NewParams struct {
	CpmProductSysID    int64
	Type               string
	DemandID           *int64
	ParentItemID       *int64
	QtyTarget          float64
	Deadline           time.Time
	RMSource           string
	ShadeCode          string
	ShadeName          string
	MachineGroupID     int64
	PreferredMachineID *int64
	Month              string
	MonthOverride      bool
	Timeline           TimelineParams
	Notes              string
	CreatedBy          int64
	// PendingParentIndex names the batch-relative position of the parent for a
	// cascade item constructed before its parent's ID exists. Nil for every
	// ordinary item; index 0 is a legitimate value, hence the pointer.
	PendingParentIndex *int
}

// New creates a validated PlanItem in DRAFT status. Exactly one of DemandID /
// ParentItemID must be set (demand-driven FG vs cascade intermediate), unless
// the item carries a pending batch-relative parent index, which stands in for
// the parent ID that does not exist yet.
func New(p NewParams) (*PlanItem, error) {
	if !IsValidType(p.Type) {
		return nil, ErrInvalidType
	}
	if err := validateParentage(p); err != nil {
		return nil, err
	}
	if p.QtyTarget <= 0 {
		return nil, ErrInvalidQty
	}
	if p.Deadline.IsZero() {
		return nil, ErrInvalidDeadline
	}
	if !IsValidRMSource(p.RMSource) {
		return nil, ErrInvalidRMSource
	}
	if p.MachineGroupID <= 0 {
		return nil, ErrMachineGroupRequired
	}
	month, err := resolveMonth(p.Month, p.Deadline, p.MonthOverride)
	if err != nil {
		return nil, err
	}
	tl, err := resolveTimeline(p.Timeline, p.Deadline)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	return &PlanItem{
		cpmProductSysID:    p.CpmProductSysID,
		itemType:           p.Type,
		demandID:           p.DemandID,
		parentItemID:       p.ParentItemID,
		qtyTarget:          p.QtyTarget,
		deadline:           p.Deadline,
		rmSource:           p.RMSource,
		shadeCode:          p.ShadeCode,
		shadeName:          p.ShadeName,
		status:             StatusDraft,
		machineGroupID:     p.MachineGroupID,
		preferredMachineID: p.PreferredMachineID,
		month:              month,
		plannedStartDate:   tl.startDate,
		plannedDuration:    tl.duration,
		durationSource:     tl.source,
		notes:              p.Notes,
		createdBy:          p.CreatedBy,
		createdAt:          now,
		updatedAt:          now,
		pendingParentIndex: p.PendingParentIndex,
	}, nil
}

// PendingParentIndex returns the batch-relative index of this item's parent, or
// nil when the parent is already a persisted ID (or there is none).
func (p *PlanItem) PendingParentIndex() *int { return p.pendingParentIndex }

// ResolvePendingParent stamps the real parent ID once the parent row has been
// written, clearing the transient index. Persistence must call this before
// writing a cascade item so the row never lands with a null parent.
func (p *PlanItem) ResolvePendingParent(parentID int64) {
	p.parentItemID = &parentID
	p.pendingParentIndex = nil
	p.demandID = nil
}

// resolveMonth derives the planning month from the deadline. A caller may
// override it (carry-forward legitimately parks a demand in a later month),
// but an un-overridden month that diverges is rejected rather than silently
// accepted — a divergent month renders the item outside its own Gantt band.
func resolveMonth(month string, deadline time.Time, override bool) (string, error) {
	derived := monthOf(deadline)
	if !override {
		if month != "" && month != derived {
			return "", ErrMonthMismatch
		}
		return derived, nil
	}
	if !monthPattern.MatchString(month) {
		return "", ErrInvalidMonth
	}
	return month, nil
}

// validateParentage enforces the demand-xor-parent invariant, with a pending
// batch-relative index counting as the parent leg.
func validateParentage(p NewParams) error {
	if p.PendingParentIndex == nil {
		return validateDemandOrParent(p.DemandID, p.ParentItemID)
	}
	if *p.PendingParentIndex < 0 {
		return ErrInvalidPendingParent
	}
	if p.DemandID != nil || p.ParentItemID != nil {
		return ErrDemandAndParentSet
	}
	return nil
}

func validateDemandOrParent(demandID, parentID *int64) error {
	hasDemand := demandID != nil && *demandID > 0
	hasParent := parentID != nil && *parentID > 0
	switch {
	case !hasDemand && !hasParent:
		return ErrDemandOrParentRequired
	case hasDemand && hasParent:
		return ErrDemandAndParentSet
	default:
		return nil
	}
}

// ReconstructParams carries all persisted fields for rebuilding a PlanItem.
type ReconstructParams struct {
	ID                 int64
	CpmProductSysID    int64
	Type               string
	DemandID           *int64
	ParentItemID       *int64
	QtyTarget          float64
	Deadline           time.Time
	RMSource           string
	ShadeCode          string
	ShadeName          string
	Sequence           int32
	Status             string
	MachineGroupID     int64
	PreferredMachineID *int64
	Month              string
	PlannedStartDate   *time.Time
	PlannedDuration    *int32
	DurationSource     string
	Notes              string
	CreatedBy          int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Reconstruct rebuilds a PlanItem from persistence (no validation).
func Reconstruct(p ReconstructParams) *PlanItem {
	return &PlanItem{
		id:                 p.ID,
		cpmProductSysID:    p.CpmProductSysID,
		itemType:           p.Type,
		demandID:           p.DemandID,
		parentItemID:       p.ParentItemID,
		qtyTarget:          p.QtyTarget,
		deadline:           p.Deadline,
		rmSource:           p.RMSource,
		shadeCode:          p.ShadeCode,
		shadeName:          p.ShadeName,
		sequence:           p.Sequence,
		status:             p.Status,
		machineGroupID:     p.MachineGroupID,
		preferredMachineID: p.PreferredMachineID,
		month:              p.Month,
		plannedStartDate:   p.PlannedStartDate,
		plannedDuration:    p.PlannedDuration,
		durationSource:     p.DurationSource,
		notes:              p.Notes,
		createdBy:          p.CreatedBy,
		createdAt:          p.CreatedAt,
		updatedAt:          p.UpdatedAt,
	}
}

// Getters.

// ID returns the plan item identifier.
func (p *PlanItem) ID() int64 { return p.id }

// CpmProductSysID returns the soft-referenced finance product sys id.
func (p *PlanItem) CpmProductSysID() int64 { return p.cpmProductSysID }

// Type returns the plan item type.
func (p *PlanItem) Type() string { return p.itemType }

// DemandID returns the linked demand id (FG items).
func (p *PlanItem) DemandID() *int64 { return p.demandID }

// ParentItemID returns the parent plan item id (intermediate items).
func (p *PlanItem) ParentItemID() *int64 { return p.parentItemID }

// QtyTarget returns the target quantity.
func (p *PlanItem) QtyTarget() float64 { return p.qtyTarget }

// Deadline returns the plan deadline.
func (p *PlanItem) Deadline() time.Time { return p.deadline }

// RMSource returns the RM sourcing mode.
func (p *PlanItem) RMSource() string { return p.rmSource }

// ShadeCode returns the shade (colour) code of the product at this level.
func (p *PlanItem) ShadeCode() string { return p.shadeCode }

// ShadeName returns the shade (colour) name of the product at this level.
func (p *PlanItem) ShadeName() string { return p.shadeName }

// Sequence returns the scheduling sequence.
func (p *PlanItem) Sequence() int32 { return p.sequence }

// Status returns the current lifecycle status.
func (p *PlanItem) Status() string { return p.status }

// MachineGroupID returns the target machine group id.
func (p *PlanItem) MachineGroupID() int64 { return p.machineGroupID }

// PreferredMachineID returns the preferred machine id, if any.
func (p *PlanItem) PreferredMachineID() *int64 { return p.preferredMachineID }

// Month returns the planning month (YYYY-MM).
func (p *PlanItem) Month() string { return p.month }

// PlannedStartDate returns the planned production start date, if any.
func (p *PlanItem) PlannedStartDate() *time.Time { return p.plannedStartDate }

// PlannedDurationDays returns the planned inclusive duration in days, if any.
func (p *PlanItem) PlannedDurationDays() *int32 { return p.plannedDuration }

// DurationSource returns DERIVED or MANUAL.
func (p *PlanItem) DurationSource() string { return p.durationSource }

// IsDurationDerived reports whether the duration is system-derived and should
// therefore be recomputed when the target quantity changes.
func (p *PlanItem) IsDurationDerived() bool {
	return p.durationSource != DurationSourceManual
}

// ApplyDerivedDuration sets a system-derived duration and back-computes the
// planned start from the deadline. It is a no-op on MANUAL items so a planner
// override survives later quantity edits.
func (p *PlanItem) ApplyDerivedDuration(days int32) {
	if !p.IsDurationDerived() {
		return
	}
	if days < MinDurationDays {
		days = MinDurationDays
	}
	if days > MaxDurationDays {
		days = MaxDurationDays
	}
	start := dateOf(p.deadline).AddDate(0, 0, -int(days-1))
	p.plannedStartDate = &start
	p.plannedDuration = &days
	p.durationSource = DurationSourceDerived
}

// Notes returns free-text notes.
func (p *PlanItem) Notes() string { return p.notes }

// CreatedBy returns the creating user id.
func (p *PlanItem) CreatedBy() int64 { return p.createdBy }

// CreatedAt returns the creation timestamp.
func (p *PlanItem) CreatedAt() time.Time { return p.createdAt }

// UpdatedAt returns the last-update timestamp.
func (p *PlanItem) UpdatedAt() time.Time { return p.updatedAt }

// FieldChange records a single field-level edit for the plan-change log.
type FieldChange struct {
	Field  string
	Before string
	After  string
}

// UpdateParams carries optional editable plan-item fields.
type UpdateParams struct {
	QtyTarget          *float64
	Deadline           *time.Time
	RMSource           *string
	Sequence           *int32
	Status             *string
	MachineGroupID     *int64
	PreferredMachineID *int64
	Timeline           TimelineParams
	Notes              *string
}

// Update applies optional field changes with validation and returns the list of
// field-level changes for the plan-change log.
func (p *PlanItem) Update(u UpdateParams) ([]FieldChange, error) {
	var changes []FieldChange
	if err := p.applyQty(u.QtyTarget, &changes); err != nil {
		return nil, err
	}
	if err := p.applyDeadline(u.Deadline, &changes); err != nil {
		return nil, err
	}
	if err := p.applyRMSource(u.RMSource, &changes); err != nil {
		return nil, err
	}
	if err := p.applyStatus(u.Status, &changes); err != nil {
		return nil, err
	}
	if err := p.applyTimeline(u.Timeline, &changes); err != nil {
		return nil, err
	}
	p.applySequence(u.Sequence, &changes)
	p.applyMachineGroup(u.MachineGroupID, &changes)
	p.applyPreferredMachine(u.PreferredMachineID, &changes)
	p.applyNotes(u.Notes, &changes)
	if len(changes) > 0 {
		p.updatedAt = time.Now()
	}
	return changes, nil
}

func (p *PlanItem) applyQty(v *float64, changes *[]FieldChange) error {
	if v == nil {
		return nil
	}
	if *v <= 0 {
		return ErrInvalidQty
	}
	record(changes, "qty_target", formatFloat(p.qtyTarget), formatFloat(*v))
	p.qtyTarget = *v
	return nil
}

func (p *PlanItem) applyDeadline(v *time.Time, changes *[]FieldChange) error {
	if v == nil {
		return nil
	}
	if v.IsZero() {
		return ErrInvalidDeadline
	}
	record(changes, "deadline", p.deadline.Format("2006-01-02"), v.Format("2006-01-02"))
	p.deadline = *v
	record(changes, "month", p.month, monthOf(*v))
	p.month = monthOf(*v)
	p.reanchorTimeline()
	return nil
}

// reanchorTimeline keeps an existing duration anchored to the (possibly new)
// deadline so start/duration/deadline never drift apart.
func (p *PlanItem) reanchorTimeline() {
	if p.plannedDuration == nil {
		return
	}
	start := dateOf(p.deadline).AddDate(0, 0, -int(*p.plannedDuration-1))
	p.plannedStartDate = &start
}

func (p *PlanItem) applyTimeline(t TimelineParams, changes *[]FieldChange) error {
	if !t.IsSet() {
		return nil
	}
	tl, err := resolveTimeline(t, p.deadline)
	if err != nil {
		return err
	}
	record(changes, "planned_start_date", datePtrString(p.plannedStartDate), datePtrString(tl.startDate))
	record(changes, "planned_duration_days", int32PtrString(p.plannedDuration), int32PtrString(tl.duration))
	record(changes, "duration_source", p.durationSource, tl.source)
	p.plannedStartDate = tl.startDate
	p.plannedDuration = tl.duration
	p.durationSource = tl.source
	return nil
}

func (p *PlanItem) applyRMSource(v *string, changes *[]FieldChange) error {
	if v == nil {
		return nil
	}
	if !IsValidRMSource(*v) {
		return ErrInvalidRMSource
	}
	record(changes, "rm_source", p.rmSource, *v)
	p.rmSource = *v
	return nil
}

func (p *PlanItem) applyStatus(v *string, changes *[]FieldChange) error {
	if v == nil {
		return nil
	}
	if !IsValidStatus(*v) {
		return ErrInvalidStatus
	}
	if *v == p.status {
		return nil
	}
	if !canTransition(p.status, *v) {
		return ErrIllegalTransition
	}
	record(changes, "status", p.status, *v)
	p.status = *v
	return nil
}

func (p *PlanItem) applySequence(v *int32, changes *[]FieldChange) {
	if v == nil {
		return
	}
	record(changes, "sequence", formatInt(int64(p.sequence)), formatInt(int64(*v)))
	p.sequence = *v
}

func (p *PlanItem) applyMachineGroup(v *int64, changes *[]FieldChange) {
	if v == nil || *v <= 0 {
		return
	}
	record(changes, "machine_group_id", formatInt(p.machineGroupID), formatInt(*v))
	p.machineGroupID = *v
}

func (p *PlanItem) applyPreferredMachine(v *int64, changes *[]FieldChange) {
	if v == nil {
		return
	}
	record(changes, "preferred_machine_id", int64PtrString(p.preferredMachineID), formatInt(*v))
	p.preferredMachineID = v
}

func (p *PlanItem) applyNotes(v *string, changes *[]FieldChange) {
	if v == nil {
		return
	}
	record(changes, "notes", p.notes, *v)
	p.notes = *v
}

// SetSequence assigns the scheduling sequence directly (used on cascade create).
func (p *PlanItem) SetSequence(seq int32) { p.sequence = seq }

func record(changes *[]FieldChange, field, before, after string) {
	if before == after {
		return
	}
	*changes = append(*changes, FieldChange{Field: field, Before: before, After: after})
}
