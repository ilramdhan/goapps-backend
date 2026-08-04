package workorder

import (
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/area"
)

// WorkOrder is the Layer-3 aggregate root (v1.2) — a concrete production
// instruction for one machine + lot, driven by a route snapshot and product
// parameters. It owns its parameters (1:N), RM allocations, and executions
// (loaded on demand by the repository).
type WorkOrder struct {
	id                  int64
	woNo                string
	lotNo               string
	area                area.Area
	machineID           int64
	crhHeadID           int64 // route snapshot (cost_route_head)
	crhVersion          int32
	planItemID          int64
	demandID            *int64
	refWoID             *int64
	refType             string // TEMPLATE / CONTINUATION
	qtyTarget           float64
	gradeRequirement    string
	deadline            time.Time
	prodCategory        string
	specSnapshot        map[string]any // JSONB, set at approve
	packingSnapshot     map[string]any // JSONB, set at approve
	revisionNo          int32
	revisionReason      string
	status              string
	autoApproveDisabled bool
	pcApprovedBy        *int64
	pcApprovedAt        *time.Time
	pmApprovedBy        *int64
	pmApprovedAt        *time.Time
	planChangeFlag      bool
	planChangeNote      string
	createdBy           int64
	createdAt           time.Time
	updatedAt           time.Time

	parameters    []*Parameter
	rmAllocations []*RmAllocation
	planItemLinks []PlanItemLink
}

// PlanItemLink is one plan item covered by a work order (wo_plan_item_link).
// A merged WO covers several; each carries the quantity it contributes to the
// WO target. The anchor (WorkOrder.PlanItemID) is always among them.
type PlanItemLink struct {
	ID              int64
	WOID            int64
	PlanItemID      int64
	QtyContribution float64
}

// Parameter holds one planned machine parameter for a WO (1:N per param). It
// carries a PPC-proposed value and, once PC confirms at approve, a PC value
// (both typed by the parameter data_type). Dual params keep independent PPC/PC
// values; single params mirror PPC into PC.
type Parameter struct {
	ID           int64
	WOID         int64
	ParamID      string // UUID, soft ref mst_parameter
	ParamCode    string // denormalized for display / well-known pinning
	ParamName    string
	DataType     string // NUMBER / TEXT / BOOLEAN
	DisplayGroup string
	DisplayOrder int32
	IsDual       bool
	ValuePPCNum  *float64
	ValuePPCText *string
	ValuePPCFlag *bool
	ValuePCNum   *float64
	ValuePCText  *string
	ValuePCFlag  *bool
}

// Execution holds one actual parameter value recorded per date+shift+param.
type Execution struct {
	ID        int64
	WOID      int64
	Date      time.Time
	Shift     string
	ParamID   string // UUID, soft ref mst_parameter
	ParamCode string
	ValueNum  *float64
	ValueText *string
	ValueFlag *bool
	InputBy   int64
	InputAt   time.Time
}

// RmAllocation is one RM component line from the route (cost_route_rm).
type RmAllocation struct {
	ID           int64
	WOID         int64
	CrmRmID      int64
	RmType       string // PRODUCT / ITEM / GROUP
	LotNo        string
	RmSource     string // STORE / CAPTIVE / MIXED
	FreshBox     string
	ShadeCode    string
	QtyAllocated float64
	Notes        string

	// Presentation-only labels resolved from the product's released route. Never
	// persisted (wo_rm_allocation stores only CrmRmID); decorated at read time so
	// no consumer has to render CrmRmID to a user. Empty when the route is
	// unavailable or the edge is no longer part of it.
	RmCode         string
	RmName         string
	RouteStageName string
	RouteLevel     int32
	RouteRmRatio   float64
}

// NewParams carries the inputs for constructing a new WorkOrder header.
type NewParams struct {
	WoNo                string
	LotNo               string
	AreaCode            string
	MachineID           int64
	CrhHeadID           int64
	CrhVersion          int32
	PlanItemID          int64
	DemandID            *int64
	RefWoID             *int64
	RefType             string
	QtyTarget           float64
	GradeRequirement    string
	Deadline            time.Time
	ProdCategory        string
	AutoApproveDisabled bool
	RevisionNo          int32
	CreatedBy           int64
	// PlanItemLinks is the full set of plan items this WO covers. Leave it
	// empty for the ordinary single-item case — the anchor PlanItemID is then
	// linked on its own for the full QtyTarget.
	PlanItemLinks []PlanItemLink
}

// New creates a validated WorkOrder in DRAFT status.
func New(p NewParams) (*WorkOrder, error) {
	ac, err := area.New(p.AreaCode)
	if err != nil {
		return nil, ErrInvalidArea
	}
	links, anchor, err := resolvePlanItemLinks(p)
	if err != nil {
		return nil, err
	}
	if p.MachineID <= 0 {
		return nil, ErrInvalidMachine
	}
	if p.CrhHeadID <= 0 || p.CrhVersion <= 0 {
		return nil, ErrInvalidRoute
	}
	if p.LotNo == "" {
		return nil, ErrEmptyLot
	}
	if p.QtyTarget <= 0 {
		return nil, ErrInvalidQty
	}
	if p.Deadline.IsZero() {
		return nil, ErrInvalidDeadline
	}
	if !IsValidProdCategory(p.ProdCategory) {
		return nil, ErrInvalidProdCategory
	}
	if p.RefWoID != nil && !IsValidRefType(p.RefType) {
		return nil, ErrInvalidRefType
	}
	prodCategory := p.ProdCategory
	if prodCategory == "" {
		prodCategory = ProdCategoryNormal
	}

	now := time.Now()
	return &WorkOrder{
		woNo:                p.WoNo,
		lotNo:               p.LotNo,
		area:                ac,
		machineID:           p.MachineID,
		crhHeadID:           p.CrhHeadID,
		crhVersion:          p.CrhVersion,
		planItemID:          anchor,
		planItemLinks:       links,
		demandID:            p.DemandID,
		refWoID:             p.RefWoID,
		refType:             p.RefType,
		qtyTarget:           p.QtyTarget,
		gradeRequirement:    p.GradeRequirement,
		deadline:            p.Deadline,
		prodCategory:        prodCategory,
		autoApproveDisabled: p.AutoApproveDisabled,
		revisionNo:          p.RevisionNo,
		status:              StatusDraft,
		createdBy:           p.CreatedBy,
		createdAt:           now,
		updatedAt:           now,
	}, nil
}

// resolvePlanItemLinks derives the WO's plan-item link set and its anchor.
//
// The rule the merge feature replaces "plan item id must be positive" with: a
// work order must cover AT LEAST ONE plan item. The anchor is the explicit
// PlanItemID when given, otherwise the first link; every link must reference a
// real plan item and contribute a positive quantity.
func resolvePlanItemLinks(p NewParams) (links []PlanItemLink, anchor int64, err error) {
	if len(p.PlanItemLinks) == 0 {
		if p.PlanItemID <= 0 {
			return nil, 0, ErrInvalidPlanItem
		}
		return []PlanItemLink{{PlanItemID: p.PlanItemID, QtyContribution: p.QtyTarget}}, p.PlanItemID, nil
	}
	anchor = p.PlanItemID
	seen := make(map[int64]bool, len(p.PlanItemLinks))
	links = make([]PlanItemLink, 0, len(p.PlanItemLinks))
	for _, l := range p.PlanItemLinks {
		if l.PlanItemID <= 0 || l.QtyContribution <= 0 {
			return nil, 0, ErrInvalidPlanItem
		}
		if seen[l.PlanItemID] {
			return nil, 0, ErrDuplicatePlanItemLink
		}
		seen[l.PlanItemID] = true
		links = append(links, l)
	}
	if anchor <= 0 {
		anchor = links[0].PlanItemID
	} else if !seen[anchor] {
		return nil, 0, ErrAnchorNotLinked
	}
	return links, anchor, nil
}

// ReconstructParams carries all persisted header fields for rebuilding a WO.
type ReconstructParams struct {
	ID                  int64
	WoNo                string
	LotNo               string
	AreaCode            string
	MachineID           int64
	CrhHeadID           int64
	CrhVersion          int32
	PlanItemID          int64
	DemandID            *int64
	RefWoID             *int64
	RefType             string
	QtyTarget           float64
	GradeRequirement    string
	Deadline            time.Time
	ProdCategory        string
	SpecSnapshot        map[string]any
	PackingSnapshot     map[string]any
	RevisionNo          int32
	RevisionReason      string
	Status              string
	AutoApproveDisabled bool
	PCApprovedBy        *int64
	PCApprovedAt        *time.Time
	PMApprovedBy        *int64
	PMApprovedAt        *time.Time
	PlanChangeFlag      bool
	PlanChangeNote      string
	CreatedBy           int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Reconstruct rebuilds a WorkOrder header from persistence (no validation). The
// area code was validated on write, so a parse failure yields the zero Area.
func Reconstruct(p ReconstructParams) *WorkOrder {
	ac, err := area.New(p.AreaCode)
	if err != nil {
		ac = area.Area{}
	}
	return &WorkOrder{
		id:                  p.ID,
		woNo:                p.WoNo,
		lotNo:               p.LotNo,
		area:                ac,
		machineID:           p.MachineID,
		crhHeadID:           p.CrhHeadID,
		crhVersion:          p.CrhVersion,
		planItemID:          p.PlanItemID,
		demandID:            p.DemandID,
		refWoID:             p.RefWoID,
		refType:             p.RefType,
		qtyTarget:           p.QtyTarget,
		gradeRequirement:    p.GradeRequirement,
		deadline:            p.Deadline,
		prodCategory:        p.ProdCategory,
		specSnapshot:        p.SpecSnapshot,
		packingSnapshot:     p.PackingSnapshot,
		revisionNo:          p.RevisionNo,
		revisionReason:      p.RevisionReason,
		status:              p.Status,
		autoApproveDisabled: p.AutoApproveDisabled,
		pcApprovedBy:        p.PCApprovedBy,
		pcApprovedAt:        p.PCApprovedAt,
		pmApprovedBy:        p.PMApprovedBy,
		pmApprovedAt:        p.PMApprovedAt,
		planChangeFlag:      p.PlanChangeFlag,
		planChangeNote:      p.PlanChangeNote,
		createdBy:           p.CreatedBy,
		createdAt:           p.CreatedAt,
		updatedAt:           p.UpdatedAt,
	}
}

// Getters.

// ID returns the work order identifier.
func (w *WorkOrder) ID() int64 { return w.id }

// WoNo returns the unique WO number.
func (w *WorkOrder) WoNo() string { return w.woNo }

// LotNo returns the PPC-generated lot number.
func (w *WorkOrder) LotNo() string { return w.lotNo }

// AreaCode returns the production area code (TXT/SPG/TWT).
func (w *WorkOrder) AreaCode() string { return w.area.String() }

// MachineID returns the assigned machine id.
func (w *WorkOrder) MachineID() int64 { return w.machineID }

// CrhHeadID returns the snapshot route head id.
func (w *WorkOrder) CrhHeadID() int64 { return w.crhHeadID }

// CrhVersion returns the snapshot route version.
func (w *WorkOrder) CrhVersion() int32 { return w.crhVersion }

// PlanItemID returns the owning plan item id.
func (w *WorkOrder) PlanItemID() int64 { return w.planItemID }

// DemandID returns the originating demand id, if any.
func (w *WorkOrder) DemandID() *int64 { return w.demandID }

// RefWoID returns the referenced prior WO id, if any.
func (w *WorkOrder) RefWoID() *int64 { return w.refWoID }

// RefType returns the WO reference type (TEMPLATE/CONTINUATION).
func (w *WorkOrder) RefType() string { return w.refType }

// QtyTarget returns the target quantity.
func (w *WorkOrder) QtyTarget() float64 { return w.qtyTarget }

// GradeRequirement returns the grade requirement.
func (w *WorkOrder) GradeRequirement() string { return w.gradeRequirement }

// Deadline returns the WO deadline.
func (w *WorkOrder) Deadline() time.Time { return w.deadline }

// ProdCategory returns the production category.
func (w *WorkOrder) ProdCategory() string { return w.prodCategory }

// SpecSnapshot returns the spec snapshot (set at approve), or nil.
func (w *WorkOrder) SpecSnapshot() map[string]any { return w.specSnapshot }

// PackingSnapshot returns the packing snapshot (set at approve), or nil.
func (w *WorkOrder) PackingSnapshot() map[string]any { return w.packingSnapshot }

// RevisionNo returns the revision number.
func (w *WorkOrder) RevisionNo() int32 { return w.revisionNo }

// RevisionReason returns the revision reason shown on the WO face.
func (w *WorkOrder) RevisionReason() string { return w.revisionReason }

// Status returns the current lifecycle status.
func (w *WorkOrder) Status() string { return w.status }

// AutoApproveDisabled reports whether the 24h auto-approve is disabled.
func (w *WorkOrder) AutoApproveDisabled() bool { return w.autoApproveDisabled }

// PCApprovedBy returns the PC approver id.
func (w *WorkOrder) PCApprovedBy() *int64 { return w.pcApprovedBy }

// PCApprovedAt returns the PC approval timestamp.
func (w *WorkOrder) PCApprovedAt() *time.Time { return w.pcApprovedAt }

// PMApprovedBy returns the PM approver id.
func (w *WorkOrder) PMApprovedBy() *int64 { return w.pmApprovedBy }

// PMApprovedAt returns the PM approval timestamp.
func (w *WorkOrder) PMApprovedAt() *time.Time { return w.pmApprovedAt }

// PlanChangeFlag reports whether a plan change is pending resolution.
func (w *WorkOrder) PlanChangeFlag() bool { return w.planChangeFlag }

// PlanChangeNote returns the plan-change note.
func (w *WorkOrder) PlanChangeNote() string { return w.planChangeNote }

// CreatedBy returns the creating user id.
func (w *WorkOrder) CreatedBy() int64 { return w.createdBy }

// CreatedAt returns the creation timestamp.
func (w *WorkOrder) CreatedAt() time.Time { return w.createdAt }

// UpdatedAt returns the last-update timestamp.
func (w *WorkOrder) UpdatedAt() time.Time { return w.updatedAt }

// Parameters returns the attached planned parameters (may be nil if unloaded).
func (w *WorkOrder) Parameters() []*Parameter { return w.parameters }

// AttachParameters sets the loaded parameters (used by the repository).
func (w *WorkOrder) AttachParameters(ps []*Parameter) { w.parameters = ps }

// RmAllocations returns the attached RM allocations (may be nil if unloaded).
func (w *WorkOrder) RmAllocations() []*RmAllocation { return w.rmAllocations }

// AttachRmAllocations sets the loaded RM allocations (used by the repository).
func (w *WorkOrder) AttachRmAllocations(rs []*RmAllocation) { w.rmAllocations = rs }

// PlanItemLinks returns every plan item this WO covers, anchor included.
func (w *WorkOrder) PlanItemLinks() []PlanItemLink { return w.planItemLinks }

// AttachPlanItemLinks sets the loaded plan-item links (used by the repository).
func (w *WorkOrder) AttachPlanItemLinks(ls []PlanItemLink) { w.planItemLinks = ls }

// SetID assigns the generated id (used by the repository after insert).
func (w *WorkOrder) SetID(id int64) { w.id = id }
