package mbheadbulk

import "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"

// kahnTopoSort reorders order so that, for every edge A -> B (A depends on B,
// i.e. A's recipe references B as a nested MB RM input), B is placed before A.
// Nodes with no dependency between them keep their original relative order
// (stable), so a batch with no within-batch references is returned unchanged.
//
// Edges pointing outside order, or self-referencing a node, are ignored — the
// caller (ListMBRefEdgesForBatch) already restricts edges to within-batch
// pairs, but this stays defensive rather than trusting that invariant blindly.
//
// Cycle handling: if a group of nodes forms a dependency cycle (A depends on
// B depends on A — should not normally happen, but recipes are not validated
// against it elsewhere), no member of that group can ever become "ready", so
// the outer loop stalls. Rather than looping forever or failing the whole
// batch, the stalled remainder is appended in its original relative order:
// one of the cyclic nodes will simply fail its own dependency lookup later
// (mbResolveRefProductSysID's clear error), the rest are unaffected.
func kahnTopoSort(order []string, edges []mbcomposition.BatchRefEdge) []string {
	dependsOn := buildDependsOn(order, edges)

	placed := make(map[string]bool, len(order))
	result := make([]string, 0, len(order))

	for len(result) < len(order) {
		var progressed bool
		result, progressed = appendReadyNodes(order, dependsOn, placed, result)
		if !progressed {
			// Cycle among the remaining nodes: fall back to original order for
			// the stalled subset instead of looping forever.
			result = appendStalledInOriginalOrder(order, placed, result)
			break
		}
	}
	return result
}

// buildDependsOn returns, for each id in order, the set of ids that must be
// placed before it. Edges pointing outside order, or self-referencing a
// node, are ignored -- the caller (ListMBRefEdgesForBatch) already restricts
// edges to within-batch pairs, but this stays defensive rather than trusting
// that invariant blindly.
func buildDependsOn(order []string, edges []mbcomposition.BatchRefEdge) map[string]map[string]bool {
	inBatch := make(map[string]bool, len(order))
	for _, id := range order {
		inBatch[id] = true
	}

	dependsOn := make(map[string]map[string]bool, len(order))
	for _, e := range edges {
		if e.MbhID == e.RefMbhID || !inBatch[e.MbhID] || !inBatch[e.RefMbhID] {
			continue
		}
		if dependsOn[e.MbhID] == nil {
			dependsOn[e.MbhID] = make(map[string]bool)
		}
		dependsOn[e.MbhID][e.RefMbhID] = true
	}
	return dependsOn
}

// isReady reports whether all of id's dependencies have already been placed.
func isReady(id string, dependsOn map[string]map[string]bool, placed map[string]bool) bool {
	for dep := range dependsOn[id] {
		if !placed[dep] {
			return false
		}
	}
	return true
}

// appendReadyNodes performs a single Kahn's-algorithm pass: it appends every
// not-yet-placed node whose dependencies are all satisfied, in original
// order (stable tie-breaking), and reports whether any node was placed.
func appendReadyNodes(order []string, dependsOn map[string]map[string]bool, placed map[string]bool, result []string) ([]string, bool) {
	progressed := false
	for _, id := range order {
		if placed[id] {
			continue
		}
		if !isReady(id, dependsOn, placed) {
			continue
		}
		result = append(result, id)
		placed[id] = true
		progressed = true
	}
	return result, progressed
}

// appendStalledInOriginalOrder appends every not-yet-placed node in its
// original relative order. Used as the cycle fallback once a pass makes no
// progress.
func appendStalledInOriginalOrder(order []string, placed map[string]bool, result []string) []string {
	for _, id := range order {
		if !placed[id] {
			result = append(result, id)
			placed[id] = true
		}
	}
	return result
}
