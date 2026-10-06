# Stream-power terrain erosion

This opt-in experiment reshapes dry valleys using drainage area and downhill
slope. The original prescribed channel cross-section produced uniform trenches;
it has been replaced by an iterative bedrock erosion solve. It does not yet
simulate flowing river water, sediment transport, or caves.

## Trying it

Build with `go build -o bin/universebuild.exe .`, then run:

```powershell
.\bin\universebuild.exe -erosion-demo -longitude=40
```

Allow about **ten seconds of startup preparation** on the development machine
(Ryzen 9 5900X). The demo finds a substantially eroded location near the requested
longitude/latitude and replaces the starting flight pose with a downstream view
at a nominal 1600 m ground clearance, looking down 24 degrees. Seed 7 currently
selects 50.238486 E, 8.874780 N. The ordinary `-erosion` flag preserves the requested
starting pose. `-erosion-strength=0..2` controls erosion (default 1); zero restores
the original surface. Erosion remains off by default.

Free-flight startup uses the initial coarse terrain for clearance, so its final
height over a deep valley can exceed the requested altitude as the mesh refines.
For repeatable ground checks, omit `-flight`; the orbit camera follows the
refined ground at the requested clearance.

## Generation and sampling

1. Sample the base terrain on six connected cube faces, with shared edge and
   corner nodes. The default resolution of 512 gives 1,572,866 unique nodes.
   True spherical cell areas weight rainfall; actual edge lengths weight slopes.
2. Route drainage to sea-level outlets using priority flood. Depression filling
   affects the routing surface only. A planet without sea outlets uses its
   lowest node. Deterministic tie-breaking prevents cycles on flat areas.
3. Accumulate upstream catchment area. Solve bedrock incision with an implicit,
   downstream-first stream-power update, proportional to the square root of
   catchment area and to slope. Large catchments erode more strongly; low-relief
   terrain erodes less. There is no prescribed channel width or cross-section.
4. Run eight erosion steps, recalculating drainage every other step. Relax steep
   valley shoulders from a snapshot of neighboring heights. This weathering
   removes material rather than depositing it elsewhere.
5. Cache the resulting height changes, with adjacent-face ghost samples and
   bicubic interpolation. Subtract them in `Planet.Elevation`, retaining the
   original terrain detail below the solve's grid spacing. Mesh generation,
   normals, material/vegetation sampling, placement and camera ground queries
   share the same immutable field. The altered geometry feeds the shadow passes.

The implicit update follows the n=1 stream-power formulation discussed by
[Braun and Willett (2013)](https://www.sciencedirect.com/science/article/pii/S0169555X12004618).
Sink routing uses the idea described in the
[Priority-Flood paper](https://arxiv.org/abs/1511.04463). See also
[Cordonnier et al. (2016)](https://cs.purdue.edu/homes/bbenes/papers/Cordonier16CGF.pdf)
for landscape generation through uplift and fluvial erosion. This implementation
was written for the planet's closed graph; it is not a port of reference code or
a complete reproduction of those models.

The prototype caps lowering at 1200 m times strength and retains at least 20%
of the original elevation above sea level. Existing submerged terrain stays
unchanged, preserving the coastline. These are deliberate showcase constraints,
not physical erosion laws. All heights and distances are in metres.

## Cost and checks

The default seed-7 solve takes roughly ten seconds once at startup. The height
cache occupies 6.1 MiB, with additional storage for demo locations. `CacheBytes`
includes both retained slices, including spare capacity. The temporary drainage
graph needs substantially more memory (hundreds of MiB) while building, then
becomes eligible for garbage collection. There is no persistent disk cache.
The current seed-7 retained cache totals 14.1 MiB, including demo locations.

The same-point elevation microbenchmark measured approximately 0.68 microseconds
for base terrain and 0.77 microseconds with erosion, with zero allocations per
query. That benchmark builds a resolution-128 field; the query algorithm is the
same at 512, but cache locality across a moving scene may differ. No solve runs
per frame and no rendering pass or per-fragment erosion shader is added.
Changed terrain still changes LOD selection, visibility and GPU work.

Tests cover closed graph topology, rainfall/catchment conservation, acyclic
routing, deterministic parallel generation, implicit-update stability, bounded
lowering, coastline preservation, cube-edge continuity, shared LOD samples and
concurrent reads. `go test -race ./planet` exercises the race detector.

Matched-eye, matched-direction before/after captures are under the ignored
`captures/erosion-solver` directory. Use the `*-matched-base` images for the
unmodified references; the earlier `*-base` images did not match the camera
because the coarse collision mesh affected initial ground clearance. JSONL
profiles include erosion settings and cache statistics. `DrainageSamples` counts
qualifying graph samples, not distinct rivers.

The canonical build, full package tests, vet and planet race check passed.
Runtime Vulkan validation passed for the refined terrain, an overhead inspection
and a 3600-frame descent/ascent tour using asynchronous mesh streaming. The tour
reached 2 m clearance, crossed water and returned to 96 orbital terrain leaves.
An erosion-disabled mountain capture was byte-identical to the README reference.
These checks are recorded under `captures/erosion-solver-validation`.

## Remaining limits

- The solve operates at regional scale: roughly kilometre-scale samples on the
  default planet. It forms broad valleys and shoulders; fine gullies need finer
  terrain-aware refinement. Larger planets have coarser drainage at this fixed
  resolution.
- Acyclic routing and an implicit update do not guarantee a continuously
  descending final riverbed. Virtual depression routing, capped erosion,
  interpolation and retained fine terrain detail can leave rises. An outlet/lake
  solution and a continuous bed profile must precede flowing river water.
- There is no sediment budget, deposition, delta formation or rock-strata model.
  The shoulder relaxation is removal-only weathering, not sediment-conserving
  thermal erosion.
- Terrain LOD now measures missing relief while building each patch and can
  refine rough regions beyond the distance-only selection. This includes the
  eroded surface. The distance score boost is capped at 2x and the 960-leaf limit
  is unchanged. Shared height samples still do not make coarse and fine
  triangles identical; skirt coverage remains, and seam-compatible morphing is
  separate work.
- Caves and true overhangs require local volumetric chunks and surface/collision
  joins. A radial heightfield has only one height per direction. Sloping rivers
  also require local water surfaces beyond the existing global sea-level sphere.
