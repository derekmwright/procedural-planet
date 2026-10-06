# Extend optional pipeline counters to named application graphics passes

Filed as [GlyphEngine #187](https://github.com/derekmwright/glyphengine/issues/187).

Procedural Planet uses application passes for wave-driven caustic projection,
filtering, underwater scattering, and atmospheric composition. Caustic
projection alone submits 2,064,386 triangles per active frame.

Engine #167 now counts these submissions and #183 adds fragment-invocation and
post-clip primitive queries to engine timing brackets. At main `5a0d940`,
`PipelineStats` contains only `[passCount]` engine arrays, whereas application
timings and `RenderStats.App.Passes` identify each custom pass by name. We cannot
tell how much caustic projection work survives clipping, or compare fragment
counts against its timer, through the new public counters.

Please extend opt-in statistics to named application graphics passes whose
rendering scope can legally hold a query. Report validity and coverage explicitly;
compute work and unbracketable scopes must not appear as measured zero. Preserve
the fence-delayed nonblocking readback and zero-query disabled path.

Acceptance: two differently sized application draws produce independently named
counts next to their existing timers; disabled passes, resize and graph rebuild
cannot return stale counts. Validation must cover the query nesting/rendering
scope rules, and users must be able to retain/copy the result safely.

This does not request a quad-overshading ratio: the RX 7900 XTX calibration in
#183 found that helper lanes are not counted. Raw counters, their measured frame
count, and the existing documented limitation are the useful contract.
