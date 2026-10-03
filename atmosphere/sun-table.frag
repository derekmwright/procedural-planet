#version 450
#extension GL_GOOGLE_include_directive : require
#define BUILD_SUN_TABLE
#include "parameters.glsl"
#include "common.glsl"
layout(location=0) out vec4 outColor;
void main() {
    vec2 uv=(gl_FragCoord.xy-0.5)/vec2(511.0,127.0);
    float x=uv.x*2.0-1.0;
    float mu=sign(x)*x*x;
    float height=uv.y*uv.y*(ATM_HEIGHT+3.0)-3.0;
    vec3 point=vec3(0,PLANET_RADIUS+height,0);
    vec3 sun=vec3(sqrt(max(1.0-mu*mu,0.0)),mu,0);
    outColor=vec4(sunlightDirect(point,sun),1);
}
