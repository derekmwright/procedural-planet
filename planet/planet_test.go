package planet

import (
	"math"
	"reflect"
	"testing"
)

func TestDeterminismAndSeed(t *testing.T) {
	p := Planet{Seed: 7, Radius: 500000}
	key := Patch{Face: 0, Level: 2, X: 1, Y: 2}
	a, b := p.BuildPatch(key, 8), p.BuildPatch(key, 8)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same seed produced different geometry")
	}
	if reflect.DeepEqual(a, (Planet{Seed: 8, Radius: 500000}).BuildPatch(key, 8)) {
		t.Fatal("seed has no effect")
	}
}

func TestCubeEdgesMeet(t *testing.T) {
	p := Planet{Seed: 7, Radius: 500000}
	// Collect every cube-face boundary sample. Each must appear on another face;
	// corners must appear on three. Compare independently generated positions/normals.
	type sample struct {
		face                        int
		direction, position, normal Vec
	}
	var all []sample
	for face := 0; face < 6; face++ {
		for y := 0; y <= 8; y++ {
			for x := 0; x <= 8; x++ {
				if x != 0 && x != 8 && y != 0 && y != 8 {
					continue
				}
				d := Direction(face, float64(x)/4-1, float64(y)/4-1)
				all = append(all, sample{face, d, p.Surface(d), p.Normal(d)})
			}
		}
	}
	for _, a := range all {
		matched := false
		for _, b := range all {
			if a.face == b.face || a.direction.Sub(b.direction).Dot(a.direction.Sub(b.direction)) > 1e-24 {
				continue
			}
			matched = true
			if a.position.Sub(b.position).Dot(a.position.Sub(b.position)) > 1e-12 || a.normal.Sub(b.normal).Dot(a.normal.Sub(b.normal)) > 1e-16 {
				t.Fatal("cube edge surface discontinuity")
			}
		}
		if !matched {
			t.Fatalf("unmatched face %d edge %v", a.face, a.direction)
		}
	}
}

func TestPatchEdgesAndWinding(t *testing.T) {
	p := Planet{Seed: 7, Radius: 500000}
	for face := 0; face < 6; face++ {
		a := p.BuildPatch(Patch{face, 2, 1, 1}, 8)
		b := p.BuildPatch(Patch{face, 2, 2, 1}, 8)
		for y := 0; y <= 8; y++ {
			pa := a.Vertices[y*9+8].Position.Add(a.Origin)
			pb := b.Vertices[y*9].Position.Add(b.Origin)
			if math.Sqrt(pa.Sub(pb).Dot(pa.Sub(pb))) > 1e-8 {
				t.Fatal("adjacent patch seam")
			}
		}
		for i := 0; i < len(a.Indices); i += 3 {
			v0, v1, v2 := a.Vertices[a.Indices[i]], a.Vertices[a.Indices[i+1]], a.Vertices[a.Indices[i+2]]
			cross := v1.Position.Sub(v0.Position).Cross(v2.Position.Sub(v0.Position))
			if cross.Dot(v0.Position.Add(a.Origin)) >= 0 {
				t.Fatal("expected engine clockwise winding")
			}
			if math.Abs(v0.Normal.Dot(v0.Normal)-1) > 1e-10 || v0.Normal.Dot(v0.Position.Add(a.Origin)) <= 0 {
				t.Fatal("invalid outward normal")
			}
		}
	}
}

func TestCoarseNormalsFollowResolvedGeometry(t *testing.T) {
	p := Planet{Seed: 7, Radius: 500000}
	m := p.BuildPatch(Patch{0, 8, 27, 128}, 32)
	filtered, unfiltered := 0.0, 0.0
	for i := 0; i < len(m.Indices); i += 3 {
		a, b, c := m.Vertices[m.Indices[i]], m.Vertices[m.Indices[i+1]], m.Vertices[m.Indices[i+2]]
		// Engine triangles are clockwise, so reverse the cross product.
		geometric := c.Position.Sub(a.Position).Cross(b.Position.Sub(a.Position)).Unit()
		for _, v := range []Vertex{a, b, c} {
			filtered += 1 - geometric.Dot(v.Normal)
			unfiltered += 1 - geometric.Dot(p.Normal(v.Position.Add(m.Origin).Unit()))
		}
	}
	t.Logf("normal/triangle mismatch: filtered %.6f, sub-meter %.6f", filtered, unfiltered)
	if filtered >= unfiltered*0.8 {
		t.Fatal("coarse normals still dominated by unresolved detail")
	}
}
