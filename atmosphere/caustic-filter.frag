#version 450
#extension GL_GOOGLE_include_directive : require
#include "caustic-spatial.glsl"
layout(location=0) out float intensity;
void main() { intensity=spatialCaustic(gl_FragCoord.xy); }
