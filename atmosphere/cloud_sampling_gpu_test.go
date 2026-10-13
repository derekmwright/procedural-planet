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

// A translated lighting field in a homogeneous cloud has a smooth radiance
// integral. The production march must not favor part of each sampling interval:
// that bias survives spatial reconstruction and appears as bands. Independent
// rows let us measure that coherent error separately from per-pixel noise.
func TestCloudViewSamplingGPU(t *testing.T) {
	if os.Getenv("PLANET_GPU_TEST") != "1" {
		t.Skip("set PLANET_GPU_TEST=1 for Vulkan shader checks")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	volume, err := os.ReadFile("cloud-volume.comp")
	if err != nil {
		t.Fatal(err)
	}
	text := string(volume)
	start, end := strings.Index(text, "float cloudPhase("), strings.Index(text, "void renderCloud(")
	if start < 0 || end <= start {
		t.Fatal("cloud march entry points changed")
	}
	march := text[start:end]
	start, end = strings.Index(march, "float cloudSunDepth("), strings.Index(march, "void appendAir(")
	if start < 0 || end <= start {
		t.Fatal("cloud lighting fixture no longer matches the production interface")
	}
	// Replace only the external density/illumination field, leaving the sample
	// placement, attenuation and front-to-back integration exactly as shipped.
	march = march[:start] + `float cloudSunDepth(vec3 p,vec3 sun) {
    return 1.5+1.5*sin(p.x*(2.0*PI/0.7)+fixturePhase);
}
` + march[end:]
	source := `#version 450
#extension GL_GOOGLE_include_directive : require
uvec2 cloudPixel;
#define AIR_SAMPLE_PIXEL cloudPixel
#define BUILD_CLOUD_SHADOW
#include "parameters.glsl"
#include "common.glsl"
float fixturePhase;
struct CloudParameters { vec4 layer; vec4 wind; };
const CloudParameters cloud=CloudParameters(vec4(2.2,3,4,0.52),vec4(1,0,96,0));
struct Lighting { vec4 sunColor; };
const Lighting pc=Lighting(vec4(1));
float cloudDensity(vec3 p,bool detail,float footprint) { return 0.01; }
` + march + `
layout(location=0) out vec4 outColor;
void main() {
    cloudPixel=uvec2(gl_FragCoord.xy);
    fixturePhase=gl_FragCoord.x*(2.0*PI/256.0);
    vec3 dir=vec3(1,0,0),sun=vec3(0,0,1);
    Air actual=Air(vec3(0),vec3(1));
    marchCloud(actual,dir,sun,vec2(0,80),0.001);
    // Dense uniform midpoint quadrature, independent of production intervals.
    float reference=0.0;
    const float dt=80.0/4096.0,sigma=0.04;
    float phase=0.85*cloudPhase(0.0,0.65)+0.15*cloudPhase(0.0,-0.2);
    for(int i=0;i<4096;i++) {
        float distance=(float(i)+0.5)*dt;
        vec3 p=EYE_PLANET+dir*distance;
        float tau=cloudSunDepth(p,sun);
        float lit=phase*exp(-tau)+0.28*cloudPhase(0.0,0.32)*exp(-tau*0.3)
            +0.10/(4.0*PI)*exp(-tau*0.1);
        float height=clamp((length(p)-PLANET_RADIUS-2.2)/3.0,0.0,1.0);
        float day=smoothstep(-0.15,0.2,dot(normalize(p),sun));
        float ambient=0.06*mix(0.015,1.0,day)*(0.35+0.65*height);
        reference+=exp(-sigma*distance)*sigma*0.99*(SUN_ENERGY*lit+ambient)*dt;
    }
    outColor=vec4(0.5+2.0*(actual.light.r/reference-1.0),0.5,
        0.5+2.0*(actual.transmittance.r-exp(-sigma*80.0)),1);
}`
	path := filepath.Join(t.TempDir(), "cloud-sampling.frag")
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
	g := &cloudSamplingProbe{t: t, code: code}
	e, err := glyph.New(g, glyph.WithTitle("Cloud view sampling regression"), glyph.WithWindowSize(256, 128), glyph.WithBackgroundWindow(), glyph.WithValidation(true), glyph.WithMSAA(1), glyph.WithVSync(false), glyph.WithMaxFrames(4))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Destroy()
	e.Run()
	if !g.checked {
		t.Fatal("cloud sampling diagnostic not captured")
	}
}

type cloudSamplingProbe struct {
	t       *testing.T
	code    []byte
	frame   int
	checked bool
}

func (g *cloudSamplingProbe) Init(e *glyph.Engine) error {
	e.Scene.Env = cloudTestEnvironment{}
	p := FrameParameters(500000, [3]float64{0, 0, 503000}, true)
	p.Rayleigh = [4]float32{}
	p.Mie[0] = 0
	data := p.Bytes()
	if err := e.Renderer().SetShaderParameters(data[:]); err != nil {
		return err
	}
	_, err := e.Renderer().CreateAppPass(renderer.AppPassDesc{Name: "cloud sampling diagnostic", Stage: renderer.StageBeforeTonemap, Load: true, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: g.code})
	return err
}

func (g *cloudSamplingProbe) Update(e *glyph.Engine, _ float32) {
	if g.frame == 2 {
		im, err := e.Renderer().CaptureFrame()
		if err != nil {
			g.t.Fatal(err)
		}
		var squared, worst float64
		for x := 0; x < 256; x++ {
			var sum float64
			for y := 0; y < 128; y++ {
				c := im.RGBAAt(x, y)
				if c.G < 100 || c.G > 220 || abs(int(c.B)-int(c.G)) > 1 {
					g.t.Fatalf("invalid cloud diagnostic or transmission at %d,%d: %v", x, y, c)
				}
				sum += float64(int(c.R) - int(c.G))
			}
			bias := sum / 128
			squared += bias * bias
			worst = math.Max(worst, math.Abs(bias))
		}
		rms := math.Sqrt(squared / 256)
		g.t.Logf("coherent cloud view bias: RMS %.3f, max %.3f (8-bit channel levels)", rms, worst)
		// Tolerate finite row sampling and capture quantization. The old bounded,
		// shared offset fails at RMS 6.93 / max 13.70 on the reference GPU.
		if rms > 3 || worst > 8 {
			g.t.Errorf("cloud march leaves coherent bands: RMS %.3f, max %.3f", rms, worst)
		}
		g.checked = true
	}
	g.frame++
}
