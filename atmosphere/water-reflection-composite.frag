#version 450
#extension GL_GOOGLE_include_directive : require
layout(set=2,binding=0) uniform sampler2D correction;
layout(set=2,binding=1) uniform sampler2D sceneDepth;
#include "air-frame.glsl"
#include "parameters.glsl"
#include "common.glsl"
#include "air-endpoint.glsl"
layout(location=0) out vec4 outColor;
void main() {
    ivec2 full=textureSize(sceneDepth,0),pixel=ivec2(gl_FragCoord.xy);
    vec2 uv=(vec2(pixel)+0.5)/vec2(full);
    vec3 direction=viewDirection(uv);
    vec2 water=waterInterval(direction);
    outColor=vec4(0);
    if(water.x<=0.0||water.x>=water.y||water.x>=12.0) return;
    float depth=texelFetch(sceneDepth,pixel,0).r;
    if(depth>0.0) {
        vec4 p=pc.inverseVP*vec4(uv*2.0-1.0,depth,1);
        if(length(p.xyz/p.w)*0.001<=water.x) return;
    }
    ivec2 size=textureSize(correction,0);
    vec2 low=uv*vec2(size)-0.5,f=fract(low);
    ivec2 base=ivec2(floor(low));
    vec3 sum=vec3(0); float weight=0.0;
    for(int y=0;y<2;y++) for(int x=0;x<2;x++) {
        vec4 c=texelFetch(correction,clamp(base+ivec2(x,y),ivec2(0),size-1),0);
        if(c.a<0.0) continue;
        vec2 bilinear=mix(1.0-f,f,vec2(x,y));
        float w=bilinear.x*bilinear.y*exp(-abs(exp2(c.a)-1.0-water.x)/max(0.002,water.x*0.03));
        sum+=c.rgb*w; weight+=w;
    }
    if(weight>0.00001) outColor=vec4(sum/weight,0);
}
