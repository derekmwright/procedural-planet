// A cube stored as six padded 2D tiles. Every level has its own gutters;
// filtering may cross a face boundary but never a neighbouring atlas tile.
const int WATER_SKY_SIZE=64;
const int WATER_SKY_LEVELS=7;
const ivec2 WATER_SKY_ATLAS=ivec2(198,282);
int waterSkySize(int level) { return WATER_SKY_SIZE>>level; }
int waterSkyRow(int level) { return 4*(WATER_SKY_SIZE-(WATER_SKY_SIZE>>level))+4*level; }
vec3 waterSkyDirection(int face,vec2 p) {
    if(face==0) return normalize(vec3(1,-p.y,-p.x));
    if(face==1) return normalize(vec3(-1,-p.y,p.x));
    if(face==2) return normalize(vec3(p.x,1,p.y));
    if(face==3) return normalize(vec3(p.x,-1,-p.y));
    if(face==4) return normalize(vec3(p.x,-p.y,1));
    return normalize(vec3(-p.x,-p.y,-1));
}
vec2 waterSkyUV(vec3 direction,int level,ivec2 extent) {
    vec3 a=abs(direction); int face; vec2 p;
    if(a.x>=a.y&&a.x>=a.z) {
        face=direction.x>=0?0:1; p=vec2(direction.x>=0?-direction.z:direction.z,-direction.y)/a.x;
    } else if(a.y>=a.z) {
        face=direction.y>=0?2:3; p=vec2(direction.x,direction.y>=0?direction.z:-direction.z)/a.y;
    } else {
        face=direction.z>=0?4:5; p=vec2(direction.z>=0?direction.x:-direction.x,-direction.y)/a.z;
    }
    int size=waterSkySize(level),tile=size+2;
    vec2 origin=vec2((face%3)*tile,waterSkyRow(level)+(face/3)*tile);
    return (origin+1.0+(p*0.5+0.5)*float(size))/vec2(extent);
}
bool waterSkyTexel(ivec2 pixel,int level,out int face,out ivec2 local) {
    int tile=waterSkySize(level)+2;
    ivec2 p=pixel-ivec2(0,waterSkyRow(level));
    if(any(lessThan(p,ivec2(0)))||any(greaterThanEqual(p,ivec2(3,2)*tile))) return false;
    face=p.x/tile+(p.y/tile)*3;
    local=p%tile-1;
    return true;
}
vec3 waterSkyOrigin() { return normalize(EYE_PLANET)*(planetData.water.x+0.001); }

vec3 waterSkyParallax(vec3 position,vec3 direction) {
    // Intersect the actual curved cloud shell, then reproject that point from
    // the sea-level probe. One representative height is an approximation for
    // a thick volume, so the caller restricts this cache to the local coast.
    vec2 hit=sphereInterval(position,direction,planetData.waterRendering.z);
    if(hit.y<=0.0||hit.y<hit.x) return direction;
    return normalize(position+direction*hit.y-waterSkyOrigin());
}
