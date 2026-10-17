#version 450
#extension GL_GOOGLE_include_directive : require
layout(set=2,binding=0) uniform sampler2D composed;
#include "air-frame.glsl"
#include "parameters.glsl"
#include "sun-disc.glsl"
layout(location=0) out vec4 outColor;

vec3 sunGlare(vec2 uv,ivec2 size) {
    if(!DEFERRED_CLOUDS||planetData.planet.z<0.5||planetData.features.y<0.5||pc.targetInfo.w<0.5)
        return vec3(0);
    vec3 direction=normalize((pc.inverseVP*vec4(uv*2.0-1.0,0,1)).xyz);
    float angle=sunSeparation(direction,normalize(pc.sunDir.xyz));
    if(angle>=0.09) return vec3(0);

    // Fixed azimuth-width lobes grow into wedges as their radius increases.
    // Instead give each spoke a broad root and a narrowing cross-section.
    vec2 radial=(uv-pc.targetInfo.yz)*vec2(size);
    float azimuth=dot(radial,radial)>1e-8?atan(radial.y,radial.x):0.0;
    const float sector=1.04719755; // six spokes, retaining their screen orientation
    float offset=mod(azimuth+0.4/6.0+0.5*sector,sector)-0.5*sector;
    float along=angle*cos(offset),across=angle*abs(sin(offset));
    float progress=clamp((along-SUN_DISC_RADIUS)/(0.085-SUN_DISC_RADIUS),0.0,1.0);
    float width=0.01*(1.0-progress);
    // A projected half-pixel softens the narrow tips without widening them
    // proportionally to distance or relying on derivatives in divergent flow.
    float discPixels=min(length(pc.sunDiscAxes.xy*vec2(size)),length(pc.sunDiscAxes.zw*vec2(size)));
    float aa=0.5*SUN_DISC_RADIUS/max(discPixels,1.0);
    float spoke=exp(-2.0*across*across/(width*width+aa*aa));
    float distance=max(angle-SUN_DISC_RADIUS,0.0);
    float profile=0.04*exp(-distance/0.005)
        +0.025*spoke*exp(-max(along-SUN_DISC_RADIUS,0.0)/0.03)
            *(1.0-smoothstep(0.06,0.085,along));
    if(profile<1e-6) return vec3(0);

    // Integrate the visible disc, not the surrounding bright sky. The same
    // fixed samples are used by every pixel/frame: no animated grain or
    // temporal history when a cloud or a mountain crosses the source.
    vec3 source=vec3(0);
    for(int i=0;i<32;i++) {
        float a=float(i)*2.39996323;
        vec2 disk=sqrt((float(i)+0.5)/32.0)*vec2(cos(a),sin(a));
        vec2 q=pc.targetInfo.yz+pc.sunDiscAxes.xy*disk.x+pc.sunDiscAxes.zw*disk.y;
        if(any(lessThan(q,vec2(0)))||any(greaterThan(q,vec2(1)))) continue;
        // The lobe/edge branches are divergent; this single-level target does
        // not need implicit texture derivatives from neighboring lanes.
        vec4 sampleColor=textureLod(composed,q,0.0);
        // Preserve the visible source's color while excluding its scattered
        // background energy. The mask fixes luminance; color is an estimate
        // from the composed solar pixel, where direct sunlight dominates.
        source+=sampleColor.rgb*clamp(sampleColor.a/max(dot(sampleColor.rgb,SUN_LUMINANCE),1e-5),0.0,1.0);
    }
    source/=32.0;
    return source*profile;
}
void main() {
    ivec2 pixel=ivec2(gl_FragCoord.xy),size=textureSize(composed,0);
    vec2 uv=(vec2(pixel)+0.5)/vec2(size);
    outColor=vec4(texelFetch(composed,pixel,0).rgb+sunGlare(uv,size),1);
}
