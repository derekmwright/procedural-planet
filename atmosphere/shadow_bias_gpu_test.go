package atmosphere

import (
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

// Exercise real far-map depths, not an emulation of the bias formula. A broad
// blocker 400 m toward the sun must occlude even tangent receivers. Separately,
// flat through steep receiving planes must not shadow themselves. The old
// per-triangle extrapolation leaks through the blocker; removing all correction
// instead makes the inclined planes shadow themselves.
func TestShadowReceiverBiasGPU(t *testing.T) {
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
    int band=int(gl_FragCoord.y)/16;
    float u=gl_FragCoord.x/256.0;
    vec3 normal,position;
    float expected=1.0;
    if(band<2) {
        // Sweep smoothly across tangency, in both directions. Subtexel sample
        // positions vary too, so one favourable map alignment cannot pass.
        normal=normalize(vec3(1,0,mix(-0.15,0.15,u)));
        position=vec3(mix(-350.0,350.0,u)+(band==1?10000.0:0.0),
            10000.0+gl_FragCoord.y*7.0,0);
        expected=band==0?0.0:1.0;
    } else {
        vec2 slopes[7]=vec2[](vec2(0),vec2(0.5,0),vec2(-0.5,0),
            vec2(2,0),vec2(-2,0),vec2(8,8),vec2(-8,8));
        int index=band-2;
        vec2 slope=slopes[index];
        float x=mix(-2000.0,2000.0,u);
        float y=mod(gl_FragCoord.y,16.0)*100.0;
        normal=normalize(vec3(slope,1));
        position=vec3(-45000.0+float(index)*12000.0+x,
            -10000.0+y,-dot(slope,vec2(x,y)));
        // Same small vertex normal offset used by the engine's lit shaders.
        position+=normal*0.1;
    }
    float fade;
    float actual=surfaceShadowCascade(1,position,normal,fade);
    bool bad=abs(actual-expected)>0.015||fade<0.99||isnan(actual)||isinf(actual);
    color=vec4(bad?1:0,1,actual,1);
}`
	path := filepath.Join(t.TempDir(), "shadow-bias.frag")
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
	g := &shadowBiasProbe{t: t, code: code}
	e, err := glyph.New(g, glyph.WithTitle("Shadow bias regression"), glyph.WithWindowSize(256, 144), glyph.WithBackgroundWindow(), glyph.WithValidation(true), glyph.WithMSAA(1), glyph.WithVSync(false), glyph.WithMaxFrames(4))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Destroy()
	e.Run()
	if !g.checked {
		t.Fatal("shadow bias diagnostic was not captured")
	}
}

type shadowBiasProbe struct {
	t       *testing.T
	code    []byte
	frame   int
	checked bool
}

func (g *shadowBiasProbe) Init(e *glyph.Engine) error {
	e.Scene.Env = shadowSelectionEnvironment{}
	e.SetCamera(mgl32.Vec3{}, mgl32.Vec3{0, 0, -1}, mgl32.Vec3{0, 1, 0})
	if err := e.SetShadowCoverage(renderer.ShadowCoverage{Cascades: [2]renderer.ShadowCascadeCoverage{
		{Radius: 90, TowardLight: 200000, AwayFromLight: 120000},
		{Radius: 120000, TowardLight: 200000, AwayFromLight: 120000},
	}}); err != nil {
		return err
	}
	var vertices []renderer.Vertex
	var indices []uint16
	quad := func(cx, cy, z, halfWidth float32, slope [2]float32) {
		start := uint16(len(vertices))
		for _, corner := range [][2]float32{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
			x, y := corner[0]*halfWidth, corner[1]*halfWidth
			vertices = append(vertices, renderer.Vertex{Pos: [3]float32{cx + x, cy + y, z - slope[0]*x - slope[1]*y}})
		}
		// Both windings: the engine's shadow pass renders back faces only.
		for _, i := range []uint16{0, 1, 2, 0, 2, 3, 2, 1, 0, 3, 2, 0} {
			indices = append(indices, start+i)
		}
	}
	quad(0, 10000, 400, 2000, [2]float32{})
	for i, slope := range [][2]float32{{0, 0}, {0.5, 0}, {-0.5, 0}, {2, 0}, {-2, 0}, {8, 8}, {-8, 8}} {
		quad(-45000+float32(i)*12000, -10000, 0, 4000, slope)
	}
	mesh, err := e.Renderer().CreateIndexedMesh(vertices, indices)
	if err != nil {
		return err
	}
	// Keep every off-screen plane eligible for the far cascade.
	var radius float64
	for _, vertex := range vertices {
		p := vertex.Pos
		radius = math.Max(radius, math.Sqrt(float64(p[0]*p[0]+p[1]*p[1]+p[2]*p[2])))
	}
	mesh.BoundCenter, mesh.BoundRadius = [3]float32{}, float32(radius+1)
	id := e.Spawn()
	e.C.Transform.Set(id, &glyph.Transform{Scale: mgl32.Vec3{1, 1, 1}})
	e.C.MeshRef.Set(id, &glyph.MeshRef{Mesh: mesh})
	_, err = e.Renderer().CreateAppPass(renderer.AppPassDesc{Name: "shadow bias probe", Stage: renderer.StageBeforeTonemap, Load: true, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: g.code})
	return err
}

func (g *shadowBiasProbe) Update(e *glyph.Engine, _ float32) {
	if g.frame == 2 {
		im, err := e.Renderer().CaptureFrame()
		if err != nil {
			g.t.Error(err)
		} else {
			var bad [9]int
			for y := 0; y < 144; y++ {
				for x := 0; x < 256; x++ {
					c := im.RGBAAt(x, y)
					if c.R > 10 || c.G < 100 {
						bad[y/16]++
					}
				}
			}
			if bad != [9]int{} {
				g.t.Errorf("shadow bias mismatches (blocked, open, seven inclined planes): %v", bad)
			}
		}
		g.checked = true
	}
	g.frame++
}
