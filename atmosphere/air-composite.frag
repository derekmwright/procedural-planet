#version 450
#extension GL_GOOGLE_include_directive : require
layout(set=2,binding=0) uniform sampler2D sceneColor;
layout(set=2,binding=1) uniform sampler2D sceneDepth;
layout(set=2,binding=2) uniform sampler2D airLight;
layout(set=2,binding=3) uniform sampler2D airTransmission;
#include "air-frame.glsl"
#include "parameters.glsl"
#include "engine-lighting.glsl"
#include "common.glsl"
#include "air-endpoint.glsl"
layout(location=0) out vec4 outColor;
void main() {
    ivec2 full=textureSize(sceneDepth,0),pixel=ivec2(gl_FragCoord.xy);
    vec2 uv=(vec2(pixel)+0.5)/vec2(full);
    vec3 color=texelFetch(sceneColor,pixel,0).rgb;
    float endpoint=airEndpoint(uv,texelFetch(sceneDepth,pixel,0).r);
    if(endpoint<0.0) { outColor=vec4(color,1); return; }
    ivec2 size=textureSize(airLight,0);
    vec2 low=uv*vec2(size)-0.5, f=fract(low);
    ivec2 base=ivec2(floor(low));
    vec3 light=vec3(0),transmission=vec3(0);
    float weight=0.0;
    for(int y=0;y<2;y++) for(int x=0;x<2;x++) {
        ivec2 q=clamp(base+ivec2(x,y),ivec2(0),size-1);
        vec4 l=texelFetch(airLight,q,0);
        if(l.a<0.0) continue;
        vec3 t=texelFetch(airTransmission,q,0).rgb;
        float delta=abs((exp2(l.a)-1.0)-endpoint);
        vec2 b=mix(1.0-f,f,vec2(x,y));
        float w=b.x*b.y*exp(-delta/max(endpoint*0.03,0.002));
        light+=l.rgb*w; transmission+=t*w; weight+=w;
    }
    if(weight>0.00001) { light/=weight; transmission/=weight; }
    else {
        // A thin foreground edge can disappear between half-resolution samples.
        // Evaluate that pixel directly instead of borrowing distant haze or
        // leaving a dark, unfogged silhouette. No temporal history is required.
        Air air=integrateAir(EYE_PLANET,viewDirection(uv),endpoint,normalize(pc.sunDir.xyz),pc.sunColor.rgb);
        light=air.light; transmission=air.transmittance;
    }
    outColor=vec4(color*transmission+light,1);
}
