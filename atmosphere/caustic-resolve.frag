#version 450
layout(set=2,binding=0) uniform sampler2D source;
layout(location=0) out float intensity;
void main() {
    // Bilinear sampling exactly between four raw texels integrates a 2x2
    // footprint before the wider light diffusion filter. Tile edges align.
    intensity=textureLod(source,gl_FragCoord.xy/vec2(2048,1024),0.0).r;
}
