package planet

import (
	"fmt"
	"math"
	"runtime"
	"sync"
)

// ErosionSettings controls a cached, detachment-limited stream-power solve.
// It changes bedrock relief; it does not simulate water or sediment transport.
type ErosionSettings struct {
	SeaLevel, Strength float64
	Resolution         int // 0 selects 512; powers of two from 16 through 512
}

type ErosionStats struct {
	Nodes, DrainageSamples, Resolution, Iterations int
	CacheBytes                                     int // retained height/preview slice storage; excludes startup graph
	Strength, SeaLevel, MaximumCut                 float64
}

type erosionNode struct {
	direction                            Vec
	original, height, filled, rain, area float64
	down                                 int32
	neighbors                            [8]int32
	count                                int
}

type erosionPreview struct {
	direction, downstream Vec
	cut                   float64
}

type erosionField struct {
	seed                  uint64
	radius, sea, strength float64
	resolution, stride    int
	values                []float32 // six faces with one ghost sample on each side
	previews              []erosionPreview
	stats                 ErosionStats
}

func finiteErosion(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// WithErosion solves the whole closed planet before publishing an immutable
// cache. Streamed meshes, ground queries and materials all sample this field.
func (p Planet) WithErosion(s ErosionSettings) (Planet, error) {
	if s.Resolution == 0 {
		s.Resolution = 512
	}
	if !finiteErosion(p.Radius) || p.Radius <= 0 || !finiteErosion(s.SeaLevel) || !finiteErosion(s.Strength) || s.Strength < 0 || s.Strength > 2 || s.Resolution < 16 || s.Resolution > 512 || s.Resolution&(s.Resolution-1) != 0 {
		return p, fmt.Errorf("erosion requires a positive finite radius, finite sea level, strength 0..2 and power-of-two resolution 16..512")
	}
	p.erosion = nil
	if s.Strength == 0 {
		return p, nil
	}
	nodes, ids := erosionGrid(s.Resolution, p.Radius)
	sampleErosionBase(nodes, p)
	const iterations = 8
	var order []int32
	for step := 0; step < iterations; step++ {
		if step%2 == 0 {
			order = erosionDrainage(nodes, s.SeaLevel, p.Radius)
		}
		inciseTerrain(nodes, order, s.SeaLevel, p.Radius, s.Strength)
		relaxErosionSlopes(nodes, s.SeaLevel, p.Radius, s.Strength)
	}
	f := &erosionField{seed: p.Seed, radius: p.Radius, sea: s.SeaLevel, strength: s.Strength, resolution: s.Resolution, stride: s.Resolution + 3}
	f.values = make([]float32, 6*f.stride*f.stride)
	f.stats = ErosionStats{Nodes: len(nodes), Resolution: s.Resolution, Iterations: iterations, CacheBytes: len(f.values) * 4, Strength: s.Strength, SeaLevel: s.SeaLevel}
	minimumArea := 12 * 4 * math.Pi * p.Radius * p.Radius / float64(len(nodes))
	for _, n := range nodes {
		cut := n.original - n.height
		f.stats.MaximumCut = math.Max(f.stats.MaximumCut, cut)
		if n.down >= 0 && n.area >= minimumArea && n.original > s.SeaLevel+200 && cut > 40 {
			f.stats.DrainageSamples++
			delta := nodes[n.down].direction.Sub(n.direction)
			f.previews = append(f.previews, erosionPreview{n.direction, delta.Sub(n.direction.Mul(delta.Dot(n.direction))).Unit(), cut})
		}
	}
	// Each preview contains seven float64s. Include spare slice capacity too.
	f.stats.CacheBytes += cap(f.previews) * 7 * 8
	n := s.Resolution
	for face := 0; face < 6; face++ {
		for y := 0; y <= n; y++ {
			for x := 0; x <= n; x++ {
				v := nodes[ids[(face*(n+1)+y)*(n+1)+x]]
				f.values[f.index(face, x, y)] = float32(v.original - v.height)
			}
		}
	}
	f.fillGhosts()
	p.erosion = f
	return p, nil
}

// n=1 stream power admits an implicit upstream update using the already
// updated downstream height (Braun & Willett 2013). Its coefficient depends
// on sqrt(catchment area)/edge length, not on an arbitrary channel width.
func inciseTerrain(nodes []erosionNode, order []int32, sea, radius, strength float64) {
	for _, id := range order {
		n := &nodes[id]
		if n.down < 0 || n.height <= sea {
			continue
		}
		d := &nodes[n.down]
		if d.height >= n.height {
			continue
		}
		delta := n.direction.Sub(d.direction)
		length := radius * math.Sqrt(delta.Dot(delta))
		activity := .2 + .8*(1-math.Exp(-n.area/(4*n.rain)))
		coast := smooth(math.Min(1, (n.original-sea)/500))
		alpha := .20 * strength * activity * coast * math.Sqrt(n.area) / length
		n.height = math.Max(erosionFloor(n.original, sea, strength), (n.height+alpha*d.height)/(1+alpha))
	}
}

func erosionFloor(original, sea, strength float64) float64 {
	return math.Max(original-1200*strength, sea+(original-sea)*.2)
}

// Weather steep newly exposed valley shoulders. This removes rock rather
// than depositing it on the bed; sediment accounting is intentionally absent.
// Read one snapshot so sweep direction does not imprint onto the landscape.
func relaxErosionSlopes(nodes []erosionNode, sea, radius, strength float64) {
	for i := range nodes {
		nodes[i].filled = nodes[i].height
	}
	for i := range nodes {
		n := &nodes[i]
		if n.height <= sea {
			continue
		}
		target := n.height
		for _, id := range n.neighbors[:n.count] {
			d := &nodes[id]
			v := n.direction.Sub(d.direction)
			length := radius * math.Sqrt(v.Dot(v))
			target = math.Min(target, d.filled+.58*length)
		}
		n.height = math.Max(erosionFloor(n.original, sea, strength), lerp(n.height, target, .25*strength))
	}
}

type floodEntry struct {
	id    int32
	level float64
}
type floodQueue []floodEntry

func floodLess(a, b floodEntry) bool {
	if a.level == b.level {
		return a.id < b.id
	}
	return a.level < b.level
}

// Typed storage avoids millions of interface-box allocations during startup.
func (q floodQueue) sift(i int, value floodEntry) {
	for child := 2*i + 1; child < len(q); child = 2*i + 1 {
		if child+1 < len(q) && floodLess(q[child+1], q[child]) {
			child++
		}
		if !floodLess(q[child], value) {
			break
		}
		q[i] = q[child]
		i = child
	}
	q[i] = value
}
func (q *floodQueue) push(v floodEntry) {
	*q = append(*q, v)
	i := len(*q) - 1
	for i > 0 {
		parent := (i - 1) / 2
		if !floodLess(v, (*q)[parent]) {
			break
		}
		(*q)[i] = (*q)[parent]
		i = parent
	}
	(*q)[i] = v
}
func (q *floodQueue) pop() floodEntry {
	v, last := (*q)[0], (*q)[len(*q)-1]
	*q = (*q)[:len(*q)-1]
	if len(*q) > 0 {
		q.sift(0, last)
	}
	return v
}

func sampleErosionBase(nodes []erosionNode, p Planet) {
	workers := min(8, runtime.GOMAXPROCS(0), (len(nodes)+16383)/16384)
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		start, end := len(nodes)*worker/workers, len(nodes)*(worker+1)/workers
		wg.Go(func() {
			for i := start; i < end; i++ {
				nodes[i].original = p.baseElevation(nodes[i].direction)
				nodes[i].height = nodes[i].original
			}
		})
	}
	wg.Wait()
}

// Priority-flood resolves sinks on a virtual routing surface. Where possible,
// choose the physically steepest lower neighbor, using true edge lengths.
func erosionDrainage(nodes []erosionNode, sea, radius float64) []int32 {
	q, lowest := floodQueue{}, 0
	for i := range nodes {
		n := &nodes[i]
		n.down, n.filled, n.area = -2, n.height, n.rain
		if n.height < nodes[lowest].height {
			lowest = i
		}
		if n.height <= sea {
			n.down = -1
			q = append(q, floodEntry{int32(i), n.height})
		}
	}
	if len(q) == 0 {
		nodes[lowest].down = -1
		q = append(q, floodEntry{int32(lowest), nodes[lowest].height})
	}
	for i := len(q)/2 - 1; i >= 0; i-- {
		q.sift(i, q[i])
	}
	order := make([]int32, 0, len(nodes))
	settled := make([]bool, len(nodes))
	for len(q) > 0 {
		current := q.pop()
		n := &nodes[current.id]
		order = append(order, current.id)
		settled[current.id] = true
		bestSlope := 0.0
		for _, i := range n.neighbors[:n.count] {
			other := &nodes[i]
			if n.down >= 0 && settled[i] && other.filled < n.filled {
				delta := n.direction.Sub(other.direction)
				slope := (n.filled - other.filled) / (radius * math.Sqrt(delta.Dot(delta)))
				if slope > bestSlope {
					bestSlope, n.down = slope, i
				}
			}
			if other.down != -2 {
				continue
			}
			other.down, other.filled = current.id, math.Max(other.height, current.level)
			q.push(floodEntry{i, other.filled})
		}
	}
	for i := len(order) - 1; i >= 0; i-- {
		n := &nodes[order[i]]
		if n.down >= 0 {
			nodes[n.down].area += n.area
		}
	}
	return order
}
