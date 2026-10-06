package main

import (
	"errors"
	"slices"
	"testing"

	"github.com/derekmwright/glyphengine/renderer"
)

// Model submitted GPU reads independently of the application's update count.
type streamRecorder struct {
	t         *testing.T
	data      map[*renderer.InstanceSet][]renderer.MeshInstance
	submitted map[*renderer.InstanceSet]bool
	retire    []func()
	minimized bool
	fail      bool
	writes    int
}

func (r *streamRecorder) CreateInstanceSet(mesh *renderer.Mesh, _ int, p []renderer.MeshInstance) (*renderer.InstanceSet, error) {
	if r.fail {
		return nil, errors.New("allocation failed")
	}
	s := &renderer.InstanceSet{Mesh: mesh}
	r.data[s] = slices.Clone(p)
	return s, nil
}
func (r *streamRecorder) UpdateInstanceSet(s *renderer.InstanceSet, p []renderer.MeshInstance) {
	if r.submitted[s] {
		r.t.Fatal("overwrote a buffer still referenced by a submitted frame")
	}
	r.data[s] = slices.Clone(p)
	r.writes++
}
func (r *streamRecorder) DeferDestroy(fn func()) { r.retire = append(r.retire, fn) }
func (r *streamRecorder) Minimized() bool        { return r.minimized }

func TestInstanceSnapshotsSurviveUpdatesUntilRetirement(t *testing.T) {
	r := &streamRecorder{t: t, data: make(map[*renderer.InstanceSet][]renderer.MeshInstance), submitted: make(map[*renderer.InstanceSet]bool)}
	s, err := newInstanceStream(r, &renderer.Mesh{}, 4)
	if err != nil {
		t.Fatal(err)
	}
	placements := make([]renderer.MeshInstance, 4)
	var held []*renderer.InstanceSet
	var expected [][]renderer.MeshInstance
	for frame := 0; frame < 120; frame++ {
		// Vary counts as well as camera-relative transforms, reusing CPU storage.
		placements[0].Model[12] = float32(frame) * 0.01
		p := placements[:1+frame%4]
		previous := s.Set
		if err := s.update(r, p); err != nil {
			t.Fatal(err)
		}
		held = append(held, previous)
		expected = append(expected, slices.Clone(r.data[previous]))
		r.submitted[previous], r.submitted[s.Set] = true, true
		// Delayed/skipped presentation: application updates don't imply fences.
		if len(held) > 5 {
			if !slices.Equal(r.data[held[0]], expected[0]) {
				t.Fatal("old frame's placement snapshot changed")
			}
			delete(r.submitted, held[0])
			r.retire[0]()
			r.retire = r.retire[1:]
			held, expected = held[1:], expected[1:]
		}
	}
	if len(r.data) > 7 {
		t.Fatalf("retired buffers were not reused: %d allocated", len(r.data))
	}
	set, writes := s.Set, r.writes
	if err := s.update(r, s.last); err != nil {
		t.Fatal(err)
	}
	if s.Set != set || r.writes != writes {
		t.Fatal("unchanged placements caused an unnecessary GPU write")
	}
	allocated := len(r.data)
	r.minimized = true
	for frame := 0; frame < 1000; frame++ {
		placements[0].Model[12]++
		if err := s.update(r, placements); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.data) != allocated || s.Set != set || r.writes != writes {
		t.Fatal("minimized updates grew or rewrote the GPU pool")
	}
	r.minimized = false
	if err := s.update(r, placements); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.data[s.Set], placements) {
		t.Fatal("restore didn't publish the latest placements")
	}
}

func TestInstanceAllocationFailureKeepsPublishedSnapshot(t *testing.T) {
	r := &streamRecorder{t: t, data: make(map[*renderer.InstanceSet][]renderer.MeshInstance), submitted: make(map[*renderer.InstanceSet]bool)}
	s, err := newInstanceStream(r, &renderer.Mesh{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	previous := s.Set
	r.submitted[previous] = true
	r.fail = true
	if err := s.update(r, []renderer.MeshInstance{{Tint: [4]float32{1, 1, 1, 1}}}); err == nil {
		t.Fatal("expected allocation error")
	}
	if s.Set != previous || len(s.last) != 0 || len(r.retire) != 0 {
		t.Fatal("failed allocation changed the published snapshot")
	}
}
