package atmosphere

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func TestWaterSkyFrustumBounds(t *testing.T) {
	for _, aspect := range []float32{0.75, 1, 1.78, 3} {
		vp := mgl32.Perspective(mgl32.DegToRad(60), aspect, 0.1, 10000).Inv()
		for _, axis := range []mgl32.Vec3{{0, 0, -1}, {0, 0, 1}, {0, 1, 0}, {0.1, 0.9, -0.3}, {1, 0.4, -0.1}} {
			axis = axis.Normalize()
			bound, sampled := maxFrustumAlignment(vp, axis), float64(-1)
			for y := 0; y <= 64; y++ {
				for x := 0; x <= 64; x++ {
					v := vp.Mul4x1(mgl32.Vec4{float32(x)/32 - 1, float32(y)/32 - 1, 0, 1})
					dot := float64((mgl32.Vec3{v[0], v[1], v[2]}).Normalize().Dot(axis))
					sampled = math.Max(sampled, dot)
				}
			}
			if bound < sampled-1e-6 || bound > sampled+0.001 {
				t.Fatalf("aspect %g axis %v: bound %.8f, sampled %.8f", aspect, axis, bound, sampled)
			}
		}
	}
}

func TestWaterSkyVisibility(t *testing.T) {
	for _, tc := range []struct {
		name      string
		height    float64
		direction mgl32.Vec3
		visible   bool
	}{
		{"coast down", 20, mgl32.Vec3{0, 0, -1}, true},
		{"coast up", 20, mgl32.Vec3{0, 0, 1}, false},
		{"coast horizon", 20, mgl32.Vec3{1, 0, 0}, true},
		{"submerged up", -15, mgl32.Vec3{0, 0, 1}, true},
		{"submerged down", -15, mgl32.Vec3{0, 0, -1}, false},
		{"deep submerged", -1000, mgl32.Vec3{0, 0, 1}, false},
		{"submerged horizon", -15, mgl32.Vec3{1, 0, 0}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			eye := [3]float64{0, 0, 500000 + tc.height}
			p := FrameParameters(500000, eye, true)
			p.SetOcean(500000, 0, eye)
			vp := mgl32.Perspective(mgl32.DegToRad(60), 1.78, 0.1, 10000).Mul4(mgl32.LookAtV(mgl32.Vec3{}, tc.direction, mgl32.Vec3{0, 1, 0}))
			if got := waterSkyVisible(p, vp.Inv()); got != tc.visible {
				t.Fatalf("visible = %t, want %t", got, tc.visible)
			}
		})
	}
	for _, height := range []float64{0, 8000, -8000} {
		if waterSkyStrength(height) != 1 {
			t.Errorf("cache faded at height %g", height)
		}
	}
	for _, height := range []float64{32000, -32000, 800000} {
		if waterSkyStrength(height) != 0 {
			t.Errorf("cache active at height %g", height)
		}
	}
}
