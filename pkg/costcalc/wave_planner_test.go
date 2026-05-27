package costcalc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPlanWaves_SimpleChain(t *testing.T) {
	g := NewDependencyGraph()
	g.AddEdge(2, 1)
	g.AddEdge(3, 2)
	p := PlanWaves(g)
	require.Empty(t, p.Cyclic)
	require.Equal(t, 3, len(p.Waves))
	require.Equal(t, []int64{1}, p.Waves[0].Products)
	require.Equal(t, []int64{2}, p.Waves[1].Products)
	require.Equal(t, []int64{3}, p.Waves[2].Products)
}

func TestPlanWaves_Split(t *testing.T) {
	// Product 1 is the input. Products 2 and 3 are siblings that each depend on 1.
	// Product 4 depends on both 2 and 3 (merge).
	g := NewDependencyGraph()
	g.AddEdge(2, 1)
	g.AddEdge(3, 1)
	g.AddEdge(4, 2)
	g.AddEdge(4, 3)
	p := PlanWaves(g)
	require.Empty(t, p.Cyclic)
	require.Equal(t, 3, len(p.Waves))
	require.Equal(t, []int64{1}, p.Waves[0].Products)
	require.ElementsMatch(t, []int64{2, 3}, p.Waves[1].Products)
	require.Equal(t, []int64{4}, p.Waves[2].Products)
}

func TestPlanWaves_Merge(t *testing.T) {
	// 1 and 2 are independent leaves; 3 merges them; 4 follows 3.
	g := NewDependencyGraph()
	g.AddEdge(3, 1)
	g.AddEdge(3, 2)
	g.AddEdge(4, 3)
	p := PlanWaves(g)
	require.Empty(t, p.Cyclic)
	require.Equal(t, 3, len(p.Waves))
	require.ElementsMatch(t, []int64{1, 2}, p.Waves[0].Products)
	require.Equal(t, []int64{3}, p.Waves[1].Products)
	require.Equal(t, []int64{4}, p.Waves[2].Products)
}

func TestPlanWaves_Cycle(t *testing.T) {
	g := NewDependencyGraph()
	g.AddEdge(1, 2)
	g.AddEdge(2, 1)
	p := PlanWaves(g)
	require.ElementsMatch(t, []int64{1, 2}, p.Cyclic)
	require.Empty(t, p.Waves)
}

func TestPlanWaves_PartialCycle(t *testing.T) {
	// 1 is a clean leaf, 2->3->2 forms a cycle, 4 depends on 1 (clean).
	g := NewDependencyGraph()
	g.AddNode(1)
	g.AddEdge(4, 1)
	g.AddEdge(2, 3)
	g.AddEdge(3, 2)
	p := PlanWaves(g)
	require.ElementsMatch(t, []int64{2, 3}, p.Cyclic)
	require.Equal(t, 2, len(p.Waves))
	require.Equal(t, []int64{1}, p.Waves[0].Products)
	require.Equal(t, []int64{4}, p.Waves[1].Products)
}

func TestPlanWaves_DedupeDuplicateEdges(t *testing.T) {
	// Same upstream referenced twice in the same route (e.g. RM listed at two stages)
	// must not artificially increase in-degree.
	g := NewDependencyGraph()
	g.AddEdge(2, 1)
	g.AddEdge(2, 1)
	p := PlanWaves(g)
	require.Empty(t, p.Cyclic)
	require.Equal(t, 2, len(p.Waves))
}
