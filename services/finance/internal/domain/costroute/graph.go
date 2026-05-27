package costroute

import (
	"errors"
	"fmt"
)

// Sentinel errors for graph validation.
var (
	ErrLevelOneMismatch       = errors.New("level-1 seq must produce the head's product")
	ErrLevelOneMissing        = errors.New("graph must contain a level-1 seq")
	ErrUpstreamMissing        = errors.New("PRODUCT-type input has no upstream seq producing it")
	ErrUpstreamNotHigherLevel = errors.New("PRODUCT-type input must reference a seq at higher level")
	ErrInvalidRmType          = errors.New("rm_type must be PRODUCT, ITEM, or GROUP")
	ErrMultipleRmRefs         = errors.New("exactly one of rm_product_sys_id/rm_item_code/rm_group_code must be set")
	ErrRmRefTypeMismatch      = errors.New("rm_type does not match the populated ref column")
	ErrNonPositiveRatio       = errors.New("route_rm_ratio must be positive")
)

// ValidateLevels enforces the routing's level discipline:
//   - exactly one SEQ at level 1, producing head.ProductSysID;
//   - every PRODUCT-type RM references a product produced by some SEQ at a
//     STRICTLY HIGHER level (level > current SEQ level);
//   - ratio > 0 on every RM;
//   - rm_type matches the populated ref column.
//
// Returns the first violation found. Topology is naturally acyclic because
// upstream level is strictly greater than downstream level.
func (g *Graph) ValidateLevels() error { //nolint:gocognit,gocyclo // graph level-invariant validation, cohesive
	if g == nil || g.Head == nil {
		return ErrNotFound
	}
	if len(g.Seqs) == 0 {
		return ErrLevelOneMissing
	}

	// 1. Build (productSysID -> highest level it's produced at) index for
	//    upstream lookup.
	producedAt := make(map[int64]int32, len(g.Seqs))
	var levelOne *Seq
	for _, s := range g.Seqs {
		if s == nil {
			continue
		}
		if s.RouteLevel == 1 {
			if levelOne != nil {
				// Multiple level-1 seqs allowed only if they produce the same FG?
				// For now, we keep the rule strict: exactly one level-1 seq.
				return fmt.Errorf("multiple level-1 seqs: %d and %d", levelOne.SeqID, s.SeqID)
			}
			levelOne = s
		}
		// Record producedAt with the deepest level (= highest number).
		if cur, ok := producedAt[s.ProductSysID]; !ok || s.RouteLevel > cur {
			producedAt[s.ProductSysID] = s.RouteLevel
		}
	}
	if levelOne == nil {
		return ErrLevelOneMissing
	}
	if levelOne.ProductSysID != g.Head.ProductSysID {
		return ErrLevelOneMismatch
	}

	// 2. Validate every RM.
	for _, s := range g.Seqs {
		if s == nil {
			continue
		}
		for _, rm := range s.Rms {
			if rm == nil {
				continue
			}
			if err := validateRm(rm); err != nil {
				return fmt.Errorf("seq #%d level=%d seq=%d rm #%d: %w", s.SeqID, s.RouteLevel, s.RouteSeq, rm.RmID, err)
			}
			if rm.RmType != RmTypeProduct {
				continue
			}
			upstreamLevel, ok := producedAt[rm.RmProductSysID]
			if !ok {
				return fmt.Errorf("seq #%d rm #%d -> product %d: %w", s.SeqID, rm.RmID, rm.RmProductSysID, ErrUpstreamMissing)
			}
			if upstreamLevel <= s.RouteLevel {
				return fmt.Errorf("seq #%d level=%d rm #%d -> product %d produced at level %d: %w",
					s.SeqID, s.RouteLevel, rm.RmID, rm.RmProductSysID, upstreamLevel, ErrUpstreamNotHigherLevel)
			}
		}
	}
	return nil
}

func validateRm(rm *Rm) error {
	if rm.RouteRmRatio <= 0 {
		return ErrNonPositiveRatio
	}
	switch rm.RmType {
	case RmTypeProduct:
		if rm.RmProductSysID == 0 || rm.RmItemCode != "" || rm.RmGroupCode != "" {
			return ErrRmRefTypeMismatch
		}
	case RmTypeItem:
		if rm.RmItemCode == "" || rm.RmProductSysID != 0 || rm.RmGroupCode != "" {
			return ErrRmRefTypeMismatch
		}
	case RmTypeGroup:
		if rm.RmGroupCode == "" || rm.RmProductSysID != 0 || rm.RmItemCode != "" {
			return ErrRmRefTypeMismatch
		}
	default:
		return ErrInvalidRmType
	}
	return nil
}

// Head behavior methods.

// MarkComplete transitions DRAFT -> COMPLETE.
func (h *Head) MarkComplete() error {
	if h.RoutingStatus != StatusDraft {
		return ErrInvalidStatusTransition
	}
	h.RoutingStatus = StatusComplete
	return nil
}

// Lock transitions COMPLETE -> LOCKED.
func (h *Head) Lock() error {
	if h.RoutingStatus != StatusComplete {
		return ErrInvalidStatusTransition
	}
	h.RoutingStatus = StatusLocked
	return nil
}

// Unlock transitions LOCKED -> COMPLETE.
func (h *Head) Unlock() error {
	if h.RoutingStatus != StatusLocked {
		return ErrInvalidStatusTransition
	}
	h.RoutingStatus = StatusComplete
	return nil
}

// IsLocked reports whether the head is in LOCKED status (edits forbidden).
func (h *Head) IsLocked() bool { return h.RoutingStatus == StatusLocked }
