package atmosphere

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/renderer"
	"github.com/derekmwright/glyphengine/shaders"
)

// A translated sequence of broad cloud shadows must not turn into coherent bands
// at the view integrator's sample locations. Compare the production integrator
// against a dense, uniform quadrature. Rows repeat each ray with independent
// spatial sampling: averaging them isolates coherent bias from sampling noise.
func TestCloudAirShadowSamplingGPU(t *testing.T) {
	if os.Getenv("PLANET_GPU_TEST") != "1" {
		t.Skip("set PLANET_GPU_TEST=1 for Vulkan shader checks")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	common, err := os.ReadFile("common.glsl")
	if err != nil {
		t.Fatal(err)
	}
	// Replace only the external visibility field with an analytic fixture. The
	// integration and sample placement remain the actual production shader.
	needle := "return cloudShadowVisibility(cloudShadowTexture,point);"
	if strings.Count(string(common), needle) != 1 {
		t.Fatal("cloud visibility fixture no longer matches the production interface")
	}
	common = []byte(strings.Replace(string(common), needle, `
    if(!fixtureShadows) return 1.0;
    return 0.1+0.9*smoothstep(-0.1,0.1,sin(point.x*(2.0*PI/4.0)+fixturePhase));`, 1))
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "common.glsl"), common, 0600); err != nil {
		t.Fatal(err)
	}
	source := `#version 450
#extension GL_GOOGLE_include_directive : require
float fixturePhase;
bool fixtureShadows;
#include "parameters.glsl"
#include "common.glsl"
layout(location=0) out vec4 outColor;
void main() {
    fixturePhase=gl_FragCoord.x*(2.0*PI/256.0);
    fixtureShadows=true;
    vec3 direction=vec3(1,0,0),sun=vec3(0,0,1);
    float actual=integrateAir(EYE_PLANET,direction,80.0,sun,vec3(1)).light.r;
    // Independent reference: fixed 78 m intervals, no sparse visibility grid.
    float reference=0.0;
    vec2 optical=vec2(0);
    const float dt=80.0/1024.0;
    for(int i=0;i<1024;i++) {
        vec3 p=EYE_PLANET+direction*((float(i)+0.5)*dt);
        vec2 d=densityAt(p),segment=d*dt;
        float phaseR=3.0/(16.0*PI),g=0.76;
        float phaseM=(1.0-g*g)/(4.0*PI*pow(1.0+g*g,1.5));
        float weight=extinction(optical+segment*0.5).r*sunlight(p,sun).r;
        reference+=weight*(BETA_R.r*d.x*phaseR+BETA_M*d.y*phaseM)*dt*SUN_ENERGY*cloudSunVisibility(p);
        optical+=segment;
    }
    fixtureShadows=false;
    float unit=max(integrateAir(EYE_PLANET,direction,80.0,sun,vec3(1)).light.r,0.00001);
    outColor=vec4(0.5+2.0*(actual-reference)/unit,0.5,reference/unit,1);
}`
	path := filepath.Join(dir, "cloud-air-probe.frag")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("glslc", "-O", "--target-env=vulkan1.0", "-I.", path, "-o", path+".spv").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	code, err := os.ReadFile(path + ".spv")
	if err != nil {
		t.Fatal(err)
	}
	g := &cloudAirProbe{t: t, code: code}
	e, err := glyph.New(g, glyph.WithTitle("Cloud haze sampling regression"), glyph.WithWindowSize(256, 128), glyph.WithBackgroundWindow(), glyph.WithValidation(true), glyph.WithMSAA(1), glyph.WithVSync(false), glyph.WithMaxFrames(4))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Destroy()
	e.Run()
	if !g.checked {
		t.Fatal("cloud haze diagnostic not captured")
	}
}

type cloudAirProbe struct {
	t       *testing.T
	code    []byte
	frame   int
	checked bool
}

func (g *cloudAirProbe) Init(e *glyph.Engine) error {
	e.Scene.Env = cloudTestEnvironment{}
	p := FrameParameters(10000000, [3]float64{0, 0, 10001000}, true)
	p.Rayleigh = [4]float32{0.003, 0.004, 0.005}
	p.Mie[0] = 0.001
	data := p.Bytes()
	if err := e.Renderer().SetShaderParameters(data[:]); err != nil {
		return err
	}
	_, err := e.Renderer().CreateAppPass(renderer.AppPassDesc{Name: "cloud haze diagnostic", Stage: renderer.StageBeforeTonemap, Load: true, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: g.code})
	return err
}

func (g *cloudAirProbe) Update(e *glyph.Engine, _ float32) {
	if g.frame == 2 {
		im, err := e.Renderer().CaptureFrame()
		if err != nil {
			g.t.Fatal(err)
		}
		var squared, maxBias float64
		for x := 0; x < 256; x++ {
			var sum float64
			for y := 0; y < 128; y++ {
				c := im.RGBAAt(x, y)
				if c.G < 100 || c.G > 220 || c.B < 80 || c.B > 230 {
					g.t.Fatalf("invalid diagnostic output at %d,%d: %v", x, y, c)
				}
				sum += float64(int(c.R) - int(c.G))
			}
			bias := sum / 128.0
			squared += bias * bias
			maxBias = math.Max(maxBias, math.Abs(bias))
		}
		rms := math.Sqrt(squared / 256.0)
		g.t.Logf("coherent cloud-shadow bias: RMS %.3f, max %.3f (8-bit channel levels)", rms, maxBias)
		// Allow finite 128-row sampling noise and output quantization. Coherent
		// bands survive row averaging; the old midpoint lookup exceeds 60 RMS.
		if rms > 3.0 || maxBias > 8.0 {
			g.t.Errorf("cloud shadow quadrature leaves coherent bands: RMS %.3f, max %.3f", rms, maxBias)
		}
		g.checked = true
	}
	g.frame++
}
