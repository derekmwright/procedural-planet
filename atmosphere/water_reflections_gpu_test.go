package atmosphere

import (
	"encoding/binary"
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

// Controlled coloured land above an analytic sea exercises the real trace,
// signed HDR blend, source/depth rejection and resource lifecycle. No rendered
// noise field or atmospheric lighting can accidentally satisfy the assertions.
func TestWaterLandReflectionsGPU(t *testing.T) {
	if os.Getenv("PLANET_GPU_TEST") != "1" {
		t.Skip("set PLANET_GPU_TEST=1 for Vulkan shader checks")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	compile := func(name, source string) []byte {
		path := filepath.Join(t.TempDir(), name)
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
		return code
	}
	fixture := compile("water-land-inputs.comp", `#version 450
#extension GL_GOOGLE_include_directive : require
layout(local_size_x=8,local_size_y=8) in;
layout(set=2,binding=4,rgba16f) uniform writeonly image2D scene;
layout(set=2,binding=5,r32f) uniform writeonly image2D depth;
layout(set=2,binding=12,std140) uniform Fixture { vec4 control; } fixture;
#define READ_VIEW_PROJECTION
#include "air-frame.glsl"
void main() {
    ivec2 p=ivec2(gl_GlobalInvocationID.xy),size=imageSize(scene);
    if(any(greaterThanEqual(p,size))) return;
    vec2 uv=(vec2(p)+0.5)/vec2(size);
    vec3 direction=normalize((pc.inverseVP*vec4(uv*2-1,0,1)).xyz);
    float t=direction.y>0?800.0/direction.y:1e20;
    vec3 wall=direction*t;
    int mode=int(fixture.control.x);
    bool land=abs(wall.x)<160.0&&wall.z>-100.0&&wall.z<150.0;
    if(mode==2) land=false;
    if(mode==3) land=abs(wall.x)<160.0&&wall.z<-100.5&&wall.z>-350.0;
    vec3 color=vec3(0.08);
    float z=0.0;
    if(land) {
        vec4 q=pc.viewProjection*vec4(wall,1);
        z=q.z/q.w;
        color=mode==1?vec3(0.01,0.8,0.01):vec3(0.8,0.01,0.01);
    }
    imageStore(scene,p,vec4(color,1));
    imageStore(depth,p,vec4(z));
}`)
	copy := compile("water-land-input-copy.frag", `#version 450
layout(set=2,binding=0) uniform sampler2D source;
layout(location=0) out vec4 color;
void main() { color=texelFetch(source,ivec2(gl_FragCoord.xy),0); }`)
	g := &waterLandProbe{t: t, fixture: fixture, copy: copy}
	e, err := glyph.New(g, glyph.WithTitle("Water land reflection regression"), glyph.WithBackgroundWindow(), glyph.WithWindowSize(320, 240), glyph.WithValidation(true), glyph.WithMSAA(4), glyph.WithVSync(false), glyph.WithProjection(60, 0.1, 100000), glyph.WithShaderTextureSlots(7), glyph.WithMaxFrames(104))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Destroy()
	e.Run()
	if g.checked != 12 {
		t.Fatalf("checked %d states, want 12", g.checked)
	}
}

type waterLandProbe struct {
	t                   *testing.T
	fixture, copy       []byte
	inputs              *renderer.AppCompute
	reflections         *WaterReflections
	frame, checked      int
	baseline, reflected *image.RGBA
	submerged           *image.RGBA
}

func (g *waterLandProbe) Init(e *glyph.Engine) error {
	e.Scene.Env = cloudTestEnvironment{}
	e.SetCamera(mgl32.Vec3{}, mgl32.Vec3{0, 1, -0.15}, mgl32.Vec3{0, 0, 1})
	r := e.Renderer()
	r.SetTonemap(0.4, 0, 4)
	color, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "reflection fixture color", Format: renderer.TargetRGBA16F, Scale: 1, Storage: true})
	if err != nil {
		return err
	}
	depth, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "reflection fixture depth", Format: renderer.TargetR32F, Scale: 1, Storage: true})
	if err != nil {
		return err
	}
	g.inputs, err = r.CreateAppCompute(renderer.AppComputeDesc{Name: "reflection fixture", Stage: renderer.StageBeforeBloom, Comp: g.fixture, Writes: []*renderer.RenderTarget{color, depth}, Params: 16})
	if err != nil {
		return err
	}
	g.inputs.SetDispatch(40, 30, 1)
	_, err = r.CreateAppPass(renderer.AppPassDesc{Name: "reflection fixture scene", Stage: renderer.StageBeforeBloom, Load: true, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: g.copy, Reads: []*renderer.Texture{color.Texture()}})
	if err != nil {
		return err
	}
	g.reflections, err = newWaterReflections(r, depth.Texture())
	return err
}

func (g *waterLandProbe) Update(e *glyph.Engine, _ float32) {
	if g.frame > 0 && g.frame%8 == 0 {
		im, err := e.Renderer().CaptureFrame()
		if err != nil {
			g.t.Fatal(err)
		}
		g.check(im, g.frame/8-1)
		g.checked++
	}
	if g.checked == 12 {
		e.Close()
		return
	}
	stage := g.frame / 8
	look := mgl32.Vec3{0, 1, -0.15}
	if stage == 10 {
		look = mgl32.Vec3{1, 0, -0.15}
	}
	e.SetCamera(mgl32.Vec3{}, look, mgl32.Vec3{0, 0, 1})
	if stage == 7 && g.frame%8 == 0 {
		e.Renderer().NotifyResize()
	}
	p := FrameParameters(500000, [3]float64{0, 0, 500100}, false)
	p.SetOcean(500000, 0, [3]float64{0, 0, 500100})
	if stage == 8 {
		p.SetOcean(500000, 200, [3]float64{0, 0, 500100})
	}
	g.reflections.SetEnabled(p, e.ViewProjection().Inv(), stage != 0 && stage != 4)
	if g.reflections.Active() != (stage != 0 && stage != 4 && stage != 8) {
		g.t.Errorf("unexpected active state %d", stage)
	}
	b := p.Bytes()
	if err := e.Renderer().SetShaderParameters(b[:]); err != nil {
		g.t.Fatal(err)
	}
	var control [16]byte
	mode := 0
	if stage >= 2 && stage <= 4 {
		mode = stage - 1
	}
	if stage == 5 {
		mode = 3
	}
	binary.LittleEndian.PutUint32(control[:], math.Float32bits(float32(mode)))
	if err := g.inputs.SetParams(control[:]); err != nil {
		g.t.Fatal(err)
	}
	g.frame++
}

func (g *waterLandProbe) LateUpdate(e *glyph.Engine, _ float32) {
	data := airFrameBytes(e.ViewProjection().Inv(), [3]float32{0, 0, 1}, [3]float32{1, 1, 1}, 0.5)
	if err := g.inputs.SetPushConstants(data[:]); err != nil {
		g.t.Fatal(err)
	}
	if err := g.reflections.Update(e.ViewProjection().Inv(), [3]float32{0, 0, 1}, [3]float32{1, 1, 1}); err != nil {
		g.t.Fatal(err)
	}
}

func (g *waterLandProbe) check(im *image.RGBA, stage int) {
	if stage == 0 {
		g.baseline = im
		return
	}
	if stage == 1 {
		g.reflected = im
		// The pass may change water only, never the directly visible land/sky.
		for y := 20; y < 110; y++ {
			for x := 40; x < 280; x++ {
				a, b := im.RGBAAt(x, y), g.baseline.RGBAAt(x, y)
				if abs(int(a.R)-int(b.R)) > 1 || abs(int(a.G)-int(b.G)) > 1 || abs(int(a.B)-int(b.B)) > 1 {
					g.t.Fatalf("reflection changed foreground/sky at %d,%d", x, y)
				}
			}
		}
	}
	if stage == 4 {
		g.submerged = im
		return
	}
	reference := g.baseline
	if stage == 5 {
		reference = g.submerged
	}
	changed, wrong := 0, 0
	// The wall occupies the upper view. The lower strip only contains water,
	// including the reflection's expected location, never direct wall colour.
	for y := 150; y < 225; y++ {
		for x := 40; x < 280; x++ {
			a, b := im.RGBAAt(x, y), reference.RGBAAt(x, y)
			red, green := int(a.R)-int(b.R), int(a.G)-int(b.G)
			if stage == 2 {
				red, green = green, red
			}
			if red > 8 {
				changed++
			}
			if green > 4 {
				wrong++
			}
		}
	}
	g.t.Logf("state %d: reflected pixels %d, wrong source channel %d", stage, changed, wrong)
	if stage == 3 || stage == 5 || stage == 8 || stage == 10 {
		if changed != 0 || wrong != 0 {
			g.t.Errorf("missing, submerged, or disabled reflection leaked in state %d", stage)
		}
	} else {
		if changed < 100 || wrong != 0 {
			g.t.Errorf("land reflection missing or uses stale source in state %d", stage)
		}
	}
	if stage == 6 || stage == 7 || stage == 9 || stage == 11 {
		for i, v := range im.Pix {
			if abs(int(v)-int(g.reflected.Pix[i])) > 1 {
				g.t.Errorf("reflection did not restore in state %d", stage)
				break
			}
		}
	}
}
