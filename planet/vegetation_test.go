package planet

import (
	"math"
	"testing"
)

func TestVegetationBandExcludesSeaHighlandsAndCliffs(t *testing.T) {
	d := Vec{math.Sin(-math.Pi/4) * math.Cos(-13*math.Pi/180), math.Sin(-13 * math.Pi / 180), math.Cos(-math.Pi/4) * math.Cos(-13*math.Pi/180)}
	p := Planet{Seed: 7, Radius: 500000, Vegetation: &VegetationSettings{SeaLevel: 250, MaxHeight: 2600, MaxSlope: 38}}
	normal := p.Normal(d)
	cover, color := p.GroundCover(d, normal)
	if cover < 0.8 || color[1] <= color[0] {
		t.Fatal("expected green lowland")
	}
	tangent := d.Cross(Vec{0, 1, 0}).Unit()
	if c, _ := p.GroundCover(d, tangent); c != 0 {
		t.Fatal("grass on vertical cliff")
	}
	p.Vegetation.SeaLevel = p.Elevation(d) + 1
	if c, _ := p.GroundCover(d, normal); c != 0 {
		t.Fatal("underwater vegetation")
	}
	p.Vegetation.SeaLevel = p.Elevation(d) - 3000
	if c, _ := p.GroundCover(d, normal); c != 0 {
		t.Fatal("vegetation above height band")
	}
}
