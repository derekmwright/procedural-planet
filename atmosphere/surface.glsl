// Periodic world-space material field. The CPU supplies eye modulo 4096 m,
// preserving sub-centimeter detail without converting planet-sized positions
// to floats. Integer-period wrapping keeps the field continuous at that boundary.
float materialHash(ivec3 cell) {
    uvec3 q=uvec3(cell);
    uint h=q.x*1597334677u ^ q.y*3812015801u ^ q.z*2798796415u;
    h=(h^(h>>16))*2246822519u;
    h=(h^(h>>13))*3266489917u;
    return float(h^(h>>16))/4294967295.0;
}
// Value noise plus its analytic world-space gradient.
vec4 materialNoise(vec3 position,float wavelength) {
    vec3 p=mod(position,4096.0)/wavelength;
    ivec3 cell=ivec3(floor(p));
    int period=int(4096.0/wavelength);
    vec3 f=fract(p),u=f*f*(3.0-2.0*f),du=6.0*f*(1.0-f)/wavelength;
    // All material wavelengths divide 4096 by a power of two. Wrap the two
    // corners once, then interpolate along each axis. This is the same value
    // field and analytic gradient as the eight weighted corner contributions,
    // without three nested fragment loops and per-corner signed remainders.
    ivec3 a=cell&(period-1),b=(cell+1)&(period-1);
    vec4 z0=vec4(materialHash(a),materialHash(ivec3(b.x,a.yz)),
                 materialHash(ivec3(a.x,b.y,a.z)),materialHash(ivec3(b.xy,a.z)))*2.0-1.0;
    vec4 z1=vec4(materialHash(ivec3(a.xy,b.z)),materialHash(ivec3(b.x,a.y,b.z)),
                 materialHash(ivec3(a.x,b.yz)),materialHash(b))*2.0-1.0;
    vec2 xy0=mix(z0.xz,z0.yw,u.x),xy1=mix(z1.xz,z1.yw,u.x);
    vec2 yz=mix(xy0,xy1,u.z);
    float dx=mix(mix(z0.y-z0.x,z0.w-z0.z,u.y),
                 mix(z1.y-z1.x,z1.w-z1.z,u.y),u.z);
    float dy=mix(xy0.y-xy0.x,xy1.y-xy1.x,u.z);
    float dz=mix(xy1.x-xy0.x,xy1.y-xy0.y,u.y);
    return vec4(mix(yz.x,yz.y,u.y),vec3(dx,dy,dz)*du);
}

void surfaceMaterial(inout vec3 base,inout vec3 normal) {
    // Derivatives precede the distance branch so filtering uses a valid quad.
    float footprint=max(length(dFdx(fragWorldPos)),length(dFdy(fragWorldPos)));
    float reach=1.0-smoothstep(160.0,700.0,length(fragWorldPos));
    if(fragUV.y>1.5||planetData.water.y<0.5||reach<=0.0) return;
    vec3 p=vec3(planetData.detail.yz,planetData.water.z)+fragWorldPos;
    vec3 radial=normalize(EYE_PLANET+fragWorldPos*0.001);
    float slope=1.0-clamp(dot(normal,radial),0.0,1.0);
    float rockFilter=1.0-smoothstep(0.4,1.4,footprint);
    float chipFilter=1.0-smoothstep(0.05,0.18,footprint);
    float grainFilter=1.0-smoothstep(0.012,0.045,footprint);
    vec4 broad=materialNoise(p,8.0);
    vec4 rock=rockFilter>0.0?materialNoise(p,2.0):vec4(0);
    vec4 chips=chipFilter>0.0?materialNoise(p,0.25):vec4(0);
    vec4 grain=grainFilter>0.0?materialNoise(p,0.0625):vec4(0);
    bool stone=fragUV.y>0.5;
    float rockWeight=stone?1.0:smoothstep(0.12,0.40,slope+broad.x*0.06);
    // Intermediate slopes suggest loose scree; this is a material heuristic,
    // not erosion/deposition simulation or additional geometry.
    float scree=stone?0.0:smoothstep(0.02,0.12,slope)*(1.0-smoothstep(0.30,0.5,slope));
    float grassCover=fragUV.y<0.5?clamp(fragUV.x,0.0,1.0):0.0;
    float variation=0.10*broad.x+mix(0.07,0.22,rockWeight)*rock.x*rockFilter
                   +(0.12+0.14*scree)*chips.x*chipFilter+0.09*grain.x*grainFilter;
    vec3 tint=mix(vec3(1.03,1.0,0.96),vec3(0.88,0.91,0.95),rockWeight);
    variation+=grassCover*(0.12*chips.x*chipFilter+0.08*grain.x*grainFilter);
    tint=mix(tint,vec3(0.94,1.02,0.9),grassCover);
    base*=mix(vec3(1.0),tint*(1.0+variation),reach);
    vec3 gradient=(rock.yzw*mix(0.025,0.13,rockWeight)*rockFilter
                  +chips.yzw*(0.009+0.012*scree)*chipFilter
                  +grain.yzw*0.0018*grainFilter)*reach;
    gradient*=mix(1.0,0.55,grassCover);
    gradient-=normal*dot(normal,gradient);
    normal=normalize(normal-gradient);
}
