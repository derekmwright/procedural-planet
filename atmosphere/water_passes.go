package atmosphere

import (
	_ "embed"
	"encoding/binary"
	"github.com/derekmwright/glyphengine/renderer"
	"github.com/derekmwright/glyphengine/shaders"
	"github.com/go-gl/mathgl/mgl32"
	"math"
)

//go:embed water-scatter.frag.spv
var waterScatter []byte

//go:embed water-composite.frag.spv
var waterComposite []byte

// WaterPasses separates single scattering from opaque surface shading.
// The renderer owns the targets and passes until its destruction.
type WaterPasses struct{ scatter, composite *renderer.AppPass }

func NewWaterPasses(r *renderer.Renderer) (*WaterPasses, error) {
	target, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "underwater light", Format: renderer.TargetRGBA16F, Scale: 0.5})
	if err != nil {
		return nil, err
	}
	depth := r.SceneDepth()
	scatter, err := r.CreateAppPass(renderer.AppPassDesc{Name: "water scattering", Stage: renderer.StageBeforeBloom, Target: target, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: waterScatter, Reads: []*renderer.Texture{depth}, Timed: true})
	if err != nil {
		return nil, err
	}
	composite, err := r.CreateAppPass(renderer.AppPassDesc{Name: "water composite", Stage: renderer.StageBeforeBloom, Load: true, Blend: renderer.BlendAdditive, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: waterComposite, Reads: []*renderer.Texture{target.Texture(), depth}, Timed: true})
	if err != nil {
		return nil, err
	}
	p := &WaterPasses{scatter, composite}
	p.SetEnabled(false)
	return p, nil
}

func (p *WaterPasses) SetEnabled(enabled bool) {
	p.scatter.SetEnabled(enabled)
	p.composite.SetEnabled(enabled)
}

func (p *WaterPasses) Update(inverseVP mgl32.Mat4, sun, color [3]float32) error {
	var values [24]float32
	copy(values[:16], inverseVP[:])
	copy(values[16:19], sun[:])
	copy(values[20:23], color[:])
	var data [96]byte
	for i, v := range values {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(v))
	}
	return p.scatter.SetPushConstants(data[:])
}
