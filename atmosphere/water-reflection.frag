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
    outColor.a=log2(1.0+water.x);
    vec3 normal,reflected;
    float variance,fresnel;
    waterReflectionBasis(direction,water.x,footprint,normal,reflected,variance,fresnel);
    vec2 hitUV; vec3 hitPoint;
    float confidence=traceWaterReflection(relative,reflected,hitUV,hitPoint);
    confidence*=1.0-smoothstep(8.0,12.0,water.x);
    if(confidence<=0.0) return;
    float travel=max(opaque-water.x,0.0)*1000.0;
    float coverage=smoothstep(0.0,0.5,travel);
    vec3 position=EYE_PLANET+relative*0.001,radial=normalize(position);
    vec3 local=vec3(planetData.detail.yz,planetData.water.z)+relative;
    if(planetData.features.z>0.5&&travel<150.0) {
        float along=dot(direction,radial)*travel;
        float bedDepth=max(-along-(travel*travel-along*along)/(2.0*planetData.water.x*1000.0),0.0);
        if(bedDepth<1.6)
            coverage*=1.0-shoreFoamCoverage(local,bedDepth,footprint)*(1.0-smoothstep(100.0,150.0,travel));
    }
    vec3 sun=normalize(pc.sunDir.xyz);
    Air sky=waterSky(position+radial*0.001,reflected,sun,pc.sunColor.rgb,variance);
    vec3 skyColor=sky.light+vec3(0.001,0.002,0.004)*sky.transmittance;
    // Source pixels are unfogged opaque radiance. Apply atmosphere along the
    // reflected path once; the normal air/cloud passes handle eye-to-water.
    float distance=length(hitPoint-relative)*0.001;
    Air path=integrateAir(position+radial*0.001,reflected,distance,sun,pc.sunColor.rgb);
    vec3 land=textureLod(sceneColor,hitUV,0).rgb*path.transmittance+path.light;
    outColor.rgb=(land-skyColor)*(fresnel*confidence*coverage);
}
