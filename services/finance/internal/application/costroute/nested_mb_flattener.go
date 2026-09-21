// Package costroute implements the CostRoute application use cases.
package costroute

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// maxNestDepth caps NestedMBFlattener.Flatten's recursion as defense-in-depth
// against a cyclic MB reference that visited somehow fails to catch.
const maxNestDepth = 8

// NestedMBGraphLoader loads a product's own route graph for flattening.
type NestedMBGraphLoader interface {
	// GetLatestGraphByProduct returns the most-recent non-deleted route graph
	// for productSysID regardless of routing status (unlike GetActiveByProduct,
	// which excludes LOCKED heads — a nested MB's route is normally LOCKED).
	GetLatestGraphByProduct(ctx context.Context, productSysID int64) (*costroute.Graph, error)
}

// MBProductChecker answers which product IDs are MB-typed, in bulk.
type MBProductChecker interface {
	MBProductIDs(ctx context.Context, productSysIDs []int64) (map[int64]bool, error)
}

// NestedMBFlattener recursively splices a nested MB's own route composition
// into a base route graph for display purposes only. It never mutates the
// graphs it reads and is never invoked on the SaveGraph (editor) path — see
// GetGraphHandler.WithNestedMBFlattener.
type NestedMBFlattener struct {
	loader  NestedMBGraphLoader
	checker MBProductChecker
}

// NewNestedMBFlattener constructs a NestedMBFlattener.
func NewNestedMBFlattener(loader NestedMBGraphLoader, checker MBProductChecker) *NestedMBFlattener {
	return &NestedMBFlattener{loader: loader, checker: checker}
}

// flattenNode is one PRODUCT-type Rm row pending a BFS visit, carrying the
// chain context needed to stamp origin/ratio/depth on whatever gets spliced
// in for it.
type flattenNode struct {
	rm              *costroute.Rm
	cumulativeRatio float64
	depth           int32
}

// Flatten returns a new *Graph with every nested-MB PRODUCT reference in base
// recursively spliced in, annotated with OriginHeadID/EffectiveRatio/NestDepth/
// OriginProductCode/OriginProductName. base itself is never mutated.
//
// Every PRODUCT-type Rm row is left exactly as in base except for
// EffectiveRatio (defaulted to RouteRmRatio) — flattening only ever ADDS seqs
// and rms past what base already contains, so a caller diffing on IDs cannot
// mistake spliced content for something to persist.
func (f *NestedMBFlattener) Flatten(ctx context.Context, base *costroute.Graph) (*costroute.Graph, error) {
	out := &costroute.Graph{
		Head: base.Head,
		Seqs: cloneSeqsShallow(base.Seqs),
	}

	maxLevel := maxRouteLevel(out.Seqs)
	visited := map[int64]bool{base.Head.ProductSysID: true}

	// BFS level by level so MBProductIDs can be bulk-checked once per level
	// instead of once per row.
	level := seedQueue(out.Seqs)
	for len(level) > 0 && level[0].depth <= maxNestDepth {
		nextLevel, addedSeqs, err := f.expandLevel(ctx, level, visited, &maxLevel)
		if err != nil {
			return nil, err
		}
		out.Seqs = append(out.Seqs, addedSeqs...)
		level = nextLevel
	}
	return out, nil
}

// expandLevel resolves every MB-referencing PRODUCT row in level (bulk MB
// check, one query), loads each referenced MB's own graph, splices it into
// new Seq/Rm copies stamped with origin/ratio/depth, and returns the next
// level's worklist plus the seqs that were added.
func (f *NestedMBFlattener) expandLevel(
	ctx context.Context, level []flattenNode, visited map[int64]bool, maxLevel *int32,
) ([]flattenNode, []*costroute.Seq, error) {
	candidateIDs := make([]int64, 0, len(level))
	for _, n := range level {
		if !visited[n.rm.RmProductSysID] {
			candidateIDs = append(candidateIDs, n.rm.RmProductSysID)
		}
	}
	mbSet, err := f.checker.MBProductIDs(ctx, candidateIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("nested mb flattener: check mb product ids: %w", err)
	}

	var nextLevel []flattenNode
	var addedSeqs []*costroute.Seq
	for _, n := range level {
		pid := n.rm.RmProductSysID
		if visited[pid] || !mbSet[pid] {
			continue
		}
		visited[pid] = true

		nested, loadErr := f.loader.GetLatestGraphByProduct(ctx, pid)
		if loadErr != nil {
			// A nested MB with no route yet (or any load failure) degrades
			// gracefully: leave this row un-flattened, matching today's
			// behavior, rather than failing the whole graph fetch.
			continue
		}

		spliced, children := f.spliceGraph(nested, n, maxLevel)
		addedSeqs = append(addedSeqs, spliced...)
		nextLevel = append(nextLevel, children...)
	}
	return nextLevel, addedSeqs, nil
}

// spliceGraph copies nested's seqs/rms with origin/ratio/depth stamped for
// node's chain, renumbers their RouteLevel past *maxLevel so they render as
// further-upstream columns, and returns both the new seqs and the next BFS
// level's PRODUCT-row worklist.
func (f *NestedMBFlattener) spliceGraph(
	nested *costroute.Graph, node flattenNode, maxLevel *int32,
) ([]*costroute.Seq, []flattenNode) {
	depth := node.depth + 1
	levelOffset := *maxLevel - minRouteLevel(nested.Seqs) + 1

	newSeqs := make([]*costroute.Seq, 0, len(nested.Seqs))
	var children []flattenNode
	for _, s := range nested.Seqs {
		seq := cloneSeq(s)
		seq.RouteLevel += levelOffset
		seq.OriginHeadID = nested.Head.HeadID
		seq.OriginProductCode = nested.Head.ProductCode
		seq.NestDepth = depth
		if seq.RouteLevel > *maxLevel {
			*maxLevel = seq.RouteLevel
		}

		seq.Rms = make([]*costroute.Rm, 0, len(s.Rms))
		for _, rm := range s.Rms {
			clone := cloneRm(rm)
			clone.OriginHeadID = nested.Head.HeadID
			clone.OriginProductCode = nested.Head.ProductCode
			clone.OriginProductName = nested.Head.ProductName
			clone.NestDepth = depth
			clone.EffectiveRatio = node.cumulativeRatio * clone.RouteRmRatio
			seq.Rms = append(seq.Rms, clone)

			if clone.RmType == costroute.RmTypeProduct {
				children = append(children, flattenNode{
					rm:              clone,
					cumulativeRatio: clone.EffectiveRatio,
					depth:           depth,
				})
			}
		}
		newSeqs = append(newSeqs, seq)
	}
	return newSeqs, children
}

// seedQueue builds the initial BFS worklist from every PRODUCT-type Rm row
// already present in seqs, with EffectiveRatio defaulted to RouteRmRatio for
// rows that will not turn out to reference an MB product.
func seedQueue(seqs []*costroute.Seq) []flattenNode {
	var out []flattenNode
	for _, s := range seqs {
		for _, rm := range s.Rms {
			if rm.EffectiveRatio == 0 {
				rm.EffectiveRatio = rm.RouteRmRatio
			}
			if rm.RmType == costroute.RmTypeProduct {
				out = append(out, flattenNode{rm: rm, cumulativeRatio: rm.EffectiveRatio, depth: 0})
			}
		}
	}
	return out
}

func maxRouteLevel(seqs []*costroute.Seq) int32 {
	var maxLvl int32
	for _, s := range seqs {
		if s.RouteLevel > maxLvl {
			maxLvl = s.RouteLevel
		}
	}
	return maxLvl
}

func minRouteLevel(seqs []*costroute.Seq) int32 {
	if len(seqs) == 0 {
		return 0
	}
	minLvl := seqs[0].RouteLevel
	for _, s := range seqs[1:] {
		if s.RouteLevel < minLvl {
			minLvl = s.RouteLevel
		}
	}
	return minLvl
}

func cloneSeqsShallow(seqs []*costroute.Seq) []*costroute.Seq {
	out := make([]*costroute.Seq, len(seqs))
	for i, s := range seqs {
		out[i] = cloneSeq(s)
		out[i].Rms = make([]*costroute.Rm, len(s.Rms))
		for j, rm := range s.Rms {
			out[i].Rms[j] = cloneRm(rm)
		}
	}
	return out
}

func cloneSeq(s *costroute.Seq) *costroute.Seq {
	c := *s
	c.Rms = nil
	return &c
}

func cloneRm(rm *costroute.Rm) *costroute.Rm {
	c := *rm
	return &c
}
