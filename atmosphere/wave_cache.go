package atmosphere

import (
	_ "embed"
	"github.com/derekmwright/glyphengine/renderer"
)

//go:embed wave-cache.comp.spv
var waveCacheShader []byte

type WaveCache struct{ pass *renderer.AppCompute }

func NewWaveCache(r *renderer.Renderer) (*WaveCache, error) {
	targets := make([]*renderer.RenderTarget, 2)
	for i, format := range []renderer.TargetFormat{renderer.TargetRGBA16F, renderer.TargetR16F} {
		target, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: []string{"wave gradients", "wave curvature"}[i], Format: format, Width: 512, Height: 512, Filter: renderer.FilterLinear, Wrap: renderer.WrapRepeat, Storage: true})
		if err != nil {
			return nil, err
		}
		targets[i] = target
		if err = r.SetShaderTarget(i+1, target); err != nil {
			return nil, err
		}
	}
	pass, err := r.CreateAppCompute(renderer.AppComputeDesc{Name: "wave field", Stage: renderer.StageBeforeScene, Comp: waveCacheShader, Writes: targets, Timed: true})
	if err != nil {
		return nil, err
	}
	pass.SetDispatch(64, 64, 1)
	return &WaveCache{pass}, nil
}
func (c *WaveCache) SetEnabled(enabled bool) { c.pass.SetEnabled(enabled) }
