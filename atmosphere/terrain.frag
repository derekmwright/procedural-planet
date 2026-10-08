#version 450
#extension GL_GOOGLE_include_directive : require

layout(location=0) in vec3 fragColor;
layout(location=1) in vec3 fragWorldPos;
layout(location=2) in vec3 fragWorldNormal;
layout(location=3) in vec2 fragUV;
layout(location=4) in vec3 fragShadowPos;
layout(location=0) out vec4 outColor;
layout(set=0,binding=0) uniform sampler2D texSampler;


layout(push_constant) uniform PushConstants {
    mat4 mvp; mat4 model;
    vec4 tint; vec4 sunDir; vec4 sunColor;
    vec4 pointPos; vec4 pointColor; vec4 ambient; vec4 cameraPos; vec4 fog;
} pc;

#include "parameters.glsl"
#include "engine-lighting.glsl"
#include "common.glsl"
#include "surface.glsl"
#include "ocean.glsl"

void main() {
    // Derivatives must precede divergent water/material branches. The plane
    // normal is for contact shadows only; the coarse mountain cascade needs a
    // continuous normal, before material detail perturbs the lighting normal.
    vec3 plane=cross(dFdx(fragWorldPos),dFdy(fragWorldPos));
    vec3 receiverNormal=plane/max(length(plane),0.00000001);
    vec3 ray=fragWorldPos*0.001;
    float distance=length(ray);
    vec3 direction=ray/max(distance,0.000001);
    bool submerged=cameraUnderwater();
    if(submerged) {
        float exitDistance=waterInterval(direction).y;
        if(exitDistance>0.0&&exitDistance<distance) {
            outColor=vec4(underwaterWindow(direction,exitDistance),1.0);
            return;
        }
        if(distance>WATER_OPAQUE_DISTANCE) {
            outColor=vec4(underwaterColor(vec3(0.0),distance,direction),1.0);
            return;
        }
    }
    float waterDistance=visibleOceanDistance(direction,distance);
    // At the clarity-scaled cutoff, blue bed transmission is below 1e-6.
    // No material, direct lighting, shadow sampling or bed air march is visible.
    if(waterDistance>0.0&&distance-waterDistance>WATER_OPAQUE_DISTANCE) {
        outColor=vec4(oceanComposite(vec3(0.0),vec3(0.0),direction,distance),1.0);
        return;
    }
    vec4 texel=texture(texSampler,fragUV);
    if (texel.a<0.5) discard;
    vec3 base=fragColor*texel.rgb;
    vec3 smoothNormal=normalize(fragWorldNormal);
    vec3 N=smoothNormal;
    surfaceMaterial(base,N);
    vec3 sunDir=normalize(pc.sunDir.xyz);
    vec3 point=EYE_PLANET+fragWorldPos*0.001;
    vec3 directTransmission=ATM_ENABLED?sunlight(point,sunDir):vec3(1.0);
    float waterDepth=max(planetData.water.x-length(point),0.0)*1000.0;
    // Evaluate nearby depths in camera-relative meters. Subtracting two
    // planet-sized floats per pixel quantizes the projected caustic pattern.
    if(distance<1.0&&planetData.water.x>0.0) {
        vec3 radial=normalize(EYE_PLANET);
        float along=dot(fragWorldPos,radial);
        float tangent2=max(dot(fragWorldPos,fragWorldPos)-along*along,0.0);
        waterDepth=max(-planetData.detail.x
            -along-tangent2/(2.0*length(EYE_PLANET)*1000.0),0.0);
    }
    if(planetData.features.w>0.5&&waterDepth>0.0) {
        vec3 p=vec3(planetData.detail.yz,planetData.water.z)+fragWorldPos;
        float focus=waterLightFocus(p,normalize(point),waterDepth);
        outColor=vec4(vec3(focus*0.25),1.0);
        return;
    }
    if(planetData.features.z>0.5&&fragUV.y<0.5&&dot(N,normalize(point))>0.85) {
        vec3 radial=normalize(EYE_PLANET);
        float along=dot(fragWorldPos,radial);
        float tangent2=max(dot(fragWorldPos,fragWorldPos)-along*along,0.0);
        float height=planetData.detail.x
            +along+tangent2/(2.0*length(EYE_PLANET)*1000.0);
        if(height>-4.0&&height<1.6&&distance<1.0) {
            vec3 p=vec3(planetData.detail.yz,planetData.water.z)+fragWorldPos;
            float patchiness=shoreNoise(p,4.0);
            float wash=0.5+0.5*cos(planetData.mie.z*0.8+patchiness*2.0);
            float wet=1.0-smoothstep(-0.25,0.65+wash*0.25,height-(patchiness-0.5)*0.8);
            wet*=smoothstep(-4.0,-0.5,height)*(1.0-smoothstep(0.7,1.0,distance));
            base*=mix(vec3(1),vec3(0.72,0.74,0.76),wet);
        }
    }
    // Blend the wet interface: an abrupt refracted-light switch creates a
    // visible contour even when foam and wetness are smoothly feathered.
    vec3 lightDir=normalize(mix(sunDir,-refract(-sunDir,normalize(point),1.0/1.333),smoothstep(0.0,0.4,waterDepth)));
    float ndl=max(dot(N,lightDir),0.0);
    vec3 irradiance=pc.sunColor.rgb*directTransmission*ndl
        *localShadow(fragShadowPos,receiverNormal,smoothNormal);
    if(waterDepth>0.0) irradiance*=smoothstep(0.0,0.08,dot(normalize(point),sunDir));
    irradiance*=exp(-WATER_ABSORPTION*waterDepth/max(dot(normalize(point),lightDir),0.1));
    if(waterDepth>0.0) irradiance*=seabedCaustics(fragWorldPos,point,waterDepth);
    vec3 ambient=pc.ambient.rgb;
    if (ATM_ENABLED) {
        float localDay=smoothstep(-0.12,0.25,dot(normalize(point),sunDir));
        float height=max(length(point)-PLANET_RADIUS,0.0);
        ambient+=vec3(0.055,0.085,0.14)*localDay*exp(-height/RAYLEIGH_HEIGHT);
    }
    // Diffuse rock lighting with a restrained rough-surface specular response.
    ambient*=exp(-waterDepth*0.035/WATER_CLARITY);
    vec3 V=normalize(-fragWorldPos);
    vec3 halfVector=lightDir+V;
    vec3 H=halfVector/max(length(halfVector),0.000001);
    float roughness=clamp(pc.pointColor.w,0.04,1.0);
    float power=max(2.0/(roughness*roughness)-2.0,1.0);
    vec3 lit=base*(ambient+irradiance)+irradiance*0.04*pow(max(dot(N,H),0.0),power)*0.15;
    // The viewing segment is water, not air. Keep atmospheric attenuation
    // on incoming sunlight, but do not march atmospheric haze to the seabed.
    if(submerged) {
        outColor=vec4(underwaterColor(lit,distance,direction),1.0);
        return;
    }
    Air air=Air(vec3(0.0),vec3(1.0));
    // Beyond the half-meter shore blend, only bed radiance is used by water.
    // Its atmosphere is integrated to the water surface, not to the seabed.
    if(waterDistance<0.0||distance-waterDistance<0.0005)
        air=viewAir(direction,distance,sunDir,pc.sunColor.rgb);
    outColor=vec4(oceanComposite(lit*air.transmittance+air.light,lit,direction,distance),1.0);
}
