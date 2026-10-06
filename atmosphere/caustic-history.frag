#version 450
layout(set=2,binding=0) uniform sampler2D source;
layout(location=0) out float intensity;
void main() { intensity=texelFetch(source,ivec2(gl_FragCoord.xy),0).r; }
