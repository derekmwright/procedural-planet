package main

import (
	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/input"
)

// Compare lighting at the same pose without changing materials, cloud coverage
// or camera position. Reset restores the launch values, including custom flags.
func (g *game) visualControls(e *glyph.Engine) {
	in := e.Input()
	if in.KeyPressed(input.KeyF1) {
		g.hud = !g.hud
	}
	if in.KeyPressed(input.KeyV) && g.waterLand != nil {
		g.waterLandReflections = !g.waterLandReflections
	}
	if in.KeyPressed(input.KeyComma) {
		g.haze = max(0, g.haze-0.25)
	}
	if in.KeyPressed(input.KeyPeriod) {
		g.haze = min(4, g.haze+0.25)
	}
	previous := g.exposure
	// One third of an exposure stop per press; no adaptation while moving.
	const step = 1.25992105
	if in.KeyPressed(input.KeyLeftBracket) {
		g.exposure = max(1.0/64, g.exposure/step)
	}
	if in.KeyPressed(input.KeyRightBracket) {
		g.exposure = min(16, g.exposure*step)
	}
	if in.KeyPressed(input.KeyBackslash) {
		g.haze, g.exposure = g.startHaze, g.startExposure
	}
	if g.exposure != previous && !g.causticsDebug {
		e.Renderer().SetTonemap(g.exposure, g.tonemapCurve, 4)
	}
}
