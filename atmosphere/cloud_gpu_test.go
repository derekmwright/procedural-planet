package atmosphere

import (
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/renderer"
	"github.com/derekmwright/glyphengine/shaders"
)

// Check the production GPU sampler and upload against an analytic RGB volume,
// including fractional LODs, negative coordinates and all-axis wrap seams.
// Shell cases include terrain
// clipping, inside/above/below cameras, and both lobes of a grazing orbital ray.
func TestCloudVolumeGeometryGPU(t *testing.T) {
	if os.Getenv("PLANET_GPU_TEST") != "1" {
		t.Skip("set PLANET_GPU_TEST=1 for Vulkan shader checks")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	source := `#version 450
#extension GL_GOOGLE_include_directive : require
#include "parameters.glsl"
#include "common.glsl"
#include "cloud-density.glsl"
layout(location=0) out vec4 outColor;
vec3 expectedLevel(vec3 p,int level) {
    float scale=float(1<<level),size=64.0/scale;
    vec3 q=mod(fract(p)*size+size-0.5,size);
    vec3 a=floor(q),b=mod(a+1.0,size);
    // Each box mean of the coordinate volume has a known analytic value.
    return (mix(a,b,fract(q))*scale+0.5*(scale-1.0))*4.0/255.0;
}
void main() {
    bool bad=false;
    if(gl_FragCoord.y<96.0) {
        vec3 p=vec3((gl_FragCoord.x-128.0)/63.0,(gl_FragCoord.y-16.0)/13.0,(gl_FragCoord.x+gl_FragCoord.y-44.0)/23.0);
        int row=int(gl_FragCoord.y),column=int(gl_FragCoord.x);
        float lod=float(row/8)+float(row%8)/8.0;
        if(row>=64) {
            // Exact seams, both sides, and base-level texel centres on each
            // axis. Integer translations exercise repeat beyond the unit cube.
            const float offsets[5]=float[](-0.00001,0.0,0.00001,-0.5/64.0,0.5/64.0);
            p[((row-64)/8)%3]=float((column/5)%5-2)+offsets[column%5];
            lod=float(row%8);
        }
        float footprint=exp2(lod)/64.0;
        if(row>=88) {footprint=float(column%5-2)*0.001;lod=0.0;}
        lod=clamp(lod,0.0,6.0);
        vec3 expected=mix(expectedLevel(p,int(lod)),expectedLevel(p,min(int(lod)+1,6)),fract(lod));
        vec3 actual=cloudNoiseAt(p,footprint);
        bad=any(greaterThan(abs(actual-expected),vec3(0.003)))||any(isnan(actual));
    } else {
        int c=int(gl_FragCoord.x)%8;
        vec3 eye=vec3(0,0,501),dir=vec3(0,0,1);
        float endpoint=20.0;
        vec4 expected=vec4(0,0,1.2,4.2);
        if(c==1) {eye.z=503;expected=vec4(0,2.2,2.2,2.2);}
        if(c==2) {eye.z=510;dir.z=-1;expected=vec4(4.8,7.8,20,20);}
        if(c==3) {endpoint=1;expected=vec4(0,0,1,1);}
        if(c==4) {endpoint=3;expected=vec4(0,0,1.2,3);}
        if(c==5) {eye.z=503;dir.z=-1;expected=vec4(0,0.8,20,20);}
        if(c==6) {eye.z=510;expected=vec4(0);}
        if(c==7) {
            eye=vec3(-100,501,0);dir=vec3(1,0,0);endpoint=250;
            float a=sqrt(505.2*505.2-501.0*501.0),b=sqrt(502.2*502.2-501.0*501.0);
            expected=vec4(100-a,100-b,100+b,100+a);
        }
        vec4 actual=cloudIntervals(eye,dir,endpoint);
        bad=any(greaterThan(abs(actual-expected),vec4(0.003)))||any(isnan(actual));
    }
    outColor=vec4(bad?1:0,1,0,1);
}`
	path := filepath.Join(t.TempDir(), "cloud-probe.frag")
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
	g := &cloudGeometryProbe{t: t, code: code}
	e, err := glyph.New(g, glyph.WithTitle("Cloud volume regression"), glyph.WithWindowSize(256, 128), glyph.WithBackgroundWindow(), glyph.WithValidation(true), glyph.WithMSAA(1), glyph.WithVSync(false), glyph.WithMaxFrames(4))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Destroy()
	e.Run()
	if !g.checked {
		t.Fatal("cloud diagnostic not captured")
	}
}

type cloudGeometryProbe struct {
	t       *testing.T
	code    []byte
	frame   int
	checked bool
}

func (g *cloudGeometryProbe) Init(e *glyph.Engine) error {
	e.Scene.Env = shadowSelectionEnvironment{}
	parameters := FrameParameters(500000, [3]float64{0, 0, 501000}, true)
	data := parameters.Bytes()
	if err := e.Renderer().SetShaderParameters(data[:]); err != nil {
		return err
	}
	volume := make([]byte, cloudNoiseSize*cloudNoiseSize*cloudNoiseSize*4)
	for z := 0; z < cloudNoiseSize; z++ {
		for y := 0; y < cloudNoiseSize; y++ {
			for x := 0; x < cloudNoiseSize; x++ {
				i := ((z*cloudNoiseSize+y)*cloudNoiseSize + x) * 4
				volume[i] = byte(x * 4)
				volume[i+1] = byte(y * 4)
				volume[i+2] = byte(z * 4)
				volume[i+3] = 255
			}
		}
	}
	texture, err := newCloudNoiseTexture(e.Renderer(), volume)
	if err != nil {
		return err
	}
	pass, err := e.Renderer().CreateAppPass(renderer.AppPassDesc{Name: "cloud diagnostic", Stage: renderer.StageBeforeTonemap, Load: true, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: g.code, Reads: []*renderer.Texture{texture, texture}, Reads3D: []int{0, 1}, Params: 32})
	if err != nil {
		return err
	}
	var params [32]byte
	for i, v := range []float32{2.2, 3, 4, 0.52, 1, 0, 96, 0} {
		binary.LittleEndian.PutUint32(params[i*4:], math.Float32bits(v))
	}
	return pass.SetParams(params[:])
}
func (g *cloudGeometryProbe) Update(e *glyph.Engine, _ float32) {
	if g.frame == 2 {
		im, err := e.Renderer().CaptureFrame()
		if err != nil {
			g.t.Error(err)
		} else {
			bad := 0
			for y := 0; y < im.Bounds().Dy(); y++ {
				for x := 0; x < im.Bounds().Dx(); x++ {
					c := im.RGBAAt(x, y)
					if c.R > 10 || c.G < 100 {
						bad++
					}
				}
			}
			if bad != 0 {
				g.t.Errorf("cloud volume sampler/geometry: %d failed pixels", bad)
			}
		}
		g.checked = true
	}
	g.frame++
}
