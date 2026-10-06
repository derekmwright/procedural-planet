package atmosphere

import (
	_ "embed"
	"encoding/binary"
	"math"

	"github.com/derekmwright/glyphengine/renderer"
	"github.com/derekmwright/glyphengine/shaders"
	"github.com/derekmwright/procedural-planet/planet"
	"github.com/go-gl/mathgl/mgl32"
)

//go:embed caustic-cache.vert.spv
var causticVert []byte

//go:embed caustic-cache.frag.spv
var causticFrag []byte

//go:embed caustic-filter.frag.spv
var causticFilter []byte

//go:embed caustic-temporal.frag.spv
var causticTemporal []byte

//go:embed caustic-history.frag.spv
var causticHistory []byte

//go:embed caustic-state.frag.spv
var causticState []byte

//go:embed caustic-resolve.frag.spv
var causticResolve []byte

const (
	causticSpan           = 128
	causticCoverageRadius = 47
	causticAnchorDistance = 2
	causticFilterRadius   = 1 // four 0.25 m texels; keep the whole kernel in bounds
	// A common multiple of the 0.375 m source grid and 0.25 m receiver texels.
	causticAnchorStep = 0.75
)

// CausticCache sums forward-refracted surface triangles, including overlapping
// and reversed folds. Eight receiver depths share one local, world-anchored atlas.
type CausticCache struct {
	pass            *renderer.AppPass
	resolve         *renderer.AppPass
	blur            *renderer.AppPass
	filter          *renderer.AppPass
	remember, state *renderer.AppPass
	anchor, u, v    planet.Vec
	frameOrigin     planet.Vec
	radius          float64
}

func NewCausticCache(r *renderer.Renderer) (*CausticCache, error) {
	target, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "water focusing", Format: renderer.TargetR16F, Width: 4096, Height: 2048, Filter: renderer.FilterLinear})
	if err != nil {
		return nil, err
	}
	pass, err := r.CreateAppPass(renderer.AppPassDesc{Name: "water focusing", Stage: renderer.StageBeforeScene, Target: target, Blend: renderer.BlendAdditive, Vert: causticVert, Frag: causticFrag, Timed: true})
	if err != nil {
		return nil, err
	}
	resolved, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "resolved water focusing", Format: renderer.TargetR16F, Width: 2048, Height: 1024, Filter: renderer.FilterLinear})
	if err != nil {
		return nil, err
	}
	resolve, err := r.CreateAppPass(renderer.AppPassDesc{Name: "water focusing resolve", Stage: renderer.StageBeforeScene, Target: resolved, Vert: shaders.DepthResolveVertSpv, Frag: causticResolve, Fullscreen: true, Reads: []*renderer.Texture{target.Texture()}, Timed: true})
	if err != nil {
		return nil, err
	}
	horizontal, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "horizontal water focusing", Format: renderer.TargetR16F, Width: 2048, Height: 1024, Filter: renderer.FilterLinear})
	if err != nil {
		return nil, err
	}
	blur, err := r.CreateAppPass(renderer.AppPassDesc{Name: "water focusing blur", Stage: renderer.StageBeforeScene, Target: horizontal, Vert: shaders.DepthResolveVertSpv, Frag: causticFilter, Fullscreen: true, Reads: []*renderer.Texture{resolved.Texture()}, Timed: true})
	if err != nil {
		return nil, err
	}
	filtered, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "filtered water focusing", Format: renderer.TargetR16F, Width: 2048, Height: 1024, Filter: renderer.FilterLinear})
	if err != nil {
		return nil, err
	}
	// History textures expose the previous submitted frame. Keep the current
	// result separate so the scene receives this frame's reconstruction.
	history, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "water focusing history", Format: renderer.TargetR16F, Width: 2048, Height: 1024, Filter: renderer.FilterNearest, History: true})
	if err != nil {
		return nil, err
	}
	state, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "water focusing state", Format: renderer.TargetRGBA32F, Width: 4, Height: 1, History: true})
	if err != nil {
		return nil, err
	}
	filter, err := r.CreateAppPass(renderer.AppPassDesc{Name: "water focusing filter", Stage: renderer.StageBeforeScene, Target: filtered, Vert: shaders.DepthResolveVertSpv, Frag: causticTemporal, Fullscreen: true, Reads: []*renderer.Texture{horizontal.Texture(), history.Texture(), state.Texture()}, Timed: true})
	if err != nil {
		return nil, err
	}
	remember, err := r.CreateAppPass(renderer.AppPassDesc{Name: "water focusing history", Stage: renderer.StageBeforeScene, Target: history, Vert: shaders.DepthResolveVertSpv, Frag: causticHistory, Fullscreen: true, Reads: []*renderer.Texture{filtered.Texture()}, Timed: true})
	if err != nil {
		return nil, err
	}
	statePass, err := r.CreateAppPass(renderer.AppPassDesc{Name: "water focusing state", Stage: renderer.StageBeforeScene, Target: state, Vert: shaders.DepthResolveVertSpv, Frag: causticState, Fullscreen: true, Timed: true})
	if err != nil {
		return nil, err
	}
	for axis, p := range []*renderer.AppPass{blur, filter} {
		var data [16]byte
		binary.LittleEndian.PutUint32(data[axis*4:], math.Float32bits(1))
		if err = p.SetPushConstants(data[:]); err != nil {
			return nil, err
		}
	}
	if err = r.SetShaderTarget(3, filtered); err != nil {
		return nil, err
	}
	// Pad the 128 m receiver tile so refracted rays can enter across its edges.
	const cells = 384
	// At zero travel distance the refraction map is the identity: intensity is
	// exactly one. Two triangles replace 294,912 identical microtriangles.
	vertices := make([]renderer.Vertex, 4+7*(cells+1)*(cells+1))
	for i, xy := range [4][2]float32{{-64, -64}, {64, -64}, {-64, 64}, {64, 64}} {
		vertices[i].Pos = [3]float32{xy[0], xy[1], 0}
	}
	indices := make([]uint32, 6, 6+7*cells*cells*6)
	copy(indices, []uint32{0, 1, 2, 1, 3, 2})
	for layer := 1; layer < 8; layer++ {
		base := 4 + (layer-1)*(cells+1)*(cells+1)
		for y := 0; y <= cells; y++ {
			for x := 0; x <= cells; x++ {
				vertices[base+y*(cells+1)+x].Pos = [3]float32{float32(x)*0.375 - 72, float32(y)*0.375 - 72, float32(layer)}
				if x < cells && y < cells {
					a := uint32(base + y*(cells+1) + x)
					b := a + 1
					c := a + cells + 1
					d := c + 1
					indices = append(indices, a, b, c, b, d, c)
				}
			}
		}
	}
	mesh, err := r.CreateIndexedMesh32(vertices, indices)
	if err != nil {
		return nil, err
	}
	// Static meshes are renderer-owned and released by Renderer.Destroy.
	pass.SetDraws([]renderer.RenderObject{{Mesh: mesh, Model: mgl32.Ident4()}})
	return &CausticCache{pass: pass, resolve: resolve, blur: blur, filter: filter, remember: remember, state: statePass}, nil
}

func (c *CausticCache) Update(p *Parameters, eye planet.Vec, seaRadius float64, enabled bool, sun [3]float32) {
	active := c.prepare(p, eye, seaRadius, enabled, sun)
	c.pass.SetEnabled(active)
	c.resolve.SetEnabled(active)
	c.blur.SetEnabled(active)
	c.filter.SetEnabled(active)
	c.remember.SetEnabled(active && p.Rendering[2] > 0.5)
	c.state.SetEnabled(active && p.Rendering[2] > 0.5)
	if !active {
		return
	}
	var data [16]byte
	for i, v := range sun {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(v))
	}
	// Fixed 16-byte payload is always valid for an application pass.
	if err := c.pass.SetPushConstants(data[:]); err != nil {
		panic(err)
	}
	if err := c.state.SetPushConstants(data[:]); err != nil {
		panic(err)
	}
	var filterData [32]byte
	binary.LittleEndian.PutUint32(filterData[4:], math.Float32bits(1)) // vertical
	copy(filterData[16:], data[:])
	if err := c.filter.SetPushConstants(filterData[:]); err != nil {
		panic(err)
	}
}

func (c *CausticCache) prepare(p *Parameters, eye planet.Vec, seaRadius float64, enabled bool, sun [3]float32) bool {
	up := eye.Unit()
	s := planet.Vec{float64(sun[0]), float64(sun[1]), float64(sun[2])}.Unit()
	height := math.Sqrt(eye.Dot(eye)) - seaRadius
	active := enabled && height > -80 && height < 350 && up.Dot(s) > 0.08
	p.Rendering[1] = 1 // cache selected, even when inactive: never read a stale tile
	p.CausticOrigin[3] = 0
	if !active {
		return false
	}
	center := up.Mul(seaRadius)
	// Retain one tangent frame across nearby updates. Recomputing it and placing
	// the grid at an arbitrary camera position resamples every projected fold.
	frameDistance := math.Min(256, seaRadius*0.001)
	if c.radius != seaRadius || center.Sub(c.frameOrigin).Dot(center.Sub(c.frameOrigin)) > frameDistance*frameDistance {
		c.anchor, c.radius = center, seaRadius
		c.frameOrigin = center
		axis := planet.Vec{0, 1, 0}
		if math.Abs(up[1]) > 0.95 {
			axis = planet.Vec{1, 0, 0}
		}
		c.u = axis.Cross(up).Unit()
		c.v = up.Cross(c.u)
	} else if center.Sub(c.anchor).Dot(center.Sub(c.anchor)) > causticAnchorDistance*causticAnchorDistance {
		delta := center.Sub(c.frameOrigin)
		x := math.Round(delta.Dot(c.u)/causticAnchorStep) * causticAnchorStep
		y := math.Round(delta.Dot(c.v)/causticAnchorStep) * causticAnchorStep
		// Exact snapped tangent coordinates, with radial displacement to stay on
		// the sphere. Stable form of sqrt(r*r-x*x-y*y)-r avoids cancellation.
		rise := -(x*x + y*y) / (math.Sqrt(seaRadius*seaRadius-x*x-y*y) + seaRadius)
		c.anchor = c.frameOrigin.Add(c.u.Mul(x)).Add(c.v.Mul(y)).Add(c.frameOrigin.Unit().Mul(rise))
	}
	relative := c.anchor.Sub(eye)
	p.CausticOrigin = [4]float32{float32(relative[0]), float32(relative[1]), float32(relative[2]), 1}
	p.CausticU = [4]float32{float32(c.u[0]), float32(c.u[1]), float32(c.u[2]), causticSpan}
	p.CausticV = [4]float32{float32(c.v[0]), float32(c.v[1]), float32(c.v[2]), causticCoverageRadius}
	return true
}
