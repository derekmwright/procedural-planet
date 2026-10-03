package main

import (
	"github.com/derekmwright/procedural-planet/planet"
	"math"
	"testing"
)

func TestFlightBasisAcrossPole(t *testing.T) {
	c := camera{lookPitch: -0.1}
	var previous planet.Vec
	for i := -100; i <= 100; i++ {
		a := float64(i) * 0.0002
		c.eye = planet.Vec{math.Sin(a) * 500002, math.Cos(a) * 500002, 0}
		c.flightView()
		if math.Abs(c.forward.Dot(c.up)) > 1e-10 || math.Abs(c.forward.Dot(c.forward)-1) > 1e-10 {
			t.Fatal("invalid camera basis")
		}
		if i > -100 && previous.Dot(c.forward) < 0.999 {
			t.Fatal("heading jumped crossing pole")
		}
		previous = c.forward
	}
}

func TestCameraStaysAboveVisibleTerrain(t *testing.T) {
	p := planet.Planet{Seed: 7, Radius: 500000}
	terrain := &terrain{world: p, lod: planet.NewLOD(p.Radius)}
	d := planet.Vec{0.3, 0.2, 1}.Unit()
	c := camera{eye: d.Mul(p.Radius - 1000)}
	c.constrain(terrain)
	actual := math.Sqrt(c.eye.Dot(c.eye)) - p.Radius
	mesh := p.MeshElevation(terrain.lod.At(d), planet.Segments, d)
	if actual < mesh+1.99999 || actual < p.Elevation(d)+1.99999 {
		t.Fatal("camera entered visible terrain")
	}
}
