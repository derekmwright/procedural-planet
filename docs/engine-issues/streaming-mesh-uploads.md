Runtime procedural terrain needs to upload completed patches without waiting for the whole graphics queue. The device-local mesh creation path currently submits a separate one-shot transfer and calls `QueueWaitIdle` for each buffer. A mesh with vertices and indices goes through that path twice.

Observed by code inspection at commit `facb8c523fce265b4fd76146f98a4fcc5f7c3ef9` while building UniverseBuild. This issue records a confirmed synchronization path, not a measured frame-time regression or an assertion that the current showcase is blocked.

### Evidence

- [createDeviceLocalBuffer in renderer/mesh.go](https://github.com/derekmwright/glyphengine/blob/facb8c523fce265b4fd76146f98a4fcc5f7c3ef9/renderer/mesh.go#L95) allocates staging, records a copy, and calls `endSingleTimeCommands`.
- [endSingleTimeCommands in renderer/texture.go](https://github.com/derekmwright/glyphengine/blob/facb8c523fce265b4fd76146f98a4fcc5f7c3ef9/renderer/texture.go#L324) submits to the graphics queue, waits for that queue to become idle, and frees the command buffer.
- `CreateIndexedMesh` / `CreateIndexedMesh32` do this for vertex and index data separately.

### Current consumer workaround

UniverseBuild generates geometry on one CPU worker and uploads through `CreateDynamicIndexedMesh` / `UpdateMeshData` on the frame thread. It pools meshes, stages at most two patches per rendered frame, and keeps the parent visible until all four children are ready. This works and passed Vulkan validation through a complete descent/ascent.

However, terrain data is mostly immutable after generation. The dynamic path keeps separate host-visible vertex and index buffers for each frame in flight plus staging copies. It also only accepts uint16 indices. Small patches fit that limit, so 32-bit dynamic indices are not a blocker for this showcase.

### Requested capability

A streaming device-local mesh upload path that can batch multiple copies into a frame or upload submission, retain staging data until the transfer completes, and expose readiness without a per-buffer `QueueWaitIdle`. CPU generation should remain application-owned; document whether enqueuing and publishing GPU resources must happen on the render thread.

A fence-safe release/update contract should accompany the upload path so a consuming game can stream completed patches in and retire old geometry safely. A dedicated transfer queue is optional; the capability is bounded submission and readiness, not a required queue topology.

### Acceptance

- Queue several patch uploads while frames render; verify there is no per-mesh/per-buffer queue-idle wait on this path.
- Geometry is published only after its data is ready, with no incomplete frame or stale buffer reference.
- Measure CPU upload/submit cost, GPU transfer/render cost, and memory use against the existing static and dynamic paths on the same patch workload.
- Exercise cancellation, allocation failure, resize, resource retirement, and shutdown under Vulkan validation.
- Preserve simple synchronous constructors for callers that want them.
