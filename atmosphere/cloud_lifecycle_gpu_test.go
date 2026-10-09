package atmosphere

import (
	"image"
	"os"
	"runtime"
	"testing"

	glyph "github.com/derekmwright/glyphengine"
	"github.com/go-gl/mathgl/mgl32"
)

// Exercise the real sky, compute, composite and shared present passes. A frozen
// cloud field must return unchanged after toggles and target recreation. An
// undersized dispatch models targets growing after LateUpdate during resize.
func TestCloudLifecycleGPU(t *testing.T) {
	if os.Getenv("PLANET_GPU_TEST") != "1" {
		t.Skip("set PLANET_GPU_TEST=1 for Vulkan shader checks")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	g := &cloudLifecycleProbe{t: t}
	e, err := glyph.New(g, glyph.WithTitle("Cloud lifecycle regression"), glyph.WithWindowSize(160, 90), glyph.WithBackgroundWindow(), glyph.WithValidation(true), glyph.WithMSAA(1), glyph.WithVSync(false), glyph.WithShaders(Shaders()), glyph.WithShaderTextureSlots(5), glyph.WithMaxFrames(80))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Destroy()
	e.Run()
	if g.checked != 7 {
		t.Fatalf("checked %d lifecycle states, want 7", g.checked)
	}
}

type cloudTestEnvironment struct{}

func (cloudTestEnvironment) Advance(float32) {}
func (cloudTestEnvironment) State() glyph.EnvironmentState {
	return glyph.EnvironmentState{SunDir: [3]float32{0.8, 0, 0.6}, SunColor: [3]float32{1, 1, 1}, DrawSky: true}
}

type cloudLifecycleProbe struct {
	t       *testing.T
	passes  *AirPasses
	frame   int
	stage   int
	checked int
	cloudy  *image.RGBA
}

func (g *cloudLifecycleProbe) Init(e *glyph.Engine) error {
	e.Scene.Env = cloudTestEnvironment{}
	e.SetCamera(mgl32.Vec3{}, mgl32.Vec3{0, 0.2, 1}, mgl32.Vec3{0, 1, 0})
	var err error
	g.passes, err = NewAirPasses(e.Renderer(), 0.5, 0.5, 7)
	return err
}

func (g *cloudLifecycleProbe) Update(e *glyph.Engine, _ float32) {
	if g.frame > 0 && g.frame%8 == 0 {
		im, err := e.Renderer().CaptureFrame()
		if err != nil {
			g.t.Fatal(err)
		}
		if g.cloudy == nil {
			g.cloudy = im
		} else {
			different := 0
			for y := 0; y < im.Bounds().Dy(); y++ {
				for x := 0; x < im.Bounds().Dx(); x++ {
					a, b := im.RGBAAt(x, y), g.cloudy.RGBAAt(x, y)
					if abs(int(a.R)-int(b.R)) > 1 || abs(int(a.G)-int(b.G)) > 1 || abs(int(a.B)-int(b.B)) > 1 {
						different++
					}
				}
			}
			if g.stage == 1 || g.stage == 5 {
				if different < im.Bounds().Dx()*im.Bounds().Dy()/10 {
					g.t.Errorf("cloud/atmosphere toggle stage %d did not change the view (%d pixels)", g.stage, different)
				}
			} else if different != 0 {
				g.t.Errorf("frozen clouds changed after stage %d: %d pixels", g.stage, different)
			}
		}
		g.checked++
	}
	if g.frame >= 56 {
		e.Close()
		return
	}
	g.stage = g.frame / 8
	if g.frame == 32 {
		e.Renderer().NotifyResize()
	}
	active := g.stage != 5
	clouds := active && g.stage != 1
	g.passes.SetEnabled(active, clouds)
	p := FrameParameters(500000, [3]float64{0, 0, 501000}, active)
	if active {
		p.Rendering[0] = 1
	}
	if clouds {
		p.Rendering[3] = 1
	}
	if err := g.passes.Clouds.Shadows.Update(&p, [3]float64{0, 0, 501000}, (cloudTestEnvironment{}).State().SunDir, 500000, 0, 0, 1, clouds); err != nil {
		g.t.Fatal(err)
	}
	data := p.Bytes()
	if err := e.Renderer().SetShaderParameters(data[:]); err != nil {
		g.t.Fatal(err)
	}
	g.frame++
}

func (g *cloudLifecycleProbe) LateUpdate(e *glyph.Engine, _ float32) {
	state := (cloudTestEnvironment{}).State()
	if err := g.passes.Update(e.ViewProjection().Inv(), state.SunDir, state.SunColor); err != nil {
		g.t.Fatal(err)
	}
	if err := g.passes.Clouds.Update(e.ViewProjection().Inv(), state.SunDir, state.SunColor, 500000, 0, 0, 1); err != nil {
		g.t.Fatal(err)
	}
	if g.stage == 3 {
		g.passes.Clouds.volume.SetDispatch(1, 1, 1)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
