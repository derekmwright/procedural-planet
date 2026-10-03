struct WaterWave { vec3 gradient; mat3 curvature; };
WaterWave waterWavesLegacy(vec3 p,vec3 up) {
    WaterWave w=WaterWave(vec3(0),mat3(0));
    // Short ripples in varied directions create curvature in both surface axes.
    // The previous nearly aligned waves focused only weakly in shallow water.
    // Integer lattice
    // vectors retain 4096 m world-origin wrapping without short tiled repeats.
    const float lattice=6.28318530718/4096.0;
    const vec3 frequencies[5]=vec3[5](vec3(811,213,-397)*lattice,
        vec3(-307,593,677)*lattice,vec3(419,-887,251)*lattice,vec3(1073,-293,-691)*lattice,vec3(-719,-547,1031)*lattice);
    const vec3 modulations[5]=vec3[5](vec3(173,97,-139)*lattice,
        vec3(-131,157,83)*lattice,vec3(113,-179,149)*lattice,vec3(-197,101,157)*lattice,vec3(139,211,-107)*lattice);
    const float amplitudes[5]=float[5](0.035,0.027,0.020,0.012,0.009);
    const float speeds[5]=float[5](0.7,-0.5,0.3,-0.9,1.1);
    const float bends[5]=float[5](2.1,1.8,2.3,1.7,2.5);
    for(int i=0;i<5;i++) {
        vec3 k=frequencies[i],m=modulations[i];
        float modulation=dot(p,m)+planetData.mie.z*0.1;
        float phase=dot(p,k)+planetData.mie.z*speeds[i]+bends[i]*sin(modulation);
        vec3 phaseGradient=k+m*(bends[i]*cos(modulation));
        vec3 tangent=phaseGradient-up*dot(up,phaseGradient);
        vec3 mt=m-up*dot(up,m);
        // Local wave packets bend and weaken independently, breaking long
        // uniform interference bands. Derivatives include BOTH the envelope
        // and phase warp so refraction and caustics describe the same surface.
        float envelope=0.7+0.3*cos(modulation);
        vec3 envelopeGradient=-0.3*sin(modulation)*mt;
        mat3 envelopeHessian=-0.3*cos(modulation)*outerProduct(mt,mt);
        mat3 phaseHessian=-bends[i]*sin(modulation)*outerProduct(mt,mt);
        float sn=sin(phase),cs=cos(phase),a=amplitudes[i];
        w.gradient+=a*(envelope*cs*tangent+sn*envelopeGradient);
        w.curvature+=a*(-envelope*sn*outerProduct(tangent,tangent)
            +cs*(outerProduct(envelopeGradient,tangent)+outerProduct(tangent,envelopeGradient))
            +sn*envelopeHessian+envelope*cs*phaseHessian);
    }
    return w;
}
// Hardware bilinear repeat over one 128 m period. Texel zero stores p=0,
// so offset by half a texel to preserve the original field's sample phase.
layout(set=1,binding=8) uniform sampler2D waveGradient;
layout(set=1,binding=9) uniform sampler2D waveCross;
void wavePlane(vec2 p,out vec2 g,out mat2 h) {
    vec2 uv=p/128.0+vec2(0.5/512.0);
    vec4 a=textureLod(waveGradient,uv,0.0);
    float b=textureLod(waveCross,uv,0.0).r;
    g=a.xy; h=mat2(a.z,b,b,a.w);
}
WaterWave waterWaves(vec3 p,vec3 up) {
    if(planetData.mie.w<0.5) return waterWavesLegacy(p,up);
    vec3 gradient=vec3(0); mat3 hessian=mat3(0);
    vec2 g; mat2 h;
    wavePlane(p.xy,g,h);
    gradient+=vec3(g,0); hessian+=mat3(h[0][0],h[0][1],0,h[1][0],h[1][1],0,0,0,0);
    wavePlane(p.yz+vec2(37,71),g,h);
    gradient+=vec3(0,g); hessian+=mat3(0,0,0,0,h[0][0],h[0][1],0,h[1][0],h[1][1]);
    wavePlane(p.zx+vec2(83,19),g,h);
    gradient+=vec3(g.y,0,g.x); hessian+=mat3(h[1][1],0,h[1][0],0,0,0,h[0][1],0,h[0][0]);
    mat3 tangent=mat3(1)-outerProduct(up,up);
    return WaterWave(tangent*gradient*0.57735,tangent*hessian*tangent*0.57735);
}
vec3 waterNormal(vec3 p,vec3 up,float detail) {
    return normalize(up-waterWaves(p,up).gradient*detail);
}
