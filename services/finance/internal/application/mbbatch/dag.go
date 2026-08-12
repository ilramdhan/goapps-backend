package mbbatch

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// BuildDAG resolves the topo order in which VALIDATED MB Heads must be computed so that a
// parent MB's composition read of a nested child MB's cost always finds that child's
// cst_product_cost already written in this same batch run (design doc §10.3 step order,
// deepest nested dependency first). Returns an error wrapping costcalcdom.ErrCycleDetected if
// the MB-to-MB composition graph contains a cycle (PRD §8.4 explicit requirement — must fail
// the job with a clear error, not infinite-loop or silently pick an arbitrary order).
func BuildDAG(ctx context.Context, headReader MBHeadReader, edgeReader MBEdgeReader) ([]MBHeadCandidate, error) {
	candidates, err := headReader.ListValidated(ctx)
	if err != nil {
		return nil, fmt.Errorf("build mb dag: list validated: %w", err)
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	byID := make(map[string]MBHeadCandidate, len(candidates))
	mbhIDs := make([]string, len(candidates))
	versions := make([]int32, len(candidates))
	for i, c := range candidates {
		byID[c.MBHID] = c
		mbhIDs[i] = c.MBHID
		versions[i] = c.CurrentVersion
	}

	edges, err := edgeReader.ListMBEdgesBulk(ctx, mbhIDs, versions)
	if err != nil {
		return nil, fmt.Errorf("build mb dag: list edges: %w", err)
	}

	return topoSortMBHeads(candidates, byID, edges)
}

// topoSortMBHeads performs a Kahn's-algorithm topological sort, modeled stylistically on
// costcalc.topoSortFormulas: an edge MBHID -> RefMBHID means MBHID's composition depends on
// RefMBHID's cost, so RefMBHID must be visited (computed) first. Edges pointing at an MBHID
// outside the candidate set are ignored (that dependency is external to this batch run, e.g.
// the referenced MB is not itself VALIDATED).
func topoSortMBHeads(candidates []MBHeadCandidate, byID map[string]MBHeadCandidate, edges []MBEdge) ([]MBHeadCandidate, error) { //nolint:gocognit // Kahn topological sort, cohesive
	inDegree := make(map[string]int, len(candidates))
	adj := make(map[string][]string, len(candidates))
	for _, c := range candidates {
		inDegree[c.MBHID] = 0
	}
	seenEdge := make(map[[2]string]struct{}, len(edges))
	for _, e := range edges {
		if _, ok := byID[e.RefMBHID]; !ok {
			// The referenced child MB Head is not VALIDATED, so it is not a candidate and
			// cannot be computed in this run. Say so once per edge: the parent will fall
			// back to the child's previously committed cost (see warnUnscheduledChildren).
			log.Warn().
				Str("parent_mbh_id", e.MBHID).
				Str("parent_mb_code", byID[e.MBHID].Code).
				Str("child_mbh_id", e.RefMBHID).
				Msg("mb dag: nested MB reference skipped, referenced MB Head is not VALIDATED and will not be recomputed in this batch")
			continue
		}
		if e.MBHID == e.RefMBHID {
			continue
		}
		key := [2]string{e.MBHID, e.RefMBHID}
		if _, dup := seenEdge[key]; dup {
			continue
		}
		seenEdge[key] = struct{}{}
		adj[e.RefMBHID] = append(adj[e.RefMBHID], e.MBHID)
		inDegree[e.MBHID]++
	}

	queue := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if inDegree[c.MBHID] == 0 {
			queue = append(queue, c.MBHID)
		}
	}

	sorted := make([]MBHeadCandidate, 0, len(candidates))
	visited := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		visited++
		sorted = append(sorted, byID[id])
		for _, next := range adj[id] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if visited != len(candidates) {
		return nil, fmt.Errorf("topoSortMBHeads: %w", costcalcdom.ErrCycleDetected)
	}
	return sorted, nil
}
