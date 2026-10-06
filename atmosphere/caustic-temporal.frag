#version 450
#extension GL_GOOGLE_include_directive : require
#include "parameters.glsl"
#include "caustic-spatial.glsl"
layout(set=2,binding=1) uniform sampler2D history;
layout(set=2,binding=2) uniform sampler2D historyState;
layout(location=0) out float intensity;
void main() {
    vec2 pixel=gl_FragCoord.xy;
    intensity=spatialCaustic(pixel);
    if(planetData.rendering.z<0.5) return;

    // Metadata is written beside the history on the GPU, so skipped frames,
    // minimization and target recreation cannot desynchronize its coordinates.
    vec4 oldOrigin=texelFetch(historyState,ivec2(0,0),0);
    vec4 oldU=texelFetch(historyState,ivec2(1,0),0);
    vec4 oldV=texelFetch(historyState,ivec2(2,0),0);
    vec4 oldSun=texelFetch(historyState,ivec2(3,0),0);
    float dt=mod(planetData.mie.z-oldOrigin.w+62.83185307179586,62.83185307179586);
    if(oldV.w<0.5 || dt<=0.0 || dt>0.25 || oldU.w!=planetData.water.x
        || oldSun.w!=planetData.mie.w
        || length(oldU.xyz-planetData.causticU.xyz)>0.000001
        || length(oldV.xyz-planetData.causticV.xyz)>0.000001
        || length(oldSun.xyz-pc.sun.xyz)>0.000001) return;

    vec3 origin=vec3(planetData.detail.yz,planetData.water.z)+planetData.causticOrigin.xyz;
    // The wave field repeats over the wrapped 4096m world coordinates. Unwrap
    // the translation before projection; even a seam crossing stays continuous.
    vec3 delta=mod(origin-oldOrigin.xyz+2048.0,4096.0)-2048.0;
    vec2 shift=vec2(dot(delta,oldU.xyz),dot(delta,oldV.xyz))/0.25;
    // Reanchors are exact multiples of BOTH sampling lattices. Integer fetches
    // avoid a little bilinear blur accumulating on every stationary frame.
    ivec2 oldPixel=ivec2(pixel)+ivec2(round(shift));
    ivec2 tile=ivec2(pixel)/512;
    if(any(lessThan(oldPixel,tile*512)) || any(greaterThanEqual(oldPixel,(tile+1)*512))) return;
    float previous=texelFetch(history,oldPixel,0).r;
    // A 50ms exponential integration of irradiance, independent of frame rate.
    // Both weights sum to one: steady lighting retains its energy. The wave
    // clock and spatial reconstruction footprint remain unchanged.
    float currentWeight=1.0-exp(-dt/0.05);
    intensity=mix(previous,intensity,currentWeight);
}
