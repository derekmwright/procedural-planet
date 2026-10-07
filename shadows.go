package main

import "math"

// shadowStrength controls both shader reception and engine shadow submission.
// The maps project straight sunlight, unlike the refracted underwater lighting.
// By default fade that approximation out as the eye submerges, then stop drawing
// casters. Keep the user's H setting intact so resurfacing restores shadows.
func (g *game) shadowStrength() float32 {
	if !g.shadowsEnabled || g.cam.clearance >= 12000 {
		return 0
	}
	if !g.oceanEnabled || g.underwaterShadows {
		return 1
	}
	depth := g.world.Radius + g.seaLevel - math.Sqrt(g.cam.eye.Dot(g.cam.eye))
	// Smoothstep has zero slope at both ends: no on/off snap at the interface
	// or when the shadow render work is disabled two metres below it.
	t := clamp(depth/2, 0, 1)
	return float32(1 - t*t*(3-2*t))
}
