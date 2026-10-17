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
	for i := range targets {
		target, err := r.CreateRenderTarget(renderer.RenderTargetDesc{Name: []string{"wave gradients", "wave curvature and variance"}[i], Format: renderer.TargetRGBA16F, Width: 772, Height: 529, Filter: renderer.FilterLinear, Storage: true})
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
	pass.SetDispatch(97, 67, 1)
	return &WaveCache{pass}, nil
}
func (c *WaveCache) SetEnabled(enabled bool) { c.pass.SetEnabled(enabled) }
