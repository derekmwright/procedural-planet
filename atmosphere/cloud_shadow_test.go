package atmosphere

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl64"
)

func TestCloudShadowProjection(t *testing.T) {
	for _, sun := range [][3]float32{{0, 1, 0}, {1, 0, 0}, {0.8, 0, 0.6}, {0, -1, 0}, {-0.5, 0.5, 0.70710678}} {
		var previous Parameters
		for frame := 0; frame < 800; frame++ {
			var p Parameters
			eye := [3]float64{123456 + float64(frame), -45678, 498000}
			setCloudShadowProjection(&p, eye, sun, 500000, 250, true)
			u := mgl64.Vec3{float64(p.CloudShadowU[0]), float64(p.CloudShadowU[1]), float64(p.CloudShadowU[2])}
			v := mgl64.Vec3{float64(p.CloudShadowV[0]), float64(p.CloudShadowV[1]), float64(p.CloudShadowV[2])}
			light := mgl64.Vec3{float64(sun[0]), float64(sun[1]), float64(sun[2])}.Normalize()
			if math.Abs(u.Len()-1) > 1e-6 || math.Abs(v.Len()-1) > 1e-6 || math.Abs(u.Dot(v)) > 1e-6 || u.Cross(v).Sub(light).Len() > 1e-6 {
				t.Fatalf("invalid projection basis for %v", sun)
			}
			// Camera motion may shift the field, but only by whole texels. Thus
			// the same world location retains the same sampling phase.
			const texel = 2 * cloudShadowLocalHalfSpan / cloudShadowResolution
			for axis, center := range []float32{p.CloudShadowU[3], p.CloudShadowV[3]} {
				if math.Abs(math.Remainder(float64(center), texel)) > 1e-5 {
					t.Fatal("projection center is not texel-aligned")
				}
				if frame > 0 {
					old := []float32{previous.CloudShadowU[3], previous.CloudShadowV[3]}[axis]
					if math.Abs(math.Remainder(float64(center-old), texel)) > 1e-5 {
						t.Fatal("camera movement changes world-space sampling phase")
					}
				}
			}
			if math.Abs(float64(p.CloudShadowMeta[0])-502.45) > 0.0001 || math.Abs(float64(p.CloudShadowMeta[1])-505.45) > 0.0001 || p.CloudShadowMeta[3] != 1 {
				t.Fatal("shadow layer differs from visible cloud layer")
			}
			previous = p
		}
		setCloudShadowProjection(&previous, [3]float64{}, sun, 500000, 250, false)
		if previous.CloudShadowMeta[3] != 0 {
			t.Fatal("disabled shadows can sample stale optical depth")
		}
	}
}
