package planet

import (
	"math"
	"testing"
)

func TestLODConvergesAndCoarsens(t *testing.T) {
	p := Planet{Seed: 7, Radius: 500000}
	l := NewLOD(p.Radius)
	d := Vec{math.Sin(20*math.Pi/180) * math.Cos(12.6*math.Pi/180), math.Sin(12.6 * math.Pi / 180), math.Cos(20*math.Pi/180) * math.Cos(12.6*math.Pi/180)}
	eye := p.Surface(d).Add(d.Mul(2))
	for i := 0; i < 900; i++ {
		if k, ok := l.NextSplit(p, eye); ok {
			l.Split(k)
		}
		if k, ok := l.NextMerge(p, eye); ok {
			l.Merge(k)
		}
	}
	if k, ok := l.NextSplit(p, eye); ok {
		t.Fatalf("never settled: next %v, current under camera %v, leaves %d", k, l.At(d), len(l.Leaves))
	}
	if l.At(d).Level != l.MaxLevel {
		t.Fatalf("ground detail %d, want %d", l.At(d).Level, l.MaxLevel)
	}
	for _, k := range l.SortedLeaves() {
		for _, n := range l.neighbours(k) {
			if abs(k.Level-n.Level) > 1 {
				t.Fatalf("unbalanced %v / %v", k, n)
			}
		}
	}
	eye = d.Mul(p.Radius * 3)
	for i := 0; i < 900; i++ {
		if k, ok := l.NextMerge(p, eye); ok {
			l.Merge(k)
		} else {
			break
		}
	}
	if len(l.Leaves) != 96 {
		t.Fatalf("did not coarsen back to roots: %d", len(l.Leaves))
	}
}
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func TestLODCubeSeamsAndCorners(t *testing.T) {
	p := Planet{Seed: 7, Radius: 500000}
	for _, d := range []Vec{{1, 0, 1}, {1, 1, 1}, {-1, 1, -1}, {0, 1, 0}} {
		d = d.Unit()
		l := NewLOD(p.Radius)
		eye := p.Surface(d).Add(d.Mul(2))
		for step := 0; step < 400; step++ {
			k, ok := l.NextSplit(p, eye)
			if !ok {
				break
			}
			if !l.Split(k) {
				t.Fatal("selected illegal split")
			}
			// Check after every commit, not merely after convergence.
			area := 0.0
			for leaf := range l.Leaves {
				area += math.Exp2(float64(-2 * leaf.Level))
				for _, n := range l.neighbours(leaf) {
					if abs(leaf.Level-n.Level) > 1 {
						t.Fatalf("unbalanced edge %v / %v", leaf, n)
					}
				}
			}
			if area != 6 {
				t.Fatalf("coverage changed: %v cube faces", area)
			}
		}
		if l.At(d).Level != l.MaxLevel {
			t.Fatalf("direction %v only reached level %d; leaves %d", d, l.At(d).Level, len(l.Leaves))
		}
		if len(l.Leaves) > MaxLeaves {
			t.Fatal("exceeded memory budget")
		}
	}
}

func TestMeshElevationMatchesRenderedTriangles(t *testing.T) {
	p := Planet{Seed: 42, Radius: 500000}
	for _, k := range []Patch{{0, 2, 1, 1}, {4, 9, 270, 200}, {2, 16, 32767, 32767}} {
		m := p.BuildPatch(k, Segments)
		for i := 0; i < len(m.Indices); i += 3 * 137 {
			a := m.Vertices[m.Indices[i]].Position.Add(m.Origin)
			b := m.Vertices[m.Indices[i+1]].Position.Add(m.Origin)
			c := m.Vertices[m.Indices[i+2]].Position.Add(m.Origin)
			// This barycentric point lies on the actual rendered triangle.
			point := a.Mul(0.2).Add(b.Mul(0.3)).Add(c.Mul(0.5))
			d := point.Unit()
			got := p.MeshElevation(k, Segments, d) + p.Radius
			want := math.Sqrt(point.Dot(point))
			if math.Abs(got-want) > 1e-5 {
				t.Fatalf("triangle collision drift %.8f m at %v", got-want, k)
			}
		}
	}
}

func TestTerrainSkirtsAndUploadCapacity(t *testing.T) {
	p := Planet{Seed: 7, Radius: 500000}
	k := Patch{4, 12, 2200, 1800}
	m := p.BuildTerrainPatch(k)
	if len(m.Vertices) > 65535 {
		t.Fatal("exceeds dynamic mesh index range")
	}
	for _, i := range m.Indices {
		if int(i) >= len(m.Vertices) {
			t.Fatal("invalid skirt index")
		}
	}
	for _, v := range m.Vertices[(Segments+1)*(Segments+1):] {
		point := v.Position.Add(m.Origin)
		d := point.Unit()
		if math.Sqrt(point.Dot(point)) >= p.Radius+p.Elevation(d)-0.5 {
			t.Fatal("skirt does not extend below terrain")
		}
	}
}
