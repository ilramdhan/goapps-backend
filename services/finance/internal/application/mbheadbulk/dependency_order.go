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
	inBatch := make(map[string]bool, len(order))
	for _, id := range order {
		inBatch[id] = true
	}

	// dependsOn[id] is the set of ids that must be placed before id.
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

	placed := make(map[string]bool, len(order))
	result := make([]string, 0, len(order))

	for len(result) < len(order) {
		progressed := false
		for _, id := range order {
			if placed[id] {
				continue
			}
			ready := true
			for dep := range dependsOn[id] {
				if !placed[dep] {
					ready = false
					break
				}
			}
			if !ready {
				continue
			}
			result = append(result, id)
			placed[id] = true
			progressed = true
		}
		if !progressed {
			// Cycle among the remaining nodes: fall back to original order for
			// the stalled subset instead of looping forever.
			for _, id := range order {
				if !placed[id] {
					result = append(result, id)
					placed[id] = true
				}
			}
			break
		}
	}
	return result
}
