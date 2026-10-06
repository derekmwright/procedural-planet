// Package planet describes a deterministic radial surface in meters.
// It has no renderer dependency: geometry stays in float64 until upload.
package planet

import "math"

type Vec [3]float64

func (a Vec) Add(b Vec) Vec     { return Vec{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func (a Vec) Sub(b Vec) Vec     { return a.Add(b.Mul(-1)) }
func (a Vec) Mul(s float64) Vec { return Vec{a[0] * s, a[1] * s, a[2] * s} }
func (a Vec) Dot(b Vec) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func (a Vec) Cross(b Vec) Vec {
	return Vec{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func (a Vec) Unit() Vec { return a.Mul(1 / math.Sqrt(a.Dot(a))) }

type Planet struct {
	Seed       uint64
	Radius     float64
	Vegetation *VegetationSettings
	erosion    *erosionField
}

func mix(x uint64) uint64 {
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}
func smooth(x float64) float64     { return x * x * x * (x*(x*6-15) + 10) }
func lerp(a, b, t float64) float64 { return a + (b-a)*t }
func (p Planet) noise(v Vec, salt uint64) float64 {
	x, y, z := int64(math.Floor(v[0])), int64(math.Floor(v[1])), int64(math.Floor(v[2]))
	f := Vec{smooth(v[0] - float64(x)), smooth(v[1] - float64(y)), smooth(v[2] - float64(z))}
	var rows [2]float64
	for k := 0; k < 2; k++ {
		var cols [2]float64
		for j := 0; j < 2; j++ {
			var a [2]float64
			for i := 0; i < 2; i++ {
				h := mix(uint64(x+int64(i))*0x9e3779b185ebca87 ^ uint64(y+int64(j))*0xc2b2ae3d27d4eb4f ^ uint64(z+int64(k))*0x165667b19e3779f9 ^ mix(p.Seed+salt))
				a[i] = float64(h>>11)/float64(uint64(1)<<53)*2 - 1
			}
			cols[j] = lerp(a[0], a[1], f[0])
		}
		rows[k] = lerp(cols[0], cols[1], f[1])
	}
	return lerp(rows[0], rows[1], f[2])
}

// Elevation uses physical wavelengths, so changing radius does not scale mountains.
func (p Planet) Elevation(d Vec) float64 {
	h := p.baseElevation(d)
	if p.erosion != nil && p.erosion.seed == p.Seed && p.erosion.radius == p.Radius {
		h -= p.erosion.cut(d, h)
	}
	return h
}

func (p Planet) baseElevation(d Vec) float64 {
	x := d.Mul(p.Radius)
	broad := p.noise(x.Mul(1.0/220000), 1)
	mask := smooth(math.Max(0, math.Min(1, (p.noise(x.Mul(1.0/110000), 2)+0.25)*1.4)))
	warp := Vec{p.noise(x.Mul(1.0/95000), 3), p.noise(x.Mul(1.0/95000), 4), p.noise(x.Mul(1.0/95000), 5)}.Mul(24000)
	w := x.Add(warp)
	// Rounded ridge crests keep the derivative continuous at this orbital LOD.
	r := p.noise(w.Mul(1.0/38000), 6)
	ridge := 1 - math.Sqrt(r*r+0.025)
	fine := p.noise(w.Mul(1.0/23000), 7) * 180
	// Local foothills and rock-scale relief share the same field at every LOD.
	local := p.noise(x.Mul(1.0/4500), 21)*220 + p.noise(x.Mul(1.0/700), 22)*60 + p.noise(x.Mul(1.0/90), 23)*5 + p.noise(x.Mul(1.0/12), 24)*0.3
	return broad*2200 + mask*ridge*ridge*3200 + fine + local + p.MountainHeight(d)
}

// MountainHeight adds localized chains rather than raising every foothill.
// Domain warping bends the range mask; nested ridges concentrate fine relief
// on mountain slopes while retaining broad, quieter basins between ranges.
func (p Planet) MountainHeight(d Vec) float64 {
	x := d.Mul(p.Radius)
	w := x.Add(Vec{p.noise(x.Mul(1.0/80000), 81), p.noise(x.Mul(1.0/80000), 82), p.noise(x.Mul(1.0/80000), 83)}.Mul(28000))
	belt := p.noise(Vec{w[0] / 170000, w[1] / 65000, w[2] / 110000}, 84)
	mask := smooth(math.Max(0, math.Min(1, (belt-0.03)/0.36)))
	if mask == 0 {
		return 0
	}
	sum, weight, previous := 0.0, 1.0, 1.0
	for i, wavelength := range []float64{26000, 11000, 4200, 1500} {
		n := p.noise(w.Mul(1/wavelength), uint64(85+i))
		ridge := math.Max(0, 1-math.Sqrt(n*n+0.0009))
		ridge = ridge * ridge * ridge
		sum += ridge * weight * previous
		previous = ridge
		weight *= 0.45
	}
	return mask * sum * 5700
}
func (p Planet) Surface(d Vec) Vec { return d.Mul(p.Radius + p.Elevation(d)) }
func (p Planet) Normal(d Vec) Vec {
	return p.normalAtScale(d, 0.5)
}
func (p Planet) normalAtScale(d Vec, spacing float64) Vec {
	axis := Vec{0, 1, 0}
	if math.Abs(d[1]) > 0.9 {
		axis = Vec{1, 0, 0}
	}
	u := axis.Cross(d).Unit()
	v := d.Cross(u)
	eps := spacing / p.Radius
	a := p.Surface(d.Add(u.Mul(eps)).Unit()).Sub(p.Surface(d.Sub(u.Mul(eps)).Unit()))
	b := p.Surface(d.Add(v.Mul(eps)).Unit()).Sub(p.Surface(d.Sub(v.Mul(eps)).Unit()))
	return a.Cross(b).Unit()
}
func (p Planet) Color(d Vec) [3]float32 {
	h := p.Elevation(d)
	t := math.Max(0, math.Min(1, (h+1800)/6500))
	variation := p.noise(d.Mul(p.Radius/28000), 19) * 0.045
	variation += p.noise(d.Mul(p.Radius/7), 25)*0.028 + p.noise(d.Mul(p.Radius/1.5), 26)*0.012
	return [3]float32{float32(lerp(0.16, 0.55, t) + variation), float32(lerp(0.115, 0.46, t) + variation), float32(lerp(0.085, 0.34, t) + variation)}
}

// Face bases have U cross V pointing outwards. No latitude singularity.
var bases = [6][3]Vec{
	{{1, 0, 0}, {0, 0, -1}, {0, 1, 0}}, {{-1, 0, 0}, {0, 0, 1}, {0, 1, 0}},
	{{0, 1, 0}, {1, 0, 0}, {0, 0, -1}}, {{0, -1, 0}, {1, 0, 0}, {0, 0, 1}},
	{{0, 0, 1}, {1, 0, 0}, {0, 1, 0}}, {{0, 0, -1}, {-1, 0, 0}, {0, 1, 0}},
}

func Direction(face int, u, v float64) Vec {
	b := bases[face]
	return b[0].Add(b[1].Mul(u)).Add(b[2].Mul(v)).Unit()
}

// Patch is addressed by a cube face and quadtree cell.
type Patch struct{ Face, Level, X, Y int }
type Vertex struct {
	Position, Normal Vec
	Color            [3]float32
	Cover            float32
}
type Mesh struct {
	Origin   Vec
	Vertices []Vertex
	Indices  []uint32
	// GeometricError estimates lost radial relief at cell-diagonal midpoints.
	// It is measured before skirts are added, not a certified error bound.
	GeometricError float64
}

func (p Planet) BuildPatch(k Patch, segments int) Mesh {
	n := float64(uint64(1) << k.Level)
	u0, v0 := float64(k.X)*2/n-1, float64(k.Y)*2/n-1
	span := 2 / n
	// A coarse triangle cannot represent sub-meter slopes. Match the normal
	// derivative footprint to its sampling scale instead of aliasing fine
	// rock relief into kilometer-wide lighting facets.
	normalSpacing := math.Max(0.5, p.Radius*span/float64(segments)*0.5)
	m := Mesh{Origin: p.Surface(Direction(k.Face, u0+span/2, v0+span/2))}
	for y := 0; y <= segments; y++ {
		for x := 0; x <= segments; x++ {
			d := Direction(k.Face, u0+span*float64(x)/float64(segments), v0+span*float64(y)/float64(segments))
			normal := p.normalAtScale(d, normalSpacing)
			color := p.Color(d)
			cover, grass := p.GroundCover(d, normal)
			for i := range color {
				color[i] = float32(lerp(float64(color[i]), float64(grass[i]), cover))
			}
			m.Vertices = append(m.Vertices, Vertex{Position: p.Surface(d).Sub(m.Origin), Normal: normal, Color: color, Cover: float32(cover)})
		}
	}
	for y := 0; y < segments; y++ {
		for x := 0; x < segments; x++ {
			a := uint32(y*(segments+1) + x)
			b := a + 1
			c := a + uint32(segments+1)
			d := c + 1
			// GlyphEngine uses clockwise front faces, matching CreateCube/CreateDisc.
			m.Indices = append(m.Indices, a, c, b, b, c, d)
		}
	}
	return m
}
