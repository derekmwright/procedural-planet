package atmosphere

import (
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

// Use the production integrator with a known homogeneous extinction. Its exact
// integral is extinction * length, independent of its quadrature or storage.
// Probe partial and complete paths through both shell lobes, the empty interior,
// points above clouds, and atlas edges. A single ground-only shadow map cannot
// pass the inside/above cases.
func TestCloudShadowOpticalDepthGPU(t *testing.T) {
	if os.Getenv("PLANET_GPU_TEST") != "1" {
		t.Skip("set PLANET_GPU_TEST=1 for Vulkan shader checks")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	compute, err := os.ReadFile("cloud-shadow.comp")
	if err != nil {
		t.Fatal(err)
	}
	fixture := strings.Replace(string(compute), `#include "cloud-density.glsl"`, "#include \"cloud-density.glsl\"\n#define cloudDensity(p, detail, footprint) (0.025)", 1)
	source := `#version 450
#extension GL_GOOGLE_include_directive : require
#include "parameters.glsl"
#include "cloud-shadow-sample.glsl"
layout(set=2,binding=0) uniform sampler2D field;
layout(location=0) out vec4 outColor;
void main() {
    int x=int(gl_FragCoord.x),y=int(gl_FragCoord.y);
    int cascade=(x/16)%2;
    // Texel-centre samples test depth reconstruction without adding a
    // projection interpolation error to the known integral.
    int size=textureSize(field,0).x/2;
    vec2 cell=vec2((x*13)%size,(y*43)%size);
    vec2 xy=(2.0*(cell+0.5)/float(size)-1.0)*cloudShadowSpan(cascade)+cloudShadowCenter(cascade);
    float outer=planetData.cloudShadowMeta.y,inner=planetData.cloudShadowMeta.x;
    float rho2=dot(xy,xy);
    float outerZ=sqrt(max(0.0,outer*outer-rho2));
    float innerZ=sqrt(max(0.0,inner*inner-rho2));
    float span=outerZ-innerZ;
    float z=outerZ+1.0;
    int c=x%16;
    if(c>=1&&c<=8) z=outerZ-span*(float(c)-0.5)/8.0;
    if(c==9) z=innerZ-0.1;
    if(c==10) z=0.0;
    if(c>=11&&c<=14) z=-innerZ-span*(float(c)-10.5)/4.0;
    if(c==15) z=-outerZ-1.0;
    float expected=0.1*(clamp(outerZ-z,0.0,span)+clamp(-innerZ-z,0.0,span));
    vec3 p=planetData.cloudShadowU.xyz*xy.x+planetData.cloudShadowV.xyz*xy.y+cloudShadowSun()*z;
    float actual=cloudShadowCascade(field,p,cascade);
    bool bad=abs(actual-expected)>max(0.003,expected*0.003)||isnan(actual)||isinf(actual);
    // The disabled contract must prevent stale field contents from leaking.
    bad=bad||cloudShadowDepth(field,p)!=0.0;
    outColor=vec4(bad?1:0,1,0,1);
}`
	compile := func(name, source string) []byte {
		t.Helper()
		path := filepath.Join(t.TempDir(), name)
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
		return code
	}
	g := &cloudShadowProbe{cloudGeometryProbe: cloudGeometryProbe{t: t, code: compile("shadow-probe.frag", source)}, compute: compile("shadow-fixture.comp", fixture)}
	e, err := glyph.New(g, glyph.WithTitle("Cloud shadow regression"), glyph.WithWindowSize(256, 64), glyph.WithBackgroundWindow(), glyph.WithValidation(true), glyph.WithMSAA(1), glyph.WithVSync(false), glyph.WithMaxFrames(4))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Destroy()
	e.Run()
	if !g.checked {
		t.Fatal("cloud shadow diagnostic not captured")
	}
}

type cloudShadowProbe struct {
	cloudGeometryProbe
	compute []byte
}

func (g *cloudShadowProbe) Init(e *glyph.Engine) error {
	e.Scene.Env = cloudTestEnvironment{}
	p := FrameParameters(500000, [3]float64{0, 0, 501000}, true)
	setCloudShadowProjection(&p, [3]float64{0, 0, 501000}, [3]float32{0, 0, 1}, 500000, 0, false)
	data := p.Bytes()
	if err := e.Renderer().SetShaderParameters(data[:]); err != nil {
		return err
	}
	field, err := e.Renderer().CreateRenderTarget(renderer.RenderTargetDesc{Name: "shadow test optical depth", Format: renderer.TargetRGBA16F, Width: cloudShadowResolution * 2, Height: cloudShadowResolution * 2, Storage: true, Filter: renderer.FilterLinear})
	if err != nil {
		return err
	}
	compute, err := e.Renderer().CreateAppCompute(renderer.AppComputeDesc{Name: "shadow test producer", Stage: renderer.StageBeforeScene, Comp: g.compute, Writes: []*renderer.RenderTarget{field}, Params: 32})
	if err != nil {
		return err
	}
	compute.SetDispatch(cloudShadowResolution*2/8, cloudShadowResolution*2/8, 1)
	params := cloudFrameParameters(500000, 0, 0, 1)
	if err := compute.SetParams(params[:]); err != nil {
		return err
	}
	_, err = e.Renderer().CreateAppPass(renderer.AppPassDesc{Name: "shadow test receiver", Stage: renderer.StageBeforeTonemap, Load: true, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: g.code, Reads: []*renderer.Texture{field.Texture()}})
	return err
}
