#version 450
layout(location=0) in vec2 source;
layout(location=1) flat in int layer;
layout(location=0) out float intensity;
void main() {
    // Each projected triangle contributes incident area / receiver area.
    // abs retains overturned folds; additive blending sums all light paths.
    float area=abs(determinant(mat2(dFdx(source),dFdy(source))));
    ivec2 tile=ivec2(gl_FragCoord.xy)/1024;
    if(tile.x+tile.y*4!=layer) discard;
    intensity=layer==0?1.0:min(area/(0.125*0.125),32.0);
}
