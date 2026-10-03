"""Numerical checks for waterlight.glsl's differential-area derivation.
Run with Python's standard library. This checks the mathematics, not GPU execution.
"""
import math
import random

def add(a,b): return tuple(x+y for x,y in zip(a,b))
def mul(a,s): return tuple(x*s for x in a)
def dot(a,b): return sum(x*y for x,y in zip(a,b))
def norm(a): return mul(a,1/math.sqrt(dot(a,a)))
def cross(a,b): return (a[1]*b[2]-a[2]*b[1],a[2]*b[0]-a[0]*b[2],a[0]*b[1]-a[1]*b[0])

def waves(p,up,scale=1):
    gradient=(0,0,0); h=[[0.0]*3 for _ in range(3)]
    lattice=2*math.pi/4096
    terms=[((811,213,-397),(173,97,-139),.035,.7,2.1),
           ((-307,593,677),(-131,157,83),.027,-.5,1.8),
           ((419,-887,251),(113,-179,149),.020,.3,2.3),
           ((1073,-293,-691),(-197,101,157),.012,-.9,1.7),
           ((-719,-547,1031),(139,211,-107),.009,1.1,2.5)]
    for k,m,a,speed,bend in terms:
        k=mul(k,lattice); m=mul(m,lattice)
        modulation=dot(p,m)+7.5*.1
        phase=dot(p,k)+7.5*speed+bend*math.sin(modulation)
        pg=add(k,mul(m,bend*math.cos(modulation)))
        tangent=add(pg,mul(up,-dot(up,pg)))
        mt=add(m,mul(up,-dot(up,m)))
        envelope=.7+.3*math.cos(modulation)
        eg=mul(mt,-.3*math.sin(modulation))
        sn,cs=math.sin(phase),math.cos(phase)
        gradient=add(gradient,mul(add(mul(tangent,envelope*cs),mul(eg,sn)),scale*a))
        for i in range(3):
            for j in range(3):
                eh=-.3*math.cos(modulation)*mt[i]*mt[j]
                ph=-bend*math.sin(modulation)*mt[i]*mt[j]
                h[i][j]+=scale*a*(-envelope*sn*tangent[i]*tangent[j]
                    +cs*(eg[i]*tangent[j]+tangent[i]*eg[j])+sn*eh+envelope*cs*ph)
    return gradient,h

def refract(i,n):
    eta=1/1.333; c=dot(n,i)
    return add(mul(i,eta),mul(n,-eta*c-math.sqrt(1-eta*eta*(1-c*c))))

def project(p,up,u,v,incoming,depth,scale=1):
    gradient,h=waves(p,up,scale)
    raw=add(up,mul(gradient,-1)); n=norm(raw); nlen=math.sqrt(dot(raw,raw))
    ray=refract(incoming,n); down=dot(ray,up); travel=-depth/down
    offset=(dot(ray,u)*travel,dot(ray,v)*travel)
    columns=[]
    for axis in (u,v):
        hx=tuple(dot(row,axis) for row in h)
        dn=mul(add(mul(hx,-1),mul(n,dot(n,hx))),1/nlen)
        eta=1/1.333; c=dot(n,incoming); root=math.sqrt(1-eta*eta*(1-c*c))
        dr=add(mul(n,-(eta+eta*eta*c/root)*dot(dn,incoming)),mul(dn,-eta*c-root))
        dproject=mul(add(dr,mul(ray,-dot(dr,up)/down)),travel)
        columns.append((dot(dproject,u),dot(dproject,v)))
    return offset,columns

rng=random.Random(7); maximum=0
for _ in range(200):
    up=norm(tuple(rng.uniform(-1,1) for _ in range(3)))
    axis=(0,1,0) if abs(up[1])<.95 else (1,0,0)
    u=norm(cross(axis,up)); v=cross(up,u)
    incoming=norm(add(mul(up,-1),mul(u,.4)))
    p=tuple(rng.uniform(-100,100) for _ in range(3)); depth=rng.uniform(.1,20)
    offset,columns=project(p,up,u,v,incoming,depth)
    for col,basis in zip(columns,(u,v)):
        eps=1e-4
        plus=project(add(p,mul(basis,eps)),up,u,v,incoming,depth)[0]
        minus=project(add(p,mul(basis,-eps)),up,u,v,incoming,depth)[0]
        maximum=max(maximum,max(abs(col[i]-(plus[i]-minus[i])/(2*eps)) for i in range(2)))
    # Flat water must neither focus nor defocus: identity mapping Jacobian.
    _,flat=project(p,up,u,v,incoming,depth,0)
    assert max(abs(x) for column in flat for x in column)<1e-12
    # Periodic eye rebasing must preserve wave shape.
    g,_=waves(p,up)
    for axis in range(3):
        wrapped=list(p); wrapped[axis]+=4096
        gw,_=waves(wrapped,up)
        assert max(abs(a-b) for a,b in zip(g,gw))<1e-10
assert maximum<1e-5, maximum
print(f'200 rotated receiver cases: analytic derivatives agree with finite differences; max error {maximum:.3g}.')
print('Flat-water focusing and 4096 m origin-wrap checks passed.')
