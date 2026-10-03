// Package atmosphere supplies paired sky/terrain shaders and their shared
// per-frame atmosphere data. Distances inside the shaders are kilometers.
package atmosphere

import (
	_ "embed"
	"encoding/binary"
	"math"

	"github.com/derekmwright/glyphengine/renderer"
)

//go:generate powershell -NoProfile -File compile.ps1

//go:embed sky.frag.spv
var sky []byte

//go:embed terrain.frag.spv
var terrain []byte

func Shaders() renderer.ShaderSet {
	return renderer.ShaderSet{SkyFrag: sky, LitFrag: terrain}
}

// Parameters owns eleven std140 vec4s in the engine's application UBO at
// set 1, binding 6. Engine sky colors and shadow data remain engine-owned.
type Parameters struct {
	Eye, Planet, Rayleigh, Mie, Water, Detail, Features, Rendering [4]float32
	CausticOrigin, CausticU, CausticV                              [4]float32
}

// Bytes packs explicitly rather than relying on Go struct layout.
func (p Parameters) Bytes() [176]byte {
	var result [176]byte
	for slot, values := range [11][4]float32{p.Eye, p.Planet, p.Rayleigh, p.Mie, p.Water, p.Detail, p.Features, p.Rendering, p.CausticOrigin, p.CausticU, p.CausticV} {
		for axis, value := range values {
			binary.LittleEndian.PutUint32(result[(slot*4+axis)*4:], math.Float32bits(value))
		}
	}
	return result
}

func FrameParameters(radius float64, eye [3]float64, enabled bool) Parameters {
	r := radius / 1000
	h := math.Max(1.4, math.Min(8, r*0.007))
	scale := 3.5 / h
	on := float32(0)
	if enabled {
		on = 1
	}
	remainder := func(v float64) float32 { return float32(math.Mod(math.Mod(v, 4096)+4096, 4096)) }
	return Parameters{
		Eye:      [4]float32{float32(eye[0] / 1000), float32(eye[1] / 1000), float32(eye[2] / 1000)},
		Planet:   [4]float32{float32(r), float32(h), on},
		Rayleigh: [4]float32{float32(0.0145 * scale), float32(0.0338 * scale), float32(0.08275 * scale)},
		Mie:      [4]float32{float32(0.012 * scale), float32(h * 0.34)},
		Water:    [4]float32{0, 1, remainder(eye[2])},
		Detail:   [4]float32{0, remainder(eye[0]), remainder(eye[1])},
	}
}
