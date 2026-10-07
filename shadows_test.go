package main

import (
	"math"
	"testing"

	"github.com/derekmwright/procedural-planet/planet"
)

func TestSubmergedShadowFade(t *testing.T) {
	g := game{world: planet.Planet{Radius: 500000}, seaLevel: 250, oceanEnabled: true, shadowsEnabled: true}
	g.cam.clearance = 20
	for _, c := range []struct{ depth, want float64 }{
		{-5, 1}, {0, 1}, {0.5, 0.84375}, {1, 0.5}, {1.5, 0.15625}, {2, 0}, {80, 0},
		// Ascending across the same boundary must recover without a latched state.
		{2, 0}, {1, 0.5}, {0, 1}, {-1, 1},
	} {
		g.cam.eye = planet.Vec{0, 0, g.world.Radius + g.seaLevel - c.depth}
		if got := g.shadowStrength(); math.Abs(float64(got)-c.want) > 1e-6 {
			t.Errorf("depth %g: strength %g, want %g", c.depth, got, c.want)
		}
	}
}

func TestSubmergedShadowControls(t *testing.T) {
	for _, c := range []struct {
		name                   string
		ocean, enabled, retain bool
		clearance              float64
		want                   float32
	}{
		{"submerged default", true, true, false, 20, 0},
		{"comparison override", true, true, true, 20, 1},
		{"ocean disabled", false, true, false, 20, 1},
		{"H off", true, false, true, 20, 0},
		{"orbital cutoff retained", true, true, true, 12000, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := game{world: planet.Planet{Radius: 500000}, seaLevel: 250,
				oceanEnabled: c.ocean, shadowsEnabled: c.enabled, underwaterShadows: c.retain}
			g.cam.eye, g.cam.clearance = planet.Vec{0, 0, 500240}, c.clearance
			if got := g.shadowStrength(); got != c.want {
				t.Fatalf("strength %g, want %g", got, c.want)
			}
		})
	}
}
