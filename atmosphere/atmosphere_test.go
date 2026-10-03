package atmosphere

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestMaterialCoordinatesSurviveCameraRebase(t *testing.T) {
	// Cross positive/negative wrapping boundaries at both small and planetary
	// coordinates. A fixed surface point must keep the same material location.
	for _, eye := range []float64{-10002432.001, -4096.001, -0.001, 4095.999, 10002431.999} {
		point := eye + 12.345
		var previous float64
		for frame, offset := range []float64{0, 0.003, 31.75} {
			camera := eye + offset
			parameters := FrameParameters(500000, [3]float64{camera, camera, camera}, true)
			for axis, origin := range []float32{parameters.Detail[1], parameters.Detail[2], parameters.Water[2]} {
				coordinate := float64(origin + float32(point-camera))
				if frame > 0 {
					difference := math.Remainder(coordinate-previous, 4096)
					if math.Abs(difference) > 0.001 {
						t.Fatalf("axis %d material drift %.6f m", axis, difference)
					}
				}
				if axis == 2 {
					previous = coordinate
				}
			}
		}
	}
}

func TestParameterStd140Layout(t *testing.T) {
	p := Parameters{
		Eye: [4]float32{1, 2, 3, 4}, Planet: [4]float32{5, 6, 7, 8},
		Rayleigh: [4]float32{9, 10, 11, 12}, Mie: [4]float32{13, 14, 15, 16},
		Water: [4]float32{17, 18, 19, 20}, Detail: [4]float32{21, 22, 23, 24}, Features: [4]float32{25, 26, 27, 28}, Rendering: [4]float32{29, 30, 31, 32},
		CausticOrigin: [4]float32{33, 34, 35, 36}, CausticU: [4]float32{37, 38, 39, 40}, CausticV: [4]float32{41, 42, 43, 44},
	}
	data := p.Bytes()
	for index := 0; index < 44; index++ {
		got := math.Float32frombits(binary.LittleEndian.Uint32(data[index*4:]))
		if got != float32(index+1) {
			t.Fatalf("std140 float %d = %g", index, got)
		}
	}
	zero := (Parameters{}).Bytes()
	for _, b := range zero {
		if b != 0 {
			t.Fatal("zero parameters contain stale data")
		}
	}
}
