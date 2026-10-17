package atmosphere

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func TestSunDiscProjection(t *testing.T) {
	for _, aspect := range []float32{0.75, 1, 1.78, 3} {
		inverse := mgl32.Perspective(mgl32.DegToRad(60), aspect, 0.1, 100000).Inv()
		for _, uv := range []mgl32.Vec2{{0.5, 0.5}, {0.1, 0.3}, {0.8, 0.95}} {
			ray := func(uv mgl32.Vec2) mgl32.Vec3 {
				v := inverse.Mul4x1(mgl32.Vec4{uv[0]*2 - 1, uv[1]*2 - 1, 0, 1})
				return v.Vec3().Normalize()
			}
			sun := ray(uv)
			center, axes, valid := projectSunDisc(inverse, sun)
			if !valid || center.Sub(uv).Len() > 1e-5 {
				t.Fatalf("aspect %g UV %v: center %v, valid %t", aspect, uv, center, valid)
			}
			// Unproject the sampled ellipse: it must remain a circular angular
			// disc near screen edges, in portrait, and in ultrawide views.
			for i := 0; i < 32; i++ {
				a := float64(i) * 2 * math.Pi / 32
				x, y := float32(math.Cos(a)), float32(math.Sin(a))
				q := center.Add(mgl32.Vec2{axes[0]*x + axes[2]*y, axes[1]*x + axes[3]*y})
				d := ray(q)
				angle := math.Atan2(float64(d.Cross(sun).Len()), float64(d.Dot(sun)))
				if math.Abs(angle-0.009) > 0.0002 {
					t.Errorf("aspect %g UV %v: projected solar radius %g", aspect, uv, angle)
				}
			}
		}
		for _, sun := range []mgl32.Vec3{{0, 0, 0}, {0, 0, 1}, {1, 0, 0}, {10, 0, -1}} {
			if _, _, valid := projectSunDisc(inverse, sun); valid {
				t.Errorf("invalid/behind/off-screen source %v marked visible", sun)
			}
		}
	}
}
