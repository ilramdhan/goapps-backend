package workorder

import "time"

// UpdateParams carries optional editable WO header fields.
type UpdateParams struct {
	MachineID           *int64
	LotNo               *string
	QtyTarget           *float64
	Deadline            *time.Time
	GradeRequirement    *string
	ProdCategory        *string
	AutoApproveDisabled *bool
	RevisionReason      *string
}

// Update applies optional field changes with validation. A DRAFT WO is freely
// editable; a non-DRAFT WO requires a revision reason and bumps the revision.
func (w *WorkOrder) Update(p UpdateParams) error {
	if err := w.applyRevisionGuard(p.RevisionReason); err != nil {
		return err
	}
	if err := w.applyMachine(p.MachineID); err != nil {
		return err
	}
	if err := w.applyLot(p.LotNo); err != nil {
		return err
	}
	if err := w.applyQty(p.QtyTarget); err != nil {
		return err
	}
	if err := w.applyDeadline(p.Deadline); err != nil {
		return err
	}
	if err := w.applyProdCategory(p.ProdCategory); err != nil {
		return err
	}
	if p.GradeRequirement != nil {
		w.gradeRequirement = *p.GradeRequirement
	}
	if p.AutoApproveDisabled != nil {
		w.autoApproveDisabled = *p.AutoApproveDisabled
	}
	w.updatedAt = time.Now()
	return nil
}

func (w *WorkOrder) applyRevisionGuard(reason *string) error {
	if w.status == StatusDraft {
		return nil
	}
	if reason == nil || *reason == "" {
		return ErrNotEditable
	}
	w.revisionReason = *reason
	w.revisionNo++
	return nil
}

func (w *WorkOrder) applyMachine(machineID *int64) error {
	if machineID == nil {
		return nil
	}
	if *machineID <= 0 {
		return ErrInvalidMachine
	}
	w.machineID = *machineID
	return nil
}

func (w *WorkOrder) applyLot(lotNo *string) error {
	if lotNo == nil {
		return nil
	}
	if *lotNo == "" {
		return ErrEmptyLot
	}
	w.lotNo = *lotNo
	return nil
}

func (w *WorkOrder) applyQty(qty *float64) error {
	if qty == nil {
		return nil
	}
	if *qty <= 0 {
		return ErrInvalidQty
	}
	w.qtyTarget = *qty
	return nil
}

func (w *WorkOrder) applyDeadline(deadline *time.Time) error {
	if deadline == nil {
		return nil
	}
	if deadline.IsZero() {
		return ErrInvalidDeadline
	}
	w.deadline = *deadline
	return nil
}

func (w *WorkOrder) applyProdCategory(cat *string) error {
	if cat == nil {
		return nil
	}
	if !IsValidProdCategory(*cat) {
		return ErrInvalidProdCategory
	}
	w.prodCategory = *cat
	return nil
}

// Submit moves a DRAFT WO to SUBMITTED.
func (w *WorkOrder) Submit() error {
	return w.transition(StatusSubmitted)
}

// ApprovePC records the PC (parameter confirmation) approval and moves the WO to
// PC_APPROVED. Returns false (a WO is fully approved only after PM).
func (w *WorkOrder) ApprovePC(userID int64, at time.Time) (bool, error) {
	if w.status != StatusSubmitted {
		return false, ErrNotSubmitted
	}
	if err := w.transition(StatusPCApproved); err != nil {
		return false, err
	}
	t := at
	w.pcApprovedBy = &userID
	w.pcApprovedAt = &t
	return false, nil
}

// ApprovePM records the PM approval and moves the WO to APPROVED. Approval is
// sequential (PRD v1.2): PM may only sign off after PC. Returns whether the WO
// became fully APPROVED.
func (w *WorkOrder) ApprovePM(userID int64, at time.Time) (bool, error) {
	if w.status == StatusSubmitted {
		return false, ErrPCApprovalRequired
	}
	if w.status != StatusPCApproved {
		return false, ErrNotSubmitted
	}
	if err := w.transition(StatusApproved); err != nil {
		return false, err
	}
	t := at
	w.pmApprovedBy = &userID
	w.pmApprovedAt = &t
	return true, nil
}

// Approve records one approval side identified by side (PC/PM).
func (w *WorkOrder) Approve(side string, userID int64, at time.Time) (bool, error) {
	switch side {
	case ApprovalSidePC:
		return w.ApprovePC(userID, at)
	case ApprovalSidePM:
		return w.ApprovePM(userID, at)
	default:
		return false, ErrInvalidApprovalSide
	}
}

// PCApproved reports whether the PC side has approved.
func (w *WorkOrder) PCApproved() bool { return w.pcApprovedAt != nil }

// PMApproved reports whether the PM side has approved.
func (w *WorkOrder) PMApproved() bool { return w.pmApprovedAt != nil }

// SetSnapshots records the spec + packing snapshots taken at PM approval.
func (w *WorkOrder) SetSnapshots(spec, packing map[string]any) {
	if spec != nil {
		w.specSnapshot = spec
	}
	if packing != nil {
		w.packingSnapshot = packing
	}
	w.updatedAt = time.Now()
}

// Reject sends a submitted/PC-approved WO back to PPC (REJECTED) with a reason.
func (w *WorkOrder) Reject(reason string) error {
	if reason == "" {
		return ErrEmptyReason
	}
	if err := w.transition(StatusRejected); err != nil {
		return err
	}
	w.planChangeNote = reason
	return nil
}

// Cancel revokes the WO (CANCELLED) with a mandatory reason (manual, audited).
func (w *WorkOrder) Cancel(reason string) error {
	if reason == "" {
		return ErrEmptyReason
	}
	if err := w.transition(StatusCancelled); err != nil {
		return err
	}
	w.planChangeNote = reason
	return nil
}

func (w *WorkOrder) transition(to string) error {
	if !canTransition(w.status, to) {
		return ErrIllegalTransition
	}
	w.status = to
	w.updatedAt = time.Now()
	return nil
}
