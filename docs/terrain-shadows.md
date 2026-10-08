# Terrain shadow filtering

## Continuous four-fetch filter (2026-10-07)

A deferred screenshot showed blocky light/dark patches on a shadowed green
hillside. The exact camera pose was not recorded. A deterministic eroded-valley
view reproduced coarse shadow boundaries: turning off material detail leaves
them, while turning off shadows removes them. This isolates a shadow problem in
that reproduction; it does not establish that every patch in the original
screenshot has the same cause.

The previous surface PCF filter used four hardware comparisons at offsets
`(+/-1, +/-1)` shadow texels. At texel-centred coordinates these sample only four
corners, omitting the centre and axial neighbours. As the receiver moves through
fractional coordinates the weights shift between different sparse patterns.
An alternating-texel shadow field exposes this as a grid that changes brightness
with sample alignment.

The filter now evaluates a separable `[1, 2, 1]` tent. For each axis, bilinear
interpolation at fractional coordinate `f` expands its weights to
`[1-f, 2-f, 1+f, f]`. Pairing adjacent weights gives two hardware samples per
axis, or four total, with all weights continuous and normalized. This first
change preserved receiver-plane correction, cascade coverage, distant caster
reach, near/far selection and underwater shadow fade. The receiver correction
was subsequently revised as described below.

The packing follows the bilinear-PCF principle explained in
[Ignacio Castaño's shadow filtering article](https://www.ludicon.com/castano/blog/articles/shadow-mapping-summary-part-1/).
This implementation derives the smaller three-tap tent above. It does not add
random offsets, temporal history, render targets or texture fetches.

## Validation and cost

`TestShadowFilterGPU` renders alternating single-texel casters into the engine's
actual far cascade. It compares the optimized filter against an independent
nine-sample convolution over 8,192 fractional sample positions. The old filter
fails 7,424 pixels; the new filter passes. The test also checks that the source
shadow pattern has contrast, preventing a uniformly empty map from passing.
`TestShadowReceiverSelectionGPU` continues to test near/far selection and full,
partial, disabled and restored shadow strength.

```powershell
$env:PLANET_GPU_TEST='1'
go test ./atmosphere -run 'TestShadow(Filter|ReceiverSelection)GPU' -count=1
```

Go tests and vet pass. Vulkan-enabled eroded-valley captures show the result with
unchanged camera/terrain/vegetation. Final ground-rock and sunset-mountain views
were also inspected and are validation-clean. Comparison artifacts and scripts
are in the ignored `captures/terrain-shadow-artifacts` directory.

On the RX 7900 XTX at 3840x2054, an old/new/new/old sequence used 1,201 frames per
scene, discarded the first 720 and collected eight complete 120-frame timing
windows per mode. Camera, terrain, vegetation and draw/instance/triangle counts
matched. Validation and screenshots were off during measurement. Values below
are medians of window means, not pooled per-frame percentiles.

| View | Total GPU old / new | Opaque old / new |
| --- | ---: | ---: |
| Ground | 3.105 / 3.121 ms | 0.687 / 0.700 ms |
| Mountains | 3.482 / 3.488 ms | 0.309 / 0.320 ms |

The measured opaque increase is about 0.01 ms in these views. Other hardware and
views may differ.

## Bounded mountain-shadow correction (2026-10-08)

A clearer follow-up screenshot showed angular light/dark patches along a soft
shadow boundary. An isolated shadow-mask capture of the eroded valley exposed
triangle-shaped discontinuities that were much harder to distinguish under
normal lighting. Extrapolating each receiving triangle's depth plane across
the far map's 117 m texels was the culprit in this reproduction. Near a tangent
sun angle, the correction could extend over a kilometre and even leak light
through an occluder 400 m away. Interpolating the receiver normal alone did not
remove the breakup. Removing the correction entirely introduced self-shadow
bands on unoccluded slopes.

The far cascade now offsets the receiver along its unperturbed, interpolated
surface normal by `1.5 * worldTexelSize * sin(normalLightAngle)`. This bounds
the offset to 176 m at the current far-map resolution, takes it smoothly to zero
for perpendicular sunlight, and avoids per-triangle depth extrapolation. The
PCF filter uses a constant reference depth plus its existing 0.5 m bias. Nearby
contact shadows retain their geometric receiver-plane correction; material
normal detail cannot affect the far-map offset. Analytic water uses its radial
normal. Atmospheric shadow sampling and underwater suppression are unchanged.

Normal offset is a shadow-map approximation: it can shift a far shadow edge by
roughly a map texel. It does not add missing map detail or replace the need for
intermediate cascades. The same four texture comparisons are used, with no new
passes, targets, history, geometry, or shadow maps.

`TestShadowReceiverBiasGPU` renders a real opaque blocker and seven inclined
planes into the engine's shadow map. It sweeps tangent receiver normals and
fractional texel positions, checking blocked, open and self-shadow cases. The
old correction incorrectly lights all 4,096 blocked samples. Simply disabling
correction fails 1,648 samples on each of four axis-aligned slopes and all 4,096
samples on each of two steep diagonal slopes.
The bounded normal offset passes all 36,864 samples. The existing filtering
and cascade-selection GPU regressions also pass.

```powershell
$env:PLANET_GPU_TEST='1'
go test ./atmosphere -run 'TestShadow(Filter|ReceiverSelection|ReceiverBias)GPU' -count=1
```

Diagnostic and normal-lit comparisons are in the ignored
`captures/terrain-shadow-artifacts` directory (`investigate-shadow-*` and
`bounded-receiver`). These establish the reproduced receiver-bias defect; the
exact camera pose in the user's screenshot was not recorded.

The normal-lit eroded valley, ground rocks, sunset mountains and shoreline were
inspected; Vulkan validation is clean. Go tests and vet pass. An old/new/new/old
4K benchmark sequence used the same 1,201-frame runs, 720-frame warmup and eight
complete windows per mode as above. Camera, terrain, vegetation and submission
counts matched across modes on the RX 7900 XTX:

| View | Total GPU previous / bounded | Opaque previous / bounded |
| --- | ---: | ---: |
| Ground | 3.127 / 3.094 ms | 0.704 / 0.680 ms |
| Mountains | 3.496 / 3.486 ms | 0.301 / 0.304 ms |

These measurements show essentially unchanged total cost, rather than evidence
of a general speedup. Raw runs are `oct8-perf-*` in the same capture directory.

## Remaining resolution limit

The engine supplies two 2048-square directional maps. Our 90 m and 120 km radii
give approximately 0.088 m and 117.2 m texels. The improved filter fills sampling
holes, but cannot recover missing detail from the far map: coarse shadow edges
remain. Reducing its coverage would lose distant mountain and atmospheric
shadows; expanding the near map would lose contact detail.

[GlyphEngine #189](https://github.com/derekmwright/glyphengine/issues/189) requests
optional intermediate cascades while preserving the current default cost and
documenting the shader interface. A middle 3 km radius map would offer about
2.93 m texels without changing the two existing coverage endpoints. Its GPU and
memory cost must be measured before adoption. The local issue proposal is
[here](engine-issues/intermediate-directional-cascades.md).
