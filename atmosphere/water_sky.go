package atmosphere

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/derekmwright/glyphengine/renderer"
	"github.com/go-gl/mathgl/mgl32"
)

//go:embed water-sky.comp.spv
var waterSkyCompute []byte

//go:embed water-sky-filter.comp.spv
var waterSkyFilter []byte

// WaterSky caches the actual cloud/air model at the sea below the camera. It
// refreshes every face before the scene, avoiding mixed ages during motion,
// cloud toggles and origin rebases. It needs seven shared texture slots.
type WaterSky struct {
	build   *renderer.AppCompute
	filters [6]*renderer.AppCompute
	active  bool
}

func NewWaterSky(r *renderer.Renderer, clouds *CloudPasses) (*WaterSky, error) {
	var ping, pong [2]*renderer.RenderTarget
	for i, name := range []string{"radiance", "transmission"} {
		var err error
		ping[i], err = r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "water sky " + name, Format: renderer.TargetRGBA16F, Width: 198, Height: 282, Storage: true, Filter: renderer.FilterLinear})
		if err != nil {
			return nil, err
		}
		pong[i], err = r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "water sky scratch " + name, Format: renderer.TargetRGBA16F, Width: 198, Height: 282, Storage: true, Filter: renderer.FilterLinear})
		if err != nil {
			return nil, err
		}
		if err := r.SetShaderTarget(5+i, ping[i]); err != nil {
			return nil, err
		}
	}
	build, err := r.CreateAppCompute(renderer.AppComputeDesc{
		Name: "water sky", Stage: renderer.StageBeforeScene, Comp: waterSkyCompute,
		Reads: []*renderer.Texture{clouds.noise, clouds.noise}, Reads3D: []int{0, 1}, Writes: ping[:],
		ReadsShadows: true, Timed: true, Params: 32,
	})
	if err != nil {
		return nil, err
	}
	s := &WaterSky{build: build}
	// Each level reduces four taps from the preceding one. Ping-pong whole
	// atlases because the renderer has no independently addressable mip views.
	// Six reductions leave the final atlas in ping, bound to the scene above.
	for i := range s.filters {
		source, dest := ping, pong
		if i%2 != 0 {
			source, dest = pong, ping
		}
		filter, err := r.CreateAppCompute(renderer.AppComputeDesc{
			Name: fmt.Sprintf("water sky filter %d", i+1), Stage: renderer.StageBeforeScene, Comp: waterSkyFilter,
			Reads: []*renderer.Texture{source[0].Texture(), source[1].Texture()}, Writes: dest[:], Timed: true, Params: 16,
		})
		if err != nil {
			return nil, err
		}
		var params [16]byte
		binary.LittleEndian.PutUint32(params[:4], uint32(i+1))
		if err := filter.SetParams(params[:]); err != nil {
			return nil, err
		}
		filter.SetDispatch(25, 36, 1)
		filter.SetEnabled(false)
		s.filters[i] = filter
	}
	build.SetDispatch(25, 36, 1)
	build.SetEnabled(false)
	return s, nil
}

func waterSkyStrength(height float64) float32 {
	t := math.Max(0, math.Min(1, (math.Abs(height)-8000)/24000))
	return float32(1 - t*t*(3-2*t))
}

// maxFrustumAlignment bounds every ray, including extrema in the middle of a
// frustum edge. Checking corners alone can incorrectly cull an overhead window.
func maxFrustumAlignment(inverseVP mgl32.Mat4, axis mgl32.Vec3) float64 {
	var corners [4]mgl32.Vec3
	for i, xy := range [4][2]float32{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
		v := inverseVP.Mul4x1(mgl32.Vec4{xy[0], xy[1], 0, 1})
		corners[i] = mgl32.Vec3{v[0], v[1], v[2]}.Normalize()
	}
	v := inverseVP.Mul4x1(mgl32.Vec4{0, 0, 0, 1})
	center := mgl32.Vec3{v[0], v[1], v[2]}.Normalize()
	best, inside := float32(-1), true
	for i, a := range corners {
		b := corners[(i+1)%4]
		n := a.Cross(b).Normalize()
		if n.Dot(center) < 0 {
			n = n.Mul(-1)
		}
		inside = inside && n.Dot(axis) >= 0
		best = max(best, a.Dot(axis))
		projected := axis.Sub(n.Mul(n.Dot(axis)))
		if projected.LenSqr() > 1e-10 {
			candidate, middle := projected.Normalize(), a.Add(b).Normalize()
			if candidate.Dot(middle) >= a.Dot(middle)-1e-6 {
				best = max(best, candidate.Dot(axis))
			}
		}
	}
	if inside {
		return 1
	}
	return math.Min(1, float64(best)+1e-5)
}

func waterSkyVisible(p Parameters, inverseVP mgl32.Mat4) bool {
	up := mgl32.Vec3{p.Eye[0], p.Eye[1], p.Eye[2]}.Normalize()
	radius, height := float64(p.Water[0]), float64(p.Detail[0])*0.001
	if radius <= 0 {
		return false
	}
	if height < 0 {
		// Match ocean.glsl's WATER_OPAQUE_DISTANCE (0.8 km): beyond it
		// underwaterWindow never reads the sky. This needs no assumption about
		// wave tilt or the shape of Snell's window, so it remains conservative.
		const distance = 0.8
		required := (-height*(2*radius+height) - distance*distance) / (2 * (radius + height) * distance)
		return maxFrustumAlignment(inverseVP, up) >= required-1e-5
	}
	ratio := radius / (radius + height)
	horizon := -math.Sqrt(math.Max(0, 1-ratio*ratio))
	return -maxFrustumAlignment(inverseVP, up.Mul(-1)) <= horizon+1e-5
}

func (s *WaterSky) Active() bool { return s.active }

func (s *WaterSky) Update(p *Parameters, inverseVP mgl32.Mat4, sun, color [3]float32, radius, seaLevel, seconds, coverage float64, clouds, enabled bool) error {
	strength := float32(0)
	if enabled && waterSkyVisible(*p, inverseVP) {
		strength = waterSkyStrength(float64(p.Detail[0]))
	}
	p.WaterRendering[0] = strength
	p.WaterRendering[2] = float32((radius+seaLevel)/1000 + 3.4)
	s.active = strength > 0
	s.build.SetEnabled(s.active)
	for _, filter := range s.filters {
		filter.SetEnabled(s.active)
	}
	if !s.active {
		return nil
	}
	if !clouds {
		coverage = 0
	}
	params := cloudFrameParameters(radius, seaLevel, seconds, coverage)
	if err := s.build.SetParams(params[:]); err != nil {
		return err
	}
	push := airFrameBytes(mgl32.Ident4(), sun, color, 1)
	return s.build.SetPushConstants(push[:])
}
