// GlyphEngine's shared atmosphere/shadow ABI, mirrored in one place.
// Sky binds the same UBO at set 0 / binding 1; lit and application passes at 1/0.
#ifndef ENGINE_ATMOSPHERE_SET
#define ENGINE_ATMOSPHERE_SET 1
#define ENGINE_ATMOSPHERE_BINDING 0
#endif
layout(set=ENGINE_ATMOSPHERE_SET,binding=ENGINE_ATMOSPHERE_BINDING) uniform AtmosphereData {
    mat4 cascadeVP[2]; vec4 nightGrade; vec4 skyPalette[6]; vec4 volumetric;
} atm;
layout(set=1,binding=1) uniform sampler2DArrayShadow terrainShadowMap;
#define TERRAIN_SHADOWS
