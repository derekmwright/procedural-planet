The public mesh API cannot draw independent index/vertex ranges from shared geometry buffers. For adaptive procedural terrain, each visible patch therefore owns a separate mesh allocation and draw even though patches share the pipeline and material. The existing InstanceSet API batches repeated placements of one mesh; it does not batch different patch geometry.

Observed at commit `facb8c523fce265b4fd76146f98a4fcc5f7c3ef9`. This is a capability/performance investigation, not a claim that draw calls currently dominate UniverseBuild.

### Current path

- [Mesh in renderer/mesh.go](https://github.com/derekmwright/glyphengine/blob/facb8c523fce265b4fd76146f98a4fcc5f7c3ef9/renderer/mesh.go#L13) privately owns the Vulkan vertex and index buffers.
- Ordinary indexed draws in [renderer/commands.go](https://github.com/derekmwright/glyphengine/blob/facb8c523fce265b4fd76146f98a4fcc5f7c3ef9/renderer/commands.go) use the entire mesh with `firstIndex=0` and `vertexOffset=0`.
- [InstanceSet](https://github.com/derekmwright/glyphengine/blob/facb8c523fce265b4fd76146f98a4fcc5f7c3ef9/renderer/instancedmesh.go#L27) addresses the separate repeated-geometry case resolved by #3 and #71.

UniverseBuild's default near-ground case reached 378 active patches and 472 retained GPU mesh objects (including cached parents). Those are mesh objects, not individual VkBuffers; dynamic meshes own vertex/index buffers for each frame in flight. Frustum culling reduces actual submitted draws. A separate 100-meter-altitude capture had 294 active patches but 80 total draws, including non-terrain passes, so active patch count must not be reported as draw count.

### Requested capability and sequence

1. Expose draw ranges (`firstIndex`, `indexCount`, `baseVertex` or an equivalent mesh-view abstraction) into shared vertex/index storage, with per-range transforms and bounds.
2. Keep culling granularity, camera-relative patch placement, material selection, and buffer lifetime explicit. Sharing storage alone does not reduce draw calls, and merging a whole planet into one uncullable mesh is not equivalent.
3. Measure whether CPU command recording is a bottleneck. If it is, investigate indexed indirect/multi-draw submission with per-draw data that can represent each patch's transform. The current per-object push constants cannot simply be varied inside one multi-draw call.

This should expose general renderer capability; terrain partitioning, generation, LOD, and streaming policy belong in the consuming game.

### Acceptance

- Different patch geometry can share storage while retaining independent draw ranges, bounds, and camera-relative transforms.
- Compare output against the separate-mesh path under fixed inputs, including nonzero index/base-vertex offsets and both index widths.
- Report allocation count/memory, visible draw count, CPU recording time, and GPU time separately.
- If indirect submission is added, test support detection/fallback, per-draw transforms, and buffer updates under Vulkan validation.
