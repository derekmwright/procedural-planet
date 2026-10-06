package planet

import "math"

// Shared cube-edge IDs make a single closed drainage graph. Only the small
// face boundaries need a map; interior nodes have unique, contiguous storage.
func erosionGrid(n int, radius float64) ([]erosionNode, []int32) {
	nodes := make([]erosionNode, 0, 6*n*n+2)
	ids := make([]int32, 6*(n+1)*(n+1))
	boundary := make(map[[3]int]int32)
	for face, b := range bases {
		for y := 0; y <= n; y++ {
			for x := 0; x <= n; x++ {
				var key [3]int
				for c := range key {
					key[c] = int(b[0][c])*n + int(b[1][c])*(2*x-n) + int(b[2][c])*(2*y-n)
				}
				id, exists := int32(0), false
				edge := x == 0 || y == 0 || x == n || y == n
				if edge {
					id, exists = boundary[key]
				}
				if !exists {
					id = int32(len(nodes))
					nodes = append(nodes, erosionNode{direction: Vec{float64(key[0]), float64(key[1]), float64(key[2])}.Unit()})
					if edge {
						boundary[key] = id
					}
				}
				ids[(face*(n+1)+y)*(n+1)+x] = id
			}
		}
	}
	link := func(a, b int32) {
		for _, pair := range [][2]int32{{a, b}, {b, a}} {
			p := &nodes[pair[0]]
			found := false
			for _, i := range p.neighbors[:p.count] {
				if i == pair[1] {
					found = true
					break
				}
			}
			if !found {
				p.neighbors[p.count] = pair[1]
				p.count++
			}
		}
	}
	for face := 0; face < 6; face++ {
		at := func(x, y int) int32 { return ids[(face*(n+1)+y)*(n+1)+x] }
		for y := 0; y <= n; y++ {
			for x := 0; x <= n; x++ {
				id := at(x, y)
				if x > 0 {
					link(id, at(x-1, y))
				}
				if y > 0 {
					link(id, at(x, y-1))
					if x > 0 {
						link(id, at(x-1, y-1))
					}
					if x < n {
						link(id, at(x+1, y-1))
					}
				}
				if x == n || y == n {
					continue
				}
				quad := [4]int32{id, at(x+1, y), at(x+1, y+1), at(x, y+1)}
				a, b, c, d := nodes[quad[0]].direction, nodes[quad[1]].direction, nodes[quad[2]].direction, nodes[quad[3]].direction
				area := (sphericalArea(a, b, c) + sphericalArea(a, c, d)) * radius * radius * .25
				for _, i := range quad {
					nodes[i].rain += area
				}
			}
		}
	}
	return nodes, ids
}

func sphericalArea(a, b, c Vec) float64 {
	return 2 * math.Atan2(math.Abs(a.Dot(b.Cross(c))), 1+a.Dot(b)+b.Dot(c)+c.Dot(a))
}

func (f *erosionField) index(face, x, y int) int { return (face*f.stride+y+1)*f.stride + x + 1 }

func (f *erosionField) fillGhosts() {
	n := f.resolution
	for face := 0; face < 6; face++ {
		for y := -1; y <= n+1; y++ {
			for x := -1; x <= n+1; x++ {
				if x >= 0 && y >= 0 && x <= n && y <= n {
					continue
				}
				d := Direction(face, float64(x)*2/float64(n)-1, float64(y)*2/float64(n)-1)
				f.values[f.index(face, x, y)] = float32(f.bilinear(d))
			}
		}
	}
}

func (f *erosionField) coordinates(d Vec) (face, x, y int, tx, ty float64) {
	face, u, v := Coordinates(d)
	px := math.Max(0, math.Min(float64(f.resolution), (u+1)*.5*float64(f.resolution)))
	py := math.Max(0, math.Min(float64(f.resolution), (v+1)*.5*float64(f.resolution)))
	x, y = min(int(px), f.resolution-1), min(int(py), f.resolution-1)
	return face, x, y, px - float64(x), py - float64(y)
}

func (f *erosionField) bilinear(d Vec) float64 {
	face, x, y, tx, ty := f.coordinates(d)
	i := f.index(face, x, y)
	a := lerp(float64(f.values[i]), float64(f.values[i+1]), tx)
	b := lerp(float64(f.values[i+f.stride]), float64(f.values[i+f.stride+1]), tx)
	return lerp(a, b, ty)
}

func cubicErosion(a, b, c, d, t float64) float64 {
	return b + .5*t*(c-a+t*(2*a-5*b+4*c-d+t*(3*(b-c)+d-a)))
}

func (f *erosionField) sample(d Vec) float64 {
	face, x, y, tx, ty := f.coordinates(d)
	var row [4]float64
	for j := 0; j < 4; j++ {
		i := f.index(face, x-1, y+j-1)
		row[j] = cubicErosion(float64(f.values[i]), float64(f.values[i+1]), float64(f.values[i+2]), float64(f.values[i+3]), tx)
	}
	return math.Max(0, math.Min(1200*f.strength, cubicErosion(row[0], row[1], row[2], row[3], ty)))
}

func (f *erosionField) cut(d Vec, elevation float64) float64 {
	if elevation <= f.sea {
		return 0
	}
	return math.Min(f.sample(d), (elevation-f.sea)*.8)
}

func (p Planet) ErosionStats() ErosionStats {
	if p.erosion == nil {
		return ErosionStats{}
	}
	return p.erosion.stats
}

func (p Planet) ErosionPreview(near Vec) (Vec, Vec, bool) {
	if p.erosion == nil {
		return Vec{}, Vec{}, false
	}
	best, d, downstream := 0.0, Vec{}, Vec{}
	for _, candidate := range p.erosion.previews {
		proximity := math.Max(0, candidate.direction.Dot(near)-.85) / .15
		score := candidate.cut * proximity * proximity
		if score > best {
			best, d, downstream = score, candidate.direction, candidate.downstream
		}
	}
	return d, downstream, best > 0
}
