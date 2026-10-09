#version 450
#extension GL_GOOGLE_include_directive : require

layout(location=0) in vec2 fragUV;
layout(location=0) out vec4 outColor;
layout(push_constant) uniform PushConstants {
    mat4 invVP; mat4 model;
    vec4 tint; vec4 sunDir; vec4 sunColor;
    vec4 pointPos; vec4 pointColor; vec4 ambient; vec4 cameraPos; vec4 fog;
} pc;

#define ENGINE_ATMOSPHERE_SET 0
#define ENGINE_ATMOSPHERE_BINDING 1
#include "engine-lighting.glsl"
#include "parameters.glsl"
#include "common.glsl"
#include "ocean.glsl"

void main() {
    vec4 world=pc.invVP*vec4(fragUV*2.0-1.0,0.0,1.0);
    // The application keeps the rendering camera at the origin.
    vec3 dir=normalize(world.xyz);
    vec3 sunDir=normalize(pc.sunDir.xyz);
    // Derivatives must execute before ocean/underwater early returns.
    // Divergent neighbor lanes otherwise produce stray sun-disc pixels.
    float separation=atan(length(cross(dir,sunDir)),dot(dir,sunDir));
    float aa=max(fwidth(separation),0.00015);
    vec3 background=vec3(0.001,0.002,0.004);
    float limit=PLANET_RADIUS*30.0;
    vec2 ground=sphereInterval(EYE_PLANET,dir,solidRadius());
    bool blocked=ground.x>0.0&&ground.x<ground.y;
    if (blocked) limit=ground.x;
    if(cameraUnderwater()) {
        float exitDistance=waterInterval(dir).y;
        outColor=vec4(exitDistance>0.0&&exitDistance<limit
            ?underwaterWindow(dir,exitDistance):underwaterColor(vec3(0.0),limit,dir),1.0);
        return;
    }
    float waterDistance=visibleOceanDistance(dir,limit);
    if(waterDistance>0.0&&limit-waterDistance>WATER_OPAQUE_DISTANCE) {
        outColor=vec4(oceanComposite(vec3(0.0),vec3(0.0),dir,limit),1.0);
        return;
    }
    Air air=DEFERRED_CLOUDS?Air(vec3(0),vec3(1)):
        integrateAir(EYE_PLANET,dir,limit,sunDir,pc.sunColor.rgb);
    // Slightly enlarged angular disc, constant in space and at the surface.
    float disc=1.0-smoothstep(0.009-aa,0.009+aa,separation);
    if (!blocked) background += disc*pc.sunColor.rgb*12.0;
    outColor=vec4(oceanComposite(background*air.transmittance+air.light,background,dir,limit),1.0);
}
