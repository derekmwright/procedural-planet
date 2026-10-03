package planet

import (
	"math"
	"sort"
)

const Segments = 32
const MaxLeaves = 960

func (k Patch) Children() [4]Patch {
	return [4]Patch{{k.Face, k.Level + 1, k.X * 2, k.Y * 2}, {k.Face, k.Level + 1, k.X*2 + 1, k.Y * 2}, {k.Face, k.Level + 1, k.X * 2, k.Y*2 + 1}, {k.Face, k.Level + 1, k.X*2 + 1, k.Y*2 + 1}}
}
func (k Patch) Parent() Patch { return Patch{k.Face, k.Level - 1, k.X / 2, k.Y / 2} }
func (k Patch) Bounds() (u, v, span float64) {
	n := math.Exp2(float64(k.Level))
	return float64(k.X)*2/n - 1, float64(k.Y)*2/n - 1, 2 / n
}

// Coordinates maps a direction back onto the dominant cube face.
func Coordinates(d Vec) (face int, u, v float64) {
	best := math.Inf(-1)
	for i, b := range bases {
		if s := d.Dot(b[0]); s > best {
			face = i
			best = s
		}
	}
	b := bases[face]
	return face, d.Dot(b[1]) / best, d.Dot(b[2]) / best
}

type LOD struct {
	Leaves   map[Patch]bool
	MaxLevel int
}

func NewLOD(radius float64) *LOD {
	l := &LOD{Leaves: make(map[Patch]bool), MaxLevel: int(math.Ceil(math.Log2(2 * radius / (Segments * 0.5))))}
	for f := 0; f < 6; f++ {
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				l.Leaves[Patch{f, 2, x, y}] = true
			}
		}
	}
	return l
}
func less(a, b Patch) bool {
	if a.Level != b.Level {
		return a.Level < b.Level
	}
	if a.Face != b.Face {
		return a.Face < b.Face
	}
	if a.Y != b.Y {
		return a.Y < b.Y
	}
	return a.X < b.X
}
func (l *LOD) SortedLeaves() []Patch {
	out := make([]Patch, 0, len(l.Leaves))
	for k := range l.Leaves {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return less(out[i], out[j]) })
	return out
}
func (l *LOD) At(d Vec) Patch {
	f, u, v := Coordinates(d)
	for level := 2; level <= l.MaxLevel; level++ {
		n := int(1) << level
		x := min(n-1, max(0, int((u+1)*0.5*float64(n))))
		y := min(n-1, max(0, int((v+1)*0.5*float64(n))))
		k := Patch{f, level, x, y}
		if l.Leaves[k] {
			return k
		}
	}
	panic("LOD coverage missing")
}

// Two probes per edge catch both halves of a finer neighbour. Extending the
// cube-face coordinates crosses face boundaries without a special seam table.
func (l *LOD) neighbours(k Patch) []Patch {
	u, v, s := k.Bounds()
	e := s * 1e-6
	out := make([]Patch, 0, 8)
	for _, t := range []float64{0.25, 0.75} {
		for _, uv := range [][2]float64{{u - e, v + s*t}, {u + s + e, v + s*t}, {u + s*t, v - e}, {u + s*t, v + s + e}} {
			out = append(out, l.At(Direction(k.Face, uv[0], uv[1])))
		}
	}
	return out
}
func (l *LOD) CanSplit(k Patch) bool {
	if !l.Leaves[k] || k.Level >= l.MaxLevel || len(l.Leaves)+3 > MaxLeaves {
		return false
	}
	for _, n := range l.neighbours(k) {
		if n.Level < k.Level {
			return false
		}
	}
	return true
}
func (l *LOD) Split(k Patch) bool {
	if !l.CanSplit(k) {
		return false
	}
	delete(l.Leaves, k)
	for _, c := range k.Children() {
		l.Leaves[c] = true
	}
	return true
}
func (l *LOD) CanMerge(k Patch) bool {
	if k.Level < 2 {
		return false
	}
	for _, c := range k.Children() {
		if !l.Leaves[c] {
			return false
		}
		for _, n := range l.neighbours(c) {
			if n.Level > c.Level {
				return false
			}
		}
	}
	return true
}
func (l *LOD) Merge(k Patch) bool {
	if !l.CanMerge(k) {
		return false
	}
	for _, c := range k.Children() {
		delete(l.Leaves, c)
	}
	l.Leaves[k] = true
	return true
}
func (p Planet) DetailScore(k Patch, eye Vec) float64 {
	u, v, s := k.Bounds()
	b := bases[k.Face]
	den := eye.Dot(b[0])
	var d Vec
	if den <= 0 {
		d = Direction(k.Face, u+s/2, v+s/2)
	} else {
		d = Direction(k.Face, math.Max(u, math.Min(u+s, eye.Dot(b[1])/den)), math.Max(v, math.Min(v+s, eye.Dot(b[2])/den)))
	}
	delta := eye.Sub(p.Surface(d))
	distance := math.Sqrt(delta.Dot(delta))
	return p.Radius * s / math.Max(distance, 1)
}
func (l *LOD) NextSplit(p Planet, eye Vec) (Patch, bool) {
	bestScore := 2.2
	var best Patch
	found := false
	for _, k := range l.SortedLeaves() {
		if k.Level >= l.MaxLevel {
			continue
		}
		score := p.DetailScore(k, eye)
		if score > bestScore {
			best = k
			bestScore = score
			found = true
		}
	}
	if !found || len(l.Leaves)+3 > MaxLeaves {
		return Patch{}, false
	}
	// Refine a coarse neighbour first to maintain 2:1 balance at every commit.
	for {
		blocker := best
		for _, n := range l.neighbours(best) {
			if n.Level < blocker.Level || (n.Level == blocker.Level && n.Level < best.Level && less(n, blocker)) {
				blocker = n
			}
		}
		if blocker == best {
			break
		}
		best = blocker
	}
	return best, l.CanSplit(best)
}
func (l *LOD) NextMerge(p Planet, eye Vec) (Patch, bool) {
	// Propagate refinement demand through the whole chain of coarse neighbours,
	// including neighbours whose own distance would not require refinement.
	required := make(map[Patch]bool)
	var queue []Patch
	for k := range l.Leaves {
		if k.Level < l.MaxLevel && p.DetailScore(k, eye) > 2.2 {
			required[k] = true
			queue = append(queue, k)
		}
	}
	for len(queue) > 0 {
		k := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		for _, n := range l.neighbours(k) {
			if n.Level < k.Level && !required[n] {
				required[n] = true
				queue = append(queue, n)
			}
		}
	}
	for _, c := range l.SortedLeaves() {
		k := c.Parent()
		if k.Level >= 2 && p.DetailScore(k, eye) < 1.45 && l.CanMerge(k) {
			// A neighbour that still needs to split may have forced these
			// children into existence. Keep them until that demand is gone;
			// otherwise split/merge can oscillate before the neighbour is ready.
			needed := false
			for _, child := range k.Children() {
				if required[child] {
					needed = true
				}
				for _, n := range l.neighbours(child) {
					if n.Level == child.Level && required[n] {
						needed = true
					}
				}
			}
			if needed {
				continue
			}
			return k, true
		}
	}
	return Patch{}, false
}

// MeshElevation intersects the radial ray with the exact triangle used by
// BuildPatch. Collision must follow the visible LOD while refinement is pending.
func (p Planet) MeshElevation(k Patch, segments int, d Vec) float64 {
	b := bases[k.Face]
	den := d.Dot(b[0])
	u, v, s := k.Bounds()
	x := math.Max(0, math.Min(float64(segments), (d.Dot(b[1])/den-u)/s*float64(segments)))
	y := math.Max(0, math.Min(float64(segments), (d.Dot(b[2])/den-v)/s*float64(segments)))
	ix, iy := min(int(x), segments-1), min(int(y), segments-1)
	point := func(dx, dy int) Vec {
		return p.Surface(Direction(k.Face, u+s*float64(ix+dx)/float64(segments), v+s*float64(iy+dy)/float64(segments)))
	}
	a, c, bp := point(0, 0), point(0, 1), point(1, 0)
	if x-float64(ix)+y-float64(iy) > 1 {
		a = point(1, 1)
	}
	n := c.Sub(a).Cross(bp.Sub(a))
	return n.Dot(a)/n.Dot(d) - p.Radius
}

// BuildTerrainPatch adds inward skirts to conceal the T-junctions between
// balanced LODs. This is a coverage technique, not watertight edge stitching.
func (p Planet) BuildTerrainPatch(k Patch) Mesh {
	m := p.BuildPatch(k, Segments)
	_, _, span := k.Bounds()
	cell := p.Radius * span / Segments
	depth := math.Max(1, cell*2)
	edges := make([][]uint32, 4)
	for i := 0; i <= Segments; i++ {
		edges[0] = append(edges[0], uint32(i))
		edges[1] = append(edges[1], uint32(i*(Segments+1)+Segments))
		edges[2] = append(edges[2], uint32(Segments*(Segments+1)+Segments-i))
		edges[3] = append(edges[3], uint32((Segments-i)*(Segments+1)))
	}
	for _, edge := range edges {
		start := uint32(len(m.Vertices))
		for _, i := range edge {
			v := m.Vertices[i]
			d := v.Position.Add(m.Origin).Unit()
			v.Position = v.Position.Sub(d.Mul(depth))
			m.Vertices = append(m.Vertices, v)
		}
		for i := 0; i < Segments; i++ {
			a, b, c, d := edge[i], edge[i+1], start+uint32(i), start+uint32(i+1)
			m.Indices = append(m.Indices, a, b, c, b, d, c, a, c, b, b, c, d)
		}
	}
	return m
}
