// An analytic spherical surface, composited only when it is closer than the
// opaque fragment. Terrain/rocks provide the shoreline and shallow bed color.
// Distances are km; ripples use camera-relative meters plus periodic eye origin.
// Larger clarity keeps the same color absorption ratios over a longer path.
// One shared setting controls view absorption, seabed light, and cutoff distance.
const float WATER_CLARITY = 2.0;
#define WATER_ABSORPTION (vec3(0.20,0.075,0.035)/WATER_CLARITY)
#define WATER_OPAQUE_DISTANCE (0.4*WATER_CLARITY)

#include "waterlight.glsl"

float seabedCaustics(vec3 relativePosition,vec3 point,float depth) {
    vec3 p=vec3(planetData.detail.yz,planetData.water.z)+relativePosition;
    float footprint=max(length(dFdx(p)),length(dFdy(p)));
    float visibility=(1.0-smoothstep(0.15,0.8,footprint))
        *(1.0-smoothstep(150.0,350.0,length(relativePosition)));
    if(visibility<=0.0||depth>50.0) return 1.0;
    return mix(1.0,waterLightFocus(p,normalize(point),depth),visibility);
}

vec3 underwaterShafts(vec3 direction,float travel) {
    if(planetData.features.y<0.5) return vec3(0);
    vec3 up=normalize(EYE_PLANET),sun=normalize(pc.sunDir.xyz);
    float day=smoothstep(0.08,0.4,dot(up,sun));
    float depth=max(-planetData.detail.x,0.0);
    if(day<=0.0||depth>80.0) return vec3(0);
    vec3 origin=vec3(planetData.detail.yz,planetData.water.z);
    vec3 waterSun=-refract(-sun,up,1.0/1.333);
    float span=min(travel*1000.0,60.0);
    vec3 sum=vec3(0);
    // Bounded single scattering through the SAME refracted light field as
    // the seabed. No camera-centered spokes or painted caustic masks.
    // Stratified integration avoids coherent sample-plane bands. Stable screen
    // jitter trades those bands for fine grain. The default path evaluates this
    // at half resolution and upsamples with scene depth; no temporal history.
    float jitter=fract(52.9829189*fract(dot(gl_FragCoord.xy,vec2(0.06711056,0.00583715))));
    // Cached focusing makes additional integration samples affordable; this
    // reduces grain without changing scattering energy or the phase function.
    int samples=planetData.rendering.y>0.5?12:6;
    for(int i=0;i<samples;i++) {
        float t=span*(float(i)+jitter)/float(samples);
        float d=max(depth-dot(direction,up)*t,0.0);
        float focusing=waterLightFocus(origin+direction*t,up,d);
        vec3 transmission=exp(-WATER_ABSORPTION*(t+d/max(dot(waterSun,up),0.15)));
        sum+=transmission*focusing;
    }
    float cosine=dot(direction,waterSun);
    float g=0.65;
    float phase=(1.0-g*g)/pow(1.0+g*g-2.0*g*cosine,1.5);
    return pc.sunColor.rgb*vec3(0.0002,0.0006,0.0008)*day*phase*sum*(span/float(samples));
}
float visibleOceanDistance(vec3 direction,float opaqueDistance) {
    float radius=planetData.water.x;
    if(radius<=0.0||planetData.detail.x<0.0) return -1.0;
    vec2 hit=waterInterval(direction);
    return hit.x>0.0&&hit.x<hit.y&&hit.x<opaqueDistance?hit.x:-1.0;
}

vec3 underwaterColor(vec3 color,float travel,vec3 direction) {
    vec3 transmission=exp(-WATER_ABSORPTION*travel*1000.0);
    float day=smoothstep(-0.12,0.25,dot(normalize(EYE_PLANET),normalize(pc.sunDir.xyz)));
    float depth=max(-planetData.detail.x,0.0);
    return color*transmission+vec3(0.006,0.035,0.047)*(0.01+day*exp(-depth*0.025/WATER_CLARITY))*(1.0-transmission)
        +(planetData.water.w>0.5?vec3(0):underwaterShafts(direction,travel));
}

bool cameraUnderwater() {
    return planetData.water.x>0.0&&planetData.detail.x<0.0;
}

vec3 underwaterWindow(vec3 direction,float distance) {
    // Beyond the clarity-scaled cutoff, blue transmission is below 1e-6. Avoid tracing sky
    // through water whose absorption makes that sky invisible.
    if(distance>WATER_OPAQUE_DISTANCE) return underwaterColor(vec3(0.0),distance,direction);
    vec3 position=EYE_PLANET+direction*distance;
    vec3 radial=normalize(position),sun=normalize(pc.sunDir.xyz);
    vec3 p=vec3(planetData.detail.yz,planetData.water.z)+direction*(distance*1000.0);
    float time=planetData.mie.z;
    float footprint=max(length(dFdx(p)),length(dFdy(p)));
    float detail=1.0-smoothstep(0.25,3.0,footprint);
    // The same physical wave normals drive reflection, refraction and focusing.
    vec3 normal=waterNormal(p,radial,detail);
    vec3 airDirection=refract(direction,-normal,1.333);
    float day=smoothstep(0.0,0.3,dot(radial,sun));
    vec3 surface=vec3(0.008,0.04,0.055)*(0.05+day);
    // Beyond Snell's window show a dark reflective underside, not open sky.
    if(dot(airDirection,airDirection)>0.01) {
        Air above=integrateAir(position,airDirection,PLANET_RADIUS*8.0,sun,pc.sunColor.rgb);
        float alignment=max(dot(airDirection,sun),0.0);
        float glint=pow(alignment,650.0)*2.0+pow(alignment,32.0)*0.09;
        float cosine=max(dot(normal,direction),0.0);
        float sin2=1.333*1.333*(1.0-cosine*cosine);
        float transmission=(1.0-(0.02+0.98*pow(1.0-cosine,5.0)))
            *(1.0-smoothstep(0.80,1.0,sin2));
        surface=mix(surface,above.light+vec3(0.015,0.04,0.07)*above.transmittance
            +pc.sunColor.rgb*above.transmittance*glint*day,transmission);
    }
    return underwaterColor(surface,distance,direction);
}

// Periodic value noise in metres; world anchored across camera rebasing.
float shoreNoise(vec3 p,float scale) {
    p=mod(p,4096.0)/scale;
    vec3 cell=floor(p),f=fract(p); f=f*f*(3.0-2.0*f);
    float sum=0.0;
    for(int z=0;z<2;z++) for(int y=0;y<2;y++) for(int x=0;x<2;x++) {
        vec3 corner=vec3(x,y,z),q=mod(cell+corner,4096.0/scale);
        vec3 w=mix(1.0-f,f,corner);
        uvec3 u=uvec3(q);
        uint hash=u.x*1597334677u ^ u.y*3812015801u ^ u.z*2798796415u;
        hash=(hash^(hash>>16u))*2246822519u;
        hash=(hash^(hash>>13u))*3266489917u;
        float h=float(hash^(hash>>16u))*(1.0/4294967296.0);
        sum+=h*w.x*w.y*w.z;
    }
    return sum;
}
float shoreFoamCoverage(vec3 p,float depth,float footprint) {
    float edge=1.0-smoothstep(0.45,1.6,depth);
    float patchiness=shoreNoise(p,4.0);
    float cells=shoreNoise(p+vec3(17,31,7),1.0);
    float phase=planetData.mie.z*0.8+depth*4.0
        +patchiness*3.5+cells*0.6;
    // Broad ragged wash, with soft holes rather than bright bubble dots.
    float front=smoothstep(0.2,0.9,cos(phase));
    float breakup=smoothstep(0.25,0.72,cells+0.15*patchiness);
    breakup=mix(breakup,0.45,smoothstep(0.2,1.0,footprint));
    return edge*smoothstep(0.02,0.18,depth)*breakup*(front*0.7+0.04);
}

vec3 oceanComposite(vec3 underlying,vec3 bedRadiance,vec3 direction,float opaqueDistance) {
    float radius=planetData.water.x;
    if(radius<=0.0) return underlying;
    vec2 interval=waterInterval(direction);
    if(interval.x>interval.y||interval.y<=0.0) return underlying;
    bool submerged=planetData.detail.x<0.0;
    float surfaceDistance=submerged?interval.y:interval.x;
    if(surfaceDistance<=0.0||surfaceDistance>=opaqueDistance) {
        return submerged?underwaterColor(underlying,opaqueDistance,direction):underlying;
    }
    if(submerged) return underwaterWindow(direction,surfaceDistance);
    vec3 position=EYE_PLANET+direction*surfaceDistance;
    vec3 radial=normalize(position);
    vec3 sun=normalize(pc.sunDir.xyz);
    vec3 view=-direction;
    vec3 local=vec3(planetData.detail.yz,planetData.water.z)+direction*(surfaceDistance*1000.0);
    float time=planetData.mie.z;
    // Pure shading ripples: no vertex displacement or shoreline movement.
    float rippleFade=1.0-smoothstep(0.3,2.0,surfaceDistance);
    float footprint=max(length(dFdx(local)),length(dFdy(local)));
    float detail=1.0-smoothstep(0.25,3.0,footprint);
    vec3 normal=waterNormal(local,radial,detail*rippleFade);
    float nv=max(dot(normal,view),0.02);
    float fresnel=0.02+0.98*pow(1.0-nv,5.0);
    float travel=max(opaqueDistance-surfaceDistance,0.0)*1000.0;
    vec3 transmission=exp(-WATER_ABSORPTION*travel);
    float day=smoothstep(-0.12,0.25,dot(radial,sun));
    vec3 body=vec3(0.007,0.055,0.075)*(0.05+day);
    vec3 bed=bedRadiance*transmission+body*(1.0-transmission);
    vec3 reflected=reflect(direction,normal);
    reflected=normalize(reflected+radial*max(0.0,0.015-dot(reflected,radial)));
    Air sky=integrateAir(position+radial*0.001,reflected,PLANET_RADIUS*8.0,sun,pc.sunColor.rgb);
    vec3 skyColor=sky.light+vec3(0.001,0.002,0.004)*sky.transmittance;
    vec3 halfVector=sun+view;
    // Broaden subpixel glints according to normal variation and preserve
    // approximate lobe energy instead of allowing unresolved sparkle.
    vec3 nx=dFdx(normal),ny=dFdy(normal);
    float variance=0.5*(dot(nx,nx)+dot(ny,ny));
    float specPower=2.0/(2.0/320.0+variance);
    float spec=pow(max(dot(normal,halfVector/max(length(halfVector),0.000001)),0.0),specPower)*(specPower/320.0);
    vec3 direct=(ATM_ENABLED?sunlight(position+radial*0.001,sun):vec3(1.0))
        *localShadow(direction*(surfaceDistance*1000.0),radial);
    vec3 water=mix(bed,skyColor,fresnel)+pc.sunColor.rgb*direct*spec*0.65;
    if(planetData.features.z>0.5) {
        // Convert optical travel to radial bed depth: a grazing view must
        // not make the foam band wider or narrower in world space.
        float along=dot(direction,radial)*travel;
        float bedDepth=max(-along-(travel*travel-along*along)/(2.0*radius*1000.0),0.0);
        if(bedDepth<1.6&&travel<150.0) {
            float coverage=shoreFoamCoverage(local,bedDepth,footprint)*(1.0-smoothstep(100.0,150.0,travel));
            vec3 foam=vec3(0.82,0.87,0.84)*(pc.sunColor.rgb*direct*max(dot(radial,sun),0.0)*0.5+vec3(0.12)*day+0.004);
            water=mix(water,foam,coverage);
        }
    }
    // Preserve a narrow clear shallows band rather than drawing a hard rim.
    float shoreFade=smoothstep(0.0,0.5,travel);
    Air air=viewAir(direction,surfaceDistance,sun,pc.sunColor.rgb);
    return mix(underlying,water*air.transmittance+air.light,shoreFade);
}
