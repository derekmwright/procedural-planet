package atmosphere

import (
	_ "embed"
	"github.com/derekmwright/glyphengine/renderer"
	"github.com/derekmwright/glyphengine/shaders"
)

//go:embed sun-table.frag.spv
var sunTable []byte

func NewSunTable(r *renderer.Renderer) error {
	target, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: "sun transmission", Format: renderer.TargetRGBA16F, Width: 512, Height: 128})
	if err != nil {
		return err
	}
	_, err = r.CreateAppPass(renderer.AppPassDesc{Name: "sun transmission", Stage: renderer.StageBeforeScene, Target: target, Fullscreen: true, Vert: shaders.DepthResolveVertSpv, Frag: sunTable, Timed: true})
	if err != nil {
		return err
	}
	return r.SetShaderTarget(0, target)
}
