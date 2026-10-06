package main

import (
	"testing"

	"github.com/derekmwright/glyphengine/renderer"
)

func TestPipelineWindowExcludesUnavailableMeasurements(t *testing.T) {
	frames := make([]perfFrame, 3)
	frames[0].pipeline.FragmentInvocations[renderer.PassOpaque] = 9999 // unavailable
	frames[1].pipeline.Valid, frames[2].pipeline.Valid = true, true
	frames[1].pipeline.FragmentInvocations[renderer.PassOpaque] = 120
	frames[2].pipeline.FragmentInvocations[renderer.PassOpaque] = 240
	frames[1].pipeline.ClippingPrimitives[renderer.PassOpaque] = 10
	frames[2].pipeline.ClippingPrimitives[renderer.PassOpaque] = 30
	passes, valid := meanPipelinePasses(frames)
	if valid != 2 || passes[renderer.PassOpaque.String()] != (perfPipelinePass{180, 20}) {
		t.Fatalf("invalid measurement diluted the mean: %d, %v", valid, passes)
	}
	if _, ok := passes[renderer.PassWater.String()]; ok {
		t.Fatal("unbracketed water pass was reported as measured zero")
	}
	if value, ok := passes[renderer.PassDepthPrepass.String()]; !ok || value != (perfPipelinePass{}) {
		t.Fatal("measured zero must remain distinguishable from an absent query")
	}
	passes, valid = meanPipelinePasses(frames[:1])
	if valid != 0 || passes != nil {
		t.Fatal("unavailable statistics must not claim measured work")
	}
}
