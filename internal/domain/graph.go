package domain

// GraphEdge is one line segment of a commit-graph row. Lanes are columns.
// An edge runs from lane From at the row's top (or its middle when FromMid)
// to lane To at the row's middle (or its bottom when ToBottom).
type GraphEdge struct {
	From, To int
	Color    int
	// Upper edges run top→middle; lower edges middle→bottom.
	Lower bool
}

// GraphRow is the drawing input of one row of the log: where the commit's
// node sits, which edges cross the row, and how many lanes it uses.
type GraphRow struct {
	Lane      int
	Color     int
	Edges     []GraphEdge
	Width     int // number of lanes in use, for the column width
	HasParent bool
	IsMerge   bool
}

// GraphLayout lays out the commit graph incrementally: Append can be called
// with each page of commits as it arrives, and the lane state carries over
// so edges never break at page boundaries.
type GraphLayout struct {
	lanes     []string // the commit each lane is waiting for
	colors    []int
	nextColor int
}

// lane finds the first lane waiting for hash, or -1.
func (g *GraphLayout) lane(hash string) int {
	for i, h := range g.lanes {
		if h == hash {
			return i
		}
	}
	return -1
}

func (g *GraphLayout) newColor() int {
	c := g.nextColor
	g.nextColor++
	return c
}

// Append lays out the next commit row.
func (g *GraphLayout) Append(c Commit) GraphRow {
	row := GraphRow{HasParent: len(c.Parents) > 0, IsMerge: len(c.Parents) > 1}
	// Lanes that end in this commit.
	node := -1
	var incoming []int
	for i, h := range g.lanes {
		if h == c.Hash {
			if node < 0 {
				node = i
			}
			incoming = append(incoming, i)
		}
	}
	if node < 0 {
		// A branch tip: take the first free lane or open a new one.
		for i, h := range g.lanes {
			if h == "" {
				node = i
				break
			}
		}
		if node < 0 {
			node = len(g.lanes)
			g.lanes = append(g.lanes, "")
			g.colors = append(g.colors, 0)
		}
		g.colors[node] = g.newColor()
	}
	row.Lane = node
	row.Color = g.colors[node]

	// Upper half: every live lane continues to the middle, the ones waiting
	// for this commit converge into its node.
	for i, h := range g.lanes {
		if h == "" {
			continue
		}
		to := i
		if h == c.Hash {
			to = node
		}
		row.Edges = append(row.Edges, GraphEdge{From: i, To: to, Color: g.colors[i]})
	}
	for _, i := range incoming {
		if i != node {
			g.lanes[i] = ""
		}
	}

	// The node's lane continues to its first parent; further parents of a
	// merge reuse a lane already waiting for them or open one.
	type out struct{ lane, color int }
	var outs []out
	if len(c.Parents) == 0 {
		g.lanes[node] = ""
	} else {
		g.lanes[node] = c.Parents[0]
		outs = append(outs, out{node, g.colors[node]})
		for _, p := range c.Parents[1:] {
			if j := g.lane(p); j >= 0 && j != node {
				outs = append(outs, out{j, g.colors[j]})
				continue
			}
			j := -1
			for k := node + 1; k < len(g.lanes); k++ {
				if g.lanes[k] == "" {
					j = k
					break
				}
			}
			if j < 0 {
				j = len(g.lanes)
				g.lanes = append(g.lanes, "")
				g.colors = append(g.colors, 0)
			}
			g.lanes[j] = p
			g.colors[j] = g.newColor()
			outs = append(outs, out{j, g.colors[j]})
		}
	}

	// Compact empty lanes so the graph does not grow wider forever; the
	// lower half carries every live lane to its new column.
	remap := make([]int, len(g.lanes))
	var lanes []string
	var colors []int
	for i, h := range g.lanes {
		remap[i] = -1
		if h != "" {
			remap[i] = len(lanes)
			lanes = append(lanes, h)
			colors = append(colors, g.colors[i])
		}
	}
	width := len(g.lanes)
	for _, o := range outs {
		row.Edges = append(row.Edges, GraphEdge{From: node, To: remap[o.lane], Color: o.color, Lower: true})
	}
	for i, h := range g.lanes {
		if h == "" || i == node {
			continue
		}
		isOut := false
		for _, o := range outs {
			if o.lane == i {
				isOut = true
			}
		}
		if isOut && !wasLive(row.Edges, i) {
			continue
		}
		if wasLive(row.Edges, i) {
			row.Edges = append(row.Edges, GraphEdge{From: i, To: remap[i], Color: g.colors[i], Lower: true})
		}
	}
	g.lanes, g.colors = lanes, colors
	if len(lanes) > width {
		width = len(lanes)
	}
	row.Width = width
	return row
}

// wasLive reports a lane that reached this row's middle through the upper
// half and therefore continues downward.
func wasLive(edges []GraphEdge, lane int) bool {
	for _, e := range edges {
		if !e.Lower && e.To == lane && e.From == lane {
			return true
		}
	}
	return false
}
