#version 450
layout(set=2,binding=0) uniform sampler2D sceneColor;
layout(location=0) out vec4 outColor;
void main() {
    outColor=texelFetch(sceneColor,ivec2(gl_FragCoord.xy),0);
}
