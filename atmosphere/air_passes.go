package atmosphere

import (
	_ "embed"
	"encoding/binary"
	"math"

	"github.com/derekmwright/glyphengine/renderer"
	"github.com/derekmwright/glyphengine/shaders"
	"github.com/go-gl/mathgl/mgl32"
)

//go:embed air-scatter.frag.spv
var airScatter []byte

//go:embed air-transmission.frag.spv
var airTransmission []byte

//go:embed air-composite.frag.spv
var airComposite []byte

//go:embed air-present.frag.spv
var airPresent []byte

// AirPasses owns view-path radiance and RGB transmittance separately. Surface
// BRDFs and direct shadows remain full resolution. No temporal history is used.
type AirPasses struct {
	scatter                          *renderer.AppPass
	transmission, composite, present *renderer.AppPass
	scale                            float32
	Clouds                           *CloudPasses
}

func NewAirPasses(r *renderer.Renderer, scale, cloudScale float32, seed uint64) (*AirPasses, error) {
	light, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "air radiance and endpoint", Format: renderer.TargetRGBA16F, Scale: scale})
	if err != nil {
		return nil, err
	}
	transmission, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "air RGB transmission", Format: renderer.TargetRGBA16F, Scale: scale})
	if err != nil {
		return nil, err
	}
	composed, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "atmosphere composed scene", Format: renderer.TargetRGBA16F, Scale: 1})
	if err != nil {
		return nil, err
	}
	depth := r.SceneDepth()
	scatter, err := r.CreateAppPass(renderer.AppPassDesc{Name: "air scattering", Stage: renderer.StageBeforeBloom, Target: light, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: airScatter, Reads: []*renderer.Texture{depth}, Timed: true})
	if err != nil {
		return nil, err
	}
	transmit, err := r.CreateAppPass(renderer.AppPassDesc{Name: "air transmission", Stage: renderer.StageBeforeBloom, Target: transmission, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: airTransmission, Reads: []*renderer.Texture{depth}, Timed: true})
	if err != nil {
		return nil, err
	}

	composite, err := r.CreateAppPass(renderer.AppPassDesc{Name: "air composite", Stage: renderer.StageBeforeBloom, Target: composed, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: airComposite, Reads: []*renderer.Texture{r.SceneColor(), depth, light.Texture(), transmission.Texture()}, Timed: true})
	if err != nil {
		return nil, err
	}
	// Both modes share the full-resolution output and present draw. Register
	// cloud composition before the common present pass, not after it.
	clouds, err := NewCloudPasses(r, cloudScale, seed, composed)
	if err != nil {
		return nil, err
	}
	present, err := r.CreateAppPass(renderer.AppPassDesc{Name: "air present", Stage: renderer.StageBeforeBloom, Load: true, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: airPresent, Reads: []*renderer.Texture{composed.Texture()}, Timed: true})
	if err != nil {
		return nil, err
	}
	p := &AirPasses{scatter, transmit, composite, present, scale, clouds}
	p.SetEnabled(false, false)
	return p, nil
}
func (p *AirPasses) SetEnabled(enabled, clouds bool) {
	p.scatter.SetEnabled(enabled && !clouds)
	p.transmission.SetEnabled(enabled && !clouds)
	p.composite.SetEnabled(enabled && !clouds)
	p.Clouds.SetEnabled(enabled && clouds)
	p.present.SetEnabled(enabled)
}
func (p *AirPasses) Update(inverseVP mgl32.Mat4, sun, color [3]float32) error {
	data := airFrameBytes(inverseVP, sun, color, p.scale)
	// Shaders derive target extent from the live scene-depth texture and scale.
	// This also covers the first frame after resize, before the next Update.
	if err := p.scatter.SetPushConstants(data[:]); err != nil {
		return err
	}
	if err := p.transmission.SetPushConstants(data[:]); err != nil {
		return err
	}
	return p.composite.SetPushConstants(data[:])
}

func airFrameBytes(inverseVP mgl32.Mat4, sun, color [3]float32, scale float32) [112]byte {
	var values [28]float32
	copy(values[:16], inverseVP[:])
	copy(values[16:19], sun[:])
	copy(values[20:23], color[:])
	values[24] = scale
	var data [112]byte
	for i, v := range values {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(v))
	}
	return data
}
