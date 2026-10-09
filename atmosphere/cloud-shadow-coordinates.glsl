// A directional-light ray intersects two cloud-shell lobes. Parameterize each
// separately so four depth knots resolve a 3 km cloud bank rather than the
// hundreds of kilometres of empty space between its near and far sides.
vec2 cloudShadowHeights(vec2 lightXY) {
    float r2=dot(lightXY,lightXY);
    return sqrt(max(planetData.cloudShadowMeta.xy*planetData.cloudShadowMeta.xy-vec2(r2),vec2(0)));
}
vec3 cloudShadowSun() {
    return normalize(cross(planetData.cloudShadowU.xyz,planetData.cloudShadowV.xyz));
}
vec2 cloudShadowCenter(int cascade) {
    return cascade==0?vec2(planetData.cloudShadowU.w,planetData.cloudShadowV.w):vec2(0);
}
float cloudShadowSpan(int cascade) {
    return cascade==0?planetData.cloudShadowMeta.z:planetData.cloudShadowMeta.y;
}
