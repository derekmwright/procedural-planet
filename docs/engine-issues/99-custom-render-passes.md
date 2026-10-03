The procedural planet showcase needs a custom offscreen light-focusing pass and a reduced-resolution underwater scattering pass. ShaderSet only replaces SPIR-V within fixed pipelines and requires their existing descriptor layouts; CreateTexture/CreateDataTexture upload CPU images. I could not find a public API to allocate an application-owned color render target, schedule a graphics/compute pass, and sample its output in later custom shaders.

Checked at facb8c523fce265b4fd76146f98a4fcc5f7c3ef9: renderer/shaderset.go documents the fixed-layout contract; renderer/texture.go owns uploads; renderer/commands.go records built-in passes.

Use case: Evan Wallace's WebGL Water caustic pass projects refracted light patches and accumulates incident-area / receiving-area into a texture (https://github.com/evanw/webgl-water/blob/master/renderer.js). The showcase currently recomputes an inverse differential-area map inside each material/scattering fragment. It works as a bounded prototype but repeats substantial work and cannot cheaply accumulate overlapping light paths.

Requested capability:
- Application-owned floating-point color/depth targets with resize and lifetime management.
- Custom pass scheduling with declared resource reads/writes and renderer-managed barriers.
- Bind custom sampled outputs in subsequent shaders (related to #94, but resource bindings and pass scheduling are separate).
- Additive accumulation, configurable resolution, and access to resolved scene depth for depth-aware scattering upsampling.
- Optional history targets and GPU timings per custom pass.

Acceptance example: render a refracted light mesh into an R16F/RGBA16F target, sample it from a custom lit shader, then run a half-resolution scattering pass before tone mapping. Validation must remain clean through resize, recreation, and shutdown. No water-specific feature is required; a general pass API would serve other showcase effects too.
