Rough ordinary meshes are shaded as if their normals point toward global +Y. A procedural planet exposed this at its terminator: rock with `MeshRef.Roughness = 0.98` is incorrectly illuminated even where the actual surface normal is perpendicular to the sun. The material's roughness changes its lighting orientation, not just its specular response.

Observed in GlyphEngine commit `facb8c523fce265b4fd76146f98a4fcc5f7c3ef9`, Windows / RX 7900 XTX, while building the UniverseBuild consumer showcase.

The cause is in [shaders/lit.frag](https://github.com/derekmwright/glyphengine/blob/facb8c523fce265b4fd76146f98a4fcc5f7c3ef9/shaders/lit.frag#L116):

```glsl
float upBias = smoothstep(0.9, 1.0, roughness);
N = normalize(mix(N, vec3(0.0, 1.0, 0.0), upBias * 0.7));
```

The comment describes a grass stabilization heuristic, but this is the generic lit mesh fragment shader. It affects rock, walls, undersides, and spherical worlds. Roughness 0.98 gives a blend weight of about 0.627 toward world up; this is a large normal change.

### Reproduce

1. Render a smooth sphere through the ordinary lit mesh path with constant albedo, roughness 0.98, metallic 0, directional lighting, no fog, and no cast shadows.
2. Use normalized light direction `(-0.55, 0.45, 0.70)`.
3. Inspect the equatorial surface near longitude 51.8427734 degrees (position convention `x=sin(longitude), z=cos(longitude)`). Its radial normal is perpendicular to the light, but the +Y bias produces positive direct illumination.
4. Compare roughness 0.9 under an identical fixed camera/clock. Then isolate the normal behavior by temporarily disabling only the bias in the shader, retaining roughness 0.98.

The consumer workaround is roughness 0.9. Its controlled before/after captures confirmed the illumination change, but the workaround also changes the material's roughness; it is not the desired API. The later custom atmosphere shader uses the supplied terrain normal directly.

### Requested behavior

Ordinary lit materials should honor their supplied normals at every roughness. If grass needs normal stabilization, make it an explicit opt-in or keep it in the grass pipeline. Do not infer geometry semantics from roughness.

### Acceptance

- A non-grass sphere or rotated plane at roughness 0.98 follows its actual normal, including the day/night boundary and downward-facing surfaces.
- Changing roughness does not rotate the lighting coordinate frame.
- If the grass behavior is retained, compare grass captures under a fixed clock and check motion; do not assume the heuristic is necessary based on its comment.
- Regenerate committed SPIR-V and run Vulkan validation.
