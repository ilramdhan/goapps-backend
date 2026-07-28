package demand

import (
	"context"
	"time"

	demanddomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/demand"
)

// CarryForwardSplit is one child in a SPLIT carry-forward action.
type CarryForwardSplit struct {
	Qty      float64
	Deadline time.Time
}

// ProcessCarryForwardCommand carries inputs for a month-start carry-forward.
type ProcessCarryForwardCommand struct {
	SourceDemandID int64
	Action         string
	TargetMonth    string
	NewDeadline    *time.Time
	CarryQty       *float64
	Splits         []CarryForwardSplit
	ActedBy        int64
}

// ListCarryCandidates returns demands eligible for carry-forward in a month.
func (s *Service) ListCarryCandidates(ctx context.Context, sourceMonth string) ([]*demanddomain.Demand, error) {
	return s.repo.ListCarryCandidates(ctx, sourceMonth)
}

// ProcessCarryForward executes one of the five carry-forward actions against a
// source demand, returning any newly-created demand(s). SPLIT enforces
// SUM(child qty) <= source remaining.
func (s *Service) ProcessCarryForward(ctx context.Context, cmd ProcessCarryForwardCommand) ([]*demanddomain.Demand, error) {
	source, err := s.repo.GetByID(ctx, cmd.SourceDemandID)
	if err != nil {
		return nil, err
	}
	if !source.IsCarryCandidate() {
		return nil, demanddomain.ErrNotCarryCandidate
	}

	switch cmd.Action {
	case demanddomain.CarryActionAsIs:
		return s.carryAsIs(ctx, source, cmd)
	case demanddomain.CarryActionPartial:
		return s.carryPartial(ctx, source, cmd)
	case demanddomain.CarryActionSplit:
		return s.carrySplit(ctx, source, cmd)
	case demanddomain.CarryActionDefer:
		return s.carryDefer(ctx, source)
	case demanddomain.CarryActionCancel:
		return s.carryCancel(ctx, source)
	default:
		return nil, demanddomain.ErrInvalidCarryAction
	}
}

// newChild builds a CARRY_FORWARD child demand cloned from source with a new qty,
// deadline, and target month.
func (s *Service) newChild(source *demanddomain.Demand, qty float64, deadline time.Time, month string, actedBy int64) (*demanddomain.Demand, error) {
	fromID := source.ID()
	return demanddomain.New(demanddomain.NewParams{
		Type:            source.Type(),
		SubType:         carrySubType(source),
		Source:          demanddomain.SourceCarryForward,
		CpmProductSysID: source.CpmProductSysID(),
		QtyOriginal:     qty,
		Deadline:        deadline,
		GradeReq:        source.GradeReq(),
		AxMinPct:        source.AxMinPct(),
		AmMaxPct:        source.AmMaxPct(),
		CustomerID:      source.CustomerID(),
		ContractNo:      source.ContractNo(),
		ContractDate:    source.ContractDate(),
		Incoterm:        source.Incoterm(),
		LcStatus:        source.LcStatus(),
		StuffAdvanceNo:  source.StuffAdvanceNo(),
		Month:           month,
		MonthOverride:   true, // carry-forward legitimately parks a remainder in a later month
		CarryFromID:     &fromID,
		CarryAction:     source.CarryAction(),
		CreatedBy:       actedBy,
	})
}

// carrySubType keeps CONTRACT carry-forwards classified as CF_EXPORT; other
// types keep their original sub-type.
func carrySubType(source *demanddomain.Demand) string {
	if source.Type() == demanddomain.TypeContract {
		return demanddomain.SubTypeCFExport
	}
	return source.SubType()
}

func (s *Service) carryAsIs(ctx context.Context, source *demanddomain.Demand, cmd ProcessCarryForwardCommand) ([]*demanddomain.Demand, error) {
	deadline := carryDeadline(source, cmd.NewDeadline)
	child, err := s.newChild(source, source.QtyRemaining(), deadline, cmd.TargetMonth, cmd.ActedBy)
	if err != nil {
		return nil, err
	}
	return s.persistCarry(ctx, source, []*demanddomain.Demand{child})
}

func (s *Service) carryPartial(ctx context.Context, source *demanddomain.Demand, cmd ProcessCarryForwardCommand) ([]*demanddomain.Demand, error) {
	if cmd.CarryQty == nil || *cmd.CarryQty <= 0 {
		return nil, demanddomain.ErrInvalidQty
	}
	if *cmd.CarryQty > source.QtyRemaining() {
		return nil, demanddomain.ErrSplitExceedsRemaining
	}
	deadline := carryDeadline(source, cmd.NewDeadline)
	child, err := s.newChild(source, *cmd.CarryQty, deadline, cmd.TargetMonth, cmd.ActedBy)
	if err != nil {
		return nil, err
	}
	return s.persistCarry(ctx, source, []*demanddomain.Demand{child})
}

func (s *Service) carrySplit(ctx context.Context, source *demanddomain.Demand, cmd ProcessCarryForwardCommand) ([]*demanddomain.Demand, error) {
	if len(cmd.Splits) == 0 {
		return nil, demanddomain.ErrNoSplitChildren
	}
	var sum float64
	for _, sp := range cmd.Splits {
		if sp.Qty <= 0 {
			return nil, demanddomain.ErrInvalidQty
		}
		sum += sp.Qty
	}
	if sum > source.QtyRemaining() {
		return nil, demanddomain.ErrSplitExceedsRemaining
	}

	children := make([]*demanddomain.Demand, 0, len(cmd.Splits))
	for _, sp := range cmd.Splits {
		child, err := s.newChild(source, sp.Qty, sp.Deadline, cmd.TargetMonth, cmd.ActedBy)
		if err != nil {
			return nil, err
		}
		children = append(children, child)
	}
	if err := source.MarkSplit(); err != nil {
		return nil, err
	}
	return s.persistChildrenThenSource(ctx, source, children)
}

func (s *Service) carryDefer(ctx context.Context, source *demanddomain.Demand) ([]*demanddomain.Demand, error) {
	if err := source.MarkDeferred(); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, source); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *Service) carryCancel(ctx context.Context, source *demanddomain.Demand) ([]*demanddomain.Demand, error) {
	if err := source.Cancel(); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, source); err != nil {
		return nil, err
	}
	return nil, nil
}

// persistCarry marks the source CARRIED_OVER then persists source + children.
func (s *Service) persistCarry(ctx context.Context, source *demanddomain.Demand, children []*demanddomain.Demand) ([]*demanddomain.Demand, error) {
	if err := source.MarkCarriedOver(); err != nil {
		return nil, err
	}
	return s.persistChildrenThenSource(ctx, source, children)
}

func (s *Service) persistChildrenThenSource(ctx context.Context, source *demanddomain.Demand, children []*demanddomain.Demand) ([]*demanddomain.Demand, error) {
	for _, child := range children {
		if err := s.repo.Create(ctx, child); err != nil {
			return nil, err
		}
	}
	if err := s.repo.Update(ctx, source); err != nil {
		return nil, err
	}
	return children, nil
}

func carryDeadline(source *demanddomain.Demand, override *time.Time) time.Time {
	if override != nil && !override.IsZero() {
		return *override
	}
	return source.Deadline()
}
