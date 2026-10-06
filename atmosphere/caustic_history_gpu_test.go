package atmosphere

// Opt-in test of the actual reconstruction shaders and engine history bindings.
// PLANET_GPU_TEST=1 go test ./atmosphere -run TestCausticHistoryGPU -count=1
// Requires Vulkan and glslc; ordinary unit tests never open a window.
import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/renderer"
	"github.com/derekmwright/glyphengine/shaders"
)

type historyCase struct {
	mode                         int
	name                         string
	dt, value, origin            float32
	pattern, reset, skip, resize bool
}

type historyProbe struct {
	t                                        *testing.T
	cases                                    []historyCase
	step                                     int
	clock, expected                          float64
	lastClock                                float64
	valid                                    bool
	previous                                 historyCase
	sourceCode, inspectCode                  []byte
	source, filter, remember, state, inspect *renderer.AppPass
}

func TestCausticHistoryGPU(t *testing.T) {
	if os.Getenv("PLANET_GPU_TEST") != "1" {
		t.Skip("set PLANET_GPU_TEST=1 for Vulkan shader checks")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	compile := func(name, source string) []byte {
		t.Helper()
		path := filepath.Join(t.TempDir(), name+".frag")
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
	g := &historyProbe{t: t, clock: 62.7} // exercise the animation clock wrap too
	g.sourceCode = compile("source", `#version 450
#extension GL_GOOGLE_include_directive : require
#include "parameters.glsl"
layout(push_constant) uniform Push { layout(offset=128) vec4 values; } pc;
layout(location=0) out float intensity;
void main() {
    float x=mod(gl_FragCoord.x,512.0)*0.25+planetData.causticOrigin.x+planetData.detail.y;
    intensity=pc.values.y>0.5 ? 1.0+0.4*sin(x*6.28318530718/8.0) : pc.values.x;
}`)
	g.inspectCode = compile("inspect", `#version 450
layout(push_constant) uniform Push { layout(offset=128) vec4 values; } pc;
layout(set=2,binding=0) uniform sampler2D result;
layout(set=2,binding=1) uniform sampler2D reference;
layout(location=0) out vec4 color;
void main() {
    // Sample the interior of a nonzero depth tile, including many history texels.
    ivec2 p=ivec2(gl_FragCoord.xy)*2+ivec2(576,96);
    float actual=texelFetch(result,p,0).r;
    float expected=pc.values.y>0.5 ? texelFetch(reference,p,0).r : pc.values.x;
    color=vec4(abs(actual-expected)>0.008?1.0:0.0,expected*0.5,actual*0.5,1);
}`)
	g.cases = append(g.cases, historyCase{name: "first frame", dt: 1.0 / 60, value: 1})
	for i := 0; i < 36; i++ {
		dt := []float32{1.0 / 30, 1.0 / 60, 1.0 / 120}[i%3]
		g.cases = append(g.cases, historyCase{name: fmt.Sprintf("pulsing energy %d", i), dt: dt, value: 0.6 + float32(i%2)*0.8})
	}
	for _, mode := range []int{1, 2, 3, 4, 0} {
		g.cases = append(g.cases, historyCase{name: fmt.Sprintf("reject changed coordinates/light %d", mode), dt: 1.0 / 60, value: 0.7 + float32(mode)*0.2, mode: mode})
	}
	// A long gap invalidates old irradiance. Fixed-size history survives resize.
	g.cases = append(g.cases, historyCase{name: "resume after pause", dt: 1, value: 1.7, reset: true}, historyCase{name: "resize", dt: 1.0 / 60, value: 0.4, resize: true}, historyCase{name: "first frame after resize", dt: 1.0 / 60, value: 0.7})
	// Static world pattern must be unchanged by either sign of a cache shift,
	// including the wrapped world origin. A skipped submission must not poison it.
	for i, x := range []float32{4095, 4095.75, 4096.5, 4098, 4096.5, 4095.75, 4098, 4101, 4097.25} {
		g.cases = append(g.cases, historyCase{name: fmt.Sprintf("world anchor %g", x), dt: 1.0 / 60, origin: x, pattern: true, reset: i == 0, skip: i == 5})
	}
	e, err := glyph.New(g, glyph.WithTitle("Caustic history test"), glyph.WithWindowSize(160, 80), glyph.WithBackgroundWindow(), glyph.WithValidation(true), glyph.WithMSAA(1), glyph.WithVSync(false), glyph.WithFixedFrameTime(time.Second/60), glyph.WithMaxFrames(len(g.cases)+2))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Destroy()
	e.Run()
	if g.step < len(g.cases) {
		t.Fatal("history checks did not finish")
	}
}

func historyPush(p *renderer.AppPass, values ...float32) error {
	data := make([]byte, len(values)*4)
	for i, v := range values {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(v))
	}
	return p.SetPushConstants(data)
}

func (g *historyProbe) Init(e *glyph.Engine) error {
	r := e.Renderer()
	r.SetBloom(0, 1, 0.5, 1)
	target := func(name string, format renderer.TargetFormat, width, height uint32, history bool) (*renderer.RenderTarget, error) {
		return r.CreateRenderTarget(renderer.RenderTargetDesc{Name: name, Format: format, Width: width, Height: height, History: history, Filter: renderer.FilterLinear})
	}
	source, err := target("input", renderer.TargetR16F, 2048, 1024, false)
	if err != nil {
		return err
	}
	result, err := target("result", renderer.TargetR16F, 2048, 1024, false)
	if err != nil {
		return err
	}
	history, err := target("history", renderer.TargetR16F, 2048, 1024, true)
	if err != nil {
		return err
	}
	state, err := target("state", renderer.TargetRGBA32F, 4, 1, true)
	if err != nil {
		return err
	}
	pass := func(name string, dst *renderer.RenderTarget, code []byte, reads ...*renderer.Texture) (*renderer.AppPass, error) {
		return r.CreateAppPass(renderer.AppPassDesc{Name: name, Stage: renderer.StageBeforeScene, Target: dst, Vert: shaders.DepthResolveVertSpv, Frag: code, Reads: reads, Fullscreen: true})
	}
	g.source, err = pass("input", source, g.sourceCode)
	if err != nil {
		return err
	}
	g.filter, err = pass("filter", result, causticTemporal, source.Texture(), history.Texture(), state.Texture())
	if err != nil {
		return err
	}
	g.remember, err = pass("history", history, causticHistory, result.Texture())
	if err != nil {
		return err
	}
	g.state, err = pass("state", state, causticState)
	if err != nil {
		return err
	}
	g.inspect, err = r.CreateAppPass(renderer.AppPassDesc{Name: "inspect", Stage: renderer.StageBeforeTonemap, Load: true, Vert: shaders.DepthResolveVertSpv, Frag: g.inspectCode, Reads: []*renderer.Texture{result.Texture(), source.Texture()}, Fullscreen: true})
	return err
}

func (g *historyProbe) Update(e *glyph.Engine, _ float32) {
	if g.step > 0 && !g.previous.skip && !g.previous.resize {
		im, err := e.Renderer().CaptureFrame()
		if err != nil {
			g.t.Error(err)
			e.Close()
			return
		}
		bad := 0
		for y := 0; y < im.Bounds().Dy(); y++ {
			for x := 0; x < im.Bounds().Dx(); x++ {
				c := im.RGBAAt(x, y)
				if c.R > 10 || c.G < 30 {
					bad++
				}
			}
		}
		if bad > 0 {
			g.t.Errorf("%s: %d pixels failed irradiance check; center RGBA %v expected intensity %g", g.previous.name, bad, im.RGBAAt(80, 40), g.expected)
		}
	}
	if g.step >= len(g.cases) {
		e.Close()
		return
	}
	c := g.cases[g.step]
	g.step++
	g.clock += float64(c.dt)
	if c.reset {
		g.clock += 0.5
	}
	if c.resize {
		e.Renderer().NotifyResize()
	}
	if c.skip {
		e.Renderer().ProvokeSkipNextFrame()
	}
	// Forced acquire skips do not submit. NotifyResize finishes the current
	// submission before rebuilding; its fixed-size history targets survive.
	if !c.skip {
		if !g.valid || c.reset || c.mode != g.previous.mode {
			g.expected = float64(c.value)
		} else {
			alpha := 1 - math.Exp(-(g.clock-g.lastClock)/0.05)
			g.expected += (float64(c.value) - g.expected) * alpha
		}
		g.lastClock = g.clock
		g.valid = true
	}
	p := Parameters{}
	p.Water[0] = 500
	p.Mie[2] = float32(math.Mod(g.clock, 62.83185307179586))
	p.Mie[3] = 1
	p.Rendering[2] = 1
	p.Detail[1] = float32(math.Mod(float64(c.origin), 4096))
	p.CausticU = [4]float32{1, 0, 0, 128}
	p.CausticV = [4]float32{0, 0, 1, 47}
	sun := []float32{0, 1, 0, 0}
	switch c.mode {
	case 1:
		p.CausticU = [4]float32{0, 1, 0, 128}
	case 2:
		p.Water[0] = 501
	case 3:
		sun = []float32{0.2, 1, 0, 0}
	case 4:
		p.Mie[3] = 0
	}
	data := p.Bytes()
	pattern := float32(0)
	if c.pattern {
		pattern = 1
	}
	for _, err := range []error{e.Renderer().SetShaderParameters(data[:]), historyPush(g.source, c.value, pattern, 0, 0), historyPush(g.filter, append([]float32{0, 1, 0, 0}, sun...)...), historyPush(g.state, sun...), historyPush(g.inspect, float32(g.expected), pattern, 0, 0)} {
		if err != nil {
			g.t.Error(err)
			e.Close()
		}
	}
	g.previous = c
}
