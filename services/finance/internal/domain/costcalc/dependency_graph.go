package costcalc

import "slices"

// DependencyGraph models product-level dependencies.
// edges[d] = list of upstream products that d depends on (i.e. d's RMs that are products).
type DependencyGraph struct {
	edges map[int64][]int64
	nodes map[int64]struct{}
}

// NewDependencyGraph constructs an empty graph.
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		edges: map[int64][]int64{},
		nodes: map[int64]struct{}{},
	}
}

// AddNode registers a node (idempotent).
func (g *DependencyGraph) AddNode(id int64) { g.nodes[id] = struct{}{} }

// AddEdge records that downstream depends on upstream. Both nodes are added.
func (g *DependencyGraph) AddEdge(downstream, upstream int64) {
	g.AddNode(downstream)
	g.AddNode(upstream)
	g.edges[downstream] = append(g.edges[downstream], upstream)
}

// Nodes returns all known nodes sorted ascending.
func (g *DependencyGraph) Nodes() []int64 {
	out := make([]int64, 0, len(g.nodes))
	for id := range g.nodes {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

// Upstream returns the upstream dependencies of a node (may contain duplicates).
func (g *DependencyGraph) Upstream(id int64) []int64 { return g.edges[id] }

// HasNode reports whether the node exists in the graph.
func (g *DependencyGraph) HasNode(id int64) bool { _, ok := g.nodes[id]; return ok }
