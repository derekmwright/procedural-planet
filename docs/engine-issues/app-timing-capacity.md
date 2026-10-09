# Allow more than sixteen named application GPU timings

Procedural Planet's cloud prototype reached the fixed `maxAppTimings = 16`
limit on GlyphEngine `5a0d940cc8f7`. Creating the seventeenth timed application
pass fails with `Timed: at most sixteen timed passes` even when several passes
are mutually exclusive at runtime. This is an instrumentation capacity limit,
not a measured GPU bottleneck.

The current workload registers sixteen timed graphics/compute passes:

- Sun transmission table: 1.
- Wave field: 1.
- Caustic generation, resolve, filtering, history and state: 6.
- Underwater scattering and composition: 2.
- Clear-air scattering, transmission, composition, shared presentation: 4.
- Joint cloud/air integration and cloud composition: 2.

Water and air view passes do not run together. Cloud and clear-air view passes
are also exclusive, but all remain registered for immediate runtime toggles.
Sharing the full-resolution atmospheric output and final presentation reduced
our initial seventeen-pass design to sixteen. Future cloud shadow generation
or temporal reconstruction will need additional named measurements.

Minimal reproduction: register sixteen `Timed: true` application graphics or
compute passes, disable some of them, then register one more. Registration still
fails. `renderer/apppass.go:validateAppTiming` counts registered timed passes;
`renderer/gputimer.go` uses fixed arrays/query capacity. The descriptor-pool
budget in `renderer/texture.go` also references this constant.

Request: expose a bounded configurable timing capacity at renderer creation,
or safely grow the capacity when registering passes. Preserve explicit failure
when measurement is unavailable; silently dropping timings would make profiling
misleading. Please expose the effective capacity if there is still a limit.

Validation should cover mixed graphics/compute timers beyond sixteen, disabled
and re-enabled passes, stable names/results across frames in flight, swapchain
recreation, and useful errors for any requested capacity that cannot be met.

This does not block the first cloud prototype. It prevents future work from
requiring untimed effects or artificial grouping solely to fit query slots.
