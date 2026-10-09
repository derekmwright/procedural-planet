package atmosphere

import (
	_ "embed"
	"math"

	"github.com/derekmwright/glyphengine/renderer"
	"github.com/go-gl/mathgl/mgl64"
)

// Four tiles use 2 MiB total. The local projection resolves broad cloud shadows
// at 500 metres per texel; filtering belongs to the density integral as well as
// the final lookup so distant samples do not alias unresolved cloud detail.
const cloudShadowResolution = 256
const cloudShadowLocalHalfSpan = 64.0 // kilometres, independent of camera height

//go:embed cloud-shadow.comp.spv
var cloudShadowCompute []byte

// CloudShadow stores sun-path optical depth at four knots in each shell lobe,
// with local and whole-planet projections. The altitude knots distinguish a
// receiver below a cloud from one inside or above the same cloud.
type CloudShadow struct {
	pass   *renderer.AppCompute
	Target *renderer.RenderTarget
}

func NewCloudShadow(r *renderer.Renderer, noise *renderer.Texture) (*CloudShadow, error) {
	target, err := r.CreateRenderTarget(renderer.RenderTargetDesc{
		Name: "cloud sunlight optical depth", Format: renderer.TargetRGBA16F,
		Width: cloudShadowResolution * 2, Height: cloudShadowResolution * 2,
		Storage: true, Filter: renderer.FilterLinear,
	})
	if err != nil {
		return nil, err
	}
	pass, err := r.CreateAppCompute(renderer.AppComputeDesc{
		Name: "cloud shadows", Stage: renderer.StageBeforeScene, Comp: cloudShadowCompute,
		Reads: []*renderer.Texture{noise, noise}, Writes: []*renderer.RenderTarget{target},
		Timed: true, Params: 32,
	})
	if err != nil {
		return nil, err
	}
	pass.SetDispatch(cloudShadowResolution*2/8, cloudShadowResolution*2/8, 1)
	pass.SetEnabled(false)
	if err := r.SetShaderTarget(4, target); err != nil {
		return nil, err
	}
	return &CloudShadow{pass: pass, Target: target}, nil
}

func (s *CloudShadow) Update(p *Parameters, eye [3]float64, sun [3]float32, radius, seaLevel, seconds, coverage float64, enabled bool) error {
	s.pass.SetEnabled(enabled)
	setCloudShadowProjection(p, eye, sun, radius, seaLevel, enabled)
	data := cloudFrameParameters(radius, seaLevel, seconds, coverage)
	return s.pass.SetParams(data[:])
}

func setCloudShadowProjection(p *Parameters, eye [3]float64, sun [3]float32, radius, seaLevel float64, enabled bool) {
	direction := mgl64.Vec3{float64(sun[0]), float64(sun[1]), float64(sun[2])}.Normalize()
	axis := mgl64.Vec3{0, 1, 0}
	if math.Abs(direction.Dot(axis)) > 0.95 {
		axis = mgl64.Vec3{1, 0, 0}
	}
	u := direction.Cross(axis).Normalize()
	v := direction.Cross(u)
	position := mgl64.Vec3(eye).Mul(0.001)
	texel := 2 * cloudShadowLocalHalfSpan / cloudShadowResolution
	// Snapping moves the projection by complete texels. Existing world samples
	// retain their phase as the camera moves or the render origin rebases.
	centerU := math.Round(position.Dot(u)/texel) * texel
	centerV := math.Round(position.Dot(v)/texel) * texel
	p.CloudShadowU = [4]float32{float32(u[0]), float32(u[1]), float32(u[2]), float32(centerU)}
	p.CloudShadowV = [4]float32{float32(v[0]), float32(v[1]), float32(v[2]), float32(centerV)}
	inner := float32(radius/1000) + float32(seaLevel/1000+2.2)
	p.CloudShadowMeta = [4]float32{inner, inner + 3, cloudShadowLocalHalfSpan, 0}
	if enabled {
		p.CloudShadowMeta[3] = 1
	}
}
