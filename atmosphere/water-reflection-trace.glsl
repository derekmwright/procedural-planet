// A bounded perspective-correct screen trace through opaque scene depth.
// Positions are camera-relative metres. Missing/off-screen geometry is a miss;
// it must leave the sky reflection intact, never stretch the last screen texel.
const float WATER_REFLECTION_RANGE=12000.0;

vec3 reflectionScenePoint(vec2 uv,float depth) {
    vec4 p=pc.inverseVP*vec4(uv*2.0-1.0,depth,1);
    return p.xyz/p.w;
}
float reflectionHeight(vec3 relative) {
    float r=length(EYE_PLANET)*1000.0;
    float along=dot(relative,normalize(EYE_PLANET));
    float delta=2.0*r*along+dot(relative,relative);
    return planetData.detail.x+delta/(sqrt(max(r*r+delta,0.0))+r);
}
float reflectionDepthDifference(vec3 q,float k,vec2 uv,out vec3 scene) {
    float depth=textureLod(sceneDepth,uv,0).r;
    if(depth<=0.0) { scene=vec3(0); return -1e20; }
    scene=reflectionScenePoint(uv,depth);
    // The depth buffer still contains the bed underneath analytic water.
    // It is not an occluder for a reflected ray travelling above the sea.
    if(reflectionHeight(scene)<=0.1) return -1e20;
    return length(q/k)-length(scene);
}
float traceWaterReflection(vec3 origin,vec3 direction,out vec2 hitUV,out vec3 hitPoint) {
    vec3 start=origin+direction*0.1;
    vec4 a=pc.viewProjection*vec4(start,1);
    vec4 d=pc.viewProjection*vec4(direction,0);
    if(a.w<=0.01) return 0.0;
    float reach=WATER_REFLECTION_RANGE;
    if(d.w<0.0) reach=min(reach,(a.w-0.01)/-d.w);
    vec3 end=start+direction*reach;
    vec4 b=pc.viewProjection*vec4(end,1);
    if(b.w<=0.001) return 0.0;
    vec2 ua=a.xy/a.w*0.5+0.5,ub=b.xy/b.w*0.5+0.5;
    if(any(lessThan(ua,vec2(0)))||any(greaterThan(ua,vec2(1)))) return 0.0;
    float ka=1.0/a.w,kb=1.0/b.w;
    vec3 qa=start*ka,qb=end*kb;
    // Clip the projected segment to the visible rectangle, preserving its
    // homogeneous interpolation. Never clamp sample coordinates to an edge.
    float limit=1.0;
    for(int axis=0;axis<2;axis++) {
        if(ub[axis]<0.0) limit=min(limit,-ua[axis]/(ub[axis]-ua[axis]));
        if(ub[axis]>1.0) limit=min(limit,(1.0-ua[axis])/(ub[axis]-ua[axis]));
    }
    vec2 pixels=(ub-ua)*vec2(textureSize(sceneDepth,0))*limit;
    int steps=int(clamp(ceil(length(pixels)/3.0),8.0,64.0));
    float previous=0.0;
    for(int i=1;i<=64;i++) {
        if(i>steps) break;
        float t=limit*float(i)/float(steps);
        vec2 uv=mix(ua,ub,t);
        vec3 scene;
        float delta=reflectionDepthDifference(mix(qa,qb,t),mix(ka,kb,t),uv,scene);
        if(delta>=0.0) {
            float lo=previous,hi=t;
            // A few pixels at the horizon span hundreds of metres. Short
            // refinement rejects real coastal hits against the thin depth
            // tolerance, leaving a bright broken seam below the shoreline.
            for(int refine=0;refine<12;refine++) {
                float middle=(lo+hi)*0.5;
                vec3 candidate;
                float diff=reflectionDepthDifference(mix(qa,qb,middle),mix(ka,kb,middle),mix(ua,ub,middle),candidate);
                if(diff>=0.0) hi=middle; else lo=middle;
            }
            hitUV=mix(ua,ub,hi);
            vec3 point=mix(qa,qb,hi)/mix(ka,kb,hi);
            float diff=reflectionDepthDifference(mix(qa,qb,hi),mix(ka,kb,hi),hitUV,hitPoint);
            float travel=length(point-origin);
            float thickness=clamp(travel*0.002,0.5,12.0);
            float height=reflectionHeight(hitPoint);
            // Reject depth discontinuities, hidden surfaces, and the seabed.
            // Raw scene colour over submerged geometry already contains water.
            if(diff>=0.0&&diff<thickness&&height>0.1&&travel>0.5) {
                vec2 edge=min(hitUV,1.0-hitUV);
                return smoothstep(0.0,0.06,min(edge.x,edge.y))
                    *(1.0-smoothstep(8000.0,WATER_REFLECTION_RANGE,travel))
                    *smoothstep(0.1,0.8,height);
            }
            // Only the nearest crossing is represented by this depth buffer.
            // Continuing behind it would invent hidden land reflections.
            return 0.0;
        }
        previous=t;
    }
    return 0.0;
}
