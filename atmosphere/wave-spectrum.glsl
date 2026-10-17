// 48 deterministic Fourier components per plane: unequal wavelengths, broad
// directions, random phases. Integer frequencies preserve exact tile continuity.
// These are shading ripples, not a hydrodynamic ocean simulation.
void waveSpectrumFiltered(vec2 p,float footprint,out vec2 gradient,out vec3 hessian,out vec3 variance) {
    gradient=vec2(0); hessian=vec3(0); variance=vec3(0);
    for(int i=0;i<48;i++) {
        float f=float(i);
        float angle=f*2.39996323+0.37;
        float frequency=6.0+43.0*fract(f*0.618033989+0.13);
        if(i>=32) frequency=49.0+36.0*fract(f*0.618033989+0.13);
        vec2 k=round(vec2(cos(angle),sin(angle))*frequency)*(6.28318530718/128.0);
        float lengthK=length(k);
        float phase=dot(p,k)+fract(sin(f*17.13+3.7)*43758.5453)*6.28318530718;
        float speed=round(sqrt(9.81*lengthK)*10.0)*0.1;
        phase-=planetData.mie.z*speed;
        float amplitude=0.023/(max(lengthK,0.2)*sqrt(32.0/8.0));
        // Shorter ripples add curvature without increasing large-wave height.
        // Their normals also drive visible reflection/refraction, not just caustics.
        if(i>=32) amplitude=0.009/lengthK;
        // Gaussian pixel footprint. The unresolved part becomes slope
        // covariance, rather than disappearing into a perfectly smooth sea.
        // Each independent Fourier component has mean-square amplitude a²/2.
        float weight=exp(-0.125*lengthK*lengthK*footprint*footprint);
        variance+=0.5*amplitude*amplitude*(1.0-weight*weight)*vec3(k.x*k.x,k.y*k.y,k.x*k.y);
        amplitude*=weight;
        gradient+=amplitude*cos(phase)*k;
        hessian-=amplitude*sin(phase)*vec3(k.x*k.x,k.y*k.y,k.x*k.y);
    }
}
void waveSpectrum(vec2 p,out vec2 gradient,out vec3 hessian) {
    vec3 variance;
    waveSpectrumFiltered(p,0.0,gradient,hessian,variance);
}
