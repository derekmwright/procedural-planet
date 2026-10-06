# Shared vertex-deformation data for lit, shadow, and depth-prepass stages

Filed as [GlyphEngine #186](https://github.com/derekmwright/glyphengine/issues/186).

Procedural Planet streams unique terrain patches and retains a parent until
all four child upload tickets are ready. We want to morph children from the
parent surface to their final shape without uploading every vertex each frame.
The morph, neighbor-edge constraints, and terrain error calculation belong to
the application. The missing mechanism is consistent access to its data across
the engine's geometry passes.

Reviewed engine main `5a0d940`:

- `ShaderSet` exposes lit, instanced, shadow and prepass vertex stages.
- `SetShaderParameters` supplies a global 4096-byte vertex/fragment block for
  lit draws. The static/instanced shadow pipeline layout in `renderer/shadow.go`
  has only 128 bytes of push constants (MVP and model), with no descriptor sets.
- `renderer.Vertex` fixes position, RGB, normal and UV. There is no additional
  application vertex stream or indexed per-object data binding. Parent positions,
  parent normals and morph controls would otherwise compete with existing data.
- The new prepass requires positions bit-identical to the lit stage. A public
  shader override alone does not establish the necessary shared data contract.

Please expose an optional, frame-safe application vertex-data mechanism shared
by the lit, directional/point shadow, and depth-prepass paths. A documented
per-draw record/index plus an application buffer is one possible shape; a
restricted extra vertex stream is another. Preserve the existing zero-option
path and define how instancing/arena batching select the right record.

Acceptance: a consumer-authored deforming mesh uses the same position function
in all enabled passes, under camera-relative origin shifts and async geometry
replacement. Lit/prepass images match with the prepass on/off; shadow geometry
tracks the morph; Vulkan and synchronization validation remain clean. No CPU
mesh rewrite each frame or material/UV field repurposing should be necessary.

Related: #94 (global shader parameters), #170/#177 (application-pass private
parameters), #156 (parked authored cluster DAG). This is a smaller rendering
contract, not a request to move procedural terrain or erosion policy into core.
