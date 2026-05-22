package costcalc

import "slices"

// Wave is a single layer of the topological plan.
type Wave struct {
	Number   int
	Products []int64
}

// WavePlan is the orchestration plan: ordered list of waves + any cyclic products.
type WavePlan struct {
	Waves  []Wave
	Cyclic []int64
}

// PlanWaves performs Kahn's algorithm on the graph.
// Wave 0 contains nodes with no upstream dependencies (deepest leaves).
// Subsequent waves contain nodes whose upstream dependencies were all in earlier waves.
// Any nodes participating in a cycle end up in Cyclic.
func PlanWaves(g *DependencyGraph) *WavePlan {
	allNodes := g.Nodes()
	inDeg := make(map[int64]int, len(allNodes))
	// Reverse adjacency: for each upstream U, list of downstream nodes that depend on it.
	rev := map[int64][]int64{}

	// Compute unique edges (dedupe -- same RM may be referenced multiple times in a route)
	seenEdge := map[[2]int64]struct{}{}
	for _, n := range allNodes {
		inDeg[n] = 0
	}
	for _, n := range allNodes {
		for _, u := range g.Upstream(n) {
			key := [2]int64{n, u}
			if _, dup := seenEdge[key]; dup {
				continue
			}
			seenEdge[key] = struct{}{}
			inDeg[n]++
			rev[u] = append(rev[u], n)
		}
	}

	// Initial wave: nodes with zero in-degree
	current := []int64{}
	for n, d := range inDeg {
		if d == 0 {
			current = append(current, n)
		}
	}
	slices.Sort(current)

	plan := &WavePlan{}
	waveNo := 0
	visited := map[int64]struct{}{}
	for len(current) > 0 {
		for _, n := range current {
			visited[n] = struct{}{}
		}
		plan.Waves = append(plan.Waves, Wave{Number: waveNo, Products: current})

		next := []int64{}
		for _, n := range current {
			for _, d := range rev[n] {
				inDeg[d]--
				if inDeg[d] == 0 {
					next = append(next, d)
				}
			}
		}
		slices.Sort(next)
		current = next
		waveNo++
	}

	// Anything not visited participates in a cycle
	for _, n := range allNodes {
		if _, ok := visited[n]; !ok {
			plan.Cyclic = append(plan.Cyclic, n)
		}
	}
	slices.Sort(plan.Cyclic)
	return plan
}
