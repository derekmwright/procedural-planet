package main

import (
	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/renderer"
	"github.com/derekmwright/procedural-planet/planet"
	"math"
)

type stone struct {
	direction, position, up planet.Vec
	size, yaw               float64
	height, support         float64
	variant                 int
	shade                   float32
}
type rockScatter struct {
	sets        [3]*instanceStream
	placements  [3][]renderer.MeshInstance
	stones      []stone
	anchor      planet.Vec
	revision    int
	count       int
	initialized bool
}

func rockHash(x uint64) uint64 {
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}
func randomRock(state *uint64) float64 {
	*state += 0x9e3779b97f4a7c15
	return float64(rockHash(*state)>>11) / float64(uint64(1)<<53)
}

// Fixed cube-face cells make placement independent of travel history and LOD.
// Query every nearby face, including both sides of cube seams.
func scatterStones(p planet.Planet, center planet.Vec) []stone {
	const reach = 260.0
	n := int(math.Ceil(2 * p.Radius / 32))
	step := 2 / float64(n)
	var result []stone
	for face := 0; face < 6; face++ {
		normal := planet.Direction(face, 0, 0)
		facing := center.Dot(normal)
		if facing < 0.5 {
			continue
		}
		uAxis := planet.Direction(face, 1, 0).Mul(math.Sqrt2).Sub(normal)
		vAxis := planet.Direction(face, 0, 1).Mul(math.Sqrt2).Sub(normal)
		u, v := center.Dot(uAxis)/facing, center.Dot(vAxis)/facing
		pad := reach * 3 / p.Radius
		loX, hiX := max(0, int(math.Floor((u-pad+1)/step))), min(n-1, int(math.Floor((u+pad+1)/step)))
		loY, hiY := max(0, int(math.Floor((v-pad+1)/step))), min(n-1, int(math.Floor((v+pad+1)/step)))
		for y := loY; y <= hiY; y++ {
			for x := loX; x <= hiX; x++ {
				seed := rockHash(p.Seed ^ uint64(face+1)*0x165667b19e3779f9 ^ uint64(x)*0x9e3779b185ebca87 ^ uint64(y)*0xc2b2ae3d27d4eb4f)
				for i := 0; i < 3; i++ {
					d := planet.Direction(face, -1+(float64(x)+randomRock(&seed))*step, -1+(float64(y)+randomRock(&seed))*step)
					sizeRoll, yaw, shade := randomRock(&seed), randomRock(&seed)*2*math.Pi, float32(0.75+randomRock(&seed)*0.5)
					variant := int(randomRock(&seed) * 3)
					delta := d.Sub(center).Mul(p.Radius)
					if delta.Dot(delta) > reach*reach {
						continue
					}
					size := 0.12 + 0.7*math.Pow(sizeRoll, 3)
					if sizeRoll > 0.96 {
						size = 1.0 + (sizeRoll-0.96)*45
					}
					up := p.Normal(d)
					if up.Dot(d) < 0.8 {
						continue
					}
					height := p.Elevation(d)
					// A rock belongs to the immutable surface, not the temporary
					// collision floor used to keep the camera above coarse triangles.
					position := d.Mul(p.Radius + height - size*0.08)
					result = append(result, stone{direction: d, position: position, up: up, height: height, size: size, yaw: yaw, variant: variant, shade: shade})
				}
			}
		}
	}
	return result
}

// Irregular closed meshes, with flat facets and clockwise triangles. All
// instance scaling is uniform; the engine's instanced normal transform expects it.
func rockGeometry(variant int) ([]renderer.Vertex, []uint16) {
	const sides = 9
	seed := uint64(variant+1) * 173
	rings := make([]planet.Vec, 4*sides)
	for j := 0; j < 4; j++ {
		for i := 0; i < sides; i++ {
			a := float64(i)*2*math.Pi/sides + float64(j%2)*0.13
			radius := []float64{0.46, 0.9, 0.72, 0.25}[j] * (0.8 + randomRock(&seed)*0.35)
			rings[j*sides+i] = planet.Vec{math.Cos(a) * radius, []float64{-0.35, -0.03, 0.43, 0.67}[j] + randomRock(&seed)*0.10, math.Sin(a) * radius * (0.7 + float64(variant)*0.15)}
		}
	}
	var vertices []renderer.Vertex
	var indices []uint16
	tri := func(a, b, c planet.Vec) {
		normal := b.Sub(a).Cross(c.Sub(a)).Unit()
		if normal.Dot(a.Add(b).Add(c)) < 0 {
			b, c = c, b
			normal = normal.Mul(-1)
		}
		shade := float32(1)
		for _, v := range []planet.Vec{a, c, b} {
			indices = append(indices, uint16(len(vertices)))
			vertices = append(vertices, renderer.Vertex{Pos: [3]float32(vec(v)), Normal: [3]float32(vec(normal)), UV: [2]float32{0, 1}, Color: [3]float32{0.34 * shade, 0.29 * shade, 0.23 * shade}})
		}
	}
	for i := 0; i < sides; i++ {
		next := (i + 1) % sides
		tri(planet.Vec{0, -0.4, 0}, rings[next], rings[i])
		tri(planet.Vec{0, 0.78, 0}, rings[3*sides+i], rings[3*sides+next])
		for j := 0; j < 3; j++ {
			a, b, c, d := rings[j*sides+i], rings[j*sides+next], rings[(j+1)*sides+i], rings[(j+1)*sides+next]
			tri(a, b, c)
			tri(b, d, c)
		}
	}
	// Area-weighted shared normals remove the triangulated lighting pattern,
	// while a small face-normal contribution retains the chipped silhouette.
	normals := map[[3]float32]planet.Vec{}
	for i := 0; i < len(vertices); i += 3 {
		a, b, c := vertices[i], vertices[i+1], vertices[i+2]
		cv := func(p [3]float32) planet.Vec { return planet.Vec{float64(p[0]), float64(p[1]), float64(p[2])} }
		area := cv(c.Pos).Sub(cv(a.Pos)).Cross(cv(b.Pos).Sub(cv(a.Pos)))
		for j := 0; j < 3; j++ {
			key := vertices[i+j].Pos
			normals[key] = normals[key].Add(area)
		}
	}
	for i := range vertices {
		n := vertices[i].Normal
		face := planet.Vec{float64(n[0]), float64(n[1]), float64(n[2])}
		vertices[i].Normal = [3]float32(vec(normals[vertices[i].Pos].Unit().Mul(0.8).Add(face.Mul(0.2)).Unit()))
	}
	return vertices, indices
}
func (r *rockScatter) init(e *glyph.Engine) error {
	for i := range r.sets {
		vertices, indices := rockGeometry(i)
		mesh, err := e.Renderer().CreateIndexedMesh(vertices, indices)
		if err != nil {
			return err
		}
		set, err := newInstanceStream(e.Renderer(), mesh, 4096)
		if err != nil {
			return err
		}
		r.sets[i] = set
		id := e.Spawn()
		e.C.InstancedMesh.Set(id, &set.InstancedMesh)
		e.C.MeshRef.Set(id, &glyph.MeshRef{Mesh: mesh, Roughness: 0.9})
	}
	return nil
}

// Wait for the visible mesh to support the fixed rock position. Lifting props
// to terrain.ground makes them hop on every LOD replacement and can pull a
// submerged rock (and its shadow caster) up through the water surface.
func (r *rockScatter) updateSupport(t *terrain) {
	for i := range r.stones {
		s := &r.stones[i]
		meshHeight := t.world.MeshElevation(t.lod.At(s.direction), planet.Segments, s.direction)
		error := math.Abs(meshHeight - s.height)
		// Small relief can be hidden by the embedded base. Bigger disagreement
		// must remove the instance from both the color and shadow draws.
		full, hidden := 0.08+s.size*0.15, 0.18+s.size*0.5
		f := clamp((error-full)/(hidden-full), 0, 1)
		s.support = 1 - f*f*(3-2*f)
	}
}

func (r *rockScatter) update(e *glyph.Engine, t *terrain, eye planet.Vec, clearance float64) error {
	for i := range r.placements {
		r.placements[i] = r.placements[i][:0]
	}
	r.count = 0
	if clearance < 300 {
		d := eye.Unit()
		delta := d.Sub(r.anchor).Mul(t.world.Radius)
		changed := !r.initialized || delta.Dot(delta) > 24*24
		if changed {
			r.stones = scatterStones(t.world, d)
			r.anchor = d
			r.initialized = true
		}
		revision := t.splits + t.merges
		if changed || revision != r.revision {
			r.updateSupport(t)
			r.revision = revision
		}
		for _, s := range r.stones {
			pos := s.position.Sub(eye)
			distance := math.Sqrt(pos.Dot(pos))
			fade := clamp((230-distance)/50, 0, 1)
			scale := s.size * fade * s.support
			if scale < 0.015 {
				continue
			}
			north, east := northEast(s.up)
			x := east.Mul(math.Cos(s.yaw)).Add(north.Mul(math.Sin(s.yaw)))
			z := x.Cross(s.up)
			x = x.Mul(scale)
			up := s.up.Mul(scale)
			z = z.Mul(scale)
			instance := renderer.MeshInstance{Model: [16]float32{
				float32(x[0]), float32(x[1]), float32(x[2]), 0,
				float32(up[0]), float32(up[1]), float32(up[2]), 0,
				float32(z[0]), float32(z[1]), float32(z[2]), 0,
				float32(pos[0]), float32(pos[1]), float32(pos[2]), 1}, Tint: [4]float32{s.shade, s.shade, s.shade, 1}}
			if len(r.placements[s.variant]) < 4096 {
				r.placements[s.variant] = append(r.placements[s.variant], instance)
				r.count++
			}
		}
	}
	for i, set := range r.sets {
		if err := set.update(e.Renderer(), r.placements[i]); err != nil {
			return err
		}
	}
	return nil
}
