#include "cloud-shadow-coordinates.glsl"

float cloudShadowKnot(vec4 knots,float fraction) {
    float f=clamp(fraction,0.0,1.0)*4.0;
    int k=min(int(f),3);
    return mix(k==0?0.0:knots[k-1],knots[k],f-float(k));
}
vec4 cloudShadowRead(sampler2D field,vec2 uv,int cascade,int lobe) {
    vec2 tile=vec2(textureSize(field,0)/2);
    // Clamp inside each tile. Linear filtering must never borrow a different
    // cascade or the opposite shell lobe at an atlas boundary.
    uv=clamp(uv,0.5/tile,1.0-0.5/tile);
    return textureLod(field,(vec2(cascade,lobe)+uv)*0.5,0.0);
}
float cloudShadowCascade(sampler2D field,vec3 p,int cascade) {
    vec2 xy=vec2(dot(p,planetData.cloudShadowU.xyz),dot(p,planetData.cloudShadowV.xyz));
    vec2 height=cloudShadowHeights(xy);
    float span=height.y-height.x;
    if(span<=0.0001) return 0.0;
    float z=dot(p,cloudShadowSun());
    if(z>=height.y) return 0.0;
    vec2 uv=(xy-cloudShadowCenter(cascade))/(2.0*cloudShadowSpan(cascade))+0.5;
    vec4 near=cloudShadowRead(field,uv,cascade,0);
    float tau=cloudShadowKnot(near,(height.y-z)/span);
    if(z < -height.x) {
        vec4 far=cloudShadowRead(field,uv,cascade,1);
        tau+=cloudShadowKnot(far,(-height.x-z)/span);
    }
    return tau;
}
float cloudShadowDepth(sampler2D field,vec3 p) {
    if(planetData.cloudShadowMeta.w<=0.0) return 0.0;
    vec2 xy=vec2(dot(p,planetData.cloudShadowU.xyz),dot(p,planetData.cloudShadowV.xyz));
    vec2 local=(xy-cloudShadowCenter(0))/cloudShadowSpan(0);
    float farWeight=smoothstep(0.65,0.9,max(abs(local.x),abs(local.y)));
    float near=0.0;
    if(farWeight<1.0) near=cloudShadowCascade(field,p,0);
    if(farWeight<=0.0) return near;
    return mix(near,cloudShadowCascade(field,p,1),farWeight);
}
