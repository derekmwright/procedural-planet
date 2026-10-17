// Shared angular source for the sky and the post-cloud glare mask.
const float SUN_DISC_RADIUS=0.009;
const float SUN_DISC_RADIANCE=12.0;
const vec3 SUN_LUMINANCE=vec3(0.2126,0.7152,0.0722);
float sunSeparation(vec3 direction,vec3 sun) {
    return atan(length(cross(direction,sun)),dot(direction,sun));
}
float sunDisc(float separation,float aa) {
    return 1.0-smoothstep(SUN_DISC_RADIUS-aa,SUN_DISC_RADIUS+aa,separation);
}
