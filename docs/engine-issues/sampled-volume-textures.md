# Sampled 3D textures for cached volumetric fields

Filed as [GlyphEngine #190](https://github.com/derekmwright/glyphengine/issues/190).

Procedural Planet is preparing a spherical volumetric cloud layer. Its view
march and light march need a stable, seeded density field without evaluating
several procedural noise octaves at every sample. A native sampled volume would
let the application cache that field and use hardware spatial filtering and
volume mip levels. This is an efficiency/interface request, not a prerequisite
for an initial cloud prototype.

Verified against GlyphEngine main `5a0d940cc8f7` on 2026-10-09:

- Public texture constructors accept width and height only.
- `renderer/texture.go` creates `ImageType2D`, extent depth 1, and
  `ImageViewType2D`.
- `RenderTargetDesc.Depth` requests a depth attachment; it is not a volume
  extent. Application compute can dispatch in Z but its image targets remain 2D.
- `x/sky/clouds.frag` evaluates procedural 3D value noise. It does not expose a
  sampled volume resource to consumers.

The available fallback is a 2D slice atlas with two bilinear fetches and manual
interpolation between slices, or procedural noise. Atlas gutters, repeat seams,
and separate mip layouts become application work; ordinary 2D mip generation
does not implement volume filtering. The relative GPU benefit must be measured,
not assumed from texture-fetch counts.

Requested capability:

- Upload a CPU-generated sampled 3D texture with explicit width, height, depth,
  format, spatial filter, and addressing on all three axes. Compact linear-data
  formats such as R8 and RGBA8 are sufficient for the first cloud consumer;
  preserve an explicit format/capability contract rather than forcing sRGB.
- Bind that resource through application graphics/compute texture inputs with
  a documented `sampler3D` contract. Report incompatible usage/dimensions clearly
  rather than silently binding a 2D fallback to a 3D declaration.
- Support an explicitly supplied volume mip chain, or documented native volume
  mip generation where supported. Validate extents, byte counts, format support,
  device limits, and unavailable optional operations.
- Retain fence-safe release and descriptor replacement. Existing 2D constructors
  and shader interfaces should keep their behavior and resource cost.

Acceptance: a small known volume sampled by both an app graphics pass and an
app compute pass interpolates along Z as well as X/Y, repeats continuously in
all axes, and selects correct volume mip levels. Resource replacement,
destruction with frames in flight, resize, and unsupported inputs are covered
with Vulkan validation enabled.

The initial use case can upload seeded noise once. Compute writes into native
3D storage images would be useful later, but are not required for this request.
Cloud shape, spherical coordinates, lighting, and temporal reconstruction remain
application work.
