package atmosphere

import (
	_ "embed"
	"encoding/binary"
	"math"

	"github.com/derekmwright/glyphengine/renderer"
	"github.com/derekmwright/glyphengine/shaders"
	"github.com/go-gl/mathgl/mgl32"
)

//go:embed cloud-volume.comp.spv
var cloudVolume []byte

// CloudPasses replaces the clear-air view passes while active. It integrates
// cloud and air extinction/scattering in depth order, then reuses the depth-aware
// reconstruction. There is deliberately no temporal history in this first path.
type CloudPasses struct {
	Shadows   *CloudShadow
	volume    *renderer.AppCompute
	composite *renderer.AppPass
	light     *renderer.RenderTarget
	scale     float32
}

// NewCloudPasses requires at least five shared shader texture slots. The full
// showcase registers seventeen application timers and requests a capacity of 32.
func NewCloudPasses(r *renderer.Renderer, scale float32, seed uint64, composed *renderer.RenderTarget) (*CloudPasses, error) {
	noise, err := r.CreateTextureLinear(cloudNoiseAtlas(seed), cloudNoiseWidth, cloudNoiseHeight)
	if err != nil {
		return nil, err
	}
	shadow, err := NewCloudShadow(r, noise)
	if err != nil {
		return nil, err
	}
	light, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "cloud and air radiance", Format: renderer.TargetRGBA16F, Scale: scale, Storage: true})
	if err != nil {
		return nil, err
	}
	transmission, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "cloud and air transmission", Format: renderer.TargetRGBA16F, Scale: scale, Storage: true})
	if err != nil {
		return nil, err
	}
	depth := r.SceneDepth()
	volume, err := r.CreateAppCompute(renderer.AppComputeDesc{Name: "cloud volume", Stage: renderer.StageBeforeBloom, Comp: cloudVolume, Reads: []*renderer.Texture{depth, noise}, Writes: []*renderer.RenderTarget{light, transmission}, ReadsShadows: true, Timed: true, Params: 32})
	if err != nil {
		return nil, err
	}
	composite, err := r.CreateAppPass(renderer.AppPassDesc{Name: "cloud composite", Stage: renderer.StageBeforeBloom, Target: composed, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: airComposite, Reads: []*renderer.Texture{r.SceneColor(), depth, light.Texture(), transmission.Texture()}, Timed: true})
	if err != nil {
		return nil, err
	}
	p := &CloudPasses{Shadows: shadow, volume: volume, composite: composite, light: light, scale: scale}
	p.SetEnabled(false)
	return p, nil
}
func (p *CloudPasses) SetEnabled(enabled bool) {
	p.volume.SetEnabled(enabled)
	p.composite.SetEnabled(enabled)
}
func (p *CloudPasses) Update(inverseVP mgl32.Mat4, sun, color [3]float32, radius, seaLevel, seconds, coverage float64) error {
	data := airFrameBytes(inverseVP, sun, color, p.scale)
	if err := p.volume.SetPushConstants(data[:]); err != nil {
		return err
	}
	if err := p.composite.SetPushConstants(data[:]); err != nil {
		return err
	}
	params := cloudFrameParameters(radius, seaLevel, seconds, coverage)
	if err := p.volume.SetParams(params[:]); err != nil {
		return err
	}
	w, h := p.light.Extent()
	p.volume.SetDispatch((w+7)/8, (h+7)/8, 1)
	return nil
}

// The view and shadow passes must use exactly the same field and wind clock.
func cloudFrameParameters(radius, seaLevel, seconds, coverage float64) [32]byte {
	// Never use the short, wrapped wave clock: that would reset cloud motion.
	angle := math.Remainder(seconds*15/radius, 2*math.Pi)
	values := [8]float32{float32(seaLevel/1000 + 2.2), 3, 4, float32(coverage), float32(math.Cos(angle)), float32(math.Sin(angle)), 96, 0}
	var params [32]byte
	for i, v := range values {
		binary.LittleEndian.PutUint32(params[i*4:], math.Float32bits(v))
	}
	return params
}
