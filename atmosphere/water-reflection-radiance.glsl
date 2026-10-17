// Approximate the reflected cone at its first hit. A single colour fetch makes
// coastal slopes look like a perfect mirror even when unresolved waves have
// broadened the sky reflection. These bounded taps average positive radiance;
// sky, submerged surfaces and unrelated depth layers contribute fallback weight.
vec4 reflectionLandRadiance(vec3 origin,vec3 direction,vec3 hit,float variance) {
    vec3 axis=abs(direction.z)<0.9?vec3(0,0,1):vec3(0,1,0);
    vec3 u=normalize(cross(direction,axis)),v=cross(direction,u);
    // Reflection doubles slope angles. Retain a small micro-ripple lobe even
    // where every large wave is resolved, rather than reverting to a mirror.
    float spread=min(2.0*sqrt(max(variance,0.0)+0.0004),0.25);
    float radius=length(hit-origin)*spread;
    const vec2 disk[9]=vec2[9](vec2(0),vec2(1,0),vec2(-1,0),vec2(0,1),vec2(0,-1),
        vec2(0.7071,0.7071),vec2(-0.7071,0.7071),vec2(0.7071,-0.7071),vec2(-0.7071,-0.7071));
    ivec2 size=textureSize(sceneDepth,0);
    vec3 sum=vec3(0); float weight=0.0;
    for(int i=0;i<9;i++) {
        vec3 samplePoint=hit+(u*disk[i].x+v*disk[i].y)*radius;
        vec4 q=pc.viewProjection*vec4(samplePoint,1);
        if(q.w<=0.01) continue;
        vec2 uv=q.xy/q.w*0.5+0.5;
        if(any(lessThan(uv,vec2(0)))||any(greaterThanEqual(uv,vec2(1)))) continue;
        ivec2 pixel=ivec2(uv*vec2(size));
        float depth=texelFetch(sceneDepth,pixel,0).r;
        if(depth<=0.0) continue;
        vec3 point=reflectionScenePoint((vec2(pixel)+0.5)/vec2(size),depth);
        if(reflectionHeight(point)<=0.1) continue;
        // Do not smear a nearby rock over a distant hill or fetch water below it.
        float separation=abs(length(point)-length(samplePoint));
        float w=(i==0?4.0:1.0)*(1.0-smoothstep(max(radius,1.0),max(radius*2.0,2.0),separation));
        sum+=max(texelFetch(sceneColor,pixel,0).rgb,vec3(0))*w;
        weight+=w;
    }
    return vec4(sum/max(weight,0.00001),weight/12.0);
}
