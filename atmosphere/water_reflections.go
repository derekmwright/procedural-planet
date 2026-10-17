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

//go:embed water-reflection-source.frag.spv
var waterReflectionSource []byte

// WaterReflections adds nearby opaque land to the existing sky reflection.
// Register before AirPasses: the source is unfogged scene radiance, and the
// reflection must precede eye-path atmosphere/cloud composition. Snapshot HDR
// before tracing so the final pass can replace reflected light at full resolution
// without reading its own output or filtering signed colour differences.
// No history, second scene render, or additional shared texture slot is needed.
// As with WaterSky, the renderer needs seven shared shader texture slots.
type WaterReflections struct {
	source, trace, composite *renderer.AppPass
	scene                    *renderer.RenderTarget
	active                   bool
}

func NewWaterReflections(r *renderer.Renderer) (*WaterReflections, error) {
	return newWaterReflections(r, r.SceneDepth())
}

func newWaterReflections(r *renderer.Renderer, depth *renderer.Texture) (*WaterReflections, error) {
	scene, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "water source and atmosphere scene", Format: renderer.TargetRGBA16F, Scale: 1, Filter: renderer.FilterLinear})
	if err != nil {
		return nil, err
	}
	source, err := r.CreateAppPass(renderer.AppPassDesc{Name: "water land source", Stage: renderer.StageBeforeBloom, Target: scene, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: waterReflectionSource, Reads: []*renderer.Texture{r.SceneColor()}, Timed: true})
	if err != nil {
		return nil, err
	}
	target, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "water land reflected radiance", Format: renderer.TargetRGBA16F, Scale: 0.5})
	if err != nil {
		return nil, err
	}
	trace, err := r.CreateAppPass(renderer.AppPassDesc{Name: "water land reflection", Stage: renderer.StageBeforeBloom, Target: target, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: waterReflection, Reads: []*renderer.Texture{scene.Texture(), depth}, Timed: true})
	if err != nil {
		return nil, err
	}
	composite, err := r.CreateAppPass(renderer.AppPassDesc{Name: "water land composite", Stage: renderer.StageBeforeBloom, Load: true, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: waterReflectionComposite, Reads: []*renderer.Texture{target.Texture(), depth, scene.Texture()}, Timed: true})
	if err != nil {
		return nil, err
	}
	p := &WaterReflections{source: source, trace: trace, composite: composite, scene: scene}
	p.source.SetEnabled(false)
	p.trace.SetEnabled(false)
	p.composite.SetEnabled(false)
	return p, nil
}

func (p *WaterReflections) SetEnabled(parameters Parameters, inverseVP mgl32.Mat4, enabled bool) {
	unfogged := parameters.Planet[2] < 0.5 || parameters.Rendering[0] > 0.5
	p.active = enabled && unfogged && parameters.Water[0] > 0 && parameters.Detail[0] >= 0 && parameters.Detail[0] < 12000 && waterSkyVisible(parameters, inverseVP)
	p.source.SetEnabled(p.active)
	p.trace.SetEnabled(p.active)
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
