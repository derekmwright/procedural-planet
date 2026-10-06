#version 450
#extension GL_GOOGLE_include_directive : require
layout(set=2,binding=0) uniform sampler2D sceneDepth;
layout(push_constant) uniform Push {
    layout(offset=128) mat4 inverseVP;
    vec4 sunDir;
    vec4 sunColor;
} pc;
#include "parameters.glsl"
#include "common.glsl"
#include "ocean.glsl"
layout(location=0) out vec4 outColor;
void main() {
    ivec2 fullSize=textureSize(sceneDepth,0);
    vec2 uv=gl_FragCoord.xy/vec2(max(fullSize/2,ivec2(1)));
    float depth=texelFetch(sceneDepth,clamp(ivec2(uv*fullSize),ivec2(0),fullSize-1),0).r;
    vec4 point=pc.inverseVP*vec4(uv*2.0-1.0,max(depth,1e-8),1.0);
    vec3 position=point.xyz/point.w;
    vec3 direction=normalize(position);
    float travel=min(length(position)*0.001,max(waterInterval(direction).y,0.0));
    outColor=vec4(underwaterShafts(direction,travel),depth);
}
