#version 450
layout(set=2,binding=0) uniform sampler2D correction;
layout(location=0) out vec4 outColor;
void main() {
    ivec2 p=ivec2(gl_FragCoord.xy),size=textureSize(correction,0);
    vec4 center=texelFetch(correction,p,0);
    outColor=center;
    if(center.a<0.0) return;
    float distance=exp2(center.a)-1.0;
    vec3 sum=vec3(0); float weight=0.0;
    for(int y=-1;y<=1;y++) for(int x=-1;x<=1;x++) {
        vec4 sampleColor=texelFetch(correction,clamp(p+ivec2(x,y),ivec2(0),size-1),0);
        if(sampleColor.a<0.0) continue;
        float w=exp(-0.75*float(x*x+y*y)-abs(exp2(sampleColor.a)-1.0-distance)/max(0.002,distance*0.03));
        sum+=sampleColor.rgb*w; weight+=w;
    }
    outColor.rgb=sum/max(weight,0.00001);
}
