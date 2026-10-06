# Wave-driven water lighting

## Current caustic field (2026-10-02)

The longstanding broad dark rosettes were reproduced with a fixed camera at
longitude20, latitude12.6, sea level150.4257m, ground clearance5m, heading-72,
pitch-12. Actual camera depth is20m. This is independent of the atmosphere
refactor. The old single-branch inverse solver discards folded light paths;
the wave spectrum also lacked shorter ripples. Neither problem was an engine
regression.

Default `-caustic-cache=true` now forward-projects a surface mesh along refracted
sunlight onto eight local receiver planes. Reversed and overlapping triangles
remain visible and add their light into an R16F atlas. Each triangle contributes
incident area / projected area. Reflection, refraction and focusing share the
same wave normals. The 32 original spectral terms remain, with16 weaker short
terms (roughly1.5-2.6m wavelengths) adding curvature and breaking up broad cells.
No caustic decal or camera-facing radial mask is used.

This is an independent adaptation of the projected differential-area principle
in Evan Wallace's [WebGL Water renderer](https://github.com/evanw/webgl-water/blob/master/renderer.js).
No reference source or assets were copied.

Implementation and bounds:

- `water-wave.glsl`: shared wave normals/curvature, backed by the existing compute field.
- `caustic-cache.vert/.frag`:384x384 source cells per nonzero depth, eight depths
  {0,2,5,10,17,26,38,50}m, with additive accumulation and no back-face culling.
  The source spans144m; the inner receiver area is128m square. This submits
  2,064,386 triangles in one draw per active frame. The zero-depth map is the
  identity (uniform intensity one), so it uses only a quad. Nonzero-depth
  sampling density is unchanged by this performance optimization.
- A4096x2048 R16F projection target uses4x2 tiles of1024x1024. A2x2 box resolve
  integrates four subpixel samples into2048x1024, followed by separable nine-tap
  Gaussian filtering. The normalized kernel spreads unresolved peaks, with
  increasing diffusion at depth. Sampling interpolates between depths.
  Four targets use28MiB total, excluding the unchanged static mesh buffers.
- The patch anchors to the sea sphere, using a camera-relative float origin
  computed from double-precision world coordinates. It follows after2m of
  lateral motion in0.75m grid-aligned increments, preserving both the source
  and receiver sampling phases. The tangent frame persists for256m (less on
  small planets), then refreshes. Focusing fades to neutral18.8-47m around the current camera
  (horizontal distance, independent of sun direction and atlas orientation) and
  between20-50m depth. This is local lighting, not a planet-wide photon map.
- Generation disables with oceans off, low/no sun, or camera height outside
  -80..350m relative to sea level. Inactive maps cannot supply stale light.
- Both seabed direct light and underwater scattering sample this field. The
  cheaper lookups permit12 scattering samples instead of6, with unchanged
  scattering strength and phase function. `-caustic-cache=false` retains the
  old inverse solver and6 samples for comparison. Both variants share the new
  wave spectrum; `-wave-cache=false` separately compares the historical5-wave field.
- GPU timings: `water focusing`, `water focusing resolve`, `water focusing blur`,
  `water focusing filter`, `water scattering`, `water composite`.
  HUD focusing time combines the first four. JSONL records
  the selected CausticCache flag and individual timers.

The overhead-sun test still has much weaker visible shaft contrast than the
oblique test. There is no overhead cutoff: the forward-scattering phase and
view direction strongly affect visibility. This change does not claim to solve
all overhead-angle appearance. The surface still has no displaced geometry;
receiver planes approximate curved/sloped terrain, peaks are capped, and there
is no incoming terrain occlusion in the volume, multiple scattering or temporal
reconstruction. Some cellular structure and spatial sampling grain remain.
Moving-camera judgment still benefits from user review.

### Caustic stability refinement (2026-10-02)

The weak0.75-texel tent filter left narrow focused triangles flashing between
receiver samples. Moving the cache also changed its sampling phase every2m.
The current implementation uses four spatial samples per resolved texel,
normalized separable diffusion (0.35m base sigma, widening with depth), and
grid-aligned movement with a persistent local tangent frame. Wave amplitudes,
frequencies and speeds are unchanged. There is no temporal history or lagged
screen-space accumulation. The47m coverage bound includes the full1m filter
footprint, both interpolated depths, and maximum2m anchor drift.

`captures/check_caustic_stability.py` records four consecutive fixed60Hz frame
times (841..844) in separate deterministic runs, plus a normal lit view. Camera
and engine-counted geometry match across before/after at both poses. The
grayscale diagnostic isolates focusing; `captures/measure_caustic_stability.go`
measures the lower central image region. RMS second temporal differences fell
1.788->1.009 and4.465->2.692 (8-bit values) for the shallow and overhead views.
Mean brightness changed137.545->137.958 and132.663->134.448. Contrast also fell;
after normalizing by contrast, improvement is23% and5%, respectively. This is
a short numerical stability check, not proof that all perceptible shimmer is
gone. Gaussian-only intermediate results are in `caustic-stability-after`;
the final supersampled results are in `caustic-stability-sampled`.

Before/after4K timings use two1201-frame runs per pose,600-frame warmup, and
ten complete120-frame windows per variant, engine b099462. Camera and scene
geometry match. Median window-mean total GPU time: oblique3.466->3.709ms,
overhead3.732->4.001ms. Combined focusing passes cost0.340->0.644ms and
0.330->0.641ms. This refinement spends about0.25ms total GPU time for quality;
it is not a performance optimization. Render-target memory grows8->28MiB.
Baseline and final measurements, binary hashes and commands live under
`captures/caustic-stability-perf-before` and `-sampled`.

### Original forward-cache cost comparison (before stability refinement)

RX7900XTX,3840x2054, engine b099462, same executable and wave spectrum, VSync off,
shadows on, synchronous terrain, two runs per variant in alternating order.
1,201 frames/run;600-frame warmup;five complete120-frame windows/run. Camera
and engine-counted scene geometry match throughout the measured windows.
Values are medians of10 window means, not pooled per-frame percentiles.

| Scene | Inverse solver GPU ms | Forward cache GPU ms |
| --- | ---: | ---: |
| Oblique sunlight,20m underwater |4.324|3.552|
| Overhead sunlight,20m underwater |4.612|3.804|

Projection+filter costs about0.35-0.36ms; scattering falls from0.61-0.67ms to
0.20ms even with twice the samples. Total improvement is about18% in these
views, not a claim for every resolution/view. At1080p the fixed atlas cost can
outweigh the fragment savings. Full suite and screenshots:
`captures/caustic-final-4k/summary.json`.

Reproduce:
```
python tools/profile_scene.py --scenes caustic-oblique caustic-overhead --variants caustic-legacy caustic-forward --repeats 2 --frames 1201 --warmup 600 --screenshots
```

The earlier `captures/caustic-forward-4k` suite intentionally failed its geometry
parity check because terrain was still settling at frame480. Do not use that
suite as the final result.

Engine `RenderStats` currently omits application mesh passes. The additional
2,359,296 projection triangles are therefore absent from the HUD's submitted
count even though their GPU time is measured. Filed
[engine #162](https://github.com/derekmwright/glyphengine/issues/162) for explicit
application submission counts. No engine change was needed for this visual fix.

### Validation

Shader generation, Go tests/vet, wave derivative/seam checks and refraction
checks pass. The cache unit test verifies camera-relative anchor stability,
reanchoring onto the sea sphere, tangent orientation and stale-cache exclusion.
Vulkan validation is clean for oblique underwater, shoreline, shallow water,
legacy uncached waves with full-resolution scattering, oceans disabled, and
an entire3,600-frame descent/ascent tour. The tour enters and exits water and
activates/deactivates focusing. Logs and captures are in
`captures/caustic-final-validation/`. The canonical executable is rebuilt and
left closed. These checks do not substitute for the user's moving-camera review.

## Cache coverage correction (2026-10-02)

A user flight exposed a diagonal cutoff at the square cache boundary. The first
version centered surface-entry coordinates, so refraction shifted coverage away
from the camera. Up to24m of anchor lag could expose the edge nearby. Isolated
caustic captures reproduce the square footprint; it was showcase cache coverage,
not a terrain shadow or engine regression.

Each depth plane now covers the receiving water column under the camera. Its
source mesh is offset sunward by the flat refracted path for that depth. Lookup
interpolates along that same beam between planes; using identical UVs after
this change would incorrectly blend unrelated vertical columns. The visible
contribution uses a broad circular19.2-48m fade around the live camera, and the
anchor follows within2m. Coverage is independent of both atlas axes and sunlight.
Atlas dimensions, mesh triangle count and number of passes are unchanged.

`TestCausticCoverageContainsEveryDepthLookup` reads the actual shader depth knots
and checks that the maximum refracted slope at the horizon, anchor drift, and
coverage radius keep BOTH depth lookups inside valid texel centers. The anchor
rebase/inactivation test now exercises the2m threshold. Go tests/vet and shader
compilation pass. Matched before/after captures and Vulkan validation logs:
`captures/caustic-edge-before/`, `captures/caustic-edge-after/`. The earlier4K
performance table predates this coverage correction; it is not a fresh benchmark
of these few shader-coordinate changes. The user's exact flight pose is unknown.
A1,201-frame moving underwater orbit also passed validation and crossed about75m
of surface travel, exercising repeated cache reanchoring. Captures and logs:
`captures/caustic-edge-motion/`.

## Historical implementation notes

The following sections record earlier implementations and measurements; the
current forward cache above supersedes the single-branch inverse solver.

Reference: Evan Wallace, [WebGL Water](https://github.com/evanw/webgl-water),
[renderer.js](https://github.com/evanw/webgl-water/blob/master/renderer.js),
causticsShader. The source header states Copyright 2011 Evan Wallace, MIT license.
No source or assets are copied. This implementation independently adapts the
refracted differential-area principle (incident area / receiving area).

`atmosphere/waterlight.glsl` supplies a single periodic wave gradient and analytic
Hessian. Reflection, transmission and light focusing use that same normal field.
Incoming sunlight refracts using IOR 1.333. The derivative of its intersection
with a local receiver-depth plane gives an area Jacobian. An inverse solve finds
the surface entry associated with each shaded point. The inverse determinant
provides focusing: expansion dims light, compression brightens it. There is no
view-dependent sun-reflection mask and no painted caustic web.

Underwater single scattering integrates this same field through the visible
water segment. The old angular starburst is removed. Six stratified samples
use stable screen-space jitter to avoid coherent sample planes. This can show
fine grain and is more expensive than the old fake. R disables scattering.

This is a bounded prototype, not a port of the reference renderer or a full
photon mapper. The reference uses a projected mesh and a caustics render target;
GlyphEngine currently exposes shader replacements, not application-owned passes
and render targets. This direct shader approach avoids engine modifications.

Limitations:
- Local tangent receiver planes; no terrain interception along incoming rays.
- Single inverse-map branch; folded/multiple ray paths fall back to neutral
  lighting. Focusing fades between 20 and 50 m to bound this approximation.
- Small-amplitude surface normals; the analytic sea sphere is not displaced.
- Peak focusing is clamped to avoid singular points and unresolved sparkle.
- Existing local receiver shadows remain; scattering is not terrain-shadowed.
- No multiple scattering, full terrain reflections, or temporal reconstruction.

The reduced-resolution scattering pass below is now implemented. A future
projected refracted light mesh with additive accumulation into floating-point
lighting targets and temporal history remains possible. This can handle overlapping
light paths and amortize the focusing calculation across pixels.

## Validation and cost

`python tools/check_waterlight.py` checks analytic derivatives against finite
ray-projection differences for 200 orientations (maximum error 1.38e-8), flat
water's unit focusing, and 4096 m rebasing. These check the math, not GPU execution.
Go tests and vet pass. Vulkan validation covered shallow, deep, above-water,
scattering-disabled, and full descent/ascent views.

At 3840x2054 on RX 7900 XTX, 450 fixed frames, synchronous terrain, VSync off,
7.8 m underwater: six-sample scattering measured 11.140 ms GPU; disabled measured
2.280 ms. Single runs, not a general benchmark. The ten-sample experiment was
15.933 ms. Use R for an immediate scattering comparison. This prototype can put
substantial load on the GPU at 4K; it has not preserved the old fake's cost.

Engine capability request: https://github.com/derekmwright/glyphengine/issues/99.
Captures/logs: captures/water-physical-*. Shader math check: tools/check_waterlight.py.

## Wave appearance follow-up

The initial three short, crossing harmonics made a regular crosshatch. Waves now
use unequal, predominantly aligned integer-lattice frequencies and long-period
phase modulation, with lower slopes. Both the gradient and Hessian include the
modulation derivatives; reflection, transmission and focusing still agree.
Numerical derivative checks pass (maximum error 2.05e-10) including 4096 m wrapping.

Matched above-water runs at 3840x2054: 14.570 ms before, 14.798 ms after (single
runs). Opaque dominates: 13.305 / 13.526 ms. These runs do not execute underwater
scattering. They do not demonstrate a performance improvement. Command:
-longitude -62 -latitude 22 -altitude 70 -flight -heading -72 -pitch -15
-sync-terrain -frames 450 -vsync=false -validate. Actual final clearance is
336.09 m because startup terrain refinement constrains the flight camera.
Captures/logs: captures/waves-before.* and captures/waves-after.*.

## Engine update integration

At engine df04ab7, planet/water parameters now use the application-owned
std140 block at set 1, binding 6. `Parameters.Bytes` packs six vec4s (96 bytes)
explicitly in little-endian order. Sky and terrain read that same block; the
engine sky palette is no longer repurposed. Unit checks cover packing and camera
rebasing. This migration does not cache lighting or reduce scattering resolution.

The checkout also includes configurable directional shadow volumes and sky
shadow bindings. Existing shadow coverage remains unchanged to avoid introducing
new shadow cost while the rendering-pass work is underway. Custom offscreen
render targets/passes from issue #99 are not present in this engine revision.


## Half-resolution scattering (2026-09-24)

Uses the engine custom targets/passes introduced through #99 (local engine
483fada; subsequent fed81ad changes documentation). Default `-water-passes=true`:

- An RGBA16F target at half width/height stores integrated underwater light and
  opaque reverse-Z depth. World rays are reconstructed from the inverse VP.
- Integration stops at the nearer of opaque geometry and the analytic sea exit.
- A depth-aware four-tap additive composite applies it before bloom/tonemapping.
- Terrain and sky skip their original scattering term to avoid double counting.
- Passes disable above water, with ocean off, or with sun rays off (R).
- GPU HUD and JSONL profiles report `water scattering` and `water composite`.
  Timing slices are copied because the engine reuses their backing storage.

`-water-passes=false` preserves the original full-resolution implementation and
avoids allocating the targets or requesting depth resolve. This is an A/B option,
not a second executable. Seabed focusing and surface reflection remain full resolution.
No temporal reconstruction, caustic light-map cache, or terrain shadowing of the
scattering was added. Half-resolution jitter is spatially filtered; very fine
scattering detail can soften.

Three interleaved 450-frame runs at 3840x2054, RX 7900 XTX, fixed simulation,
synchronous terrain, VSync off, shallow underwater pose:

| Path | GPU frame times (ms) |
| --- | --- |
| Full resolution | 14.789, 14.736, 14.622 |
| Half resolution | 4.255, 3.984, 4.067 |

Median reduction is about 72%. These are engine timing-log samples after warmup,
not a universal performance guarantee. Pose: `-sea-level 137.3 -altitude 2 -flight
-heading -72 -pitch 20 -sync-terrain -frames 450 -vsync=false`.
Matched screenshots excluding the HUD: RGB mean absolute difference 0.4603/255,
RMS 0.8097/255, maximum 6/255. The view retains its light distribution.

Above-water coast single-run comparison: passes enabled 15.372 ms, disabled
15.018 ms. There is no above-water speedup: requesting scene depth retains a
depth-resolve cost even while the underwater passes are disabled. Reflected
atmosphere shading remains the next major performance opportunity.

Validation covered 4K shallow water, 33.7 m depth at 1001x701, shafts disabled,
and a complete 3600-frame descent/ascent tour. Go tests, vet, shader compilation,
and independent water-light derivative checks passed. Captures/logs are
`captures/water-pass-*` and `captures/water-bench-*`.


## Shallow-water caustic contrast follow-up

The previous aligned waves produced only 0.96-1.05 raw focusing at 5 m depth
in a sampled 20x20 m patch at the default planet orientation. This was a weak
source light field before absorption or compositing, not a half-resolution bug.
Three shorter, differently directed ripples now drive both normals and focusing.
The same raw projection probe spans about 0.58-2.21 at 5 m; this is diagnostic
incident-area sampling, not a receiver-integrated energy or image measurement.
Wave evaluation and integration sample counts are unchanged.

The first stronger trial exposed neutral holes at rejected folds. Final amplitudes
are lower (4.5, 3.5 and 2.5 cm), with a smooth confidence fade near the determinant
cutoff. This remains a single-branch inverse solver: overlapping refracted paths
are not accumulated. Deep-water caustics still fade; shafts remain relatively
soft. A forward light accumulation target is the appropriate future treatment
for fold caustics rather than increasing these amplitudes indefinitely.

`-caustics-debug` shows unattenuated focusing on submerged terrain/rocks, before
material color and absorption. Neutral illumination is gray. This bypasses
receiver filtering intentionally and disables the separate scattering passes;
it is for inspection, not performance comparison.

Final validation: Go tests/vet and numerical derivative checks pass (maximum
error 1.81e-9). Vulkan validation passed the shallow seabed and sun-facing captures.
At 3840x2054, 450 fixed frames, synchronous terrain, VSync off: sun-facing GPU
sample 4.146 ms (previous pass version 3.984-4.255 ms); downward view 6.121 ms.
These single-run samples indicate the previous performance gain is retained,
not an additional speedup. Above-water reflection cost is unchanged.
Final images/logs: captures/caustic-final.png and captures/caustic-final-sun.png.


## Shoreline GPU cost and rock shading (2026-09-24)

Default `-atmosphere-cache=true` builds a 512x128 RGBA16F sun-transmission table
in a StageBeforeScene pass, using the existing six-step spherical sunlight
integral. Altitude and solar zenith angle parameterize the table; nonlinear
coordinates concentrate samples around low altitudes and horizontal sun angles.
Scene shaders interpolate the table inside its altitude domain and retain the
analytic fallback outside it. The 16-step view integral is unchanged, but no
longer nests six sunlight steps per view sample. It benefits land haze, sky,
and reflected/refracted water lighting. App sampler slot 0 is reserved for it;
Planet.w enables sampling. A/B baseline: `-atmosphere-cache=false`.

The table updates each frame (radius/atmosphere data therefore cannot become
stale); its GPU cost appears in the HUD/profile as `sun transmission`.
This is caching of the current approximation, not a new scattering model.

Matched coast, 3840x2054, 450 fixed frames, synchronous terrain, VSync off:
GPU timing samples 15.005 ms without table and 6.698 ms with table (single runs,
about 55% lower). Excluding HUD, RGB absolute image error averages 0.0271/255,
maximum 1/255. A terminator pose averages 0.0592/255, maximum 2/255. These are
sampled views, not an error bound for all planet sizes and camera positions.
Validation includes those views and a full 3600-frame descent/ascent at an odd
viewport size. Go tests and vet pass. See captures/sun-cache-*.

Rocks previously combined per-triangle random color, flat normals, and terrain
slope-dependent materials. They now use area-weighted shared normals blended
with 20% face normal, consistent per-mesh color plus existing per-instance tint,
and UV.y=1 to select a rock material independent of slope. Mesh topology and
shadow-map coverage are unchanged; contact shadow limitations remain.


## Irregular wave packets

The shared wave field now uses five unequal components, including two weaker
short ripples. Each has a spatially varying amplitude envelope and a shorter
phase warp, so crests bend and weaken over local patches instead of maintaining
a planet-wide interference grid. Gradient and Hessian include the envelope,
phase warp, and their cross derivatives. Surface normals and caustics still use
one field; integer lattice vectors preserve 4096 m origin wrapping.

The inverse-map confidence fade now considers the minimum determinant across
all iterations, avoiding an abrupt transition when an intermediate solve step
approaches the fold cutoff. This does not add multi-path accumulation; strong
fold caustics are still approximate. Residual repeating structure remains
possible with a finite wave spectrum. No geometry displacement or breakers.

Numerical derivative/wrap/flat-water checks pass, maximum derivative error
7.21e-9; Go tests/vet and Vulkan validation pass for overhead, surface and
underwater views. Matched 3840x2054 overhead pose (450 fixed frames, synchronous
terrain, VSync off) timing samples: 5.191 ms old waves, 5.897 ms wave packets.
The added variation costs about 0.7 ms in this single comparison; it is not free.
The five-component 4K underwater test measured 6.974 ms before the final
minimum-determinant fade adjustment. Wave computation remains direct; no wave
cache was introduced. Existing atmospheric caching and half-resolution
scattering remain enabled. Captures/logs: captures/wave-packets-*.


## Cached broad spectrum (current default)

`-wave-cache=true` replaces the five direct components with 32 Fourier modes
cached in two 514x514 targets (RGBA16F gradient/diagonal Hessian, R16F mixed
Hessian). Three offset coordinate-plane projections create a continuous 3D
height field; its gradient and Hessian are projected onto the local planetary
tangent. Reflection, refraction and caustic inversion sample this same field.
Integer wavevectors preserve a 128 m period, compatible with 4096 m eye rebasing.
Two gutter texels preserve interpolation across the period. Speed rounding
preserves the existing 62.83 second animation loop. These are shading waves,
not displaced ocean geometry, an FFT solver, or breaking-wave simulation.

Both tables refresh before scene rendering and disable when ocean is off or the
camera is more than 4 km from sea level. Distant rendering uses the previous
field, where existing wave detail fades away. App sampler slots 1 and 2 are
reserved for the wave cache; Mie.w enables it. `-wave-cache=false` retains the
previous five-wave baseline. HUD/JSONL include wave gradients/curvature timings.

The engine currently creates nearest-only render-target samplers. Explicit
four-tap bilinear sampling removes visible cache texel blocks; configurable
filtering is filed as https://github.com/derekmwright/glyphengine/issues/124.
A variance-based specular lobe adjustment also broadens unresolved glints while
reducing peak energy. The cache uses 0.25 m samples; interpolation softens the
finest focusing patterns. The finite tile can still repeat at large distances.

Build now targets Go 1.27 to match engine 9088bdf and its updated dependencies.
Go tests/vet, Vulkan validation (overhead, sun reflection, underwater and complete
orbit/surface tour), and tools/check_wave_spectrum.py pass. The independent
numerical check covers spectral derivatives, coordinate-plane mapping, periodic
rebasing and gutter continuity; it is not a GPU interpolation error bound.
Maximum derivative discrepancy: 5.62e-10.

Final-strength, bilinear-filtered 4K captures measured 5.828 ms underwater and
6.909 ms for the sun reflection (single timing samples before the final specular
variance adjustment). Matched old-wave reflection was 7.162 ms. Cache generation
was about 0.07 ms in the HUD; these samples do not establish a general speedup.
Captures/logs: captures/spectrum-*. Final reflection: spectrum-ready-glint.png.

## Showcase shoreline foam

Enabled by default; `-shore-foam=false` disables foam and wet sand for comparison.
The existing ocean surface shader derives radial bed depth from the opaque
intersection and shades broken, animated foam fronts in the first 1.6 m of water.
Periodic world-space noise breaks up the fronts and adds fine bubble coverage;
subpixel bubble detail fades toward its average. The bands move toward shallower
water and fade offshore. Gently sloping terrain receives a softly varying wet
strip, with a smooth submerged fade. No extra mesh, render target or pass is added.
Application parameter Eye.w carries the enable flag.

This is surface shading, not a breaking-wave or fluid simulation. It uses the
visible bed intersection, so cliffs/occlusion and grazing views remain approximate;
foam is limited to a 150 m water sightline. The underside of the water does not
render foam yet. The existing sea surface geometry stays spherical.

Validation: Go tests/vet pass, shaders compile, beach-height and overhead captures
run with Vulkan validation without errors. Matched 3840x2054 overhead timing samples
were 5.736 ms with foam and 5.719 ms without (single runs, within timing noise).
Captures: foam-beach.png and foam-final-true.png; baseline foam-final-false.png.

### Shoreline close-up refinement

Replaced the sine-based noise hash with an integer hash and removed the tiny
bright bubble layer. Foam now uses broader ragged coverage and lower diffuse
brightness; its long water-sightline cutoff fades smoothly. Wet sand has a wider,
noise-varied transition. Terrain sunlight direction blends into refraction over
the first 0.4 m of water, removing an abrupt lighting contour at sea level.
This interface blend is a visual approximation for the unresolved wet edge.

Shader generation, canonical build, go test ./... and go vet ./... pass.
Beach-height and 4K overhead runs passed Vulkan validation; inspected captures
are captures/foam-soft-beach.png and captures/foam-soft-overhead.png. These are
visual checks, not a controlled performance benchmark.

### Engine linear/repeat field samplers (#127)

The two wave-field targets now use FilterLinear and WrapRepeat. Hardware sampling
replaces the manual four-fetch interpolation for each field. Targets are 512x512
without duplicate gutters; texel zero stores world coordinate zero, and normalized
sampling includes a half-texel offset to preserve the previous field phase.
The 128 m period and 0.25 m spacing are unchanged. This supersedes the nearest-only
sampler workaround described above. Canonical executable rebuilt; surface and underwater 3840x2054 captures passed Vulkan validation and visual inspection. Captures/logs: captures/linear-repeat-*. Single GPU timing samples were 7.221 ms surface and 5.358 ms underwater; these are smoke-test observations, not a controlled speedup measurement.

### Dark-limb orange specks

Reproduced at longitude 100, latitude 12.6, altitude 300000, 1920x1080.
The sky sun-disc fwidth was evaluated after ocean/underwater early returns.
Divergent fragment neighbors at the ocean silhouette made the antialias width
undefined, admitting sun-disc fragments on the dark limb. Atmosphere tinted them
orange; they were white with atmosphere off. Oceans off removed them, while
shafts, foam, and atmosphere-cache toggles did not. Water reflection/specular
isolation also left the specks intact.

Moved the angular-separation derivative before every early return in sky.frag.
Matched affected-region orange pixel count fell from 79 to zero. Temporary
isolation shader edits were removed; no engine changes were necessary.
Go tests/vet and shader compilation pass. Vulkan validation runs cover the
reproduction at 1080p/4K and a surface sun view. Captures: limb-100.png,
limb-derivative-fixed.png, limb-fixed-4k.png, limb-fix-sun-check.png.

## Asynchronous terrain uploads (2026-10-01)

Streamed terrain children now use CreateIndexedMeshAsync by default. Initial
coverage is uploaded synchronously before rendering. A split publishes all four
children only when every UploadTicket is ready; its parent remains visible and
provides collision coverage until then. Copies are queued on the renderer thread;
the background worker still only generates geometry. Since engine #164, both
pending and settled uploads use the same runtime-safe DestroyMesh call; the
engine handles cancellation and deferred GPU retirement.

Use `-async-terrain-uploads=false` to compare the previous pooled dynamic buffers.
The performance HUD/profile records waiting children and renderer UploadsSkipped.
MeshArena/range batching is not part of this change.

Validation: tests and vet pass, including incomplete-child publication coverage.
Vulkan-validated fixed-generation descent/ascent: 94 splits and 94 merges, returning
to 96 terrain meshes. Sampled skipped draws remained zero; final waiting was zero.
Settled A/B at 1920x1080: both modes reached 348 leaves, LOD15, 84 splits and 432
retained meshes. All 1,113,600 compared scene pixels below the HUD (y>=500) matched.
Single engine GPU timing samples: async 3.100 ms, dynamic 3.118 ms. This does not
establish a speedup; device-local storage is the architectural benefit. Async
uploads add fence latency to refinement, hidden by keeping the parent visible.
Captures/logs: captures/async-tour.*, captures/upload-true.*, captures/upload-false.*.


Stability-build validation: shader compilation, Go tests/vet, and Vulkan checks
passed for shallow/coast views, legacy waves with inline water, repeated moving
cache updates, oceans off, and the3600-frame descent/ascent. The latter enters
and exits water; focusing enables and disables as expected. Files are in
`captures/caustic-stability-validation`. The canonical executable is rebuilt.
## Waterline stability during terrain streaming

Rocks are anchored to the analytic terrain surface, including erosion. They no
longer use the camera's collision floor (`max(analytic, current mesh height)`),
which could lift submerged rocks through the sea while a coarse patch was
visible. A support weight scales instances out when mesh/analytic disagreement
exceeds what the embedded rock base can hide. The same instance data drives
color and shadow draws. LOD replacements update support without moving anchors.

Water intersection uses a CPU-computed signed eye height in metres and a stable
quadratic root calculation. This avoids subtracting two roughly 500 km roots
to locate a surface centimetres or metres away. Sky, terrain, atmospheric
endpoints and underwater scattering share that intersection and underwater
classification. Water shadow receivers remain camera-relative, and material
depth/caustics use the actual surface position rather than the shadow-biased
position. No additional rendering pass is introduced.

Regression tests cover fixed rock anchors through coastal refinement, removal
of unsupported instances, and millimetre-scale waterline motion on 500 km and
Earth-sized spheres. The reproduced old placement lifted visible coastal rocks
by up to 2.8 m and could create false waterline crossings. Captures in the ignored
`captures/rock-waterline` directory show the early approach, settled surface and
underwater views with shadows disabled. The early approach is intentionally
unsettled and is not a matched-geometry performance benchmark; settled surface
and underwater camera/terrain/rock counts match between builds. The old floating
rocks disappear in the corrected approach view, while underwater rocks remain.

Shader generation, full tests, vet and the canonical build passed. Vulkan
validation passed for eroded ridges, sunset mountains, seabed and a 3600-frame
asynchronous erosion descent/ascent tour. These cover the reproduced defect;
other locations may still expose unrelated temporal artifacts.

## Rock flicker during camera motion

A separate temporal defect survived those placement fixes: ordinary engine
instance updates overwrite a shared GPU buffer while previous frames can still
read it. Camera rebasing then makes a submitted frame use the next frame's rock
positions, producing momentary rock silhouettes over water. Standing still or
waiting for a screenshot can hide the race. Disabling shadows does not fix it.

`instance_stream.go` protects rock and grass snapshots with a small reusable
buffer pool. A set returns to the pool only after the engine's retirement
callback; unchanged placements need no upload, and minimized updates wait until
restore. Ordinary group culling, shadow casters and depth-prepass participation
are preserved. This is an application workaround for
[GlyphEngine #188](https://github.com/derekmwright/glyphengine/issues/188).

The 3200x1800 moving-camera stress reproduction freezes lighting and captures
the previous frame after the next placement upload. The old path varies at
4,167 pixels by more than 8 channel levels between identical views; the protected
path produces four pixel-identical captures. Both paths pass ordinary Vulkan
validation, so validation alone cannot diagnose the host-write race. These
captures include readback stalls and are not performance measurements. The water
shader and its appearance were not changed by this fix.
