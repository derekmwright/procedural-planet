// Shared single-scattering approximation. The sky integrates to the atmosphere
// exit; surface haze ends at visible terrain or analytic water. The separated
// view pass reconstructs that endpoint from depth; the inline reference uses
// the surface fragment. Both paths share the same integration below.
// The integration follows the surface/sky treatment described in GPU Gems 2,
// chapter 16 (Sean O'Neil), with a bounded numerical sun-path integral.

#define PLANET_RADIUS planetData.planet.x
#define RAYLEIGH_HEIGHT planetData.planet.y
#define ATM_ENABLED (planetData.planet.z > 0.5)
#define EYE_PLANET planetData.eyePlanet.xyz
#define BETA_R planetData.rayleigh.xyz
#define BETA_M planetData.mie.x
#define MIE_HEIGHT planetData.mie.y
#define ATM_HEIGHT (RAYLEIGH_HEIGHT*6.0)
#define SUN_ENERGY 6.0

// Shared terrain visibility for surface sunlight and in-scattered light.
// Positions are camera-relative metres; the spherical transmission LUT stays
// independent of terrain and can still be reused as the camera moves.
#ifdef TERRAIN_SHADOWS
float surfaceShadowCascade(int c,vec3 position,float ndl,out float fade) {
    vec3 p=(atm.cascadeVP[c]*vec4(position,1.0)).xyz;
    p.xy=p.xy*0.5+0.5;
    fade=0.0;
    if(any(lessThan(p,vec3(0)))||any(greaterThan(p,vec3(1)))) return 1.0;
    float edge=min(min(p.x,1.0-p.x),min(p.y,1.0-p.y));
    fade=smoothstep(0.01,0.10,edge)*smoothstep(0.0,0.04,min(p.z,1.0-p.z));
    if(fade<=0.0) return 1.0;
    // Convert a world-space bias using the actual orthographic depth row.
    // Coverage can change without silently scaling the receiver bias.
    float depthScale=length(vec3(atm.cascadeVP[c][0][2],atm.cascadeVP[c][1][2],atm.cascadeVP[c][2][2]));
    float xyScale=length(vec3(atm.cascadeVP[c][0][0],atm.cascadeVP[c][1][0],atm.cascadeVP[c][2][0]));
    float worldTexel=2.0/(xyScale*float(textureSize(terrainShadowMap,0).x));
    float bias=(c==0 ? 0.04+0.15*(1.0-ndl)
        : worldTexel*(0.15+1.5*(1.0-ndl)))*depthScale;
    vec2 texel=1.0/vec2(textureSize(terrainShadowMap,0).xy);
    float sum=0.0;
    for(int y=-1;y<=1;y+=2) for(int x=-1;x<=1;x+=2)
        sum+=textureGrad(terrainShadowMap,vec4(p.xy+vec2(x,y)*texel,float(c),p.z-bias),vec2(0),vec2(0));
    return sum*0.25;
}
#endif
float localShadow(vec3 position, float ndl) {
#ifdef TERRAIN_SHADOWS
    if (planetData.features.x<0.5) return 1.0;
    float nearFade;
    float nearShadow=surfaceShadowCascade(0,position,ndl,nearFade);
    // Inside the near cascade its weight is exactly one. The far lookup was
    // previously evaluated and then completely overwritten at these pixels.
    if(nearFade>=1.0) return nearShadow;
    float farFade;
    float farShadow=surfaceShadowCascade(1,position,ndl,farFade);
    return mix(mix(1.0,farShadow,farFade),nearShadow,nearFade);
#else
    return 1.0;
#endif
}

// Haze needs only the mountain cascade and one hardware-filtered comparison;
// avoid doing eight surface PCF taps for every one of the 16 air samples.
float airShadow(vec3 position) {
#ifdef TERRAIN_SHADOWS
    if (planetData.features.x<0.5) return 1.0;
    vec3 p=(atm.cascadeVP[1]*vec4(position,1.0)).xyz;
    p.xy=p.xy*0.5+0.5;
    if(any(lessThan(p,vec3(0)))||any(greaterThan(p,vec3(1)))) return 1.0;
    float edge=min(min(p.x,1.0-p.x),min(p.y,1.0-p.y));
    float fade=smoothstep(0.01,0.10,edge)*smoothstep(0.0,0.04,min(p.z,1.0-p.z));
    float depthScale=length(vec3(atm.cascadeVP[1][0][2],atm.cascadeVP[1][1][2],atm.cascadeVP[1][2][2]));
    return mix(1.0,textureGrad(terrainShadowMap,vec4(p.xy,1.0,p.z-2.0*depthScale),vec2(0),vec2(0)),fade);
#else
    return 1.0;
#endif
}

const float PI = 3.141592653589793;
const int VIEW_STEPS = 16;
const int SUN_STEPS = 6;

// Closest-approach form avoids subtracting two large squared distances for
// near-radial rays. Negative discriminants represent misses, never NaNs.
vec2 sphereInterval(vec3 origin, vec3 dir, float radius) {
    float b = dot(origin, dir);
    vec3 closest = origin - b * dir;
    float h = radius * radius - dot(closest, closest);
    if (h < 0.0) return vec2(1.0, -1.0);
    h = sqrt(max(h, 0.0));
    return vec2(-b - h, -b + h);
}

vec2 densityAt(vec3 p) {
    float h = max(length(p) - PLANET_RADIUS, 0.0);
    return exp(-h / vec2(RAYLEIGH_HEIGHT, MIE_HEIGHT));
}

vec3 extinction(vec2 depth) {
    return exp(-min(BETA_R * depth.x + vec3(BETA_M * depth.y), vec3(80.0)));
}

// An inset sphere accounts for the solid planet without falsely putting
// below-datum basins in permanent shadow. Terrain-on-terrain shadows are separate.
float solidRadius() { return max(PLANET_RADIUS - 3.0, PLANET_RADIUS * 0.95); }

vec3 sunlightDirect(vec3 point, vec3 sunDir) {
    vec2 outer = sphereInterval(point, sunDir, PLANET_RADIUS + ATM_HEIGHT);
    if (outer.y <= 0.0 || outer.x > outer.y) return vec3(1.0);
    vec2 ground = sphereInterval(point, sunDir, solidRadius());
    if (ground.x > 0.001 && ground.x < outer.y && ground.x < ground.y) return vec3(0.0);
    float begin = max(outer.x, 0.0);
    float distance = max(outer.y - begin, 0.0);
    vec2 depth = vec2(0.0);
    // Quadratic intervals concentrate samples near the dense end of the ray.
    for (int i = 0; i < SUN_STEPS; i++) {
        float a = float(i) / float(SUN_STEPS);
        float b = float(i + 1) / float(SUN_STEPS);
        float lo = begin + a*a*distance;
        float hi = begin + b*b*distance;
        depth += densityAt(point + sunDir * ((lo+hi)*0.5)) * (hi-lo);
    }
    return extinction(depth);
}

// Cache the sun-path integral by altitude and sun zenith angle. The table
// contains the same spherical integral; scene pixels retain their view march.
#ifndef BUILD_SUN_TABLE
layout(set=1,binding=7) uniform sampler2D sunTransmissionTable;
#endif
vec3 sunlight(vec3 point,vec3 sunDir) {
#ifndef BUILD_SUN_TABLE
    float height=length(point)-PLANET_RADIUS;
    if(planetData.planet.w>0.5&&height>=-3.0&&height<=ATM_HEIGHT) {
        float mu=clamp(dot(normalize(point),sunDir),-1.0,1.0);
        vec2 uv=vec2(0.5+0.5*sign(mu)*sqrt(abs(mu)),
            sqrt(clamp((height+3.0)/(ATM_HEIGHT+3.0),0.0,1.0)));
        vec2 size=vec2(textureSize(sunTransmissionTable,0));
        return textureLod(sunTransmissionTable,(uv*(size-1.0)+0.5)/size,0.0).rgb;
    }
#endif
    return sunlightDirect(point,sunDir);
}

// Static stratified jitter decorrelates shadow boundaries between pixels.
// Fixed midpoint samples otherwise align into visible bands across the sky.
// No frame-time seed: a stationary camera must not shimmer.
float airSampleOffset(int interval, int sampleIndex) {
    #ifdef COMPUTE_PASS
    uvec2 pixel=gl_GlobalInvocationID.xy;
#else
    uvec2 pixel=uvec2(gl_FragCoord.xy);
#endif
    uint h=pixel.x*1973u+pixel.y*9277u+uint(interval)*26699u+uint(sampleIndex)*31847u;
    h=(h^(h>>16u))*2246822519u;
    h=(h^(h>>13u))*3266489917u;
    h^=h>>16u;
    return float(h&0x00ffffffu)/16777216.0;
}

struct Air { vec3 light; vec3 transmittance; };

Air integrateAir(vec3 origin, vec3 dir, float maxDistance, vec3 sunDir, vec3 sunColor) {
    Air result = Air(vec3(0.0),vec3(1.0));
    if (!ATM_ENABLED) return result;
    vec2 hit = sphereInterval(origin,dir,PLANET_RADIUS+ATM_HEIGHT);
    float begin=max(hit.x,0.0), end=min(hit.y,maxDistance);
    if (end <= begin) return result;

    float mu=clamp(dot(dir,sunDir),-1.0,1.0);
    float phaseR=3.0/(16.0*PI)*(1.0+mu*mu);
    const float g=0.76;
    float phaseM=(1.0-g*g)/(4.0*PI*pow(max(1.0+g*g-2.0*g*mu,0.001),1.5));
    vec2 opticalDepth=vec2(0.0);
    for (int i=0;i<VIEW_STEPS;i++) {
        float a=float(i)/float(VIEW_STEPS), b=float(i+1)/float(VIEW_STEPS);
        float lo=begin+a*a*(end-begin), hi=begin+b*b*(end-begin);
        float stepLength=hi-lo;
        vec3 point=origin+dir*((lo+hi)*0.5);
        vec2 density=densityAt(point);
        vec2 segment=density*stepLength;
        // Average visibility across the integration interval. A single midpoint
        // projects an isolated mountain silhouette into otherwise empty air.
        float visibility=0.0;
        // Close terrain spans less than a far-shadow texel per interval.
        // Reserve supersampling for long paths that cross shadow boundaries.
        int shadowSamples=int(clamp(ceil(stepLength/0.25),1.0,4.0));
        for(int j=0;j<shadowSamples;j++) {
            float t=lo+(float(j)+airSampleOffset(i,j))/float(shadowSamples)*stepLength;
            visibility+=airShadow((origin-EYE_PLANET+dir*t)*1000.0);
        }
        vec3 attenuation=extinction(opticalDepth+segment*0.5)*sunlight(point,sunDir)
            *(visibility/float(shadowSamples));
        result.light += attenuation * (BETA_R*density.x*phaseR+vec3(BETA_M*density.y*phaseM)) * stepLength;
        opticalDepth+=segment;
    }
    result.light *= sunColor*SUN_ENERGY;
    result.transmittance=extinction(opticalDepth);
    return result;
}

// Only the eye-to-visible-surface segment is deferred. Reflected/refracted
// rays and the sky continue to use integrateAir directly.
Air viewAir(vec3 direction,float distance,vec3 sun,vec3 color) {
    if(DEFERRED_AIR) return Air(vec3(0),vec3(1));
    return integrateAir(EYE_PLANET,direction,distance,sun,color);
}
