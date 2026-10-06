Title: Stage ordinary instance updates until the frame fence, using per-frame buffers

Procedural Planet rebases rock and grass instance transforms around its camera every frame using `UpdateInstanceSet` in `Game.Update`. Rocks intermittently flicker during movement, including over water with shadows disabled. A moving-camera A/B reproduction now confirms the unsafe buffer updates produce the transient corruption; retaining immutable buffers through retirement removes it.

At engine commit `5a0d940cc8f7`:

- `renderer/instancedmesh.go:149` copies placements directly into the one persistently mapped, host-coherent vertex buffer. There is no staging or frame-slot selection.
- `app.go:1597` calls `Game.Update` before rendering.
- `renderer/renderer.go:2079` waits for the current frame slot's fence inside `DrawFrame`, after the application has already overwritten the shared buffer. Other slots can also still reference that same buffer.

Host coherency does not retire GPU reads. Changing camera-relative transforms or placement order/count can therefore modify a previous frame's vertex data while its draws execute. Shadow, depth-prepass and color draws can observe different data. Ordinary Vulkan validation does not establish safety for CPU writes through mapped memory.

Please retain the ordinary `InstanceSet` API and group bounds/prepass behavior, stage updates in CPU memory, and copy into a frame-owned instance buffer only after that slot's fence has signaled. CPU `InstanceSetLOD` already demonstrates this upload ordering, but is not an equivalent workaround: it also culls individual instances against the camera (losing offscreen shadow casters) and excludes these draws from the opaque prepass.

Suggested regression: alternate transform arrays and counts every frame under GPU load with multiple frames in flight. Assert that `UpdateInstanceSet` never writes submitted GPU memory, every submitted slot retains its placement snapshot until retirement, and all passes use the same slot. Include skipped/minimized/resized frames and destruction. Avoid a `DeviceWaitIdle`/queue-idle workaround in the normal update path.

## Reproduction measured in Procedural Planet

At 3200x1800 with 4x MSAA, VSync off, the adaptive depth prepass enabled and shadows disabled, settle the coast for 600 frames. Freeze wave time at 10 seconds and alternate the camera between two positions 4 m apart radially each subsequent frame, rebasing instance transforms as usual. At updates 721, 841, 961 and 1081, capture the previous submitted frame **after** uploading the next placements, before its `DrawFrame`. Capturing before the update would wait for GPU completion and mask the race. This is a diagnostic stress test, not a performance benchmark.

- Old single-buffer update: captures 721/841 differ by more than 8 channel levels at 4,167 pixels; maximum difference 207/255.
- Buffers retained until the `DeferDestroy` retirement callback: all four captures are pixel-identical.
- Old versus protected update at frame 841: 3,445 pixels differ by more than 8 levels, maximum 250/255. The difference image traces rock silhouettes; the water and landscape match.
- Both arms finish with the same 360 terrain leaves and 597 placed rocks. Both are clean under ordinary Vulkan validation, which misses this host-write hazard.

The showcase currently pools ordinary sets and returns an old set to reuse only through `DeferDestroy`, skips unchanged placements, and defers publishing while minimized. This is a temporary application workaround; the engine should own safe updates without each caller having to implement a pool. CPU LOD was evaluated but not used because its rendering semantics differ.
