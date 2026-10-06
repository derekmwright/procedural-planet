package main

import (
	"fmt"
	"math"
	"sync"
	"time"

	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/ecs"
	"github.com/derekmwright/glyphengine/renderer"
	"github.com/derekmwright/procedural-planet/planet"
	"github.com/go-gl/mathgl/mgl32"
)

type patchEntity struct {
	entity ecs.Entity
	origin planet.Vec
	mesh   *renderer.Mesh
	ticket *renderer.UploadTicket
}
type splitResult struct {
	key      planet.Patch
	meshes   [4]planet.Mesh
	duration time.Duration
}
type terrain struct {
	world                     planet.Planet
	lod                       *planet.LOD
	patches                   map[planet.Patch]patchEntity
	pool                      []*renderer.Mesh
	jobs                      chan planet.Patch
	results                   chan splitResult
	stop                      chan struct{}
	wg                        sync.WaitGroup
	inflight                  bool
	ready                     *splitResult
	uploaded                  []patchEntity
	synchronous               bool
	asyncUploads              bool
	allocated, splits, merges int
	generationMS, uploadMS    float64
}

func newTerrain(p planet.Planet, synchronous bool) *terrain {
	t := &terrain{world: p, lod: planet.NewLOD(p.Radius), patches: make(map[planet.Patch]patchEntity), jobs: make(chan planet.Patch, 1), results: make(chan splitResult, 1), stop: make(chan struct{}), synchronous: synchronous}
	if !synchronous {
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			for {
				select {
				case <-t.stop:
					return
				case k := <-t.jobs:
					r := buildChildren(p, k)
					select {
					case t.results <- r:
					case <-t.stop:
						return
					}
				}
			}
		}()
	}
	return t
}
func buildChildren(p planet.Planet, k planet.Patch) splitResult {
	start := time.Now()
	r := splitResult{key: k}
	for i, c := range k.Children() {
		r.meshes[i] = p.BuildTerrainPatch(c)
	}
	r.duration = time.Since(start)
	return r
}
func (t *terrain) close() { close(t.stop); t.wg.Wait() }
func (t *terrain) upload(e *glyph.Engine, m planet.Mesh, streaming bool) (patchEntity, error) {
	vertices := make([]renderer.Vertex, len(m.Vertices))
	indices := make([]uint16, len(m.Indices))
	radius := 0.0
	for i, v := range m.Vertices {
		vertices[i] = renderer.Vertex{Pos: [3]float32(vec(v.Position)), Normal: [3]float32(vec(v.Normal)), Color: v.Color, UV: [2]float32{v.Cover, 0}}
		radius = math.Max(radius, math.Sqrt(v.Position.Dot(v.Position)))
	}
	for i, index := range m.Indices {
		if index > 65535 {
			return patchEntity{}, fmt.Errorf("terrain index exceeds uint16")
		}
		indices[i] = uint16(index)
	}
	var mesh *renderer.Mesh
	var ticket *renderer.UploadTicket
	var err error
	if t.asyncUploads {
		if streaming {
			mesh, ticket, err = e.Renderer().CreateIndexedMeshAsync(vertices, indices)
		} else {
			// Initial coverage is ready before the first rendered frame.
			mesh, err = e.Renderer().CreateIndexedMesh(vertices, indices)
		}
		if err != nil {
			return patchEntity{}, err
		}
		t.allocated++
	} else {
		if n := len(t.pool); n > 0 {
			mesh = t.pool[n-1]
			t.pool = t.pool[:n-1]
		} else {
			mesh, err = e.Renderer().CreateDynamicIndexedMesh(len(vertices), len(indices))
			if err != nil {
				return patchEntity{}, err
			}
			t.allocated++
		}
		if err = e.Renderer().UpdateMeshData(mesh, vertices, indices); err != nil {
			return patchEntity{}, err
		}
	}
	mesh.BoundCenter = [3]float32{}
	mesh.BoundRadius = float32(radius + 1)
	id := e.Spawn()
	e.C.Transform.Set(id, &glyph.Transform{Scale: mgl32.Vec3{1, 1, 1}})
	// Rough diffuse terrain response; the old engine world-up normal bias
	// was fixed in #93 and no longer constrains this material value.
	e.C.MeshRef.Set(id, &glyph.MeshRef{Mesh: mesh, Roughness: 0.9})
	e.C.Hidden.Set(id, &glyph.Hidden{})
	return patchEntity{entity: id, origin: m.Origin, mesh: mesh, ticket: ticket}, nil
}
func (t *terrain) init(e *glyph.Engine) error {
	for _, k := range t.lod.SortedLeaves() {
		mesh := t.world.BuildTerrainPatch(k)
		p, err := t.upload(e, mesh, false)
		if err != nil {
			return err
		}
		t.patches[k] = p
		t.lod.SetGeometricError(k, mesh.GeometricError)
		e.C.Hidden.Remove(p.entity)
	}
	return nil
}
func (t *terrain) recycle(e *glyph.Engine, p patchEntity) {
	e.Despawn(p.entity)
	if t.asyncUploads {
		// The engine's runtime release contract retires pending and settled
		// uploads after their final use. An extra defer would retain them twice.
		e.Renderer().DestroyMesh(p.mesh)
		t.allocated--
		return
	}
	// Dynamic meshes are restaged through the renderer's per-frame fence path;
	// recycling does not destroy buffers still referenced by an in-flight frame.
	t.pool = append(t.pool, p.mesh)
}

// Publish all four children together only after their GPU copies have retired.
// Until then the parent stays in the LOD, visible and available for collision.
func childrenReady(children []patchEntity) bool {
	if len(children) != 4 {
		return false
	}
	for _, child := range children {
		if child.ticket != nil && !child.ticket.Ready() {
			return false
		}
	}
	return true
}
func (t *terrain) update(e *glyph.Engine, eye planet.Vec) error {
	start := time.Now()
	defer func() { t.uploadMS = float64(time.Since(start).Microseconds()) / 1000 }()
	if t.ready == nil && t.inflight {
		select {
		case r := <-t.results:
			t.ready = &r
		default:
		}
	}
	if r := t.ready; r != nil {
		if !t.lod.CanSplit(r.key) {
			for _, p := range t.uploaded {
				t.recycle(e, p)
			}
			t.uploaded = nil
			t.ready = nil
			t.inflight = false
		} else {
			// At most two small mesh uploads per rendered frame.
			for i := 0; i < 2 && len(t.uploaded) < 4; i++ {
				p, err := t.upload(e, r.meshes[len(t.uploaded)], true)
				if err != nil {
					return err
				}
				t.uploaded = append(t.uploaded, p)
			}
			if childrenReady(t.uploaded) {
				t.lod.Split(r.key)
				e.C.Hidden.Set(t.patches[r.key].entity, &glyph.Hidden{})
				for i, c := range r.key.Children() {
					t.patches[c] = t.uploaded[i]
					t.lod.SetGeometricError(c, r.meshes[i].GeometricError)
					e.C.Hidden.Remove(t.uploaded[i].entity)
				}
				t.generationMS = float64(r.duration.Microseconds()) / 1000
				t.splits++
				t.uploaded = nil
				t.ready = nil
				t.inflight = false
			}
		}
	}
	for i := 0; i < 2; i++ {
		k, ok := t.lod.NextMerge(t.world, eye)
		if !ok {
			break
		}
		t.lod.Merge(k)
		e.C.Hidden.Remove(t.patches[k].entity)
		for _, c := range k.Children() {
			t.recycle(e, t.patches[c])
			delete(t.patches, c)
		}
		t.merges++
	}
	if !t.inflight {
		if k, ok := t.lod.NextSplit(t.world, eye); ok {
			t.inflight = true
			if t.synchronous {
				r := buildChildren(t.world, k)
				t.ready = &r
			} else {
				t.jobs <- k
			}
		}
	}
	return nil
}
func (t *terrain) renderPositions(e *glyph.Engine, eye planet.Vec) {
	for k := range t.lod.Leaves {
		p := t.patches[k]
		transform, _ := e.C.Transform.Get(p.entity)
		transform.Position = vec(p.origin.Sub(eye))
	}
}
func (t *terrain) ground(d planet.Vec) float64 {
	return math.Max(t.world.Elevation(d), t.world.MeshElevation(t.lod.At(d), planet.Segments, d))
}
func (t *terrain) stats() (leaves, level, triangles int) {
	leaves = len(t.lod.Leaves)
	for k := range t.lod.Leaves {
		level = max(level, k.Level)
	}
	triangles = leaves * (planet.Segments*planet.Segments*2 + planet.Segments*4*4)
	return
}
