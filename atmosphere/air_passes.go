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
	composed, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "atmosphere composed scene", Format: renderer.TargetRGBA16F, Scale: 1, Filter: renderer.FilterLinear})
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
	present, err := r.CreateAppPass(renderer.AppPassDesc{Name: "air present", Stage: renderer.StageBeforeBloom, Load: true, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: airPresent, Reads: []*renderer.Texture{composed.Texture(), depth}, Timed: true})
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
	if err := p.composite.SetPushConstants(data[:]); err != nil {
		return err
	}
	return p.present.SetPushConstants(data[:])
}

func airFrameBytes(inverseVP mgl32.Mat4, sun, color [3]float32, scale float32) [128]byte {
	var values [32]float32
	copy(values[:16], inverseVP[:])
	copy(values[16:19], sun[:])
	copy(values[20:23], color[:])
	values[24] = scale
	center, axes, valid := projectSunDisc(inverseVP, mgl32.Vec3(sun))
	if valid {
		values[25], values[26], values[27] = center[0], center[1], 1
		copy(values[28:], axes[:])
	}
	var data [128]byte
	for i, v := range values {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(v))
	}
	return data
}

// Project the angular solar disc once on the CPU. The rendering camera is at
// the origin; w=0 projects a direction, including when the planet is rebased.
// Sample axes follow a tangent plane, accurate over the 0.009-radian disc.
func projectSunDisc(inverseVP mgl32.Mat4, sun mgl32.Vec3) (center mgl32.Vec2, axes mgl32.Vec4, valid bool) {
	if sun.LenSqr() == 0 {
		return
	}
	sun = sun.Normalize()
	viewProjection := inverseVP.Inv()
	project := func(direction mgl32.Vec3) (mgl32.Vec2, bool) {
		p := viewProjection.Mul4x1(direction.Vec4(0))
		if p[3] <= 1e-5 {
			return mgl32.Vec2{}, false
		}
		return mgl32.Vec2{0.5 + 0.5*p[0]/p[3], 0.5 + 0.5*p[1]/p[3]}, true
	}
	center, valid = project(sun)
	if !valid || center[0] < -0.1 || center[0] > 1.1 || center[1] < -0.1 || center[1] > 1.1 {
		valid = false
		return
	}
	u := sun.Cross(mgl32.Vec3{0, 1, 0})
	if u.LenSqr() < 0.001 {
		u = sun.Cross(mgl32.Vec3{1, 0, 0})
	}
	u = u.Normalize().Mul(float32(math.Tan(0.009)))
	v := sun.Cross(u)
	pu, okU := project(sun.Add(u))
	mu, okMU := project(sun.Sub(u))
	pv, okV := project(sun.Add(v))
	mv, okMV := project(sun.Sub(v))
	// Symmetric axes avoid biasing the source footprint toward one side of
	// the disc in wide views near a screen edge.
	axes = mgl32.Vec4{(pu[0] - mu[0]) * 0.5, (pu[1] - mu[1]) * 0.5, (pv[0] - mv[0]) * 0.5, (pv[1] - mv[1]) * 0.5}
	valid = okU && okMU && okV && okMV
	return
}
