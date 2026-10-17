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
    if(angle>=0.07) return vec3(0);

    // Most optical glare belongs in the soft corona. Faint, unequal diffraction
    // axes give it a burst without cutting a regular bright star out of a dark
    // high-altitude sky. Their cross-sections narrow away from the disc rather
    // than maintaining a fixed azimuth and widening into wedges.
    vec2 radial=(uv-pc.targetInfo.yz)*vec2(size);
    vec2 angular=radial*(angle/max(length(radial),0.00001));
    // A projected half-pixel softens the narrow tips without widening them
    // proportionally to distance or relying on derivatives in divergent flow.
    float discPixels=min(length(pc.sunDiscAxes.xy*vec2(size)),length(pc.sunDiscAxes.zw*vec2(size)));
    float aa=0.5*SUN_DISC_RADIUS/max(discPixels,1.0);
    float distance=max(angle-SUN_DISC_RADIUS,0.0);
    float profile=0.055*exp(-distance/0.0065);
    const vec4 axes[4]=vec4[](
        vec4(0.10,0.0065,0.018,0.0040),
        vec4(1.13,0.0050,0.012,0.0028),
        vec4(1.96,0.0040,0.015,0.0035),
        vec4(2.52,0.0055,0.011,0.0023)); // angle, root width, decay length, intensity
    for(int i=0;i<4;i++) {
        vec2 axis=vec2(cos(axes[i].x),sin(axes[i].x));
        float along=abs(dot(angular,axis));
        float across=dot(angular,vec2(-axis.y,axis.x));
        float travel=max(along-SUN_DISC_RADIUS,0.0);
        float width=axes[i].y*exp(-30.0*travel);
        profile+=axes[i].w*exp(-2.0*across*across/(width*width+aa*aa)-travel/axes[i].z);
    }
    profile*=1.0-smoothstep(0.045,0.07,angle);
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
