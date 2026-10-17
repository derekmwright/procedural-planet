float cloudPhase(float cosine,float g) {
    return (1.0-g*g)/(4.0*PI*pow(max(1.0+g*g-2.0*g*cosine,0.001),1.5));
}
float cloudSunDepth(vec3 p,vec3 sun) {
    vec2 hit=sphereInterval(p,sun,PLANET_RADIUS+cloud.layer.x+cloud.layer.y);
    float distance=min(max(hit.y,0.0),12.0),depth=0.0;
    for(int j=0;j<5;j++) {
        float a=float(j)/5.0,b=float(j+1)/5.0;
        float lo=a*a*distance,hi=b*b*distance;
        depth+=cloudDensity(p+sun*((lo+hi)*0.5),false,(hi-lo)*0.5)*(hi-lo);
    }
    return depth*cloud.layer.z;
}
void appendAir(inout Air accumulated,Air segment) {
    accumulated.light+=accumulated.transmittance*segment.light;
    accumulated.transmittance*=segment.transmittance;
}
float cloudSampleOffset(int step) {
    // Interleaved gradient noise (Jorge Jimenez, SIGGRAPH 2014). Nearby pixels
    // cover the whole interval, so a small spatial filter can average the error.
    // Alternate its orientation between steps to avoid a persistent diagonal;
    // keep it independent of time so a stationary view does not shimmer.
    vec2 pixel=vec2(cloudPixel);
    if((step&1)!=0) pixel=pixel.yx;
    float shift=fract(float(step)*0.61803398875);
    return fract(52.9829189*fract(dot(pixel,vec2(0.06711056,0.00583715))+shift));
}
void marchCloud(inout Air result,vec3 origin,vec3 dir,vec3 sun,vec2 interval,float cone) {
    float span=interval.y-interval.x;
    if(span<=0.0) return;
    int steps=int(clamp(ceil(span/0.10),8.0,cloud.wind.z));
    float cosine=clamp(dot(dir,sun),-1.0,1.0);
    float phaseR=3.0/(16.0*PI)*(1.0+cosine*cosine);
    float phaseM=cloudPhase(cosine,0.76);
    float phaseC=0.85*cloudPhase(cosine,0.65)+0.15*cloudPhase(cosine,-0.2);
    for(int i=0;i<steps;i++) {
        float a=float(i)/float(steps),b=float(i+1)/float(steps);
        float lo=interval.x+a*a*span,hi=interval.x+b*b*span;
        float dt=hi-lo;
        // A shared offset restricted to [0.2,0.8] left unsampled gaps in every
        // interval. On grazing rays those gaps appeared as horizontal bands.
        float travel=mix(lo,hi,cloudSampleOffset(i));
        vec3 p=origin+dir*travel;
        vec2 airDensity=densityAt(p);
        float density=cloudDensity(p,true,max(travel*cone,dt*0.5)),sigmaC=density*cloud.layer.z;
        vec3 sigmaA=BETA_R*airDensity.x+vec3(BETA_M*airDensity.y);
        vec3 incident=sunlight(p,sun)*pc.sunColor.rgb*SUN_ENERGY;
        float terrainVisibility=airShadow((p-EYE_PLANET)*1000.0);
        vec3 source=(BETA_R*airDensity.x*phaseR+vec3(BETA_M*airDensity.y*phaseM))*incident*terrainVisibility*cloudSunVisibility(p);
        if(density>0.001) {
            float tau=cloudSunDepth(p,sun);
            // Bounded multiple-scattering approximation; attenuate and broaden
            // each successive lobe. This is not a full multiple-scattering solve.
            float lit=phaseC*exp(-tau)+0.28*cloudPhase(cosine,0.32)*exp(-tau*0.3)
                +0.10/(4.0*PI)*exp(-tau*0.1);
            float height=clamp((length(p)-PLANET_RADIUS-cloud.layer.x)/cloud.layer.y,0.0,1.0);
            float day=smoothstep(-0.15,0.2,dot(normalize(p),sun));
            vec3 skyFill=vec3(0.06,0.085,0.12)*mix(0.015,1.0,day)*(0.35+0.65*height);
            source+=sigmaC*0.99*(incident*terrainVisibility*lit+skyFill);
        }
        vec3 sigma=sigmaA+vec3(sigmaC),transmit=exp(-sigma*dt);
        result.light+=result.transmittance*source*(vec3(1.0)-transmit)/max(sigma,vec3(0.000001));
        result.transmittance*=transmit;
        if(max(result.transmittance.r,max(result.transmittance.g,result.transmittance.b))<0.002) break;
    }
}
