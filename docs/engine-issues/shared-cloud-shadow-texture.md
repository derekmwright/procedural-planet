# Additional scene-wide sampled resources for cloud visibility

Filed as [GlyphEngine #191](https://github.com/derekmwright/glyphengine/issues/191).

Procedural Planet needs to share a generated cloud sun-transmission map between
its custom terrain/ocean lighting and atmospheric scattering. The existing
scene-wide application texture bindings are already occupied. Please expose an
opt-in way to bind additional application textures/targets across these shader
consumers, with the same lifetime and frame-graph guarantees as the existing
slots.

Verified against GlyphEngine main `5a0d940cc8f7` on 2026-10-09:

- `renderer.ShaderTextureSlots` is fixed at four, at set 1 bindings 7 through 10.
- `SetShaderTexture` and `SetShaderTarget` reject indices outside that range.
- Procedural Planet uses slot 0 for atmospheric sun transmission, slots 1 and 2
  for the wave field, and slot 3 for the caustic atlas. All four can be required
  together in terrain/ocean shading.
- `AppPassDesc.Reads` can supply pass-local resources but does not extend the
  bindings available to ordinary custom lit draws.

A cloud pass can start with pass-local inputs today. This limitation arises when
adding coherent cloud shadows to the existing surface lighting. Possible
workarounds include repacking unrelated fields into one atlas or taking over
per-object material textures, but those couple independent rendering systems
and their sampling/lifetime requirements.

The minimum consumer requirement is one more shared sampled 2D target for cloud
visibility. A documented extensible resource layout would also serve weather
maps and future atmospheric fields. This does not require bindless textures or
a prescribed descriptor-set number; the engine should choose an API that fits
its shader compatibility policy.

Acceptance:

- Keep the existing four-slot shader contract working; opt-in extensions must
  report their bindings and device limits explicitly.
- A before-scene app pass can write a cloud-visibility target which custom lit
  draws and later app passes read in the same frame, with correct graph ordering
  and barriers. Consumers can pass the same target as a pass-local input where
  appropriate.
- Preserve per-frame descriptor updates, history semantics where applicable,
  resize behavior, and fence-safe resource destruction. Disabling/replacing a
  producer must have documented behavior and must not expose retired resources.
- A test reads distinct data from all four original fields and the additional
  map, then replaces/resizes/destroys it with frames in flight. Vulkan validation
  stays clean; unused extension resources do not add rendering work.

Cloud coverage generation, projection, atmospheric composition, and the choice
of which direct-light terms to attenuate remain application responsibilities.
