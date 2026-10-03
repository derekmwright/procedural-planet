package main

import (
	"github.com/derekmwright/procedural-planet/planet"
	"math"
	"reflect"
	"testing"
)

func TestRockScatterStableAcrossTravelAndCubeSeam(t *testing.T) {
	p := planet.Planet{Seed: 7, Radius: 500000}
	a := planet.Vec{1, 0, 1}.Unit()
	b := a.Add(planet.Vec{0, 0.00002, 0}).Unit()
	first, second := scatterStones(p, a), scatterStones(p, b)
	if len(first) == 0 || !reflect.DeepEqual(first, scatterStones(p, a)) {
		t.Fatal("empty or nondeterministic scatter")
	}
	byDirection := map[planet.Vec]stone{}
	faces := map[int]bool{}
	for _, s := range first {
		if _, exists := byDirection[s.direction]; exists {
			t.Fatal("duplicate stone")
		}
		byDirection[s.direction] = s
		f, _, _ := planet.Coordinates(s.direction)
		faces[f] = true
	}
	if len(faces) < 2 {
		t.Fatal("scatter stops at cube seam")
	}
	overlap := 0
	for _, s := range second {
		if old, ok := byDirection[s.direction]; ok {
			overlap++
			if old != s {
				t.Fatal("travel changed existing rock")
			}
		}
	}
	if overlap < len(first)*8/10 {
		t.Fatal("insufficient stable overlap")
	}
}

func TestRockGeometryClosedWinding(t *testing.T) {
	for variant := 0; variant < 3; variant++ {
		vertices, indices := rockGeometry(variant)
		for i := 0; i < len(indices); i += 3 {
			a, b, c := vertices[indices[i]], vertices[indices[i+1]], vertices[indices[i+2]]
			v := func(p [3]float32) planet.Vec { return planet.Vec{float64(p[0]), float64(p[1]), float64(p[2])} }
			cross := v(b.Pos).Sub(v(a.Pos)).Cross(v(c.Pos).Sub(v(a.Pos)))
			if cross.Dot(v(a.Normal)) >= 0 || math.Abs(v(a.Normal).Dot(v(a.Normal))-1) > 1e-5 {
				t.Fatal("invalid clockwise facet")
			}
		}
	}
}
