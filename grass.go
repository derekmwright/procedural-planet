package main

import (
	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/renderer"
	"github.com/derekmwright/procedural-planet/planet"
	"math"
	"sync"
)

type grassTuft struct {
	direction, position, normal planet.Vec
	color                       [3]float32
	scale, yaw, phase           float64
}
type grassResult struct {
	anchor planet.Vec
	tufts  []grassTuft
}
type grassScatter struct {
	set                               *renderer.InstanceSet
	placements                        []renderer.MeshInstance
	tufts                             []grassTuft
	anchor                            planet.Vec
	revision, count                   int
	density                           float64
	synchronous, pending, initialized bool
	jobs                              chan planet.Vec
	results                           chan grassResult
	stop                              chan struct{}
	wg                                sync.WaitGroup
}

func grassCandidates(p planet.Planet, center planet.Vec, density float64) []grassTuft {
	if p.Vegetation == nil || density <= 0 {
		return nil
	}
	const reach = 60.0
	n := int(math.Ceil(2 * p.Radius / (2.2 / math.Sqrt(density))))
	step := 2 / float64(n)
	var result []grassTuft
	for face := 0; face < 6; face++ {
		axis := planet.Direction(face, 0, 0)
		facing := center.Dot(axis)
		if facing < 0.5 {
			continue
		}
		uAxis := planet.Direction(face, 1, 0).Mul(math.Sqrt2).Sub(axis)
		vAxis := planet.Direction(face, 0, 1).Mul(math.Sqrt2).Sub(axis)
		u, v := center.Dot(uAxis)/facing, center.Dot(vAxis)/facing
		pad := reach * 3 / p.Radius
		loX, hiX := max(0, int(math.Floor((u-pad+1)/step))), min(n-1, int(math.Floor((u+pad+1)/step)))
		loY, hiY := max(0, int(math.Floor((v-pad+1)/step))), min(n-1, int(math.Floor((v+pad+1)/step)))
		for y := loY; y <= hiY; y++ {
			for x := loX; x <= hiX; x++ {
				seed := rockHash(p.Seed ^ uint64(face+1)*0x9e3779b185ebca87 ^ uint64(x)*0x165667b19e3779f9 ^ uint64(y)*0xc2b2ae3d27d4eb4f ^ 9191)
				d := planet.Direction(face, -1+(float64(x)+randomRock(&seed))*step, -1+(float64(y)+randomRock(&seed))*step)
				delta := d.Sub(center).Mul(p.Radius)
				if delta.Dot(delta) > reach*reach {
					continue
				}
				elevation := p.Elevation(d)
				if elevation < p.Vegetation.SeaLevel+8 || elevation > p.Vegetation.SeaLevel+p.Vegetation.MaxHeight {
					continue
				}
				normal := p.Normal(d)
				cover, color := p.GroundCover(d, normal)
				if randomRock(&seed) > cover {
					continue
				}
				scale := 0.6 + randomRock(&seed)*0.9
				yaw := randomRock(&seed) * 2 * math.Pi
				phase := randomRock(&seed) * 2 * math.Pi
				shade := float32(0.8 + randomRock(&seed)*0.4)
				for i := range color {
					color[i] *= shade
				}
				result = append(result, grassTuft{d, p.Surface(d), normal, color, scale, yaw, phase})
			}
		}
	}
	return result
}

func grassGeometry() ([]renderer.Vertex, []uint16) {
	var vertices []renderer.Vertex
	var indices []uint16
	seed := uint64(7351)
	for blade := 0; blade < 9; blade++ {
		angle := randomRock(&seed) * 2 * math.Pi
		x, z := (randomRock(&seed)-0.5)*0.8, (randomRock(&seed)-0.5)*0.8
		h := 0.26 + randomRock(&seed)*0.38
		width := 0.012 + randomRock(&seed)*0.02
		side := planet.Vec{math.Cos(angle) * width, 0, math.Sin(angle) * width}
		base := planet.Vec{x, 0, z}
		bend := planet.Vec{math.Sin(angle) * 0.12, 0, -math.Cos(angle) * 0.12}
		mid := base.Add(planet.Vec{0, h * 0.55, 0}).Add(bend.Mul(0.3))
		points := []planet.Vec{base.Sub(side), base.Add(side), mid.Sub(side.Mul(0.55)), mid.Add(side.Mul(0.55)), base.Add(planet.Vec{0, h, 0}).Add(bend)}
		start := uint16(len(vertices))
		for _, point := range points {
			tip := float32(point[1] / h)
			brightness := float32(0.6) + tip*0.55
			vertices = append(vertices, renderer.Vertex{Pos: [3]float32(vec(point)), Normal: [3]float32{0, 1, 0}, Color: [3]float32{brightness, brightness, brightness}, UV: [2]float32{tip, 2}})
		}
		for _, index := range []uint16{0, 2, 1, 1, 2, 3, 2, 4, 3} {
			indices = append(indices, start+index)
		}
	}
	return vertices, indices
}
func (g *grassScatter) init(e *glyph.Engine, p planet.Planet, synchronous bool, density float64) error {
	g.synchronous, g.density = synchronous, density
	g.jobs = make(chan planet.Vec, 1)
	g.results = make(chan grassResult, 1)
	g.stop = make(chan struct{})
	vertices, indices := grassGeometry()
	mesh, err := e.Renderer().CreateIndexedMesh(vertices, indices)
	if err != nil {
		return err
	}
	g.set, err = e.Renderer().CreateInstanceSet(mesh, 20000, nil)
	if err != nil {
		return err
	}
	id := e.Spawn()
	e.C.InstancedMesh.Set(id, &glyph.InstancedMesh{Set: g.set})
	e.C.MeshRef.Set(id, &glyph.MeshRef{Mesh: mesh, Roughness: 0.9})
	e.C.DoubleSided.Set(id, &glyph.DoubleSided{})
	e.C.NoCastShadow.Set(id, &glyph.NoCastShadow{})
	if !synchronous {
		g.wg.Add(1)
		go func() {
			defer g.wg.Done()
			for {
				select {
				case <-g.stop:
					return
				case anchor := <-g.jobs:
					result := grassResult{anchor, grassCandidates(p, anchor, density)}
					select {
					case g.results <- result:
					case <-g.stop:
						return
					}
				}
			}
		}()
	}
	return nil
}
func (g *grassScatter) close() {
	if g.stop != nil {
		close(g.stop)
		g.wg.Wait()
	}
}
func (g *grassScatter) update(e *glyph.Engine, t *terrain, eye, forward planet.Vec, clearance, elapsed float64, enabled bool) {
	g.placements = g.placements[:0]
	g.count = 0
	newResult := false
	if g.pending {
		select {
		case result := <-g.results:
			g.tufts = result.tufts
			g.anchor = result.anchor
			g.pending = false
			g.initialized = true
			newResult = true
		default:
		}
	}
	if enabled && clearance < 65 && t.world.Vegetation != nil {
		d := eye.Unit()
		delta := d.Sub(g.anchor).Mul(t.world.Radius)
		if (!g.initialized || delta.Dot(delta) > 8*8) && !g.pending {
			if g.synchronous {
				g.tufts = grassCandidates(t.world, d, g.density)
				g.anchor = d
				g.initialized = true
				newResult = true
			} else {
				g.jobs <- d
				g.pending = true
			}
		}
		revision := t.splits + t.merges
		if newResult || revision != g.revision {
			for i := range g.tufts {
				s := &g.tufts[i]
				s.position = s.direction.Mul(t.world.Radius + t.ground(s.direction) - 0.04)
			}
			g.revision = revision
		}
		for _, s := range g.tufts {
			pos := s.position.Sub(eye)
			distance := math.Sqrt(pos.Dot(pos))
			if pos.Dot(forward) < -1 || distance > 50 {
				continue
			}
			scale := s.scale * clamp((50-distance)/15, 0, 1)
			if scale < 0.025 {
				continue
			}
			north, east := northEast(s.normal)
			up := s.normal.Add(east.Mul(0.055 * math.Sin(elapsed*1.3+s.phase))).Unit()
			x := east.Mul(math.Cos(s.yaw)).Add(north.Mul(math.Sin(s.yaw)))
			x = x.Sub(up.Mul(x.Dot(up))).Unit()
			z := x.Cross(up).Mul(scale)
			x = x.Mul(scale)
			up = up.Mul(scale)
			instance := renderer.MeshInstance{Model: [16]float32{
				float32(x[0]), float32(x[1]), float32(x[2]), 0,
				float32(up[0]), float32(up[1]), float32(up[2]), 0,
				float32(z[0]), float32(z[1]), float32(z[2]), 0,
				float32(pos[0]), float32(pos[1]), float32(pos[2]), 1}, Tint: [4]float32{s.color[0], s.color[1], s.color[2], 1}}
			if len(g.placements) < 20000 {
				g.placements = append(g.placements, instance)
			}
		}
	}
	g.count = len(g.placements)
	e.Renderer().UpdateInstanceSet(g.set, g.placements)
}
