#version 450
#extension GL_GOOGLE_include_directive : require
layout(location=0) out vec4 outColor;
layout(set=2,binding=0) uniform sampler2D sceneDepth;

#include "air-frame.glsl"
#include "parameters.glsl"
#include "engine-lighting.glsl"

#include "common.glsl"
#include "air-endpoint.glsl"
void main() {
    ivec2 pixel=ivec2(gl_FragCoord.xy),size=max(ivec2(vec2(textureSize(sceneDepth,0))*pc.targetInfo.x),ivec2(1));
    if(any(greaterThanEqual(pixel,size))) return;
    ivec2 full=textureSize(sceneDepth,0);
    vec2 uv=(vec2(pixel)+0.5)/vec2(size);
    ivec2 source=clamp(ivec2(uv*vec2(full)),ivec2(0),full-1);
    // Use the exact full-resolution sample location that supplied the depth.
    uv=(vec2(source)+0.5)/vec2(full);
    float endpoint=airEndpoint(uv,texelFetch(sceneDepth,source,0).r);
    Air air=Air(vec3(0),vec3(1));
    if(endpoint>=0.0) air=integrateAir(EYE_PLANET,viewDirection(uv),endpoint,normalize(pc.sunDir.xyz),pc.sunColor.rgb);
    outColor=vec4(air.light,endpoint<0.0?-1.0:log2(1.0+endpoint));
}
