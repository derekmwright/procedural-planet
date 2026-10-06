# Rendering budget and cloud preparation

## Engine update and adaptive depth prepass (2026-10-04)

The application now pins engine `5a0d940` and defaults to
`-depth-prepass=auto`. Engine #181 adds a bounds-based estimate and hysteresis
to decide whether to render opaque depth before the expensive surface shaders.
`-depth-prepass=off` retains the baseline; `on` forces the extra pass for tests.
This reduces hidden fragment shading, not terrain LOD popping or mesh complexity.

Measured on the RX 7900 XTX at 3840x2054, MSAA 4, fixed 60 Hz simulation,
HUD/vsync/validation/pipeline queries off. Two interleaved runs per variant,
1201 frames each, 600-frame warmup, five complete 120-frame windows per run.
Values below are medians of the ten window means, not pooled frame percentiles.
Camera, source terrain counts/LOD and rock/grass counts matched. Submission
totals intentionally differ because the prepass submits geometry a second time.

| View | Prepass off GPU ms | Auto GPU ms |
| --- | ---: | ---: |
| Mountains | 3.693 | 3.401 |
| Shoreline | 16.989 | 6.520 |
| Underwater seabed | 2.655 | 2.540 |
| Ground | 3.838 | 3.054 |
| Orbit | 2.758 | 1.698 |

Auto selected the prepass in every settled window of these views; this does not
prove that its estimate is optimal for every camera. Paired final images were
pixel-identical for mountains, ground and orbit. Coast and seabed differed by
at most one 8-bit channel level (RMS 0.000206/255). Benchmark evidence is local
in `captures/engine-prepass-review` and `captures/engine-prepass-controls`.
Reproduce with `python tools/profile_scene.py --variants prepass-off prepass-auto --warmup 600 --screenshots`, selecting the desired scenes.

Final Vulkan validation passed for orbit, mountains, shoreline, underwater and
a 3600-frame moving tour that crossed the water surface. Diagnostic runs used
the default auto mode with pipeline queries enabled; all four settled views
reported complete 120-sample counter windows. Their 1600x900 captures were
pixel-identical to the README images from the previous engine pin. These
validation runs are separate from the query-free 4K timing measurements above.

Engine #167 now includes application work in total submissions. The HUD and
JSONL separately expose that work, including each application pass and compute
dispatch counts; a larger triangle total is not automatically a scene regression.
The caustic projection's 2,064,386 submitted triangles are now visible there.

`-pipeline-stats` enables engine #183's delayed GPU fragment-invocation and
post-clip primitive counters. JSONL contains rolling means and valid-sample
counts, omitting unbracketed passes rather than reporting them as measured zero.
These differ from CPU submission counts. On the calibrated RX 7900 XTX the
fragment counter excludes helper lanes: it is not a quad-overshading measurement.
Queries stay off for ordinary timing runs. `--pipeline-stats` enables them in
the comparison script when diagnosing work rather than establishing timing.

### Terrain streaming implications

- Async uploads, upload tickets, runtime-safe release, `MeshArena.AllocAsync`
  and opt-in range batching are available. The showcase still uses standalone
  async meshes; shared arena storage/batching needs a separate measured migration.
- Instance LOD with dithered transitions and GPU selection is available for
  repeated foliage/props. It does not provide a parent-to-four-children morph
  for the current unique quadtree patches.
- [Engine #156](https://github.com/derekmwright/glyphengine/issues/156), an
  authored-mesh cluster DAG, remains parked. #174 recorded and removed the Hi-Z
  experiment; closed #154 is not evidence of a shipped occlusion-culling path.
- Surface error bounds, neighbor-compatible morphs, erosion/drainage generation
  and collision agreement remain application work. Merely streaming faster does
  not make a discrete patch swap visually continuous.
- Filed [#186](https://github.com/derekmwright/glyphengine/issues/186) for shared
  application deformation inputs across lit/shadow/prepass vertex stages and
  [#187](https://github.com/derekmwright/glyphengine/issues/187) for the missing
  named application-pass pipeline counters. Their local proposals are in
  `docs/engine-issues/`.

## Current ownership

Surface BRDFs, material detail and direct mountain shadows remain at full
resolution. The default view atmosphere now has separate GPU timings and uses
half width and half height. It outputs in-scattered radiance and RGB
transmittance, then composes `surface * transmittance + radiance` in linear HDR.
Sky and water reflection/refraction rays still integrate their own atmosphere.
Underwater scattering retains its existing separate passes.

The view endpoint is reconstructed from opaque scene depth, then shortened to
the analytic sea sphere where water is closer. Using seabed depth for the air
segment would incorrectly fog the entire water column as air. The same endpoint
function is shared by scattering, transmission, and composition.

Depth-aware reconstruction rejects samples from unrelated surfaces. Pixels
without a suitable low-resolution neighbor use the full-resolution integration,
so thin foreground silhouettes cannot borrow distant haze or lose haze entirely.
There is no temporal accumulation or stale lighting history in this version.
The atmospheric pass is disabled underwater, when atmosphere is off, and during
the caustic debug view. Ordinary sky pixels retain their inline atmosphere.

The four application passes are:

| Timing | Work |
| --- | --- |
| air scattering | Half-resolution shadowed view radiance; logarithmic endpoint distance in alpha |
| air transmission | Half-resolution RGB extinction; density integration without sun/shadow sampling |
| air composite | Full-resolution depth-aware reconstruction into a separate HDR target |
| air present | Copy back to the scene HDR target without reading and writing the same image |

These targets add about 90 MiB at 3840x2054. Full-resolution quality comparison
uses about 181 MiB. They are allocated when `-air-passes=true`, even when the
camera currently disables their execution. The renderer owns their lifetime and
recreates scaled targets on resize. Sampling uses the live depth-texture size.

The extra transmission pass was a compatibility choice. Engine #165 now resolves
[engine #157](https://github.com/derekmwright/glyphengine/issues/157), exposing
directional shadows to compute with explicit frame-graph dependencies. A compute
implementation can now evaluate radiance and transmission together. The active
implementation still uses the validated graphics passes; migrating them is a
separate measured change, not part of the seabed optimization below.

## Application cleanup

- `parameters.glsl` names the eight std140 parameter groups. Features use
  explicit flags instead of bit packing or repurposed camera components.
  `Parameters.Bytes` and its packing test define the 128-byte upload.
- `engine-lighting.glsl` centralizes the mirrored engine shadow/atmosphere ABI.
  Sky's different UBO binding is selected explicitly at the include site.
- One `wave field` compute dispatch writes both cached gradient/curvature
  targets, evaluating the wave spectrum once. Linear/repeat sampling is retained.
- Material frequencies whose distance filter is zero skip their noise work.

## Repeatable profiling

Build the canonical executable, then run:

```powershell
go build -o bin/universebuild.exe .
python tools/profile_scene.py --scenes ground mountains coast underwater --repeats 2
```

The tool uses fixed simulation time, synchronous deterministic terrain generation,
VSync off, the entire HUD disabled, and serial hidden runs. Debug text contributes
triangles to renderer statistics, so hiding only the performance lines is not
enough for geometry comparisons. It alternates variant order between repetitions.
It discards the first 480 frames and compares identical non-overlapping 120-frame
windows. It rejects incomplete GPU timings, mismatched camera/geometry, and
validation diagnostics. Output includes exact commands, executable SHA-256,
per-run JSONL/logs, and `summary.json`. Direct camera motion can require a longer
warmup; a mismatch is reported instead of silently accepting a faster scene.

Useful options:

```powershell
# Quality comparison, with final-frame captures and Vulkan validation:
python tools/profile_scene.py --scenes mountains coast --variants half full inline --repeats 1 --validate --screenshots

# Shorter lower-resolution checks:
python tools/profile_scene.py --scenes underwater orbit --width 1920 --height 1080 --repeats 1
```

For free-flight recording, `-profile captures/session.jsonl` keeps the existing
one-report-per-second behavior. `-profile-frame-step=120` selects frame cadence.
F8 still writes a marker. Reports have run IDs and feature settings so appended
sessions can be separated. `AirPasses` is the requested setting;
`AirPassesActive` records whether it applies to the current view.

Summary GPU values are medians of window means. The reported p95 summary is a
median of window p95 values, **not** a pooled frame percentile. CPU engine timings
are lifetime means and should not be compared as if they were windowed GPU
timings. An isolated GPU result is not a guarantee of overall frame rate or fan
behavior; draw submission, terrain refinement, pacing, clocks, and other GPU
work can change the wall frame time.

## What clouds still need

This change creates a measurable atmosphere budget and explicit view endpoints;
it does not implement volumetric clouds. Before adding them:

1. Establish their own GPU budget in the same ground/coast/orbit views. The
   remaining expensive water reflection path needs separate attention if coast
   performance is still limiting.
2. Integrate cloud radiance and transmission with the air segments in depth
   order. A cloud layer cannot simply be drawn over already-fogged opaque terrain.
   Ocean endpoints must clip it consistently with terrain endpoints.
3. Share sun/cloud visibility with ground and ocean direct light. Keep terrain
   shadowing and cloud shadowing distinguishable in timings and toggles.
4. Add temporal reconstruction only with camera-rebase, resize, water crossing,
   and disocclusion invalidation. Retain a non-temporal reference mode.

The current engine screen-space sun shafts execute before the application air
composition, so their composition differs slightly from the inline reference.
That ordering needs an explicit decision when clouds join the pipeline. Far
terrain shadow coverage/resolution remains finite; this refactor does not add
planet-wide terrain ray tracing.

## Measured results, 2026-10-02

AMD Radeon RX 7900 XTX, 3840x2054, shadows on, engine b099462. Two runs per
variant, alternating order, 1,201 frames per run. Six complete measured windows
per run after the 480-frame warmup. Camera, submitted draw/instance/triangle
counts match throughout the measured windows. These compare the current
separated and inline atmosphere paths in the same executable; the wave/material
cleanup is present in both.

| Scene | Inline GPU ms | Separated GPU ms | Reduction |
| --- | ---: | ---: | ---: |
| Ground and rocks | 12.41 | 4.10 | 67% |
| Mountain sunset | 10.92 | 3.71 | 66% |
| Shallow shoreline | 21.08 | 17.08 | 19% |
| Underwater | 4.28 | 4.29 | Unchanged within run variation |

Values are medians of the twelve window means per variant. Full data and exact
commands: `captures/cloud-prep-clean/summary.json`. The half-resolution ground
atmosphere passes together take about 0.63 ms. The shoreline still spends about
12.5 ms in opaque shading and exceeds the 16.67 ms GPU budget before adding
clouds. Water's reflected-air integration remains inside that opaque shader; its
individual cost has not yet been isolated. Do not generalize the land improvement
to every view.

Matched final PNGs have mean absolute 8-bit RGB-channel differences of 0.0241
(ground), 0.1108 (mountains), and 0.0201 (coast), out of 255. The 99th-percentile
channel differences are 1, 2, and 1 respectively. Underwater captures are exactly
equal. These are single stationary frames after tone mapping, not a motion or
perceptual quality guarantee. Grain and subtle shaft-order differences remain
possible while moving.

Shader generation, `go test ./...`, `go vet ./...`, wave derivative/seam checks,
and refracted-water-light derivative checks pass. Vulkan validation is clean for
full-resolution mountains, odd-sized half-resolution coast, underwater, orbit,
atmosphere off, shadows off, and the 3,600-frame descent/ascent tour. The tour
profile confirms switching between above-water air passes and underwater
rendering. Logs/captures are under `captures/cloud-prep-validation/`.


## Underwater caustic follow-up (2026-10-02)

The underwater field now uses a forward-projected eight-depth caustic atlas.
Separate timers expose projection and filtering; both terrain and scattering
sample the result. At3840x2054, matched settled oblique/overhead scenes measured
4.324->3.552ms and4.612->3.804ms total GPU, despite raising scattering to12 samples.
See [water-lighting.md](water-lighting.md) for exact methodology and limits.
This does not change the earlier coastline reflected-atmosphere bottleneck.

The atlas has a fixed cost that can increase total time at lower resolutions.
It originally submitted2,359,296 application-pass triangles that engine RenderStats does
not currently count; #162 tracks that discrepancy. Geometry parity in these
comparisons means engine-counted scene geometry, with the additional atlas work
measured by its own GPU timers. Cloud budgets must include those timers.

The later stability refinement supersamples the caustic atlas, then resolves
and diffuses it; its four focusing timers are included in the HUD aggregate.
Matched4K before/after runs measured total GPU3.466->3.709ms (oblique) and
3.732->4.001ms (overhead), with focusing itself about0.34->0.64ms. This is a
quality/cost tradeoff, not a new optimization. Targets use28MiB instead of8MiB;
source mesh geometry is unchanged. Details and temporal diagnostics are in
[water-lighting.md](water-lighting.md#caustic-stability-refinement-2026-10-02).

## Seabed performance pass (2026-10-02)

The user identified looking down at the seabed as the costly view. This pass
preserves the accepted caustic spectrum, motion, supersampling, diffusion,
coverage and ray-march sample count. It removes redundant work:

- Material value noise and its analytic gradient use direct trilinear
  interpolation instead of three nested loops. Power-of-two lattice wrapping
  replaces per-corner signed remainders. The sampled field is unchanged.
- The zero-depth caustic plane has no refractive travel, so its focus is exactly
  one. A quad replaces 294,912 projected triangles for that layer. The other
  seven layers retain their full meshes. Total projection is now 2,064,386
  triangles; target memory remains 28 MiB.
- Surface shadows sample the near cascade first. Where its blend weight is
  exactly one, the far-cascade lookup cannot affect the result and is skipped.
  Coverage, bias, filtering and cascade blending are unchanged.
- Water composition expresses its four bilateral samples directly and finds
  the nearest-depth fallback only when reconstruction has no usable weight.
- Terrain recycling uses the engine's newly uniform runtime-safe DestroyMesh
  contract (#164), removing an extra deferred retirement for settled uploads.

Same-engine comparison: RX 7900 XTX, 3840x2054, engine f61683a, two serial runs
per scene/build, 1,201 frames with a 600-frame warmup. Each result is the median
of ten complete 120-frame window means. Optimized runs preceded the reference
runs; earlier baseline and intermediate runs confirm the seabed direction.
Camera and engine-counted geometry match throughout. The application caustic
mesh reduction is intentionally outside those counters (#162 remains open).

| Scene | Reference GPU ms | Optimized GPU ms |
| --- | ---: | ---: |
| Seabed, oblique sun | 2.971 | 2.702 |
| Seabed, overhead sun | 4.434 | 3.966 |
| Ground and rocks | 4.119 | 3.906 |
| Coast | 17.458 | 17.222 |
| Mountain distance view | 3.692 | 3.729 |

The seabed improvement is 9.1–10.6%. Mountains are slightly slower in these runs;
this is not a blanket scene speedup. The expensive above-water reflected-air
path remains, and the coast still exceeds a 16.67 ms GPU budget in this view.

Matched final-frame images differ by at most one 8-bit RGB level in a handful
of channels; the mountain images match exactly. This checks stationary image
equivalence, not every possible camera path. Commands, binary hashes, timing
windows and captures are in `captures/performance-pass-reference` and
`captures/performance-pass-final`; `captures/performance-pass-comparison.json`
also verifies camera/geometry parity and records image differences.

Repeat the seabed poses with:

```powershell
python tools/profile_scene.py --scenes seabed-oblique seabed-overhead --variants half --repeats 2 --warmup 600 --screenshots
go run tools/compare_frames.go before.png after.png
```

Validation: regenerated shaders, Go tests and vet pass. Ten hidden Vulkan runs
at four consecutive fixed simulation times plus lit captures reproduce the
accepted caustic temporal statistics exactly (second-difference RMS 1.0086 and
2.6922); the first debug frames also match pixel-for-pixel. Additional odd-sized
coast, legacy-wave/inline-water, shadows-off, and 3,600-frame descent/ascent runs
have no Vulkan warnings or errors. The tour exercises streamed mesh release and
water crossings. Logs are in `captures/caustic-stability-performance-final` and
`captures/performance-pass-validation`.

The final canonical executable uses engine 17662f1. Its changes after the
controlled f61683a benchmark export shader includes and introduce the optional
x module; core runtime changes are comments. The temporary benchmark engine
worktree was removed. No alternate application executable was created.


## Steep-slope shadow reception (2026-10-04)

A sunlit mountain view revealed repeated triangular/terraced shading. Reproducing
it with erosion disabled and then disabling shadows isolated the broad pattern
to our surface shadow lookup. Disabling material detail did not remove it.

The previous four-tap PCF lookup compared neighboring shadow-map texels against
one depth, with a bias based on the smoothed lighting normal. On a steep triangle,
those texels belong at different depths. The far cascade covers 240 km, making
that discrepancy particularly large. The revised lookup fits each tap's reference
depth to the actual triangle plane. Screen derivatives are evaluated before
water/material branches; the shading normal remains smooth. Water supplies its
analytic radial normal. A residual bias covers the hardware filter footprint,
and the calculation guards near-parallel planes. Sample count and pass count
are unchanged.

This uses the receiver-plane principle described in Microsoft's
[Cascaded Shadow Maps documentation](https://learn.microsoft.com/en-us/windows/win32/dxtecharts/cascaded-shadow-maps#calculating-a-per-texel-depth-bias-with-ddx-and-ddy-for-large-pcfs),
with the depth gradient obtained from the geometric normal and orthographic
projection axes. No engine modification was necessary.

Reproduction: longitude 1, latitude 38.5, altitude 50, flight heading 84.43,
pitch -15; 1600x900, 1201 synchronous frames, VSync and HUD off. The before/after
cameras and terrain counts matched. Five settled 120-frame windows measured
median window GPU means of 0.820/0.819 ms (opaque 0.135/0.138 ms), with validation
off for those timings. This single-pose smoke comparison does not establish a
performance change; it showed no material increase there.

Captures and JSONL logs are under `captures/terrain-facets`: `before`, `after`,
`no-shadows`, `no-materials`, `eroded`, and `after-eroded`. A temporary no-skirts
capture was diagnostic only; it introduced cracks and did not explain the broad
bands. Skirt coverage is restored. Actual LOD silhouette coarseness remains a
separate mesh issue; this change does not add geometric refinement or morphing.

Package tests, vet, shader regeneration and the canonical build passed. Vulkan
validation passed for the corrected mountain, erosion-enabled variant, sunset
mountain shadows, ground rocks, underwater view and 3600-frame asynchronous
erosion tour. Mountain and rock shadows were visually checked after the change.

### Terrain refinement from measured relief

Terrain workers now measure radial deviation between the analytic surface and
each cell's rendered diagonal midpoint, before adding skirts. The LOD selector
uses the maximum sampled deviation alongside the existing distance score. An
angular target of 0.004 radians triggers extra detail; the score boost is capped
at 2x, and the 960-leaf budget and 2:1 neighbor balance are unchanged. This is a
sampled estimate, not a certified bound or a pixel-error guarantee. It cannot
detect every feature between samples, and the cap can leave error above target.

Error metadata becomes active with the four-child GPU replacement, stays with
hidden parents for merge decisions, and is discarded with merged children.
The extra elevation queries occur during mesh generation, not per frame or in
shaders. This improves sampling density without changing the heightfield; it
does not stitch edges or morph transitions. More refined patches still cost
generation time, memory and GPU geometry work.

The CPU-only erosion reproduction at the mountain screenshot pose changed from
261 to 363 settled leaves. The p95 of per-patch sampled angular-error estimates
in the forward 100 km region fell from 22.93 to 16.10 milliradians. These numbers
use patch-center distance and are a refinement diagnostic, not rendered pixel
measurements. Tests cover ridge error reduction, near-ground detail, selection
hysteresis, cube-face balance, the leaf cap and metadata retirement on ascent.
Full tests, vet and the planet race check pass. The canonical executable was
rebuilt and matched 3840x2054 captures were inspected. The eroded ridge shows
more resolved valley sides and distant outlines. Camera position/direction and
rock/grass counts matched exactly between builds; terrain counts remained
settled during the measured windows.

| View | GPU before / after (ms) | Leaves before / after |
| --- | ---: | ---: |
| Eroded ridge | 2.607 / 2.675 | 261 / 363 |
| Seabed | 2.795 / 2.785 | 378 / 390 |
| Ground | 3.030 / 2.997 | 375 / 387 |

These are median window means from five complete 120-frame windows per run,
after warmup, with validation/VSync off and synchronous terrain generation.
One before/after pair per view is a smoke comparison, not a broad performance
claim. The ridge's terrain CPU update increased from 1.193 to 1.423 ms; the extra
geometry is not free even though GPU impact was small in these views. Source
generation also performs 1024 additional elevation queries per new patch.

Separate Vulkan validation runs passed for the eroded ridge, sunset mountains
and seabed. A 3600-frame asynchronous erosion tour reached 2 m ground clearance,
crossed water, peaked at 396 leaves and returned to 96 orbital leaves without
validation diagnostics. Captures, profiles, executable hashes and reproduction
scripts are in the ignored `captures/terrain-error` directory. Skirt seams and
LOD morphing remain separate limitations.
