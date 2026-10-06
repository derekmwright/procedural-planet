package main

import (
	"fmt"
	"slices"

	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/renderer"
)

type instanceStreamRenderer interface {
	CreateInstanceSet(*renderer.Mesh, int, []renderer.MeshInstance) (*renderer.InstanceSet, error)
	UpdateInstanceSet(*renderer.InstanceSet, []renderer.MeshInstance)
	DeferDestroy(func())
	Minimized() bool
}

// Ordinary InstanceSet updates currently write live GPU memory (engine #188).
// Keep submitted sets immutable until the engine's retirement callback returns
// them to this pool. Unlike a CPU LOD set, this retains group culling, offscreen
// shadow casters and opaque-prepass eligibility. The renderer owns destruction
// of every allocated set at shutdown; the callback here only permits reuse.
type instanceStream struct {
	glyph.InstancedMesh
	mesh     *renderer.Mesh
	capacity int
	free     []*renderer.InstanceSet
	last     []renderer.MeshInstance
}

func newInstanceStream(r instanceStreamRenderer, mesh *renderer.Mesh, capacity int) (*instanceStream, error) {
	set, err := r.CreateInstanceSet(mesh, capacity, nil)
	if err != nil {
		return nil, err
	}
	return &instanceStream{InstancedMesh: glyph.InstancedMesh{Set: set}, mesh: mesh, capacity: capacity}, nil
}

func (s *instanceStream) update(r instanceStreamRenderer, placements []renderer.MeshInstance) error {
	// Minimized frames don't retire buffers. Leave the current snapshot intact
	// and publish the latest placements on restore, instead of growing a queue.
	if r.Minimized() || slices.Equal(s.last, placements) {
		return nil
	}
	if len(placements) > s.capacity {
		return fmt.Errorf("instance stream: %d placements exceed capacity %d", len(placements), s.capacity)
	}
	var next *renderer.InstanceSet
	if n := len(s.free); n > 0 {
		next = s.free[n-1]
		s.free = s.free[:n-1]
	} else {
		var err error
		next, err = r.CreateInstanceSet(s.mesh, s.capacity, nil)
		if err != nil {
			return fmt.Errorf("instance stream: %w", err)
		}
	}
	r.UpdateInstanceSet(next, placements)
	previous := s.Set
	s.Set = next
	s.last = append(s.last[:0], placements...)
	// Do not guess maxFramesInFlight or rotate on application update counts:
	// paused/minimized/skipped draws need the renderer's actual retirement.
	r.DeferDestroy(func() { s.free = append(s.free, previous) })
	return nil
}
