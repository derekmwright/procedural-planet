#version 450
#extension GL_GOOGLE_include_directive : require
layout(location=0) out vec4 outColor;
layout(set=2,binding=0) uniform sampler2D sceneDepth;

#include "air-frame.glsl"
#include "parameters.glsl"

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
    vec2 opticalDepth=vec2(0);
    if(endpoint>=0.0) {
        vec3 dir=viewDirection(uv);
        vec2 hit=sphereInterval(EYE_PLANET,dir,PLANET_RADIUS+ATM_HEIGHT);
        float begin=max(hit.x,0.0),end=min(hit.y,endpoint);
        if(end>begin) for(int i=0;i<VIEW_STEPS;i++) {
            float a=float(i)/float(VIEW_STEPS),b=float(i+1)/float(VIEW_STEPS);
            float lo=begin+a*a*(end-begin),hi=begin+b*b*(end-begin);
            opticalDepth+=densityAt(EYE_PLANET+dir*((lo+hi)*0.5))*(hi-lo);
        }
    }
    outColor=vec4(extinction(opticalDepth),0);
}
