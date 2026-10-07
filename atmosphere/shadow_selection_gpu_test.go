package atmosphere

import (
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

// A small caster is resolved by the near map but not by the mountain map.
// Probe actual common.glsl lookups along the light axis: distant receivers
// must agree with the far map even inside the near map's long caster volume.
func TestShadowReceiverSelectionGPU(t *testing.T) {
	if os.Getenv("PLANET_GPU_TEST") != "1" {
		t.Skip("set PLANET_GPU_TEST=1 for Vulkan shader checks")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	source := `#version 450
#extension GL_GOOGLE_include_directive : require
#include "parameters.glsl"
#include "engine-lighting.glsl"
#include "common.glsl"
layout(location=0) out vec4 color;
void main() {
    int cell=int(gl_FragCoord.x)/16;
    // Near, middle of blend, its outer limit, and far along BOTH light rays.
    float distances[8]=float[](10,36,54,72,108,360,10000,-500);
    vec3 p=vec3(0,0,-distances[cell]);
    float fade;
    float nearValue=surfaceShadowCascade(0,p,vec3(0,0,1),fade);
    float farValue=surfaceShadowCascade(1,p,vec3(0,0,1),fade);
    float actual=localShadow(p,vec3(0,0,1));
    // Independently specified behavior for a 90 m radius near map.
    float expected=farValue;
    if(cell<2) expected=nearValue;
    if(cell==2) expected=(nearValue+farValue)*0.5;
    expected=mix(1.0,expected,planetData.features.x);
    bool bad=abs(actual-expected)>0.015;
    // Green checks that real shadow maps disagree, so the test is sensitive.
    color=vec4(bad?1:0,abs(nearValue-farValue)>0.5?1:0,actual,1);
}`
	path := filepath.Join(t.TempDir(), "selection.frag")
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
	g := &shadowSelectionProbe{t: t, code: code}
	e, err := glyph.New(g, glyph.WithTitle("Shadow receiver regression"), glyph.WithWindowSize(128, 32), glyph.WithBackgroundWindow(), glyph.WithValidation(true), glyph.WithMSAA(1), glyph.WithVSync(false), glyph.WithMaxFrames(6))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Destroy()
	e.Run()
	if g.checked != 4 {
		t.Fatalf("checked %d shadow strengths, want 4", g.checked)
	}
}

type shadowSelectionEnvironment struct{}

func (shadowSelectionEnvironment) Advance(float32) {}

func (shadowSelectionEnvironment) State() glyph.EnvironmentState {
	return glyph.EnvironmentState{SunDir: [3]float32{0, 0, 1}, SunColor: [3]float32{1, 1, 1}, CastShadows: true}
}

type shadowSelectionProbe struct {
	t       *testing.T
	code    []byte
	frame   int
	checked int
}

func (g *shadowSelectionProbe) Init(e *glyph.Engine) error {
	e.Scene.Env = shadowSelectionEnvironment{}
	e.SetCamera(mgl32.Vec3{}, mgl32.Vec3{0, 0, -1}, mgl32.Vec3{0, 1, 0})
	if err := e.SetShadowCoverage(renderer.ShadowCoverage{Cascades: [2]renderer.ShadowCascadeCoverage{
		{Radius: 90, TowardLight: 200000, AwayFromLight: 120000},
		{Radius: 120000, TowardLight: 200000, AwayFromLight: 120000},
	}}); err != nil {
		return err
	}
	// This off-screen caster must still reach nearby shadow receivers. Keeping
	// the full caster depth is essential; shortening it is not a valid fix.
	vertices := []renderer.Vertex{
		{Pos: [3]float32{-20, -20, 1000}}, {Pos: [3]float32{20, -20, 1000}},
		{Pos: [3]float32{20, 20, 1000}}, {Pos: [3]float32{-20, 20, 1000}},
	}
	mesh, err := e.Renderer().CreateIndexedMesh(vertices, []uint16{0, 1, 2, 0, 2, 3, 2, 1, 0, 3, 2, 0})
	if err != nil {
		return err
	}
	mesh.BoundCenter, mesh.BoundRadius = [3]float32{0, 0, 1000}, 30
	id := e.Spawn()
	e.C.Transform.Set(id, &glyph.Transform{Scale: mgl32.Vec3{1, 1, 1}})
	e.C.MeshRef.Set(id, &glyph.MeshRef{Mesh: mesh})
	_, err = e.Renderer().CreateAppPass(renderer.AppPassDesc{Name: "shadow selection probe", Stage: renderer.StageBeforeTonemap, Load: true, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: g.code})
	return err
}

func (g *shadowSelectionProbe) Update(e *glyph.Engine, _ float32) {
	if g.frame > 0 && g.frame <= 4 {
		im, err := e.Renderer().CaptureFrame()
		if err != nil {
			g.t.Error(err)
		} else {
			for cell := 0; cell < 8; cell++ {
				c := im.RGBAAt(cell*16+8, 16)
				if c.R > 10 || c.G < 100 {
					g.t.Errorf("frame %d, receiver %d: diagnostic pixel %v (red = wrong cascade blend, missing green = insensitive fixture)", g.frame, cell, c)
				}
			}
		}
		g.checked++
	}
	p := Parameters{}
	// Check fractional strength on both the near-map early return and far blend,
	// including a complete fade-out followed by restoration on resurfacing.
	p.Features[0] = []float32{1, 0.25, 0, 1}[min(g.frame, 3)]
	data := p.Bytes()
	if err := e.Renderer().SetShaderParameters(data[:]); err != nil {
		g.t.Error(err)
		e.Close()
	}
	g.frame++
}
