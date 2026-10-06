package planet

import (
	"math"
	"testing"
)

func TestMeasuredRidgeErrorFallsWithRefinement(t *testing.T) {
	p := Planet{Seed: 7, Radius: 500000}
	lon, lat := math.Pi/180, 38.5*math.Pi/180
	d := Vec{math.Sin(lon) * math.Cos(lat), math.Sin(lat), math.Cos(lon) * math.Cos(lat)}
	f, u, v := Coordinates(d)
	k := Patch{f, 6, int((u + 1) * 32), int((v + 1) * 32)}
	coarse := p.BuildTerrainPatch(k)
	finest := k
	for i := 0; i < 3; i++ {
		finest.Level++
		n := math.Exp2(float64(finest.Level))
		finest.X, finest.Y = int((u+1)*n/2), int((v+1)*n/2)
	}
	fine := p.BuildTerrainPatch(finest)
	if math.IsNaN(coarse.GeometricError) || math.IsInf(coarse.GeometricError, 0) || coarse.GeometricError <= 0 || fine.GeometricError >= coarse.GeometricError*0.5 {
		t.Fatalf("ridge error did not fall with sampling density: %.3f -> %.3f m", coarse.GeometricError, fine.GeometricError)
	}
	// Measuring relief must not change any source surface samples or triangles.
	plain := p.BuildPatch(k, Segments)
	for i, vertex := range plain.Vertices {
		if vertex != coarse.Vertices[i] {
			t.Fatal("error estimate changed terrain vertices")
		}
	}
}

func TestErrorRefinementIsBoundedAndPreservesNearGroundSampling(t *testing.T) {
	p := Planet{Seed: 7, Radius: 500000}
	l := NewLOD(p.Radius)
	k := Patch{4, 2, 2, 2}
	u, v, span := k.Bounds()
	d := Direction(k.Face, u+span/2, v+span/2)
	eye := p.Surface(d).Add(d.Mul(p.Radius * span / 1.6))
	base := p.DetailScore(k, eye)
	l.SetGeometricError(k, 0)
	if l.detailScore(p, k, eye) != base {
		t.Fatal("flat/error-free patch changed distance sampling")
	}
	l.SetGeometricError(k, 1e6)
	if got := l.detailScore(p, k, eye); got <= 2.2 || got > 2*base+1e-12 {
		t.Fatalf("rough terrain did not refine within one-level cap: %f (base %f)", got, base)
	}
	for _, bad := range []float64{-1, math.NaN(), math.Inf(1)} {
		l.SetGeometricError(k, bad)
		if l.errors[k] != 1e6 {
			t.Fatal("invalid worker error poisoned LOD selection")
		}
	}
}

func TestErrorDrivenLODSettlesAndRetiresMeasurements(t *testing.T) {
	p := Planet{Seed: 7, Radius: 500000}
	l := NewLOD(p.Radius)
	d := Vec{1, 1, 1}.Unit() // exercises three cube-face seams
	eye := p.Surface(d).Add(d.Mul(2))
	// Synthetic roughness is deliberately large enough to hit the extra-level
	// cap, so this checks the worst selection pressure without expensive builds.
	register := func(k Patch) { _, _, span := k.Bounds(); l.SetGeometricError(k, p.Radius*span) }
	for k := range l.Leaves {
		register(k)
	}
	settled := false
	for step := 0; step < 900; step++ {
		changed := false
		if k, ok := l.NextMerge(p, eye); ok {
			l.Merge(k)
			changed = true
		}
		if k, ok := l.NextSplit(p, eye); ok {
			if !l.Split(k) {
				t.Fatal("invalid error-driven split")
			}
			for _, c := range k.Children() {
				register(c)
			}
			changed = true
		}
		if !changed {
			settled = true
			break
		}
	}
	if !settled || len(l.Leaves) > MaxLeaves || l.At(d).Level != l.MaxLevel {
		t.Fatalf("rough LOD did not settle at ground detail: settled=%t leaves=%d level=%d", settled, len(l.Leaves), l.At(d).Level)
	}
	for k := range l.Leaves {
		for _, n := range l.neighbours(k) {
			if abs(k.Level-n.Level) > 1 {
				t.Fatalf("unbalanced error refinement %v / %v", k, n)
			}
		}
	}
	eye = d.Mul(p.Radius * 3)
	for step := 0; step < 900; step++ {
		k, ok := l.NextMerge(p, eye)
		if !ok {
			break
		}
		l.Merge(k)
	}
	if len(l.Leaves) != 96 || len(l.errors) != 96 {
		t.Fatalf("did not retire child geometry/error metadata: leaves=%d errors=%d", len(l.Leaves), len(l.errors))
	}
	if k, ok := l.NextSplit(p, eye); ok {
		t.Fatalf("coarsened LOD immediately wants to split %v", k)
	}
}

func BenchmarkTerrainPatchWithError(b *testing.B) {
	p := Planet{Seed: 7, Radius: 500000}
	k := Patch{4, 8, 129, 229}
	for b.Loop() {
		p.BuildTerrainPatch(k)
	}
}

func BenchmarkTerrainPatchSurfaceOnly(b *testing.B) {
	p := Planet{Seed: 7, Radius: 500000}
	k := Patch{4, 8, 129, 229}
	for b.Loop() {
		p.BuildPatch(k, Segments)
	}
}
