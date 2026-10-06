package planet

import (
	"math"
	"reflect"
	"slices"
	"sync"
	"testing"
)

func TestFloodQueueOrdering(t *testing.T) {
	var entries []floodEntry
	for i := 0; i < 1000; i++ {
		entries = append(entries, floodEntry{int32(i), float64((i * 71) % 113)})
	}
	want := slices.Clone(entries)
	slices.SortFunc(want, func(a, b floodEntry) int {
		if floodLess(a, b) {
			return -1
		}
		if floodLess(b, a) {
			return 1
		}
		return 0
	})
	for _, bulk := range []bool{false, true} {
		var q floodQueue
		if bulk {
			q = slices.Clone(entries)
			for i := len(q)/2 - 1; i >= 0; i-- {
				q.sift(i, q[i])
			}
		} else {
			for _, e := range entries {
				q.push(e)
			}
		}
		for _, e := range want {
			if got := q.pop(); got != e {
				t.Fatalf("bulk=%v: got %v, want %v", bulk, got, e)
			}
		}
	}
}

func TestErosionGridIsClosed(t *testing.T) {
	nodes, _ := erosionGrid(16, 500000)
	if len(nodes) != 6*16*16+2 {
		t.Fatal("cube boundaries were not shared")
	}
	area := 0.0
	for i, n := range nodes {
		area += n.rain
		if n.count != 6 && n.count != 8 {
			t.Fatalf("unexpected degree %d", n.count)
		}
		for _, j := range n.neighbors[:n.count] {
			matched := false
			for _, back := range nodes[j].neighbors[:nodes[j].count] {
				if int(back) == i {
					matched = true
				}
			}
			if !matched {
				t.Fatal("one-way graph edge")
			}
		}
	}
	if math.Abs(area/(4*math.Pi*500000*500000)-1) > 1e-12 {
		t.Fatal("rainfall does not cover the sphere")
	}
}

func TestErosionDrainageConservesCatchmentsWithoutCycles(t *testing.T) {
	nodes, _ := erosionGrid(16, 500000)
	for i := range nodes {
		d := nodes[i].direction
		nodes[i].height = 500*d[0] + 400*math.Sin(d[1]*12)*math.Cos(d[2]*10)
	}
	for _, sea := range []float64{0, -2000} {
		order := erosionDrainage(nodes, sea, 500000)
		if len(order) != len(nodes) {
			t.Fatal("drainage graph is disconnected")
		}
		rank := make([]int, len(nodes))
		for i, id := range order {
			rank[id] = i
		}
		rain, outlets := 0.0, 0.0
		for i, n := range nodes {
			rain += n.rain
			if n.down < 0 {
				outlets += n.area
				continue
			}
			if rank[n.down] >= rank[i] {
				t.Fatal("parent is not earlier in drainage order")
			}
			if n.filled < nodes[n.down].filled {
				t.Fatal("routing runs uphill on filled field")
			}
		}
		if math.Abs(outlets/rain-1) > 1e-12 {
			t.Fatal("catchment water was lost or duplicated")
		}
	}
}

func TestImplicitIncisionDoesNotInvertSlopes(t *testing.T) {
	nodes := make([]erosionNode, 4)
	for i, h := range []float64{1000, 800, 100, 0} {
		nodes[i] = erosionNode{direction: Vec{math.Sin(float64(i) * .1), 0, math.Cos(float64(i) * .1)}, original: h, height: h, rain: 1e6, area: float64(i+1) * 1e6, down: int32(i + 1)}
	}
	nodes[3].down = -1
	inciseTerrain(nodes, []int32{3, 2, 1, 0}, 0, 1000, 2)
	for i, n := range nodes[:3] {
		if n.height > n.original || n.height < nodes[i+1].height {
			t.Fatal("implicit solve overshot or inverted a slope")
		}
	}
	if nodes[0].height >= 900 {
		t.Fatal("test did not exercise substantial incision")
	}
}

func TestErosionDeterminismAndBounds(t *testing.T) {
	base := Planet{Seed: 7, Radius: 500000}
	// This size also exercises parallel base-height sampling.
	settings := ErosionSettings{SeaLevel: 250, Strength: 1, Resolution: 64}
	a, err := base.WithErosion(settings)
	if err != nil {
		t.Fatal(err)
	}
	b, err := base.WithErosion(settings)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.erosion, b.erosion) {
		t.Fatal("generation is not deterministic")
	}
	modified := 0
	for face := 0; face < 6; face++ {
		for y := 0; y <= 24; y++ {
			for x := 0; x <= 24; x++ {
				d := Direction(face, float64(x)/12-1, float64(y)/12-1)
				h, e := base.Elevation(d), a.Elevation(d)
				if !finiteErosion(e) || e > h || h-e > 1200.00001 {
					t.Fatal("unbounded or non-finite modification")
				}
				if h <= 250 && h != e {
					t.Fatal("submarine terrain changed")
				}
				if h > 250 && e < 250 {
					t.Fatal("new coast flooding")
				}
				if h-e > 20 {
					modified++
				}
			}
		}
	}
	if modified < 10 {
		t.Fatal("erosion did not reshape the landscape")
	}
	settings.Strength = 0
	off, err := a.WithErosion(settings)
	if err != nil || off.erosion != nil {
		t.Fatal("zero strength did not restore the original field")
	}
	a.Seed++
	d := Vec{1, 1, 1}.Unit()
	if a.Elevation(d) != (Planet{Seed: a.Seed, Radius: a.Radius}).Elevation(d) {
		t.Fatal("stale field after seed change")
	}
}

func TestErosionCacheSeamsAndLOD(t *testing.T) {
	p, err := (Planet{Seed: 7, Radius: 500000}).WithErosion(ErosionSettings{SeaLevel: 250, Strength: 1, Resolution: 32})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []float64{-.9, -.3, 0, .3, .9} {
		x, z := Direction(0, -1, v), Direction(4, 1, v)
		if p.Elevation(x) != p.Elevation(z) || p.Normal(x) != p.Normal(z) {
			t.Fatal("face-dependent surface sample")
		}
	}
	// Approach all twelve face edges from opposite sides, between grid nodes.
	for axis := 0; axis < 3; axis++ {
		for other := axis + 1; other < 3; other++ {
			for _, sa := range []float64{-1, 1} {
				for _, sb := range []float64{-1, 1} {
					for _, v := range []float64{-.999, -.913, -.327, .153, .839, .999} {
						var a Vec
						a[axis], a[other], a[3-axis-other] = sa, sb, v
						b := a
						a[other] *= 1 - 1e-9
						b[other] *= 1 + 1e-9
						if math.Abs(p.erosion.sample(a.Unit())-p.erosion.sample(b.Unit())) > 1e-3 {
							t.Fatalf("cache discontinuity across edge %v", a)
						}
					}
				}
			}
		}
	}
	parent := Patch{Face: 4, Level: 6, X: 63, Y: 32}
	coarse, fine := p.BuildPatch(parent, 8), p.BuildPatch(parent.Children()[0], 8)
	for y := 0; y <= 4; y++ {
		for x := 0; x <= 4; x++ {
			a := coarse.Vertices[y*9+x].Position.Add(coarse.Origin)
			b := fine.Vertices[(y*2)*9+x*2].Position.Add(fine.Origin)
			if a.Sub(b).Dot(a.Sub(b)) > 1e-14 {
				t.Fatal("shared LOD sample moved")
			}
		}
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for i := 0; i < 100; i++ {
				p.Normal(Direction(0, float64(i)/50-1, .2))
			}
		})
	}
	wg.Wait()
}

func TestErosionRejectsInvalidSettings(t *testing.T) {
	p := Planet{Seed: 7, Radius: 500000}
	for _, s := range []ErosionSettings{{Strength: math.NaN()}, {Strength: math.Inf(1)}, {Strength: -1}, {Strength: 3}, {Strength: 1, SeaLevel: math.Inf(1)}, {Strength: 1, Resolution: 37}} {
		if _, err := p.WithErosion(s); err == nil {
			t.Fatalf("accepted %+v", s)
		}
	}
}

func BenchmarkErosionSampling(b *testing.B) {
	base := Planet{Seed: 7, Radius: 500000}
	eroded, err := base.WithErosion(ErosionSettings{SeaLevel: 250, Strength: 1, Resolution: 128})
	if err != nil {
		b.Fatal(err)
	}
	d, _, ok := eroded.ErosionPreview(Vec{.33, .22, .92}.Unit())
	if !ok {
		b.Fatal("no eroded inspection point")
	}
	for _, v := range []struct {
		name string
		p    Planet
	}{{"base", base}, {"eroded", eroded}} {
		b.Run(v.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				v.p.Elevation(d)
			}
		})
	}
}
