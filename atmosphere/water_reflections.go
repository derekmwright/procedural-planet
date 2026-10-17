package atmosphere

import (
	_ "embed"

	"github.com/derekmwright/glyphengine/renderer"
	"github.com/derekmwright/glyphengine/shaders"
	"github.com/go-gl/mathgl/mgl32"
)

//go:embed water-reflection.frag.spv
var waterReflection []byte

//go:embed water-reflection-composite.frag.spv
var waterReflectionComposite []byte

//go:embed water-reflection-filter.frag.spv
var waterReflectionFilter []byte

// WaterReflections adds nearby opaque land to the existing sky reflection.
// Register before AirPasses: the source is unfogged scene radiance, and the
// signed HDR correction must precede eye-path atmosphere/cloud composition.
// No history, second scene render, or additional shared texture slot is needed.
// As with WaterSky, the renderer needs seven shared shader texture slots.
type WaterReflections struct {
	trace, filter, composite *renderer.AppPass
	active                   bool
}

func NewWaterReflections(r *renderer.Renderer) (*WaterReflections, error) {
	return newWaterReflections(r, r.SceneDepth())
}

func newWaterReflections(r *renderer.Renderer, depth *renderer.Texture) (*WaterReflections, error) {
	target, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "water land reflection correction", Format: renderer.TargetRGBA16F, Scale: 0.5})
	if err != nil {
		return nil, err
	}
	trace, err := r.CreateAppPass(renderer.AppPassDesc{Name: "water land reflection", Stage: renderer.StageBeforeBloom, Target: target, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: waterReflection, Reads: []*renderer.Texture{r.SceneColor(), depth}, Timed: true})
	if err != nil {
		return nil, err
	}
	filtered, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "water land filtered correction", Format: renderer.TargetRGBA16F, Scale: 0.5})
	if err != nil {
		return nil, err
	}
	filter, err := r.CreateAppPass(renderer.AppPassDesc{Name: "water land filter", Stage: renderer.StageBeforeBloom, Target: filtered, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: waterReflectionFilter, Reads: []*renderer.Texture{target.Texture()}, Timed: true})
	if err != nil {
		return nil, err
	}
	composite, err := r.CreateAppPass(renderer.AppPassDesc{Name: "water land composite", Stage: renderer.StageBeforeBloom, Load: true, Blend: renderer.BlendAdditive, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: waterReflectionComposite, Reads: []*renderer.Texture{filtered.Texture(), depth}, Timed: true})
	if err != nil {
		return nil, err
	}
	p := &WaterReflections{trace: trace, filter: filter, composite: composite}
	p.trace.SetEnabled(false)
	p.filter.SetEnabled(false)
	p.composite.SetEnabled(false)
	return p, nil
}

func (p *WaterReflections) SetEnabled(parameters Parameters, inverseVP mgl32.Mat4, enabled bool) {
	unfogged := parameters.Planet[2] < 0.5 || parameters.Rendering[0] > 0.5
	p.active = enabled && unfogged && parameters.Water[0] > 0 && parameters.Detail[0] >= 0 && parameters.Detail[0] < 12000 && waterSkyVisible(parameters, inverseVP)
	p.trace.SetEnabled(p.active)
	p.filter.SetEnabled(p.active)
	p.composite.SetEnabled(p.active)
}

func (p *WaterReflections) Update(inverseVP mgl32.Mat4, sun, color [3]float32) error {
	if !p.active {
		return nil
	}
	data := airFrameBytes(inverseVP, sun, color, 0.5)
	if err := p.trace.SetPushConstants(data[:]); err != nil {
		return err
	}
	return p.composite.SetPushConstants(data[:])
}

func (p *WaterReflections) Active() bool { return p != nil && p.active }
