#version 450
#extension GL_GOOGLE_include_directive : require
layout(set=2,binding=0) uniform sampler2D sceneColor;
layout(set=2,binding=1) uniform sampler2D sceneDepth;
#define READ_VIEW_PROJECTION
#include "air-frame.glsl"
#include "parameters.glsl"
#include "engine-lighting.glsl"
#include "common.glsl"
#include "air-endpoint.glsl"
#include "ocean.glsl"
#include "water-reflection-trace.glsl"
#include "water-reflection-radiance.glsl"
layout(location=0) out vec4 outColor;
void main() {
    ivec2 full=textureSize(sceneDepth,0);
    vec2 extent=max(floor(vec2(full)*pc.targetInfo.x),vec2(1));
    vec2 uv=gl_FragCoord.xy/extent;
    vec3 direction=viewDirection(uv);
    vec2 water=waterInterval(direction);
    vec3 relative=direction*(max(water.x,0.0)*1000.0);
    // Derivatives belong before divergent coverage branches. Resolve waves
    // at this pass's footprint so a subpixel ripple cannot flip a whole cell
    // repeatedly between land and sky as the camera moves.
    float footprint=max(length(dFdx(relative)),length(dFdy(relative)));
    outColor=vec4(0,0,0,-1);
    float depth=textureLod(sceneDepth,uv,0).r;
    float opaque=depth>0.0?length(reflectionScenePoint(uv,depth))*0.001:PLANET_RADIUS*30.0;
    if(water.x<=0.0||water.x>=water.y||water.x>=opaque||water.x>=12.0) return;
    // Include valid water misses in reconstruction, so silhouettes fade into
    // sky instead of extending the nearest successful reflection across them.
    outColor.a=0.0;
    vec3 normal,reflected;
    float variance,fresnel;
    waterReflectionBasis(direction,water.x,footprint,normal,reflected,variance,fresnel);
    vec2 hitUV; vec3 hitPoint;
    float confidence=traceWaterReflection(relative,reflected,hitUV,hitPoint);
    confidence*=1.0-smoothstep(8.0,12.0,water.x);
    if(confidence<=0.0) return;
    vec3 position=EYE_PLANET+relative*0.001,radial=normalize(position);
    vec3 sun=normalize(pc.sunDir.xyz);
    // Source pixels are unfogged opaque radiance. Apply atmosphere along the
    // reflected path once; the normal air/cloud passes handle eye-to-water.
    float distance=length(hitPoint-relative)*0.001;
    Air path=integrateAir(position+radial*0.001,reflected,distance,sun,pc.sunColor.rgb);
    vec4 source=reflectionLandRadiance(relative,reflected,hitPoint,variance);
    confidence*=source.a;
    vec3 land=source.rgb*path.transmittance+path.light;
    // Filter premultiplied light and confidence together. In particular, never
    // filter a signed sky subtraction across different wave normals/Fresnel.
    outColor=vec4(land*confidence,confidence);
}
