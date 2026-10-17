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
)

// Exercise the shipped compute atlas and sampling code. In particular, moving
// curvature to a larger atlas must not shift the wave field used by caustics.
func TestWaveFilteringGPU(t *testing.T) {
	if os.Getenv("PLANET_GPU_TEST") != "1" {
		t.Skip("set PLANET_GPU_TEST=1 for Vulkan shader checks")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	code := compileWaterProbe(t, `#version 450
#extension GL_GOOGLE_include_directive : require
#include "parameters.glsl"
#include "water-wave.glsl"
#include "wave-spectrum.glsl"
layout(location=0) out vec4 outColor;
void main() {
    bool bad=false;
    vec2 p=(floor(gl_FragCoord.xy)-vec2(128,32))*0.25;
    if(gl_FragCoord.y<24) {
        vec2 g,expected; mat2 h; vec3 curvature;
        wavePlane(p,g,h);
        waveSpectrum(mod(p,128.0),expected,curvature);
        // Allow one half-float ULP (including round-toward-zero storage) and
        // float-coordinate interpolation error. Near-zero values need an
        // absolute allowance as well as the relative storage precision.
        bad=any(greaterThan(abs(g-expected),abs(expected)*0.001+0.00002))
            ||any(greaterThan(abs(vec3(h[0][0],h[1][1],h[0][1])-curvature),abs(curvature)*0.001+0.00006));
        vec2 periodic; mat2 hp;
        wavePlane(p+vec2(128,-128),periodic,hp);
        bad=bad||distance(periodic,g)>0.00001;
    } else if(gl_FragCoord.y<48) {
        float lod=gl_FragCoord.x*9.0/256.0;
        vec2 g,gp; mat2 c,cp;
        waveSurfacePlane(p,lod,g,c);
        waveSurfacePlane(p+vec2(-128,128),lod,gp,cp);
        bad=distance(g,gp)>0.00001||abs(c[0][0]-cp[0][0])>0.000001;
        bad=bad||c[0][0]<0||c[1][1]<0||determinant(c)<-0.00000001||any(isnan(g));
        // Within a level transition there must be no discontinuous phase jump.
        waveSurfacePlane(p,min(lod+0.00001,9.0),gp,cp);
        bad=bad||distance(g,gp)>0.00001;
    } else {
        vec3 q=vec3(p,gl_FragCoord.x*0.15),up=normalize(vec3(0.3,0.4,0.5));
        vec3 normal; float variance;
        waterSurface(q,up,0,normal,variance);
        bad=distance(normal,waterNormal(q,up,1))>0.00001||variance>0.00000001;
        waterSurface(q,vec3(0,0,1),128,normal,variance);
        // Negative control: discarding unresolved waves must be detected.
        if(gl_FragCoord.y>=64) variance=0.0;
        // Fourier energy: 32*(.023/2)^2/2 + 16*.009^2/2 = .002764
        // per plane, then 2/3 survives the three-plane tangent projection.
        bad=bad||abs(variance-0.0018426667)>0.000008||distance(normal,vec3(0,0,1))>0.00001;
    }
    outColor=vec4(bad?1:0,1,0,1);
}`)
	g := &waveFilterProbe{t: t, code: code}
	e, err := glyph.New(g, glyph.WithTitle("Water wave filtering regression"), glyph.WithWindowSize(256, 80), glyph.WithBackgroundWindow(), glyph.WithValidation(true), glyph.WithMSAA(1), glyph.WithVSync(false), glyph.WithShaderTextureSlots(7), glyph.WithMaxFrames(4))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Destroy()
	e.Run()
	if !g.checked {
		t.Fatal("wave diagnostic was not captured")
	}
}

func compileWaterProbe(t *testing.T, source string) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "water-probe.frag")
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

type waveFilterProbe struct {
	t       *testing.T
	code    []byte
	frame   int
	checked bool
}

func (g *waveFilterProbe) Init(e *glyph.Engine) error {
	e.Scene.Env = shadowSelectionEnvironment{}
	p := FrameParameters(500000, [3]float64{0, 0, 500000}, true)
	p.Mie[2], p.Mie[3] = 9.25, 1
	data := p.Bytes()
	if err := e.Renderer().SetShaderParameters(data[:]); err != nil {
		return err
	}
	if _, err := NewWaveCache(e.Renderer()); err != nil {
		return err
	}
	_, err := e.Renderer().CreateAppPass(renderer.AppPassDesc{Name: "wave filtering diagnostic", Stage: renderer.StageBeforeTonemap, Load: true, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: g.code})
	return err
}

func (g *waveFilterProbe) Update(e *glyph.Engine, _ float32) {
	if g.frame == 2 {
		im, err := e.Renderer().CaptureFrame()
		if err != nil {
			g.t.Fatal(err)
		}
		var bad [4]int
		for y := 0; y < 80; y++ {
			for x := 0; x < 256; x++ {
				c := im.RGBAAt(x, y)
				if y >= 64 {
					if c.R < 100 || c.G < 100 {
						bad[3]++
					}
				} else if c.R > 10 || c.G < 100 {
					bad[min(y/24, 2)]++
				}
			}
		}
		if bad != [4]int{} {
			g.t.Errorf("wave diagnostic failed: base field %d, filtered continuity %d, energy %d, negative control %d pixels", bad[0], bad[1], bad[2], bad[3])
		}
		g.checked = true
	}
	g.frame++
}
