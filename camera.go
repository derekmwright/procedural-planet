package main

import (
	"github.com/derekmwright/glyphengine/input"
	"github.com/derekmwright/procedural-planet/planet"
	"math"
)

type camera struct {
	yaw, latitude, clearance float64
	eye, forward, up         planet.Vec
	flight, auto             bool
	lookPitch                float64
	bearing                  planet.Vec
}

func clamp(x, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, x)) }
func northEast(up planet.Vec) (north, east planet.Vec) {
	axis := planet.Vec{0, 1, 0}
	if math.Abs(up[1]) > 0.9999 {
		axis = planet.Vec{0, 0, -1}
	}
	east = axis.Cross(up).Unit()
	north = up.Cross(east).Unit()
	return
}
func (c *camera) reset(radius float64) {
	*c = camera{yaw: 0.35, latitude: 0.22, clearance: radius * 1.8}
}
func (c *camera) direction() planet.Vec {
	return planet.Vec{math.Sin(c.yaw) * math.Cos(c.latitude), math.Sin(c.latitude), math.Cos(c.yaw) * math.Cos(c.latitude)}
}
func (c *camera) orbit(t *terrain) {
	d := c.direction()
	north, _ := northEast(d)
	c.eye = d.Mul(t.world.Radius + t.ground(d) + c.clearance)
	transition := clamp(math.Log(t.world.Radius*0.2/math.Max(c.clearance, 1))/math.Log(t.world.Radius*0.2/100), 0, 1)
	dip := -math.Pi/2 + (math.Pi/2-0.04)*transition
	c.forward = north.Mul(math.Cos(dip)).Add(d.Mul(math.Sin(dip)))
	c.up = d.Mul(math.Cos(dip)).Sub(north.Mul(math.Sin(dip)))
}
func (c *camera) flightView() {
	d := c.eye.Unit()
	// Carry the tangent heading across the sphere instead of re-deriving it
	// from longitude. This avoids a sudden yaw change when crossing a pole.
	horizontal := c.bearing.Sub(d.Mul(c.bearing.Dot(d)))
	if horizontal.Dot(horizontal) < 1e-12 {
		horizontal, _ = northEast(d)
	}
	horizontal = horizontal.Unit()
	c.bearing = horizontal
	c.forward = horizontal.Mul(math.Cos(c.lookPitch)).Add(d.Mul(math.Sin(c.lookPitch)))
	c.up = d.Mul(math.Cos(c.lookPitch)).Sub(horizontal.Mul(math.Sin(c.lookPitch)))
}
func (c *camera) constrain(t *terrain) {
	d := c.eye.Unit()
	radius := math.Sqrt(c.eye.Dot(c.eye))
	floor := t.world.Radius + t.ground(d)
	radius = clamp(radius, floor+2, t.world.Radius*9)
	c.eye = d.Mul(radius)
	c.clearance = radius - floor
}

func (c *camera) enterFlight() {
	// Preserve the current view when movement takes over from orbit mode.
	d := c.eye.Unit()
	c.lookPitch = math.Asin(clamp(c.forward.Dot(d), -1, 1))
	c.bearing = c.forward.Sub(d.Mul(c.forward.Dot(d)))
	if c.bearing.Dot(c.bearing) < 1e-12 {
		c.bearing = c.up
	}
	c.flight = true
	c.auto = false
}
func (c *camera) update(t *terrain, in *input.Input, dt float64) {
	dt = math.Min(dt, 0.05)
	if in.KeyPressed(input.KeyHome) {
		c.reset(t.world.Radius)
		c.orbit(t)
	}
	if in.KeyPressed(input.KeySpace) {
		c.auto = !c.auto
	}
	if in.KeyPressed(input.KeyTab) {
		if !c.flight {
			c.enterFlight()
		} else {
			c.flight = false
			d := c.eye.Unit()
			c.yaw = math.Atan2(d[0], d[2])
			c.latitude = math.Asin(d[1])
		}
	}
	if !c.flight && (in.KeyDown(input.KeyW) || in.KeyDown(input.KeyS) || in.KeyDown(input.KeyA) || in.KeyDown(input.KeyD) || in.KeyDown(input.KeyQ) || in.KeyDown(input.KeyE)) {
		c.enterFlight()
	}
	if in.MousePressed(input.MouseButtonLeft) || in.MousePressed(input.MouseButtonRight) {
		in.SetCursorLocked(true)
	}
	if !in.MouseDown(input.MouseButtonLeft) && !in.MouseDown(input.MouseButtonRight) {
		in.SetCursorLocked(false)
	}
	dx, dy := 0.0, 0.0
	if in.MouseDown(input.MouseButtonLeft) || in.MouseDown(input.MouseButtonRight) {
		dx, dy = in.MouseDelta()
	}
	_, scroll := in.ConsumeScroll()
	// The wheel always changes distance from the surface, never movement speed.
	c.clearance = clamp(c.clearance*math.Exp(-scroll*0.2), 2, t.world.Radius*8)
	if c.flight && scroll != 0 {
		d := c.eye.Unit()
		c.eye = d.Mul(t.world.Radius + t.ground(d) + c.clearance)
	}
	if c.flight {
		c.flightView()
		yawChange := dx * 0.003
		c.lookPitch = clamp(c.lookPitch-dy*0.003, -math.Pi/2, math.Pi/2)
		if in.KeyDown(input.KeyLeft) || in.KeyDown(input.KeyA) {
			yawChange -= dt
		}
		if in.KeyDown(input.KeyRight) || in.KeyDown(input.KeyD) {
			yawChange += dt
		}
		if in.KeyDown(input.KeyUp) {
			c.lookPitch = clamp(c.lookPitch+dt, -math.Pi/2, math.Pi/2)
		}
		if in.KeyDown(input.KeyDown) {
			c.lookPitch = clamp(c.lookPitch-dt, -math.Pi/2, math.Pi/2)
		}
		c.bearing = c.bearing.Mul(math.Cos(yawChange)).Add(c.bearing.Cross(c.eye.Unit()).Mul(math.Sin(yawChange)))
		c.flightView()
		radial := c.eye.Unit()
		speed := clamp(c.clearance*0.65, 4, 350000)
		if in.KeyDown(input.KeyLeftShift) {
			speed *= 5
		}
		move := planet.Vec{}
		if in.KeyDown(input.KeyW) {
			move = move.Add(c.forward)
		}
		if in.KeyDown(input.KeyS) {
			move = move.Sub(c.forward)
		}
		if in.KeyDown(input.KeyE) {
			move = move.Add(radial)
		}
		if in.KeyDown(input.KeyQ) {
			move = move.Sub(radial)
		}
		if move.Dot(move) > 0 {
			c.eye = c.eye.Add(move.Unit().Mul(speed * dt))
		}
		c.constrain(t)
		c.flightView()
		return
	}
	sensitivity := clamp(c.clearance/(t.world.Radius*0.2), 0.00002, 1)
	c.yaw -= dx * 0.004 * sensitivity
	c.latitude += dy * 0.004 * sensitivity
	s := dt * 0.6 * sensitivity
	if in.KeyDown(input.KeyLeft) {
		c.yaw -= s
	}
	if in.KeyDown(input.KeyRight) {
		c.yaw += s
	}
	if in.KeyDown(input.KeyUp) {
		c.latitude += s
	}
	if in.KeyDown(input.KeyDown) {
		c.latitude -= s
	}
	if c.auto {
		c.yaw += dt * 0.08 * sensitivity
	}
	c.latitude = clamp(c.latitude, -1.55, 1.55)
	c.orbit(t)
}
