package main

import (
	"math"
	"testing"

	"github.com/derekmwright/procedural-planet/planet"
)

func TestRocksStayAnchoredWhileCoastalLODRefines(t *testing.T) {
	p := planet.Planet{Seed: 7, Radius: 500000}
	lon, lat := 20*math.Pi/180, 12.6*math.Pi/180
	d := planet.Vec{math.Sin(lon) * math.Cos(lat), math.Sin(lat), math.Cos(lon) * math.Cos(lat)}
	tr := &terrain{world: p, lod: planet.NewLOD(p.Radius)}
	r := rockScatter{stones: scatterStones(p, d)}
	original := append([]stone(nil), r.stones...)
	hidden, visible, falseCrossings := 0, 0, 0
	for _, height := range []float64{150, 120, 100, 50, 20, 2} {
		eye := p.Surface(d).Add(d.Mul(height))
		for step := 0; step < 25; step++ {
			k, ok := tr.lod.NextSplit(p, eye)
			if !ok {
				break
			}
			tr.lod.Split(k)
		}
		r.updateSupport(tr)
		for i, s := range r.stones {
			if s.position != original[i].position {
				t.Fatal("LOD replacement moved a rock anchor")
			}
			mesh := p.MeshElevation(tr.lod.At(s.direction), planet.Segments, s.direction)
			if s.support < 0 || s.support > 1 || math.IsNaN(s.support) {
				t.Fatal("invalid support weight")
			}
			if math.Abs(mesh-s.height) >= 0.18+s.size*0.5 && s.support != 0 {
				t.Fatal("unsupported rock remained a shadow caster")
			}
			if s.support == 0 {
				hidden++
			}
			if s.support == 1 {
				visible++
			}
			// Reproduce the old lift through a shallow sea at the same site.
			const sea = 137.3
			if s.height+s.size < sea && tr.ground(s.direction)+s.size > sea {
				falseCrossings++
				if math.Sqrt(s.position.Dot(s.position))-p.Radius+s.size >= sea {
					t.Fatal("submerged rock was lifted through water")
				}
			}
		}
	}
	if hidden == 0 || visible == 0 || falseCrossings == 0 {
		t.Fatalf("fixture missed coarse/fine/waterline cases: %d %d %d", hidden, visible, falseCrossings)
	}
}
