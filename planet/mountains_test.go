package planet

import (
	"math"
	"testing"
)

func TestMountainRangesLeaveLowlands(t *testing.T) {
	p := Planet{Seed: 7, Radius: 500000}
	peak := 0.0
	var peakDir Vec
	plains := 0
	litPeak := 0.0
	var litDir Vec
	for i := 0; i < 4000; i++ {
		y := 1 - 2*(float64(i)+0.5)/4000
		a := float64(i) * 2.399963229728653
		d := Vec{math.Sin(a) * math.Sqrt(1-y*y), y, math.Cos(a) * math.Sqrt(1-y*y)}
		h := p.MountainHeight(d)
		if d.Dot((Vec{-0.55, 0.45, 0.70}).Unit()) > 0.4 && h > litPeak {
			litPeak = h
			litDir = d
		}
		if h == 0 {
			plains++
		}
		if h > peak {
			peak = h
			peakDir = d
		}
		if math.IsNaN(h) || h < 0 || h > 10000 {
			t.Fatal("invalid mountain displacement")
		}
	}
	if peak < 5000 || plains < 1000 {
		t.Fatalf("insufficient relief or lowlands: peak %.0f plains %d", peak, plains)
	}
	t.Logf("day mountain %.0f lon %.4f lat %.4f", litPeak, math.Atan2(litDir[0], litDir[2])*180/math.Pi, math.Asin(litDir[1])*180/math.Pi)
	t.Logf("peak mountain contribution %.0f m at longitude %.4f latitude %.4f; lowlands %d/4000", peak, math.Atan2(peakDir[0], peakDir[2])*180/math.Pi, math.Asin(peakDir[1])*180/math.Pi, plains)
}
