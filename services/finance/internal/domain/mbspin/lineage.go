// Package mbspin provides domain logic for Melange Batch Spin (child of MB Head) management.
package mbspin

import (
	"fmt"

	"github.com/google/uuid"
)

// cloneNameSuffix is appended to a cloned spin's name when the caller does not
// override it.
const cloneNameSuffix = " (copy)"

// mgtNameMaxLen mirrors mst_mb_spin.mbs_mgt_name VARCHAR(100) (migration 000389:9).
const mgtNameMaxLen = 100

// ParentLookup yields the mbs_parent_spin_id of one spin. It returns nil when
// that spin has no parent (chain terminates), and ErrNotFound when the row is
// absent — a dangling parent pointer, which terminates the walk rather than
// failing it, because fk_mbs_parent_spin is ON DELETE SET NULL and a soft-deleted
// ancestor is a legitimate state.
type ParentLookup func(id uuid.UUID) (*uuid.UUID, error)

// AssertNoParentCycle walks the mbs_parent_spin_id chain upward from srcID and
// reports how many ancestors it traversed.
//
// Why this lives in the domain: the RULE is pure — "the chain above the source
// must not contain the source, and must terminate within MaxLineageDepth hops".
// Only the single-hop READ is I/O, and that is injected as ParentLookup. Keeping
// the rule here makes it unit-testable with no database and gives it exactly one
// implementation; the postgres layer supplies a lookup bound to its open
// transaction so the walk observes the same locked snapshot as the INSERT.
//
// Why the rule is needed at all: migration 000484 deliberately ships WITHOUT the
// chk_mbs_parent_not_self CHECK, so the database does not reject even a 1-hop
// self-loop (A -> A), let alone A -> B -> A. This function is the only guard
// (R8/G8) — ⛔ do not assume the DB helps.
//
// Errors:
//   - ErrParentCycle       — the chain revisits srcID, or revisits any node
//     already seen on this walk (an upstream loop that srcID would join).
//   - ErrMaxDuplicateDepth — more than MaxLineageDepth hops without terminating.
func AssertNoParentCycle(srcID uuid.UUID, lookup ParentLookup) (int, error) {
	if srcID == uuid.Nil {
		return 0, ErrInvalidHeadID
	}
	seen := map[uuid.UUID]struct{}{srcID: {}}
	cur := srcID
	for depth := 0; depth <= MaxLineageDepth; depth++ {
		parent, err := lookup(cur)
		if err != nil {
			return depth, err
		}
		if parent == nil {
			return depth, nil // chain terminated cleanly
		}
		if *parent == srcID {
			return depth, ErrParentCycle
		}
		if _, dup := seen[*parent]; dup {
			return depth, ErrParentCycle
		}
		seen[*parent] = struct{}{}
		cur = *parent
	}
	return MaxLineageDepth, ErrMaxDuplicateDepth
}

// CloneMgtName decides the name a cloned spin is stored under.
//
// override != nil wins verbatim, and is validated exactly as Update would
// validate it (non-empty, at most 100 chars) so the duplicate path cannot smuggle
// in a name a normal edit would reject.
//
// With no override the source name gains " (copy)". ⚠ When that would exceed the
// VARCHAR(100) column, the SOURCE NAME IS KEPT UNSUFFIXED rather than truncated:
// spin names are not unique, so a repeated name is harmless, whereas silently
// chopping characters off a 98-character management name would corrupt it.
func CloneMgtName(sourceName string, override *string) (string, error) {
	if override != nil {
		if *override == "" {
			return "", ErrEmptyMgtName
		}
		if len(*override) > mgtNameMaxLen {
			return "", ErrMgtNameTooLong
		}
		return *override, nil
	}
	if sourceName == "" {
		return "", ErrEmptyMgtName
	}
	if len(sourceName)+len(cloneNameSuffix) > mgtNameMaxLen {
		return sourceName, nil
	}
	return sourceName + cloneNameSuffix, nil
}

// AssertRecalcFanOut rejects a recalc whose candidate set is larger than
// MaxRecalcChildren, so an unexpectedly wide head degrades to an explicit
// "do it manually" instead of a huge implicit write.
func AssertRecalcFanOut(candidates int) error {
	if candidates > MaxRecalcChildren {
		return fmt.Errorf("%w: %d candidates exceeds limit of %d", ErrTooManyChildren, candidates, MaxRecalcChildren)
	}
	return nil
}
