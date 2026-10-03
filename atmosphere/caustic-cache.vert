#version 450
#extension GL_GOOGLE_include_directive : require
#include "parameters.glsl"
#include "water-wave.glsl"
#include "caustic-depths.glsl"
layout(push_constant) uniform Push { layout(offset=128) vec4 sunDir; } pc;
layout(location=0) in vec3 inPos;
layout(location=0) out vec2 source;
layout(location=1) flat out int layer;
void main() {
    layer=int(inPos.z); source=inPos.xy;
    if(layer==0) {
        vec2 uv=(source/planetData.causticU.w+0.5)/vec2(4,2);
        gl_Position=vec4(uv*2.0-1.0,0.5,1.0);
        return;
    }
    vec3 u=planetData.causticU.xyz,v=planetData.causticV.xyz,up=normalize(cross(u,v));
    vec3 eye=vec3(planetData.detail.yz,planetData.water.z);
    vec3 incoming=-normalize(pc.sunDir.xyz);
    vec3 flatRay=refract(incoming,up,1.0/1.333);
    float depth=CAUSTIC_DEPTHS[layer];
    vec3 flatOffset=flatRay*(-depth/dot(flatRay,up));
    // Center each RECEIVER plane below the camera. Surface entry coordinates
    // move sunward with depth so low sunlight cannot shear the atlas out of view.
    vec3 entryOffset=u*dot(flatOffset,u)+v*dot(flatOffset,v);
    vec3 p=eye+planetData.causticOrigin.xyz+u*source.x+v*source.y-entryOffset;
    vec3 ray=refract(incoming,waterNormal(p,up,1.0),1.0/1.333);
    vec3 offset=ray*(-depth/min(dot(ray,up),-0.1))-flatOffset;
    vec2 dest=source+vec2(dot(offset,u),dot(offset,v));
    vec2 tile=vec2(layer%4,layer/4);
    vec2 uv=(dest/planetData.causticU.w+0.5+tile)/vec2(4,2);
    gl_Position=vec4(uv*2.0-1.0,0.5,1.0);
}
