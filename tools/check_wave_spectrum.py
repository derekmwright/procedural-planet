"""Independent finite-difference and seam checks for the cached Fourier field."""
import math, random

def plane(p):
    value=0.; gradient=[0.,0.]; h=[[0.,0.],[0.,0.]]
    for i in range(48):
        angle=i*2.39996323+.37
        freq=(6+43*((i*.618033989+.13)%1)) if i<32 else (49+36*((i*.618033989+.13)%1))
        k=[round(math.cos(angle)*freq)*2*math.pi/128,round(math.sin(angle)*freq)*2*math.pi/128]
        length=math.hypot(*k)
        phase=sum(x*y for x,y in zip(p,k))+(math.sin(i*17.13+3.7)*43758.5453%1)*2*math.pi-round(math.sqrt(9.81*length)*10)*.1*7.5
        a=.023/(max(length,.2)*2) if i<32 else .009/length
        value+=a*math.sin(phase)
        for j in range(2):
            gradient[j]+=a*math.cos(phase)*k[j]
            for l in range(2): h[j][l]-=a*math.sin(phase)*k[j]*k[l]
    return value,gradient,h

def field(p):
    value=0.; g=[0.]*3; h=[[0.]*3 for _ in range(3)]
    for a,b,offset in [(0,1,(0,0)),(1,2,(37,71)),(2,0,(83,19))]:
        v,pg,ph=plane((p[a]+offset[0],p[b]+offset[1]));value+=v
        for j,axis in enumerate((a,b)):
            g[axis]+=pg[j]
            for l,axis2 in enumerate((a,b)): h[axis][axis2]+=ph[j][l]
    return value*.57735,[x*.57735 for x in g],[[x*.57735 for x in row] for row in h]

rng=random.Random(19); worst=0.
for _ in range(100):
    p=[rng.uniform(-200,200) for _ in range(3)];v,g,h=field(p)
    for j in range(3):
        plus=p.copy();minus=p.copy();plus[j]+=1e-4;minus[j]-=1e-4
        vp,gp,_=field(plus);vm,gm,_=field(minus)
        worst=max(worst,abs((vp-vm)/2e-4-g[j]))
        worst=max(worst,max(abs((gp[k]-gm[k])/2e-4-h[k][j]) for k in range(3)))
        wrap=p.copy();wrap[j]+=4096
        vw,gw,_=field(wrap)
        assert abs(v-vw)<1e-9 and max(abs(a-b) for a,b in zip(g,gw))<1e-9
# Repeat sampling wraps the neighbors on either side of the period seam.
for y in (0,13.75,127.75):
    for a,b in ((-.25,127.75),(0,128),( .25,128.25)):
        assert max(abs(x-z) for x,z in zip(plane((a,y))[1],plane((b,y))[1]))<1e-10
assert worst<1e-6,worst
print(f'Cached spectrum derivatives, 3D plane mappings, and periodic boundaries passed; max error {worst:.3g}.')
