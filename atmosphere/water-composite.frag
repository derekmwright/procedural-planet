#version 450
layout(set=2,binding=0) uniform sampler2D scattering;
layout(set=2,binding=1) uniform sampler2D sceneDepth;
layout(location=0) out vec4 outColor;
void main() {
    ivec2 size=textureSize(scattering,0);
    vec2 uv=gl_FragCoord.xy/vec2(textureSize(sceneDepth,0));
    float depth=texelFetch(sceneDepth,ivec2(gl_FragCoord.xy),0).r;
    vec2 low=uv*vec2(size)-0.5;
    ivec2 base=ivec2(floor(low));
    vec2 f=fract(low);
    // Fixed four-sample bilateral reconstruction. Express the footprint
    // directly so the compiler need not retain nested per-pixel loops.
    vec4 a=texelFetch(scattering,clamp(base,ivec2(0),size-1),0);
    vec4 b=texelFetch(scattering,clamp(base+ivec2(1,0),ivec2(0),size-1),0);
    vec4 c=texelFetch(scattering,clamp(base+ivec2(0,1),ivec2(0),size-1),0);
    vec4 d=texelFetch(scattering,clamp(base+ivec2(1,1),ivec2(0),size-1),0);
    vec4 delta=abs(vec4(a.a,b.a,c.a,d.a)-depth);
    vec4 weights=vec4(1.0-f.x,f.x,1.0-f.x,f.x)*vec4(1.0-f.y,1.0-f.y,f.y,f.y)
        *exp(-delta/max(depth*0.08,0.000002));
    float weight=dot(weights,vec4(1));
    if(weight>0.00001) {
        outColor=vec4((a.rgb*weights.x+b.rgb*weights.y+c.rgb*weights.z+d.rgb*weights.w)/weight,0);
    } else {
        vec3 nearest=a.rgb;
        float closest=delta.x;
        if(delta.y<closest) { closest=delta.y; nearest=b.rgb; }
        if(delta.z<closest) { closest=delta.z; nearest=c.rgb; }
        if(delta.w<closest) nearest=d.rgb;
        outColor=vec4(nearest,0);
    }
}
