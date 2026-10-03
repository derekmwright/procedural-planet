UniverseBuild needs directional shadows across kilometer-scale terrain while retaining nearby rock shadows. Current engine cascades cover only 15 m and 90 m half-extents, so enabling CastShadows cannot make a distant mountain block sunlight in a valley.

Verified against facb8c523fce265b4fd76146f98a4fcc5f7c3ef9 (clean sibling checkout).

Evidence:
- renderer/shadow.go:26 hardcodes private cascadeRadii = {15, 90}.
- ComputeCascadeVPs calls computeLightVP with those values.
- computeLightVP sets the caster depth extent to radius * 2.5.
- app.go:1509-1516 computes these matrices internally and uses the far volume for shadow-caster culling. A consuming Engine game has no option to override that coverage.
- Built-in lighting returns fully lit outside both cascades.

Consumer reproduction: seeded 500 km planet, local camera-relative rendering, ridges several kilometers away, EnvironmentState.CastShadows=true. Custom LitFrag can sample the existing map successfully: nearby rocks cast shadows in paired deterministic captures, but distant terrain is beyond the maps. This is not a missing consumer sampler binding.

Requested capability: expose directional cascade coverage/caster depth configuration (or caller-provided matrices with consistent caster culling), retaining current defaults. Preserve near detail while allowing a far region large enough for a game-selected terrain scale. No request to hardcode planet behavior or make all games pay for larger coverage.

Acceptance: a distant off-camera ridge within configured caster coverage shadows a receiver; near props retain useful resolution; camera-relative movement remains stable; existing default scenes retain their current behavior. Measure resolution and GPU tradeoffs rather than assuming larger extents alone are sufficient.
