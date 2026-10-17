package atmosphere

import (
	"encoding/binary"
	"fmt"
	"image"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/renderer"
	"github.com/derekmwright/glyphengine/shaders"
	"github.com/go-gl/mathgl/mgl32"
)

// Feed controlled opaque depth and cloud transmission through the production
// composite AND present shaders. Bright foreground/cloud pixels must not seed
// glare, while transmitted and partially occluded sunlight must attenuate it.
func TestSunGlareOcclusionGPU(t *testing.T) {
	if os.Getenv("PLANET_GPU_TEST") != "1" {
		t.Skip("set PLANET_GPU_TEST=1 for Vulkan shader checks")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	source := `#version 450
#extension GL_GOOGLE_include_directive : require
layout(local_size_x=8,local_size_y=8) in;
layout(set=2,binding=4,rgba16f) uniform writeonly image2D scene;
layout(set=2,binding=5,rgba16f) uniform writeonly image2D depth;
layout(set=2,binding=6,rgba16f) uniform writeonly image2D light;
layout(set=2,binding=7,rgba16f) uniform writeonly image2D transmission;
layout(set=2,binding=12,std140) uniform Fixture { vec4 control; } fixture;
#define COMPUTE_PASS
#define BUILD_CLOUD_SHADOW
#include "air-frame.glsl"
#include "parameters.glsl"
#include "common.glsl"
#include "air-endpoint.glsl"
#include "sun-disc.glsl"
void main() {
    ivec2 p=ivec2(gl_GlobalInvocationID.xy),size=imageSize(scene);
    if(any(greaterThanEqual(p,size))) return;
    vec2 uv=(vec2(p)+0.5)/vec2(size);
    float angle=sunSeparation(viewDirection(uv),normalize(pc.sunDir.xyz));
    float disc=sunDisc(angle,0.00015);
    int mode=int(fixture.control.x);
    float z=mode==2||(mode==3&&p.x<size.x/2)?0.5:0.0;
    float transmit=mode==1?0.25:(mode==4||mode==5?0.0:1.0);
    float endpoint=airEndpoint(uv,z);
    // Keep the off-disc background constant across all cases. The foreground
    // and opaque cloud deliberately retain a bright spot at the sun position.
    imageStore(scene,p,vec4(vec3(0.1+SUN_DISC_RADIANCE*disc),1));
    imageStore(depth,p,vec4(z));
    imageStore(light,p,vec4(vec3(0.1*(1.0-transmit)+(mode==5?24.0*disc:0.0)),log2(1.0+endpoint)));
    imageStore(transmission,p,vec4(vec3(transmit),1));
}`
	path := filepath.Join(t.TempDir(), "sun-glare-inputs.comp")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("glslc", "-O", "--target-env=vulkan1.0", "-I.", path, "-o", path+".spv").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	code, err := os.ReadFile(path + ".spv")
	if err != nil {
		t.Fatal(err)
	}
	g := &sunGlareProbe{t: t, code: code}
	e, err := glyph.New(g, glyph.WithTitle("Sun glare occlusion regression"), glyph.WithWindowSize(256, 256), glyph.WithBackgroundWindow(), glyph.WithValidation(true), glyph.WithMSAA(1), glyph.WithVSync(false), glyph.WithProjection(30, 0.1, 100000), glyph.WithShaderTextureSlots(7), glyph.WithMaxFrames(100))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Destroy()
	e.Run()
	if g.checked != 15 {
		t.Fatalf("checked %d glare states, want 15", g.checked)
	}
}

type sunGlareProbe struct {
	t              *testing.T
	code           []byte
	inputs         *renderer.AppCompute
	composite      *renderer.AppPass
	present        *renderer.AppPass
	baseline, full *image.RGBA
	fullEnergy     float64
	frame, checked int
}

func (g *sunGlareProbe) Init(e *glyph.Engine) error {
	e.Scene.Env = cloudTestEnvironment{}
	e.SetCamera(mgl32.Vec3{}, mgl32.Vec3{0, 0, 1}, mgl32.Vec3{0, 1, 0})
	e.Renderer().SetTonemap(0.2, 0, 4)
	r := e.Renderer()
	targets := make([]*renderer.RenderTarget, 4)
	reads := make([]*renderer.Texture, 4)
	for i := range targets {
		var err error
		targets[i], err = r.CreateRenderTarget(renderer.RenderTargetDesc{Name: fmt.Sprintf("glare input %d", i), Format: renderer.TargetRGBA16F, Scale: 1, Storage: true})
		if err != nil {
			return err
		}
		reads[i] = targets[i].Texture()
	}
	composed, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "glare test composed", Format: renderer.TargetRGBA16F, Scale: 1, Filter: renderer.FilterLinear})
	if err != nil {
		return err
	}
	g.inputs, err = r.CreateAppCompute(renderer.AppComputeDesc{Name: "glare fixture", Stage: renderer.StageBeforeBloom, Comp: g.code, Writes: targets, Params: 16})
	if err != nil {
		return err
	}
	g.inputs.SetDispatch(32, 32, 1)
	g.composite, err = r.CreateAppPass(renderer.AppPassDesc{Name: "glare production composite", Stage: renderer.StageBeforeBloom, Target: composed, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: airComposite, Reads: reads})
	if err != nil {
		return err
	}
	g.present, err = r.CreateAppPass(renderer.AppPassDesc{Name: "glare production present", Stage: renderer.StageBeforeBloom, Load: true, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: airPresent, Reads: []*renderer.Texture{composed.Texture()}})
	return err
}

func (g *sunGlareProbe) Update(e *glyph.Engine, _ float32) {
	if g.frame > 0 && g.frame%6 == 0 {
		im, err := e.Renderer().CaptureFrame()
		if err != nil {
			g.t.Fatal(err)
		}
		g.check(im, g.frame/6-1)
		g.checked++
	}
	if g.checked == 15 {
		e.Close()
		return
	}
	stage := g.frame / 6
	if stage == 12 && g.frame%6 == 0 {
		e.Renderer().NotifyResize()
	}
	p := FrameParameters(500000, [3]float64{0, 0, 501000}, true)
	p.Rayleigh = [4]float32{}
	p.Mie[0] = 0
	p.Rendering[3], p.Features[1] = 1, 1
	mode := 0
	if stage >= 2 && stage <= 6 {
		mode = stage - 1
	}
	if stage == 0 || stage == 9 {
		p.Features[1] = 0
	}
	if stage == 7 { // Sun behind the solid planet, with a clear depth buffer.
		p.Eye[2] = -501
	}
	if stage == 8 { // Grazing ocean: misses the smaller solid-planet sphere.
		p.Eye = [4]float32{500.5, 0, -30}
		p.SetOcean(500000, 1000, [3]float64{500500, 0, -30000})
	}
	if stage == 10 {
		p.Planet[2] = 0
	}
	if stage == 13 {
		p.Rendering[3] = 0
	}
	bytes := p.Bytes()
	if err := e.Renderer().SetShaderParameters(bytes[:]); err != nil {
		g.t.Fatal(err)
	}
	var control [16]byte
	binary.LittleEndian.PutUint32(control[:], math.Float32bits(float32(mode)))
	if err := g.inputs.SetParams(control[:]); err != nil {
		g.t.Fatal(err)
	}
	g.frame++
}

func (g *sunGlareProbe) LateUpdate(e *glyph.Engine, _ float32) {
	data := airFrameBytes(e.ViewProjection().Inv(), [3]float32{0, 0, 1}, [3]float32{1, 1, 1}, 1)
	for _, err := range []error{g.inputs.SetPushConstants(data[:]), g.composite.SetPushConstants(data[:]), g.present.SetPushConstants(data[:])} {
		if err != nil {
			g.t.Fatal(err)
		}
	}
}

func (g *sunGlareProbe) check(im *image.RGBA, stage int) {
	if stage == 0 {
		g.baseline = im
		return
	}
	linear := func(v byte) float64 {
		x := float64(v) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	energy, count := 0.0, 0
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			radius := math.Hypot(float64(x)+0.5-128, float64(y)+0.5-128)
			// Outside the antialiased disc, within the compact corona/spoke roots.
			// The old 14..48 ring mostly measures the broad halo we removed.
			if radius < 7 || radius > 24 {
				continue
			}
			energy += linear(im.RGBAAt(x, y).R) - linear(g.baseline.RGBAAt(x, y).R)
			count++
		}
	}
	energy /= float64(count)
	if stage == 1 {
		g.full, g.fullEnergy = im, energy
		if energy < 0.005 {
			g.t.Fatalf("visible sun has no appreciable glare: %.6f", energy)
		}
		return
	}
	ratio := energy / g.fullEnergy
	g.t.Logf("state %d: glare energy %.6f, fraction %.4f", stage, energy, ratio)
	switch stage {
	case 2:
		if math.Abs(ratio-0.25) > 0.04 {
			g.t.Errorf("thin cloud glare fraction %.4f, want 0.25", ratio)
		}
	case 4:
		if math.Abs(ratio-0.5) > 0.12 {
			g.t.Errorf("half-occluded glare fraction %.4f, want about 0.5", ratio)
		}
	case 11, 12, 14:
		for i, v := range im.Pix {
			if abs(int(v)-int(g.full.Pix[i])) > 1 {
				g.t.Errorf("glare did not restore after toggle/resize (state %d)", stage)
				break
			}
		}
	default:
		if math.Abs(energy) > 0.0001 {
			g.t.Errorf("blocked or disabled sun still produces glare in state %d: %.6f", stage, energy)
		}
	}
}
