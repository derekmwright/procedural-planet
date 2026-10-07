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

// Render alternating single-texel casters into the real mountain cascade.
// Sweep fractional UVs over them: a complete [1 2 1] tent must average this
// signal to one half regardless of subtexel alignment. Sparse corner taps
// instead produce a repeating bright/dark grid.
func TestShadowFilterGPU(t *testing.T) {
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
    vec2 size=vec2(textureSize(terrainShadowMap,0).xy);
    vec2 uv=(size*0.5-4.0+gl_FragCoord.xy/vec2(128,64)*8.0)/size;
    vec4 world=inverse(atm.cascadeVP[1])*vec4(uv*2.0-1.0,0.7,1.0);
    float fade;
    float actual=surfaceShadowCascade(1,world.xyz/world.w,vec3(0,0,1),fade);
    // Independent nine-sample convolution, including the centre. This is a
    // flat receiving plane, so all samples have the same reference depth.
    float scale=length(vec3(atm.cascadeVP[1][0][2],atm.cascadeVP[1][1][2],atm.cascadeVP[1][2][2]));
    float reference=0.0;
    for(int y=-1;y<=1;y++) for(int x=-1;x<=1;x++) {
        float weight=float((x==0?2:1)*(y==0?2:1));
        reference+=weight*textureGrad(terrainShadowMap,vec4(uv+vec2(x,y)/size,1,0.7-0.5*scale),vec2(0),vec2(0));
    }
    reference*=1.0/16.0;
    float centre=textureGrad(terrainShadowMap,vec4(uv,1,0.7),vec2(0),vec2(0));
    bool bad=abs(actual-reference)>0.012||abs(reference-0.5)>0.012;
    color=vec4(bad?1:0,1,abs(centre-reference),1);
}`
	path := filepath.Join(t.TempDir(), "shadow-filter.frag")
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
	g := &shadowFilterProbe{t: t, code: code}
	e, err := glyph.New(g, glyph.WithTitle("Shadow filter regression"), glyph.WithWindowSize(128, 64), glyph.WithBackgroundWindow(), glyph.WithValidation(true), glyph.WithMSAA(1), glyph.WithVSync(false), glyph.WithMaxFrames(4))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Destroy()
	e.Run()
	if !g.checked {
		t.Fatal("shadow filter diagnostic was not captured")
	}
}

type shadowFilterProbe struct {
	t       *testing.T
	code    []byte
	frame   int
	checked bool
}

func (g *shadowFilterProbe) Init(e *glyph.Engine) error {
	e.Scene.Env = shadowSelectionEnvironment{}
	e.SetCamera(mgl32.Vec3{}, mgl32.Vec3{0, 0, -1}, mgl32.Vec3{0, 1, 0})
	coverage := renderer.ShadowCoverage{Cascades: [2]renderer.ShadowCascadeCoverage{
		{Radius: 90, TowardLight: 200000, AwayFromLight: 120000},
		{Radius: 120000, TowardLight: 200000, AwayFromLight: 120000},
	}}
	if err := e.SetShadowCoverage(coverage); err != nil {
		return err
	}
	matrices, err := renderer.ComputeCascadeVPsWithCoverage([3]float32{0, 0, 1}, mgl32.Vec3{}, coverage)
	if err != nil {
		return err
	}
	inverse := matrices[1].Inv()
	var vertices []renderer.Vertex
	var indices []uint16
	for y := -8; y < 8; y++ {
		for x := -8; x < 8; x++ {
			if (x+y)&1 != 0 {
				continue
			}
			start := uint16(len(vertices))
			for _, corner := range [][2]int{{0, 0}, {1, 0}, {1, 1}, {0, 1}} {
				clip := mgl32.Vec4{float32(x+corner[0]) * 2 / renderer.ShadowMapSize, float32(y+corner[1]) * 2 / renderer.ShadowMapSize, 0.25, 1}
				position := inverse.Mul4x1(clip)
				vertices = append(vertices, renderer.Vertex{Pos: [3]float32{position[0], position[1], position[2]}})
			}
			for _, i := range []uint16{0, 1, 2, 0, 2, 3, 2, 1, 0, 3, 2, 0} {
				indices = append(indices, start+i)
			}
		}
	}
	mesh, err := e.Renderer().CreateIndexedMesh(vertices, indices)
	if err != nil {
		return err
	}
	mesh.BoundCenter, mesh.BoundRadius = [3]float32{0, 0, vertices[0].Pos[2]}, 2000
	id := e.Spawn()
	e.C.Transform.Set(id, &glyph.Transform{Scale: mgl32.Vec3{1, 1, 1}})
	e.C.MeshRef.Set(id, &glyph.MeshRef{Mesh: mesh})
	_, err = e.Renderer().CreateAppPass(renderer.AppPassDesc{Name: "shadow filter probe", Stage: renderer.StageBeforeTonemap, Load: true, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: g.code})
	return err
}

func (g *shadowFilterProbe) Update(e *glyph.Engine, _ float32) {
	if g.frame == 2 {
		im, err := e.Renderer().CaptureFrame()
		if err != nil {
			g.t.Error(err)
		} else {
			bad, sensitive := 0, 0
			for y := 0; y < 64; y++ {
				for x := 0; x < 128; x++ {
					c := im.RGBAAt(x, y)
					if c.R > 10 || c.G < 100 {
						bad++
					}
					if c.B > 80 {
						sensitive++
					}
				}
			}
			if bad != 0 || sensitive == 0 {
				g.t.Errorf("shadow filter: %d mismatched pixels, %d sensitive checkerboard samples", bad, sensitive)
			}
		}
		g.checked = true
	}
	g.frame++
}
