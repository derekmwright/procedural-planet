A custom atmospheric SkyFrag cannot sample the directional shadow texture even though it can read cascade matrices. This prevents the normal sky pass from applying the same terrain occlusion as custom lit-surface aerial perspective.

Verified against facb8c523fce265b4fd76146f98a4fcc5f7c3ef9.

Evidence:
- renderer/commands.go regular DrawSky binds skyPipeline with pipelineLayout and the cloud descriptor set only.
- renderer/clouds.go populates cloud texture binding 0 and shared environment UBO binding 1.
- The directional comparison sampler exists on the shadow/light descriptor set, binding 1, available to LitFrag at set 1.
- The separate SkyVolumetricFrag path DOES bind the shadow/light set at set 1. However it is an additive local-light pass, gated by LightFlagVolumetric. Substituting it requires a dummy volumetric local light and reorganizing sky composition; it cannot simply subtract blocked sunlight from the atmosphere already drawn by SkyFrag.

Consumer: UniverseBuild integrates spherical atmospheric scattering in paired SkyFrag/LitFrag overrides. Its current visibility term checks the solid planetary sphere, so an individual ridge does not block scattered sunlight. Implementing the missing visibility term is consumer work; accessing the shadow texture in the regular sky pipeline is the engine seam.

Requested capability: make the shadow/light set available to regular custom sky shaders (or expose an explicitly scheduled replacement sky/scattering pass with those resources). Document descriptor bindings, disabled-shadow behavior, and pass ordering. Preserve current default sky appearance and avoid requiring dummy lights.

Acceptance: a custom sky shader samples the same directional map as LitFrag; terrain blocks direct in-scattering in both sky and surface views; no validation errors when shadows are enabled, disabled, resized, or recreated. Large terrain also needs independently configurable cascade coverage; this request concerns resource access, not a built-in planetary atmosphere.
