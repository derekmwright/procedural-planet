// Seeded volume atlas. Coordinates are continuous planet-space kilometres;
// rotation advects the field without re-seeding or a short animation loop.
layout(set=2,binding=1) uniform sampler2D cloudNoise;
layout(set=2,binding=12,std140) uniform CloudParameters {
    vec4 layer; // base above planet datum km, thickness km, extinction/km, coverage
    vec4 wind;  // rotation cosine/sine, integration budget, reserved
} cloud;

vec3 cloudNoiseLevel(vec3 p,int level) {
    const float rows[7]=float[](0.0,528.0,664.0,700.0,710.0,716.0,720.0);
    float size=float(64>>level),columns=min(8.0,size),tile=size+2.0;
    // Texel centres align across the explicitly averaged 3D mip chain.
    vec3 q=mod(fract(p)*size+size-0.5,size);
    float z=floor(q.z),next=mod(z+1.0,size);
    vec2 tile0=vec2(mod(z,columns),floor(z/columns));
    vec2 tile1=vec2(mod(next,columns),floor(next/columns));
    vec2 xy=q.xy+1.5;
    vec2 offset=vec2(0,rows[level]);
    return mix(textureLod(cloudNoise,(offset+tile0*tile+xy)/vec2(528,723),0).rgb,
               textureLod(cloudNoise,(offset+tile1*tile+xy)/vec2(528,723),0).rgb,fract(q.z));
}
vec3 cloudNoiseAt(vec3 p,float footprint) {
    float lod=clamp(log2(max(footprint*64.0,1.0)),0.0,6.0);
    int level=int(lod);
    vec3 a=cloudNoiseLevel(p,level);
    if(lod==0.0||level==6) return a;
    return mix(a,cloudNoiseLevel(p,level+1),fract(lod));
}

vec3 cloudCoordinate(vec3 p) {
    return vec3(cloud.wind.x*p.x+cloud.wind.y*p.z,p.y,
                -cloud.wind.y*p.x+cloud.wind.x*p.z);
}

float cloudDensity(vec3 p,bool detail,float footprint) {
    float height=(length(p)-PLANET_RADIUS-cloud.layer.x)/cloud.layer.y;
    if(height<=0.0||height>=1.0||cloud.layer.w<=0.0) return 0.0;
    vec3 q=cloudCoordinate(p);
    float weather=cloudNoiseAt(q/384.0,footprint/384.0).b;
    float coverage=clamp(cloud.layer.w+(weather-0.5)*1.2,0.0,1.0);
    float shape=cloudNoiseAt(q/48.0,footprint/48.0).r;
    float profile=smoothstep(0.0,0.12,height)*(1.0-smoothstep(0.42,1.0,height));
    float threshold=mix(0.84,0.24,coverage);
    float body=smoothstep(threshold-0.08,threshold+0.16,shape)*profile;
    if(body<=0.0) return 0.0;
    float erosion=detail?cloudNoiseAt(q/12.0,footprint/12.0).g:0.45;
    return clamp((body-0.38*(1.0-erosion))/0.62,0.0,1.0);
}

// A shell can be entered twice on a grazing space ray. Subtract the hollow
// interior, then clip both pieces to the actual visible terrain/ocean endpoint.
vec4 cloudIntervals(vec3 origin,vec3 dir,float endpoint) {
    vec2 outer=sphereInterval(origin,dir,PLANET_RADIUS+cloud.layer.x+cloud.layer.y);
    float lo=max(outer.x,0.0),hi=min(outer.y,endpoint);
    if(hi<=lo) return vec4(0);
    vec2 inner=sphereInterval(origin,dir,PLANET_RADIUS+cloud.layer.x);
    if(inner.x<inner.y&&inner.y>lo&&inner.x<hi) {
        return vec4(lo,max(lo,min(inner.x,hi)),min(hi,max(inner.y,lo)),hi);
    }
    return vec4(lo,hi,hi,hi);
}
