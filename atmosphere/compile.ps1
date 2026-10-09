$ErrorActionPreference = 'Stop'
$compiler = (Get-Command glslc -ErrorAction Stop).Source
foreach ($shader in @('sky.frag', 'terrain.frag', 'water-scatter.frag', 'water-composite.frag', 'sun-table.frag', 'wave-cache.comp', 'caustic-cache.vert', 'caustic-cache.frag', 'caustic-resolve.frag', 'caustic-filter.frag', 'caustic-temporal.frag', 'caustic-history.frag', 'caustic-state.frag', 'air-scatter.frag', 'air-transmission.frag', 'air-composite.frag', 'cloud-volume.comp', 'air-present.frag')) {
    $source = Join-Path $PSScriptRoot $shader
    & $compiler '-O' '--target-env=vulkan1.0' $source '-o' ($source + '.spv')
    if ($LASTEXITCODE -ne 0) { throw "Shader compilation failed: $shader" }
}
