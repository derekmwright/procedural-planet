# Optional intermediate directional shadow cascades

Procedural Planet needs shadow detail between nearby rocks and distant mountain
ranges. With the engine's two fixed cascades, keeping both endpoints leaves
hundred-metre shadow texels on ordinary hillsides. Configurable coverage from
#97 works as intended; the remaining limitation is the number of coverage tiers.

Verified against GlyphEngine `5a0d940cc8f7` (current main on 2026-10-07):

- `renderer/shadow.go` fixes `ShadowCascades = 2` and `ShadowMapSize = 2048`.
- `renderer.ShadowCoverage.Cascades`, the shadow array, and the shared atmosphere
  matrices use that fixed count. The application can change extents/caster reach,
  but cannot add a middle map through the public API.
- The consumer uses radii of 90 m and 120,000 m, with 200 km sunward and 120 km
  anti-sunward caster reach in both. XY texel sizes are approximately 0.088 m and
  117.2 m. The near map stops contributing beyond 72 m receiver distance; it must
  not select distant receivers merely because they project inside its XY bounds.

Reproduction in `derekmwright/procedural-planet`:

```powershell
go build -o bin/universebuild.exe .
.\bin\universebuild.exe -erosion '-longitude=50.238486' '-latitude=8.874780' -altitude=108 -flight '-heading=116.34983775' '-pitch=-5.4101525' -sync-terrain
```

The initial coarse collision mesh raises this flight pose; its settled ground
clearance is about 847 m. Inspect the sun/shadow boundary on the opposing slopes.
Disabling material detail leaves the coarse blocks; disabling shadows removes
them. Matched captures keep the camera, terrain and vegetation unchanged. An
application-side continuous tent filter removes sparse sampling holes without
extra fetches, but cannot recover missing spatial detail. Shrinking the far map
loses distant mountain/atmosphere coverage; enlarging the near map loses contact
detail around rocks. Raising the resolution of every existing map is a costly
way to bridge a 1,333x scale gap.

Requested capability: opt into at least three directional cascades with
application-selected coverage and a documented shader interface exposing the
active matrices/layers. Preserve the existing two-cascade default without extra
allocations or draws. A useful consumer configuration is radii 90 m, 3 km and
120 km, retaining independent caster depth in each. The middle tier would have
about 2.93 m texels at 2048 resolution. Per-tier resolution control would help
budget the additional allocation, but is not required to establish the seam.

Acceptance:

- A distant off-camera mountain can cast into near and middle receiver regions;
  extra XY detail must not shorten caster reach.
- Static, instanced and skinned casters, custom lit/sky/app-pass sampling and
  shadow culling agree on active layers and matrices.
- Camera-relative rebasing, snapped movement, disabling shadows, resize and
  destruction remain validation-clean. Shader ABI compatibility is explicit.
- The current two-cascade path keeps its allocation/work profile. Report the
  additional tier's GPU time and memory separately so consumers can budget it.

This requests renderer infrastructure; cascade selection/blending and the
planet's appearance remain application work. It does not request an engine-side
planet renderer, denoiser or global shadow-resolution increase.
