layout(set=1,binding=12) uniform sampler2D waterSkyLight;
layout(set=1,binding=13) uniform sampler2D waterSkyTransmission;
#include "water-sky-coordinates.glsl"

Air waterSkyLevel(vec3 direction,int level) {
    return Air(waterSkySample(waterSkyLight,direction,level).rgb,
        waterSkySample(waterSkyTransmission,direction,level).rgb);
}
Air waterSky(vec3 position,vec3 direction,vec3 sun,vec3 color,float variance) {
    float coverage=planetData.waterRendering.x*(1.0-smoothstep(8.0,32.0,distance(position,waterSkyOrigin())));
    Air result;
    if(coverage>0.0) {
        vec3 lookup=waterSkyParallax(position,direction);
        // Reflection doubles normal angles. The unresolved slope distribution
        // widens the sky footprint instead of making distant water a mirror.
        float spread=2.0*sqrt(max(variance,0.0));
        float lod=clamp(log2(max(spread*float(WATER_SKY_SIZE)*0.5,1.0)),0.0,float(WATER_SKY_LEVELS-1));
        int level=int(lod);
        result=waterSkyLevel(lookup,level);
        if(level<WATER_SKY_LEVELS-1&&fract(lod)>0.0) {
            Air next=waterSkyLevel(lookup,level+1);
            result.light=mix(result.light,next.light,fract(lod));
            result.transmittance=mix(result.transmittance,next.transmittance,fract(lod));
        }
        if(coverage>=1.0) return result;
    }
    Air direct=integrateAir(position,direction,PLANET_RADIUS*8.0,sun,color);
    if(coverage<=0.0) return direct;
    return Air(mix(direct.light,result.light,coverage),mix(direct.transmittance,result.transmittance,coverage));
}
