// Shared depth semantics for atmospheric integration and reconstruction.
// Depth buffer describes opaque terrain. Analytic water is a nearer endpoint.
vec3 viewDirection(vec2 uv) {
    vec4 p=pc.inverseVP*vec4(uv*2.0-1.0,0.0,1.0);
    return normalize(p.xyz);
}
float airEndpoint(vec2 uv,float depth) {
    vec3 direction=viewDirection(uv);
    float distance=PLANET_RADIUS*30.0;
    bool surface=depth>0.0;
    if(surface) {
        vec4 p=pc.inverseVP*vec4(uv*2.0-1.0,depth,1.0);
        distance=length(p.xyz/p.w)*0.001;
    } else {
        vec2 ground=sphereInterval(EYE_PLANET,direction,solidRadius());
        if(ground.x>0.0&&ground.x<ground.y) distance=ground.x;
    }
    float sea=planetData.water.x;
    if(sea>0.0&&planetData.detail.x>=0.0) {
        vec2 water=waterInterval(direction);
        if(water.x>0.0&&water.x<water.y&&water.x<distance) {
            distance=water.x;
            surface=true;
        }
    }
    // Cloud mode owns sky atmosphere too: clouds and air must be composed in
    // depth order, without adding a second sky haze layer behind the volume.
    return surface||DEFERRED_CLOUDS?distance:-1.0;
}
