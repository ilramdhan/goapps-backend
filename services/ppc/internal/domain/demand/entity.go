package demand

import (
	"regexp"
	"time"
)

var monthPattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}$`)

// Demand is the Layer-1 aggregate root — a production commitment to a customer.
type Demand struct {
	id              int64
	demandType      string
	subType         string
	source          string
	carryAction     string
	cpmProductSysID int64
	qtyOriginal     float64
	qtyRemaining    float64
	deadline        time.Time
	customerID      *int64
	contractNo      string
	contractDate    *time.Time
	stuffAdvanceNo  string
	incoterm        string
	lcStatus        string
	gradeReq        string
	shadeCode       string
	shadeName       string
	axMinPct        *float64
	amMaxPct        *float64
	carryFromID     *int64
	sosRef          *int64
	status          string
	// productLinkReason explains why cpmProductSysID is still 0. Non-empty only
	// while status is StatusPendingProductLink.
	productLinkReason string
	month             string
	confirmedBy       *int64
	confirmedAt       *time.Time
	createdBy         int64
	createdAt         time.Time
	updatedAt         time.Time
}

// NewParams carries the inputs for constructing a new Demand.
type NewParams struct {
	Type            string
	SubType         string
	Source          string
	CpmProductSysID int64
	QtyOriginal     float64
	Deadline        time.Time
	GradeReq        string
	ShadeCode       string
	ShadeName       string
	AxMinPct        *float64
	AmMaxPct        *float64
	SosRef          *int64
	CustomerID      *int64
	ContractNo      string
	ContractDate    *time.Time
	Incoterm        string
	LcStatus        string
	StuffAdvanceNo  string
	Month           string
	MonthOverride   bool
	CarryFromID     *int64
	CarryAction     string
	// ProductLinkReason is required when CpmProductSysID is 0 and forbidden
	// otherwise. A zero product is only legitimate as a deliberate deferred link.
	ProductLinkReason string
	CreatedBy         int64
}

// New creates a validated Demand. It starts PENDING_CONFIRMATION, or
// PENDING_PRODUCT_LINK when it carries no product yet.
func New(p NewParams) (*Demand, error) {
	if !IsValidType(p.Type) {
		return nil, ErrInvalidType
	}
	if !subTypeAllowedFor(p.Type, p.SubType) {
		return nil, ErrInvalidSubType
	}
	if !IsValidSource(p.Source) {
		return nil, ErrInvalidSource
	}
	if !IsValidGradeReq(p.GradeReq) {
		return nil, ErrInvalidGradeReq
	}
	if p.QtyOriginal <= 0 {
		return nil, ErrInvalidQty
	}
	if p.Deadline.IsZero() {
		return nil, ErrInvalidDeadline
	}
	month, err := resolveMonth(p.Month, p.Deadline, p.MonthOverride)
	if err != nil {
		return nil, err
	}
	if err := validateClausePct(p.GradeReq, p.AxMinPct, p.AmMaxPct); err != nil {
		return nil, err
	}
	status, err := resolveInitialStatus(p.CpmProductSysID, p.ProductLinkReason)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	return &Demand{
		demandType:      p.Type,
		subType:         p.SubType,
		source:          p.Source,
		carryAction:     p.CarryAction,
		cpmProductSysID: p.CpmProductSysID,
		qtyOriginal:     p.QtyOriginal,
		qtyRemaining:    p.QtyOriginal,
		deadline:        p.Deadline,
		customerID:      p.CustomerID,
		contractNo:      p.ContractNo,
		contractDate:    p.ContractDate,
		stuffAdvanceNo:  p.StuffAdvanceNo,
		incoterm:        p.Incoterm,
		lcStatus:        p.LcStatus,
		gradeReq:        p.GradeReq,
		shadeCode:       p.ShadeCode,
		shadeName:       p.ShadeName,
		axMinPct:        p.AxMinPct,
		amMaxPct:        p.AmMaxPct,
		carryFromID:     p.CarryFromID,
		sosRef:          p.SosRef,
		status:          status,
		// Kept only while unlinked: a linked demand carries no reason.
		productLinkReason: p.ProductLinkReason,
		month:             month,
		createdBy:         p.CreatedBy,
		createdAt:         now,
		updatedAt:         now,
	}, nil
}

// resolveInitialStatus derives the starting status from the product reference.
// A demand with no product is only legal as a deliberate deferred link, so it
// must name a reason; conversely a reason on a linked demand is a contradiction.
func resolveInitialStatus(sysID int64, reason string) (string, error) {
	if sysID == 0 {
		if !IsValidLinkReason(reason) {
			return "", ErrLinkReasonRequired
		}
		return StatusPendingProductLink, nil
	}
	if sysID < 0 {
		return "", ErrInvalidProduct
	}
	if reason != "" {
		return "", ErrLinkReasonNotAllowed
	}
	return StatusPendingConfirmation, nil
}

// monthOf returns the YYYY-MM projection of a deadline.
func monthOf(deadline time.Time) string { return deadline.Format("2006-01") }

// resolveMonth derives the planning month from the deadline. Carry-forward is
// the one legitimate override: it parks a remainder in a later month than the
// source demand's own deadline. Any other divergence is rejected — a demand
// whose month contradicts its deadline renders in the wrong Gantt band.
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

func validateClausePct(gradeReq string, axMin, amMax *float64) error {
	if gradeReq != GradeReqAXAMClause {
		return nil
	}
	if axMin == nil || amMax == nil {
		return ErrClausePctRequired
	}
	for _, v := range []float64{*axMin, *amMax} {
		if v < 0 || v > 100 {
			return ErrInvalidPct
		}
	}
	return nil
}

// ReconstructParams carries all persisted fields for rebuilding a Demand.
type ReconstructParams struct {
	ID                int64
	Type              string
	SubType           string
	Source            string
	CarryAction       string
	CpmProductSysID   int64
	QtyOriginal       float64
	QtyRemaining      float64
	Deadline          time.Time
	CustomerID        *int64
	ContractNo        string
	ContractDate      *time.Time
	StuffAdvanceNo    string
	Incoterm          string
	LcStatus          string
	GradeReq          string
	ShadeCode         string
	ShadeName         string
	AxMinPct          *float64
	AmMaxPct          *float64
	CarryFromID       *int64
	SosRef            *int64
	Status            string
	ProductLinkReason string
	Month             string
	ConfirmedBy       *int64
	ConfirmedAt       *time.Time
	CreatedBy         int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Reconstruct rebuilds a Demand from persistence (no validation).
func Reconstruct(p ReconstructParams) *Demand {
	return &Demand{
		id:                p.ID,
		demandType:        p.Type,
		subType:           p.SubType,
		source:            p.Source,
		carryAction:       p.CarryAction,
		cpmProductSysID:   p.CpmProductSysID,
		qtyOriginal:       p.QtyOriginal,
		qtyRemaining:      p.QtyRemaining,
		deadline:          p.Deadline,
		customerID:        p.CustomerID,
		contractNo:        p.ContractNo,
		contractDate:      p.ContractDate,
		stuffAdvanceNo:    p.StuffAdvanceNo,
		incoterm:          p.Incoterm,
		lcStatus:          p.LcStatus,
		gradeReq:          p.GradeReq,
		shadeCode:         p.ShadeCode,
		shadeName:         p.ShadeName,
		axMinPct:          p.AxMinPct,
		amMaxPct:          p.AmMaxPct,
		carryFromID:       p.CarryFromID,
		sosRef:            p.SosRef,
		status:            p.Status,
		productLinkReason: p.ProductLinkReason,
		month:             p.Month,
		confirmedBy:       p.ConfirmedBy,
		confirmedAt:       p.ConfirmedAt,
		createdBy:         p.CreatedBy,
		createdAt:         p.CreatedAt,
		updatedAt:         p.UpdatedAt,
	}
}

// Getters.

// ID returns the demand identifier.
func (d *Demand) ID() int64 { return d.id }

// Type returns the demand type.
func (d *Demand) Type() string { return d.demandType }

// SubType returns the demand sub-type.
func (d *Demand) SubType() string { return d.subType }

// Source returns the demand source.
func (d *Demand) Source() string { return d.source }

// CarryAction returns the carry-forward action (empty when not carried).
func (d *Demand) CarryAction() string { return d.carryAction }

// CpmProductSysID returns the soft-referenced finance product sys id.
func (d *Demand) CpmProductSysID() int64 { return d.cpmProductSysID }

// QtyOriginal returns the original committed quantity.
func (d *Demand) QtyOriginal() float64 { return d.qtyOriginal }

// QtyRemaining returns the remaining unfulfilled quantity.
func (d *Demand) QtyRemaining() float64 { return d.qtyRemaining }

// Deadline returns the delivery deadline.
func (d *Demand) Deadline() time.Time { return d.deadline }

// CustomerID returns the soft-referenced customer id.
func (d *Demand) CustomerID() *int64 { return d.customerID }

// ContractNo returns the contract number.
func (d *Demand) ContractNo() string { return d.contractNo }

// ContractDate returns the contract date.
func (d *Demand) ContractDate() *time.Time { return d.contractDate }

// StuffAdvanceNo returns the stuffing advance number.
func (d *Demand) StuffAdvanceNo() string { return d.stuffAdvanceNo }

// Incoterm returns the incoterm.
func (d *Demand) Incoterm() string { return d.incoterm }

// LcStatus returns the letter-of-credit status.
func (d *Demand) LcStatus() string { return d.lcStatus }

// GradeReq returns the grade requirement.
func (d *Demand) GradeReq() string { return d.gradeReq }

// ShadeCode returns the shade (colour) code carried from the Orion staging row.
func (d *Demand) ShadeCode() string { return d.shadeCode }

// ShadeName returns the shade (colour) name carried from the Orion staging row.
func (d *Demand) ShadeName() string { return d.shadeName }

// AxMinPct returns the minimum AX percentage (AX_AM_CLAUSE only).
func (d *Demand) AxMinPct() *float64 { return d.axMinPct }

// AmMaxPct returns the maximum AM percentage (AX_AM_CLAUSE only).
func (d *Demand) AmMaxPct() *float64 { return d.amMaxPct }

// CarryFromID returns the originating demand id for carried-forward demands.
func (d *Demand) CarryFromID() *int64 { return d.carryFromID }

// SosRef returns the sales-order-staging reference.
func (d *Demand) SosRef() *int64 { return d.sosRef }

// Status returns the current lifecycle status.
func (d *Demand) Status() string { return d.status }

// ProductLinkReason returns why the product is not linked yet (empty once linked).
func (d *Demand) ProductLinkReason() string { return d.productLinkReason }

// IsProductLinked reports whether the demand has a finance product mapped.
func (d *Demand) IsProductLinked() bool { return d.cpmProductSysID != 0 }

// Month returns the planning month (YYYY-MM).
func (d *Demand) Month() string { return d.month }

// ConfirmedBy returns the confirming user id.
func (d *Demand) ConfirmedBy() *int64 { return d.confirmedBy }

// ConfirmedAt returns the confirmation timestamp.
func (d *Demand) ConfirmedAt() *time.Time { return d.confirmedAt }

// CreatedBy returns the creating user id.
func (d *Demand) CreatedBy() int64 { return d.createdBy }

// CreatedAt returns the creation timestamp.
func (d *Demand) CreatedAt() time.Time { return d.createdAt }

// UpdatedAt returns the last-update timestamp.
func (d *Demand) UpdatedAt() time.Time { return d.updatedAt }

// EstProdNeeded returns the estimated production needed given a historical AX
// yield percentage (0-1). A non-positive yield returns qty_remaining unchanged.
func (d *Demand) EstProdNeeded(axYieldPct float64) float64 {
	if axYieldPct <= 0 {
		return d.qtyRemaining
	}
	return d.qtyRemaining / axYieldPct
}

// UpdateParams carries optional editable demand fields.
type UpdateParams struct {
	QtyOriginal    *float64
	Deadline       *time.Time
	GradeReq       *string
	AxMinPct       *float64
	AmMaxPct       *float64
	ContractNo     *string
	Incoterm       *string
	LcStatus       *string
	StuffAdvanceNo *string
}

// Update applies optional field changes with validation. Quantity edits adjust
// remaining by the same delta so fulfilled progress is preserved.
func (d *Demand) Update(p UpdateParams) error {
	if p.QtyOriginal != nil {
		if *p.QtyOriginal <= 0 {
			return ErrInvalidQty
		}
		delta := *p.QtyOriginal - d.qtyOriginal
		d.qtyOriginal = *p.QtyOriginal
		d.qtyRemaining += delta
		if d.qtyRemaining < 0 {
			d.qtyRemaining = 0
		}
	}
	if p.Deadline != nil {
		if p.Deadline.IsZero() {
			return ErrInvalidDeadline
		}
		d.deadline = *p.Deadline
		d.month = monthOf(*p.Deadline)
	}
	if p.GradeReq != nil {
		if !IsValidGradeReq(*p.GradeReq) {
			return ErrInvalidGradeReq
		}
		d.gradeReq = *p.GradeReq
	}
	if p.AxMinPct != nil {
		d.axMinPct = p.AxMinPct
	}
	if p.AmMaxPct != nil {
		d.amMaxPct = p.AmMaxPct
	}
	if err := validateClausePct(d.gradeReq, d.axMinPct, d.amMaxPct); err != nil {
		return err
	}
	applyStringPtr(&d.contractNo, p.ContractNo)
	applyStringPtr(&d.incoterm, p.Incoterm)
	applyStringPtr(&d.lcStatus, p.LcStatus)
	applyStringPtr(&d.stuffAdvanceNo, p.StuffAdvanceNo)
	d.updatedAt = time.Now()
	return nil
}

func applyStringPtr(dst *string, src *string) {
	if src != nil {
		*dst = *src
	}
}

// SetProduct maps a finance cost-product-master sys id onto a demand that
// currently has none. Product is locked after being mapped once — rejects
// re-mapping an already-mapped demand.
func (d *Demand) SetProduct(sysID int64) error {
	if d.cpmProductSysID != 0 {
		return ErrProductAlreadyMapped
	}
	if sysID <= 0 {
		return ErrInvalidProduct
	}
	// Linking is the single outbound transition of PENDING_PRODUCT_LINK. The
	// reason is cleared with it: the DB CHECK pairs a null product with that
	// status, so a lingering reason on a linked demand would contradict it.
	if d.status == StatusPendingProductLink {
		if err := d.transition(StatusPendingConfirmation); err != nil {
			return err
		}
		d.productLinkReason = ""
	}
	d.cpmProductSysID = sysID
	d.updatedAt = time.Now()
	return nil
}

// Confirm moves a PENDING_CONFIRMATION demand to CONFIRMED.
func (d *Demand) Confirm(userID int64) error {
	if err := d.transition(StatusConfirmed); err != nil {
		return err
	}
	now := time.Now()
	d.confirmedBy = &userID
	d.confirmedAt = &now
	return nil
}

// ApproveMTS records the Marketing decision on an MTS demand. Approval confirms
// the demand and stamps its source MTS_APPROVED; rejection cancels it.
func (d *Demand) ApproveMTS(approved bool, userID int64) error {
	if d.demandType != TypeMTS {
		return ErrNotMTS
	}
	if approved {
		if err := d.transition(StatusConfirmed); err != nil {
			return err
		}
		now := time.Now()
		d.source = SourceMTSApproved
		d.confirmedBy = &userID
		d.confirmedAt = &now
		return nil
	}
	return d.transition(StatusCancelled)
}

// MarkCarriedOver moves the demand to CARRIED_OVER after a carry-forward.
func (d *Demand) MarkCarriedOver() error { return d.transition(StatusCarriedOver) }

// MarkDeferred moves the demand to DEFERRED.
func (d *Demand) MarkDeferred() error { return d.transition(StatusDeferred) }

// MarkSplit moves the demand to SPLIT after producing split children.
func (d *Demand) MarkSplit() error { return d.transition(StatusSplit) }

// Cancel moves the demand to CANCELLED.
func (d *Demand) Cancel() error { return d.transition(StatusCancelled) }

// ReduceRemaining decreases remaining by qty, clamped at zero. Used by
// PARTIAL_CARRY to carry part of a demand and cancel the rest.
func (d *Demand) ReduceRemaining(qty float64) {
	d.qtyRemaining -= qty
	if d.qtyRemaining < 0 {
		d.qtyRemaining = 0
	}
	d.updatedAt = time.Now()
}

// IsCarryCandidate reports whether the demand may be carried forward
// (PARTIAL / IN_PRODUCTION / CONFIRMED / DEFERRED with remaining qty).
func (d *Demand) IsCarryCandidate() bool {
	switch d.status {
	case StatusPartial, StatusInProduction, StatusConfirmed, StatusDeferred:
		return d.qtyRemaining > 0
	default:
		return false
	}
}

func (d *Demand) transition(to string) error {
	if !canTransition(d.status, to) {
		return ErrIllegalTransition
	}
	d.status = to
	d.updatedAt = time.Now()
	return nil
}
