#version 450
#extension GL_GOOGLE_include_directive : require
layout(set=2,binding=0) uniform sampler2D reflection;
layout(set=2,binding=1) uniform sampler2D sceneDepth;
layout(set=2,binding=2) uniform sampler2D sceneColor;
#include "air-frame.glsl"
#include "parameters.glsl"
#include "engine-lighting.glsl"
#include "common.glsl"
#include "air-endpoint.glsl"
#include "ocean.glsl"
layout(location=0) out vec4 outColor;
void main() {
    ivec2 full=textureSize(sceneDepth,0),pixel=ivec2(gl_FragCoord.xy);
    vec2 uv=(vec2(pixel)+0.5)/vec2(full);
    vec3 direction=viewDirection(uv);
    vec2 water=waterInterval(direction);
    vec3 local=vec3(planetData.detail.yz,planetData.water.z)+direction*(max(water.x,0.0)*1000.0);
    float footprint=max(length(dFdx(local)),length(dFdy(local)));
    outColor=texelFetch(sceneColor,pixel,0);
    if(water.x<=0.0||water.x>=water.y||water.x>=12.0) return;
    float depth=texelFetch(sceneDepth,pixel,0).r;
    float opaque=PLANET_RADIUS*30.0;
    if(depth>0.0) {
        vec4 p=pc.inverseVP*vec4(uv*2.0-1.0,depth,1);
        opaque=length(p.xyz/p.w)*0.001;
        if(opaque<=water.x) return;
    }
    ivec2 size=textureSize(reflection,0);
    vec2 low=uv*vec2(size)-0.5;
    ivec2 base=ivec2(floor(low+0.5));
    vec4 sum=vec4(0); float weight=0.0;
    for(int y=-1;y<=1;y++) for(int x=-1;x<=1;x++) {
        ivec2 tap=clamp(base+ivec2(x,y),ivec2(0),size-1);
        vec4 c=texelFetch(reflection,tap,0);
        if(c.a<0.0) continue;
        float distance=waterInterval(viewDirection((vec2(tap)+0.5)/vec2(size))).x;
        vec2 delta=vec2(tap)-low;
        float w=exp(-0.75*dot(delta,delta)-abs(distance-water.x)/max(0.002,water.x*0.03));
        sum+=c*w; weight+=w;
    }
    if(weight<=0.00001) return;
    vec4 land=sum/weight;
    if(land.a<=0.00001) return;
    vec3 normal,reflected;
    float variance,fresnel;
    waterReflectionBasis(direction,water.x,footprint,normal,reflected,variance,fresnel);
    vec3 position=EYE_PLANET+direction*water.x,radial=normalize(position);
    float travel=max(opaque-water.x,0.0)*1000.0;
    float coverage=smoothstep(0.0,0.5,travel);
    if(planetData.features.z>0.5&&travel<150.0) {
        float along=dot(direction,radial)*travel;
        float bedDepth=max(-along-(travel*travel-along*along)/(2.0*planetData.water.x*1000.0),0.0);
        if(bedDepth<1.6)
            coverage*=1.0-shoreFoamCoverage(local,bedDepth,footprint)*(1.0-smoothstep(100.0,150.0,travel));
    }
    Air sky=waterSky(position+radial*0.001,reflected,normalize(pc.sunDir.xyz),pc.sunColor.rgb,variance);
    vec3 skyColor=sky.light+vec3(0.001,0.002,0.004)*sky.transmittance;
    float reflectionWeight=fresnel*coverage;
    // Remove only this full-resolution pixel's reflected sky, preserving its
    // bed, direct glint and foam. MSAA silhouettes/roundoff can disagree slightly
    // with the analytic mask, so removal cannot exceed the light actually there.
    // Negative HDR would otherwise turn into saturated colours in ACES.
    vec3 removed=min(outColor.rgb,skyColor*reflectionWeight)*clamp(land.a,0.0,1.0);
    outColor.rgb=outColor.rgb-removed+land.rgb*reflectionWeight;
}
