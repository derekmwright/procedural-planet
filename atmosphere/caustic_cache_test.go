package atmosphere

import (
	"github.com/derekmwright/procedural-planet/planet"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Even horizon sunlight must keep BOTH interpolated depth samples inside the
// atlas until their camera-centered contribution has faded completely. Read
// the shader's actual depth knots so widening a depth interval tests the guard.
func TestCausticCoverageContainsEveryDepthLookup(t *testing.T) {
	shader, err := os.ReadFile("caustic-depths.glsl")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`float\[8\]\(([^)]+)\)`).FindSubmatch(shader)
	if len(match) != 2 {
		t.Fatal("cannot read caustic depth knots")
	}
	var maxStep, previous float64
	for _, word := range strings.Split(string(match[1]), ",") {
		depth, err := strconv.ParseFloat(strings.TrimSpace(word), 64)
		if err != nil {
			t.Fatal(err)
		}
		maxStep = math.Max(maxStep, depth-previous)
		previous = depth
	}
	// Snell's law bounds the refracted tangent slope even at the horizon.
	maxSlope := 1 / math.Sqrt(1.333*1.333-1)
	lastTexelCenter := float64(causticSpan)/2 - float64(causticSpan)/512/2
	outerLookup := causticCoverageRadius + causticAnchorDistance + maxStep*maxSlope + causticFilterRadius
	if outerLookup >= lastTexelCenter {
		t.Fatalf("visible cache boundary: lookup %.3fm exceeds valid %.3fm", outerLookup, lastTexelCenter)
	}
}

// Moving the local cache must preserve the sampling lattice, not merely the
// reconstructed origin. Otherwise identical waves jump to new mesh/texel phases.
func TestCausticReanchorsPreserveSamplingPhase(t *testing.T) {
	const radius = 500000.0
	up := planet.Vec{-0.55, 0.45, 0.7}.Unit()
	sun := [3]float32{-0.55, 0.45, 0.7}
	c := &CausticCache{}
	var p Parameters
	c.prepare(&p, up.Mul(radius-7), radius, true, sun)
	origin, u, v := c.anchor, c.u, c.v
	reanchors := 0
	for step := 1; step <= 500; step++ {
		x, y := float64(step)*0.31, float64(step)*0.17
		eye := origin.Add(u.Mul(x)).Add(v.Mul(y)).Unit().Mul(radius - 7)
		before := c.anchor
		c.prepare(&p, eye, radius, true, sun)
		if c.u != u || c.v != v {
			t.Fatal("local motion rotated the sampling lattice")
		}
		if c.anchor != before {
			reanchors++
		}
		delta := c.anchor.Sub(origin)
		for _, spacing := range []float64{0.375, 0.125, 0.25} {
			for _, axis := range []planet.Vec{u, v} {
				phase := delta.Dot(axis) / spacing
				if math.Abs(phase-math.Round(phase)) > 1e-7 {
					t.Fatalf("grid phase changed: spacing %v, phase %v", spacing, phase)
				}
			}
		}
		center := eye.Unit().Mul(radius)
		if distance := math.Sqrt(center.Sub(c.anchor).Dot(center.Sub(c.anchor))); distance > causticAnchorDistance+1e-7 {
			t.Fatalf("anchor violates atlas guard: %.9f m", distance)
		}
		if math.Abs(math.Sqrt(c.anchor.Dot(c.anchor))-radius) > 1e-6 {
			t.Fatal("snapping moved the patch off the sea sphere")
		}
	}
	if reanchors < 50 {
		t.Fatal("motion did not exercise enough reanchors")
	}
	// Large travel rebuilds the local tangent frame safely.
	eye := origin.Add(u.Mul(1000)).Unit().Mul(radius - 7)
	c.prepare(&p, eye, radius, true, sun)
	if c.frameOrigin == origin || c.u.Cross(c.v).Dot(eye.Unit()) < 0.999999 {
		t.Fatal("large travel did not refresh the tangent frame")
	}
}

func TestCausticAnchorRebaseAndInactivation(t *testing.T) {
	const radius = 500000.0
	up := planet.Vec{-0.55, 0.45, 0.7}.Unit()
	sun := [3]float32{-0.55, 0.45, 0.7}
	c := &CausticCache{}
	var p Parameters
	eye := up.Mul(radius - 16)
	if !c.prepare(&p, eye, radius, true, sun) {
		t.Fatal("daylit shallow water inactive")
	}
	anchor := c.anchor
	for _, shift := range []float64{0.003, 1, 1.9} {
		moved := eye.Add(c.u.Mul(shift)).Unit().Mul(radius - 16)
		c.prepare(&p, moved, radius, true, sun)
		if c.anchor != anchor {
			t.Fatal("anchor moved inside its stable region")
		}
		// The GPU's camera-relative origin must reconstruct the same world point.
		for axis := range 3 {
			if math.Abs(moved[axis]+float64(p.CausticOrigin[axis])-anchor[axis]) > 0.00001 {
				t.Fatal("caustic pattern swims after camera rebase")
			}
		}
	}
	moved := eye.Add(c.u.Mul(3)).Unit().Mul(radius - 16)
	c.prepare(&p, moved, radius, true, sun)
	if c.anchor == anchor || math.Abs(math.Sqrt(c.anchor.Dot(c.anchor))-radius) > 1e-6 {
		t.Fatal("patch did not follow camera on sea sphere")
	}
	if math.Abs(c.u.Dot(c.v)) > 1e-10 || c.u.Cross(c.v).Dot(moved.Unit()) < 0.999999 {
		t.Fatal("invalid tangent frame")
	}
	for _, test := range []struct {
		height  float64
		enabled bool
		sun     [3]float32
	}{
		{-16, false, sun}, {-100, true, sun}, {500, true, sun}, {-16, true, [3]float32{0.55, -0.45, -0.7}},
	} {
		if c.prepare(&p, up.Mul(radius+test.height), radius, test.enabled, test.sun) || p.CausticOrigin[3] != 0 || p.Rendering[1] != 1 {
			t.Fatal("inactive cache can expose stale light")
		}
	}
}
