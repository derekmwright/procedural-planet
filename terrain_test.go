package main

import (
	"github.com/derekmwright/glyphengine/renderer"
	"github.com/derekmwright/procedural-planet/planet"
	"reflect"
	"testing"
	"time"
)

func TestWorkerBuildAndShutdown(t *testing.T) {
	p := planet.Planet{Seed: 7, Radius: 500000}
	terrain := newTerrain(p, false)
	k := planet.Patch{Face: 4, Level: 2, X: 1, Y: 1}
	terrain.jobs <- k
	select {
	case result := <-terrain.results:
		if result.key != k || !reflect.DeepEqual(result.meshes[0], p.BuildTerrainPatch(k.Children()[0])) {
			t.Fatal("worker changed deterministic geometry")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("terrain worker stalled")
	}
	// Exit with another build queued; shutdown must join the worker even if
	// its result is never consumed by a rendered frame.
	terrain.jobs <- k
	terrain.close()
}

func TestChildrenStayHiddenUntilWholeReplacementIsReady(t *testing.T) {
	children := make([]patchEntity, 4)
	for n := 0; n < 4; n++ {
		if childrenReady(children[:n]) {
			t.Fatalf("published only %d children", n)
		}
	}
	if !childrenReady(children) {
		t.Fatal("synchronous children should publish together")
	}
	for i := range children {
		children[i].ticket = &renderer.UploadTicket{}
		if childrenReady(children) {
			t.Fatalf("published with child %d still uploading", i)
		}
		children[i].ticket = nil
	}
}
