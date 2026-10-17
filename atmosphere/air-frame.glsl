// 128 application-owned bytes, packed by AirPasses.Update.
layout(push_constant) uniform Push {
    layout(offset=128) mat4 inverseVP;
    vec4 sunDir; vec4 sunColor;
    vec4 targetInfo; // target scale, projected sun UV, projection valid
    vec4 sunDiscAxes; // projected angular-disc tangent axes, UV units
} pc;
