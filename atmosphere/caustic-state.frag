#version 450
#extension GL_GOOGLE_include_directive : require
#include "parameters.glsl"
layout(push_constant) uniform Push { layout(offset=128) vec4 sun; } pc;
layout(location=0) out vec4 state;
void main() {
    int column=int(gl_FragCoord.x);
    if(column==0) state=vec4(vec3(planetData.detail.yz,planetData.water.z)+planetData.causticOrigin.xyz,planetData.mie.z);
    else if(column==1) state=vec4(planetData.causticU.xyz,planetData.water.x);
    else if(column==2) state=vec4(planetData.causticV.xyz,1.0);
    else state=vec4(pc.sun.xyz,planetData.mie.w);
}
