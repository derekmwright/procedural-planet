// Shared refracted sunlight field. The inverse path is retained for A/B tests.
#include "water-wave.glsl"
#include "caustic-depths.glsl"
vec3 waterRayDerivative(vec3 incoming,vec3 normal,vec3 dn) {
    const float eta=1.0/1.333;
    float c=dot(normal,incoming);
    float root=sqrt(max(1.0-eta*eta*(1.0-c*c),0.0001));
    return -(eta+eta*eta*c/root)*dot(dn,incoming)*normal-(eta*c+root)*dn;
}
// Trace a small patch of incident sunlight onto the plane at receiver depth.
// The Jacobian measures projected area, giving intensity = 1 / area ratio.
void waterProjection(vec3 entry,vec3 up,vec3 u,vec3 v,vec3 incoming,float depth,
    out vec2 offset,out mat2 jacobian) {
    WaterWave wave=waterWaves(entry,up);
    vec3 rawNormal=up-wave.gradient;
    float nlen=length(rawNormal);
    vec3 normal=rawNormal/nlen;
    vec3 ray=refract(incoming,normal,1.0/1.333);
    float down=min(dot(ray,up),-0.1);
    float travel=-depth/down;
    offset=vec2(dot(ray,u),dot(ray,v))*travel;
    vec3 hu=wave.curvature*u,hv=wave.curvature*v;
    vec3 du=waterRayDerivative(incoming,normal,(-hu+normal*dot(normal,hu))/nlen);
    vec3 dv=waterRayDerivative(incoming,normal,(-hv+normal*dot(normal,hv))/nlen);
    du=travel*(du-ray*(dot(du,up)/down));
    dv=travel*(dv-ray*(dot(dv,up)/down));
    jacobian=mat2(vec2(1,0)+vec2(dot(du,u),dot(du,v)),
                  vec2(0,1)+vec2(dot(dv,u),dot(dv,v)));
}
float waterLightFocusLegacy(vec3 receiver,vec3 up,float depth) {
    float fade=1.0-smoothstep(20.0,50.0,depth);
    vec3 incoming=-normalize(pc.sunDir.xyz);
    if(fade<=0.0||dot(-incoming,up)<0.08) return 1.0;
    vec3 axis=abs(up.y)<0.95?vec3(0,1,0):vec3(1,0,0);
    vec3 u=normalize(cross(axis,up)),v=cross(up,u);
    vec3 flatRay=refract(incoming,up,1.0/1.333);
    vec2 flatOffset=vec2(dot(flatRay,u),dot(flatRay,v))*(-depth/dot(flatRay,up));
    vec2 entry=-flatOffset;
    mat2 jacobian; vec2 offset;
    float minimumDet=1.0;
    // Invert the refracted map locally. Smooth waves and bounded depth keep
    // this prototype in the mostly single-valued regime.
    for(int i=0;i<3;i++) {
        waterProjection(receiver+up*depth+u*entry.x+v*entry.y,up,u,v,incoming,depth,offset,jacobian);
        float det=determinant(jacobian);
        minimumDet=min(minimumDet,det);
        if(det<0.12) return 1.0;
        vec2 error=entry+offset;
        if(i<2) entry-=clamp(inverse(jacobian)*error,vec2(-1.0),vec2(1.0));
    }
    float residual=length(entry+offset);
    float confidence=1.0-smoothstep(0.05,0.4,residual);
    // Fade the unresolved fold boundary instead of cutting a neutral hole
    // into a bright peak when the local inverse ceases to be reliable.
    confidence*=smoothstep(0.12,0.45,minimumDet);
    float intensity=clamp(1.0/determinant(jacobian),0.25,4.0);
    return mix(1.0,intensity,fade*confidence);
}

layout(set=1,binding=10) uniform sampler2D causticAtlas;
float causticSlice(vec2 uv,int layer) {
    vec2 tile=vec2(layer%4,layer/4);
    uv=clamp(uv,vec2(0.5/512.0),vec2(1.0-0.5/512.0));
    return textureLod(causticAtlas,(uv+tile)/vec2(4,2),0.0).r;
}
float waterLightFocus(vec3 receiver,vec3 up,float depth) {
    if(planetData.rendering.y<0.5) return waterLightFocusLegacy(receiver,up,depth);
    float fade=1.0-smoothstep(20.0,50.0,depth);
    if(planetData.causticOrigin.w<0.5||fade<=0.0) return 1.0;
    vec3 u=planetData.causticU.xyz,v=planetData.causticV.xyz;
    vec3 localUp=normalize(cross(u,v));
    vec3 cameraRelative=receiver-vec3(planetData.detail.yz,planetData.water.z);
    // Coverage is circular and measured from the current camera's water column,
    // independent of light direction, atlas axes, and the anchor's small drift.
    float coverageDistance=length(vec2(dot(cameraRelative,u),dot(cameraRelative,v)));
    float coverageRadius=planetData.causticV.w;
    fade*=1.0-smoothstep(coverageRadius*0.4,coverageRadius,coverageDistance);
    if(fade<=0.0) return 1.0;
    vec3 relative=cameraRelative-planetData.causticOrigin.xyz;
    vec3 flatRay=refract(-normalize(pc.sunDir.xyz),localUp,1.0/1.333);
    int layer=0;
    for(int i=1;i<7;i++) if(depth>=CAUSTIC_DEPTHS[i]) layer=i;
    float blend=clamp((depth-CAUSTIC_DEPTHS[layer])/(CAUSTIC_DEPTHS[layer+1]-CAUSTIC_DEPTHS[layer]),0.0,1.0);
    // Follow the same flat refracted beam between depth slices, rather than
    // blending unrelated vertical columns. Slice origins now include their
    // own flat-ray offset, keeping BOTH lookups inside the padded atlas.
    vec2 xy=vec2(dot(relative,u),dot(relative,v));
    vec2 slope=vec2(dot(flatRay,u),dot(flatRay,v))/(-dot(flatRay,localUp));
    vec2 uv0=(xy+slope*(CAUSTIC_DEPTHS[layer]-depth))/planetData.causticU.w+0.5;
    vec2 uv1=(xy+slope*(CAUSTIC_DEPTHS[layer+1]-depth))/planetData.causticU.w+0.5;
    float intensity=mix(causticSlice(uv0,layer),causticSlice(uv1,layer+1),blend);
    return mix(1.0,intensity,fade);
}
