package main

import (
	"github.com/derekmwright/procedural-planet/planet"
	"math"
	"reflect"
	"testing"
)

func TestGrassScatterDeterminismAndMask(t *testing.T) {
	d := planet.Vec{math.Sin(-math.Pi/4) * math.Cos(-13*math.Pi/180), math.Sin(-13 * math.Pi / 180), math.Cos(-math.Pi/4) * math.Cos(-13*math.Pi/180)}
	p := planet.Planet{Seed: 7, Radius: 500000, Vegetation: &planet.VegetationSettings{SeaLevel: 250, MaxHeight: 2600, MaxSlope: 38}}
	a, b := grassCandidates(p, d, 0.2), grassCandidates(p, d, 0.2)
	if len(a) == 0 || !reflect.DeepEqual(a, b) {
		t.Fatal("empty or nondeterministic grass")
	}
	for _, s := range a {
		if s.position.Dot(s.position) < math.Pow(p.Radius+250, 2) {
			t.Fatal("underwater tuft")
		}
		if cover, _ := p.GroundCover(s.direction, s.normal); cover <= 0 {
			t.Fatal("grass outside mask")
		}
	}
	p.Vegetation = nil
	if len(grassCandidates(p, d, 1)) != 0 {
		t.Fatal("grass with vegetation disabled")
	}
}
