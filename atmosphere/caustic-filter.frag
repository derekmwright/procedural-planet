#version 450
#extension GL_GOOGLE_include_directive : require
#include "caustic-depths.glsl"
layout(push_constant) uniform Push { layout(offset=128) vec4 axis; } pc;
layout(set=2,binding=0) uniform sampler2D source;
layout(location=0) out float intensity;
void main() {
    vec2 pixel=gl_FragCoord.xy;
    ivec2 tile=ivec2(pixel)/512;
    vec2 lo=vec2(tile*512)+0.5,hi=lo+511.0;
    float depth=CAUSTIC_DEPTHS[tile.x+tile.y*4];
    // World-space reconstruction footprint plus increasing light diffusion.
    // The old 0.75-texel tent left subpixel folds free to flash frame to frame.
    // This is a bounded diffusion approximation, not extra incident energy.
    float sigma=sqrt(0.35*0.35+pow(depth*0.012,2.0))/0.25;
    vec4 w=exp(-0.5*vec4(1,4,9,16)/(sigma*sigma));
    float norm=1.0+2.0*dot(w,vec4(1));
    // Nine Gaussian taps in five bilinear reads; two separable passes produce
    // a contiguous 9x9 footprint. Clamp inside the depth tile, never its neighbor.
    intensity=textureLod(source,pixel/vec2(2048,1024),0.0).r;
    vec2 weights=vec2(w.x+w.y,w.z+w.w);
    vec2 offsets=vec2((w.x+2.0*w.y)/weights.x,(3.0*w.z+4.0*w.w)/weights.y);
    for(int i=0;i<2;i++) {
        vec2 shift=pc.axis.xy*offsets[i];
        intensity+=weights[i]*(textureLod(source,clamp(pixel+shift,lo,hi)/vec2(2048,1024),0.0).r
                             +textureLod(source,clamp(pixel-shift,lo,hi)/vec2(2048,1024),0.0).r);
    }
    intensity/=norm;
}
