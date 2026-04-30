package flowgraph

import "github.com/tamnd/gocopy/v1/ir"

// DomTree is the immediate-dominator map for a CFG plus a
// post-order traversal used by reverse-post-order iteration.
type DomTree struct {
	Idom      map[uint32]uint32 // block ID → immediate dominator block ID
	PostOrder []*ir.Block
}

// undefined is the sentinel used by Cooper-Harvey-Kennedy's
// dominator algorithm before a block's idom has been set.
const undefined uint32 = ^uint32(0)

// Dominators computes the immediate-dominator tree using the
// engineered Cooper-Harvey-Kennedy iterative algorithm. The graphs
// gocopy produces are tiny (rarely more than ~50 blocks per
// function), so the simpler iterative form beats Lengauer-Tarjan in
// constant factors and clarity.
//
// Reference: "A Simple, Fast Dominance Algorithm",
// Cooper, Harvey, Kennedy 2001.
func Dominators(g *CFG) *DomTree {
	if g == nil || len(g.Blocks) == 0 {
		return &DomTree{Idom: map[uint32]uint32{}}
	}

	postorder := postOrder(g)
	rpo := make([]*ir.Block, len(postorder))
	for i, b := range postorder {
		rpo[len(postorder)-1-i] = b
	}
	rpoIndex := map[uint32]int{}
	for i, b := range rpo {
		rpoIndex[b.ID] = i
	}

	idom := map[uint32]uint32{}
	for _, b := range rpo {
		idom[b.ID] = undefined
	}
	idom[g.Entry.ID] = g.Entry.ID

	preds := predecessors(g)

	changed := true
	for changed {
		changed = false
		for _, b := range rpo[1:] {
			ps := preds[b.ID]
			if len(ps) == 0 {
				continue
			}
			var newIdom uint32 = undefined
			for _, p := range ps {
				// Skip predecessors that are unreachable from
				// Entry — they have no idom and would derail the
				// intersect walk.
				if _, ok := rpoIndex[p]; !ok {
					continue
				}
				if idom[p] == undefined {
					continue
				}
				if newIdom == undefined {
					newIdom = p
					continue
				}
				newIdom = intersect(newIdom, p, idom, rpoIndex)
			}
			if newIdom != undefined && idom[b.ID] != newIdom {
				idom[b.ID] = newIdom
				changed = true
			}
		}
	}

	// Convert the entry's self-dominance back to the conventional
	// "entry has no immediate dominator" representation: leave it
	// pointing at itself so downstream consumers can detect the
	// root by `idom[id] == id`.

	return &DomTree{Idom: idom, PostOrder: postorder}
}

// intersect walks up the idom chain from b1 and b2 until they meet,
// using rpoIndex to compare positions (deeper RPO index = further
// from entry).
func intersect(b1, b2 uint32, idom map[uint32]uint32, rpoIndex map[uint32]int) uint32 {
	for b1 != b2 {
		for rpoIndex[b1] > rpoIndex[b2] {
			b1 = idom[b1]
		}
		for rpoIndex[b2] > rpoIndex[b1] {
			b2 = idom[b2]
		}
	}
	return b1
}

// postOrder returns the CFG's blocks in DFS post-order starting at
// Entry.
func postOrder(g *CFG) []*ir.Block {
	visited := map[uint32]bool{}
	var out []*ir.Block
	var visit func(*ir.Block)
	visit = func(b *ir.Block) {
		if visited[b.ID] {
			return
		}
		visited[b.ID] = true
		for _, edge := range g.Edges[b.ID] {
			if edge.Dest != nil {
				visit(edge.Dest)
			}
		}
		out = append(out, b)
	}
	visit(g.Entry)
	return out
}

// predecessors inverts the CFG's edge map.
func predecessors(g *CFG) map[uint32][]uint32 {
	out := map[uint32][]uint32{}
	for srcID, edges := range g.Edges {
		for _, e := range edges {
			if e.Dest != nil {
				out[e.Dest.ID] = append(out[e.Dest.ID], srcID)
			}
		}
	}
	return out
}
