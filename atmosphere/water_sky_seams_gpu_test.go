package atmosphere

import (
	"os"
	"runtime"
	"testing"

	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/renderer"
	"github.com/derekmwright/glyphengine/shaders"
	"github.com/go-gl/mathgl/mgl32"
)

// Sample the same physical direction on either side of every cube edge and
// corner. Independent, jittered gutter marches must not cut reflected clouds
// into the projected cube faces. Exercise the actual generated cloud atlas.
func TestWaterSkySeamsGPU(t *testing.T) {
	if os.Getenv("PLANET_GPU_TEST") != "1" {
		t.Skip("set PLANET_GPU_TEST=1 for Vulkan shader checks")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	code := compileWaterProbe(t, `#version 450
#extension GL_GOOGLE_include_directive : require
#include "air-frame.glsl"
#include "parameters.glsl"
#include "engine-lighting.glsl"
#include "common.glsl"
#include "water-sky.glsl"
layout(location=0) out vec4 color;
void main() {
    int x=int(gl_FragCoord.x),row=int(gl_FragCoord.y)/10;
    vec3 a=vec3(1),b;
    int axis;
    if(x<384) {
        int edge=x/32;
        axis=edge/4;
        a[axis]=(edge&1)==0?1.0:-1.0;
        a[(axis+1)%3]=(edge&2)==0?1.0:-1.0;
        a[(axis+2)%3]=(float(x%32)+0.5)/32.0*1.9-0.95;
    } else {
        int corner=(x-384)/16;
        a=vec3((corner&1)==0?1:-1,(corner&2)==0?1:-1,(corner&4)==0?1:-1);
        axis=x%3;
    }
    b=a;
    a[axis]*=1.00001;
    b[(axis+1)%3]*=1.00001;
    a=normalize(a); b=normalize(b);
    vec3 ca,cb,ta=vec3(1),tb=vec3(1);
    if(row==7) {
        // Negative control: independently integrated gutters have unrelated
        // sample noise. Direct atlas reads reproduce the old discontinuity.
        ca=textureLod(waterSkyLight,waterSkyUV(a,0,WATER_SKY_ATLAS),0).rgb;
        cb=textureLod(waterSkyLight,waterSkyUV(b,0,WATER_SKY_ATLAS),0).rgb;
    } else {
        Air first=waterSkyLevel(a,row),second=waterSkyLevel(b,row);
        ca=first.light; cb=second.light;
        ta=first.transmittance; tb=second.transmittance;
    }
    bool bad=any(greaterThan(abs(ca-cb),0.003*(vec3(1)+max(ca,cb))))
        ||any(greaterThan(abs(ta-tb),vec3(0.003)))
        ||any(isnan(ca))||any(isinf(ca))||any(isnan(cb))||any(isinf(cb))
        ||any(isnan(ta))||any(isinf(ta))||any(isnan(tb))||any(isinf(tb));
    color=vec4(bad?1:0,1,0,1);
}`)
	g := &waterSkySeamProbe{t: t, code: code}
	e, err := glyph.New(g, glyph.WithTitle("Water sky seam regression"), glyph.WithBackgroundWindow(), glyph.WithWindowSize(512, 80), glyph.WithValidation(true), glyph.WithMSAA(1), glyph.WithVSync(false), glyph.WithShaderTextureSlots(7), glyph.WithAppTimingCapacity(32), glyph.WithMaxFrames(4))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Destroy()
	e.Run()
	if !g.checked {
		t.Fatal("seam diagnostic was not captured")
	}
}

type waterSkySeamProbe struct {
	t       *testing.T
	code    []byte
	frame   int
	checked bool
}

func (g *waterSkySeamProbe) Init(e *glyph.Engine) error {
	e.Scene.Env = cloudTestEnvironment{}
	e.SetCamera(mgl32.Vec3{}, mgl32.Vec3{0, 1, -0.2}, mgl32.Vec3{0, 0, 1})
	r := e.Renderer()
	r.SetTonemap(1, 0, 4)
	air, err := NewAirPasses(r, 0.5, 0.5, 7, nil)
	if err != nil {
		return err
	}
	air.SetEnabled(false, false)
	sky, err := NewWaterSky(r, air.Clouds)
	if err != nil {
		return err
	}
	p := FrameParameters(500000, [3]float64{0, 0, 500120}, true)
	p.SetOcean(500000, 0, [3]float64{0, 0, 500120})
	state := (cloudTestEnvironment{}).State()
	if err = sky.Update(&p, e.ViewProjection().Inv(), state.SunDir, state.SunColor, 500000, 0, 2, 0.52, true, true); err != nil {
		return err
	}
	data := p.Bytes()
	if err = r.SetShaderParameters(data[:]); err != nil {
		return err
	}
	_, err = r.CreateAppPass(renderer.AppPassDesc{Name: "water sky seam diagnostic", Stage: renderer.StageBeforeTonemap, Load: true, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: g.code})
	return err
}

func (g *waterSkySeamProbe) Update(e *glyph.Engine, _ float32) {
	if g.frame == 2 {
		im, err := e.Renderer().CaptureFrame()
		if err != nil {
			g.t.Fatal(err)
		}
		var bad [7]int
		control := 0
		for y := 0; y < 80; y++ {
			for x := 0; x < 512; x++ {
				c := im.RGBAAt(x, y)
				if c.G < 200 {
					g.t.Fatal("diagnostic did not draw")
				}
				if c.R > 100 {
					if y < 70 {
						bad[y/10]++
					} else {
						control++
					}
				}
			}
		}
		g.t.Logf("discontinuous edge/corner samples by mip: %v; old-gutter control: %d", bad, control)
		if bad != [7]int{} || control == 0 {
			g.t.Errorf("sky atlas continuity failed")
		}
		g.checked = true
	}
	g.frame++
}
