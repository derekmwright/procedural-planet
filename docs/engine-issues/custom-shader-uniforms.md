`WithShaders` lets a consumer replace shader code, but the consumer cannot bind its own per-frame uniform data through a supported renderer API. A spherical atmosphere needs the camera's planet-relative position, radius, density scale heights, scattering coefficients, and an enable switch in both sky and terrain shaders.

Observed against commit `facb8c523fce265b4fd76146f98a4fcc5f7c3ef9` while building UniverseBuild. The shader replacement seam from #4 works; this request completes data binding for custom shaders rather than adding an engine-owned planet or atmosphere system.

### Current limitation and workaround

[ShaderSet](https://github.com/derekmwright/glyphengine/blob/facb8c523fce265b4fd76146f98a4fcc5f7c3ef9/renderer/shaderset.go#L24) replaces SPIR-V but requires the engine's fixed vertex, descriptor, and push-constant layouts. The push block is already 256 bytes. The shared lit/sky UBO in [renderer/shadow.go](https://github.com/derekmwright/glyphengine/blob/facb8c523fce265b4fd76146f98a4fcc5f7c3ef9/renderer/shadow.go#L27) contains engine-owned shadow, night-grade, palette, and volumetric parameters.

UniverseBuild currently transports its atmosphere data through the six sky-palette vectors introduced by #12. Both custom shaders agree on that interpretation, so it works, but those vectors are no longer colors. It prevents safely composing with stock sky/fog/cloud shaders and depends on unrelated engine packing details.

### Requested capability

A documented way for a consumer to bind an application-owned uniform buffer to custom shaders, shared across the sky and lit passes when requested. Keep the initial API bounded: a fixed application parameter block with a published descriptor binding/alignment contract would already unblock this use case. A general render graph is not required.

Per-frame updates must respect frames in flight, with explicit resource ownership, teardown, swapchain behavior, and validation of capacity/layout. Preserve existing defaults when no custom data is supplied. If existing devices exhaust descriptor/push-constant limits, fail clearly rather than silently changing the ABI.

### Acceptance

- A consumer changes camera/radius/scattering values each frame without replacing shaders, repurposing engine fields, or rebuilding pipelines.
- The sky and terrain read the same frame's values; a minimal two-pass example verifies this visually under a fixed clock.
- Existing stock examples remain compatible without changes.
- Resize, repeated resource replacement, and shutdown pass Vulkan validation.
- Document the supported descriptor layout, alignment, lifetime, and thread on which updates are legal.
