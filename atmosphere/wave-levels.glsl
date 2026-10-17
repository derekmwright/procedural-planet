// One-texel periodic gutters around each level keep hardware bilinear reads
// inside their own tile. Level zero retains the original 0.25 m sample phase.
const ivec2 WAVE_ATLAS_SIZE=ivec2(772,529);
int waveLevelSize(int level) { return 512>>level; }
ivec2 waveLevelOrigin(int level) {
    return level==0?ivec2(0):ivec2(514,512-(512>>(level-1))+2*(level-1));
}
vec2 waveLevelUV(vec2 p,int level) {
    float size=float(waveLevelSize(level));
    return (vec2(waveLevelOrigin(level))+1.5+fract(p/128.0)*size)/vec2(WAVE_ATLAS_SIZE);
}
