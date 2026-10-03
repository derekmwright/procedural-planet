package planet

import "math"

type VegetationSettings struct {
	SeaLevel, MaxHeight, MaxSlope float64 // meters, meters above sea, degrees
}

// GroundCover is shared by orbital terrain color and local grass placement.
// Moisture is seeded, not latitude bands; elevation/slope preserve beaches and rock.
func (p Planet) GroundCover(d, normal Vec) (float64, [3]float32) {
	if p.Vegetation == nil {
		return 0, [3]float32{}
	}
	h := p.Elevation(d) - p.Vegetation.SeaLevel
	band := func(a, b, x float64) float64 { v := math.Max(0, math.Min(1, (x-a)/(b-a))); return v * v * (3 - 2*v) }
	moisture := math.Max(0, math.Min(1, 0.5+p.noise(d.Mul(p.Radius/65000), 111)*0.85+p.noise(d.Mul(p.Radius/11000), 112)*0.20))
	patches := p.noise(d.Mul(p.Radius/600), 113)
	slope := math.Acos(math.Max(-1, math.Min(1, normal.Dot(d)))) * 180 / math.Pi
	cover := band(8, 65, h) * (1 - band(p.Vegetation.MaxHeight*0.60, p.Vegetation.MaxHeight, h))
	cover *= 1 - band(p.Vegetation.MaxSlope*0.65, p.Vegetation.MaxSlope, slope)
	cover *= band(-0.75, 0.1, patches+moisture*0.5)
	green := [3]float64{0.045, 0.16, 0.032}
	gold := [3]float64{0.38, 0.29, 0.065}
	olive := [3]float64{0.15, 0.19, 0.05}
	upland := band(500, p.Vegetation.MaxHeight, h) * 0.7
	var color [3]float32
	for i := range color {
		color[i] = float32(lerp(lerp(gold[i], green[i], band(0.2, 0.8, moisture)), olive[i], upland))
	}
	return cover, color
}
