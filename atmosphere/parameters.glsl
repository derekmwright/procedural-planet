// std140 layout shared with Parameters.Bytes. Positions use km; detail origins use m.
layout(set=1,binding=6,std140) uniform PlanetParameters {
    vec4 eyePlanet; // xyz: eye in planet coordinates
    vec4 planet;    // radius km, Rayleigh height km, atmosphere enabled, sun LUT enabled
    vec4 rayleigh;  // RGB scattering coefficients
    vec4 mie;       // scattering coefficient, height km, animation seconds, wave cache enabled
    vec4 water;     // sea radius km, materials enabled, eye Z modulo 4096m, water scatter pass
    vec4 detail;    // signed eye height above sea (m), eye X/Y modulo 4096m, reserved
    vec4 features;  // cast shadow strength 0..1, sun shafts, shoreline foam, caustic debug
    vec4 rendering; // deferred view atmosphere, forward caustic cache, temporal caustics, cloud view pass
    vec4 causticOrigin; // camera-relative patch center (m), active
    vec4 causticU; // tangent U, span (m)
    vec4 causticV; // tangent V, circular coverage radius (m)
} planetData;
#define DEFERRED_AIR (planetData.rendering.x > 0.5)
#define DEFERRED_CLOUDS (planetData.rendering.w > 0.5)
