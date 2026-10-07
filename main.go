package main

import (
	"flag"
	"fmt"
	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/input"
	"github.com/derekmwright/glyphengine/renderer"
	"github.com/derekmwright/procedural-planet/atmosphere"
	"github.com/derekmwright/procedural-planet/planet"
	"github.com/go-gl/mathgl/mgl32"
	"log"
	"math"
	"runtime"
	"time"
)

func init() { runtime.LockOSThread() }

type game struct {
	causticCache                                 *atmosphere.CausticCache
	causticCacheEnabled                          bool
	causticTemporal                              bool
	hud                                          bool
	airPasses                                    *atmosphere.AirPasses
	deferredAir                                  bool
	airScale                                     float64
	shoreFoam                                    bool
	asyncTerrainUploads                          bool
	waveCache                                    *atmosphere.WaveCache
	waveTextures                                 bool
	atmosphereCache                              bool
	causticsDebug                                bool
	waterPasses                                  bool
	waterScatter                                 *atmosphere.WaterPasses
	performance                                  performance
	world                                        planet.Planet
	terrain                                      *terrain
	rocks                                        rockScatter
	grass                                        grassScatter
	grassEnabled                                 bool
	grassDensity                                 float64
	cam                                          camera
	startAltitude, startLongitude, startLatitude float64
	startFlight, auto, syncTerrain, tour         bool
	frameMS, elapsed                             float64
	lastError                                    error
	atmosphereEnabled                            bool
	sunRaysEnabled                               bool
	shadowsEnabled                               bool
	underwaterShadows                            bool
	materialsEnabled                             bool
	oceanEnabled                                 bool
	seaLevel                                     float64
	startHeading                                 float64
	startPitch                                   float64
}
type space struct {
	rays    float32
	shadows bool
}

func (space) Advance(float32) {}
func (s space) State() glyph.EnvironmentState {
	return glyph.EnvironmentState{SunDir: [3]float32{-0.55, 0.45, 0.70}, SunDiscDir: [3]float32{-0.55, 0.45, 0.70}, SunColor: [3]float32{2.4, 2.25, 2.05}, Ambient: [3]float32{0.025, 0.03, 0.04}, ClearColor: [3]float32{0.001, 0.002, 0.004}, DrawSky: true, CastShadows: s.shadows, LightShafts: s.rays, LightShaftShape: glyph.LightShaftShape{Radius: 0.55, Threshold: [2]float32{1, 2}}}
}
func vec(v planet.Vec) mgl32.Vec3 { return mgl32.Vec3{float32(v[0]), float32(v[1]), float32(v[2])} }
func (g *game) Init(e *glyph.Engine) error {
	e.Scene.Env = space{}
	// Keep nearby objects sharp; cover distant ranges and the intervening air.
	// Both cascades need the same caster reach, or the near map can overwrite
	// a valid distant mountain shadow with a falsely lit result.
	if err := e.SetShadowCoverage(renderer.ShadowCoverage{Cascades: [2]renderer.ShadowCascadeCoverage{
		{Radius: 90, TowardLight: 200000, AwayFromLight: 120000},
		{Radius: 120000, TowardLight: 200000, AwayFromLight: 120000},
	}}); err != nil {
		return err
	}
	e.Renderer().SetBloom(0, 1, 0.5, 1)
	g.terrain = newTerrain(g.world, g.syncTerrain)
	g.terrain.asyncUploads = g.asyncTerrainUploads
	if err := g.terrain.init(e); err != nil {
		return err
	}
	if err := g.rocks.init(e); err != nil {
		return err
	}
	if err := g.grass.init(e, g.world, g.syncTerrain, g.grassDensity); err != nil {
		return err
	}
	if g.deferredAir {
		var err error
		g.airPasses, err = atmosphere.NewAirPasses(e.Renderer(), float32(g.airScale))
		if err != nil {
			return err
		}
	}
	if g.waterPasses {
		var err error
		g.waterScatter, err = atmosphere.NewWaterPasses(e.Renderer())
		if err != nil {
			return err
		}
	}
	if g.atmosphereCache {
		if err := atmosphere.NewSunTable(e.Renderer()); err != nil {
			return err
		}
	}
	if g.waveTextures {
		var err error
		g.waveCache, err = atmosphere.NewWaveCache(e.Renderer())
		if err != nil {
			return err
		}
	}
	if g.causticCacheEnabled {
		var err error
		g.causticCache, err = atmosphere.NewCausticCache(e.Renderer())
		if err != nil {
			return err
		}
	}
	g.cam.reset(g.world.Radius)
	g.cam.auto = g.auto
	g.cam.yaw = g.startLongitude * math.Pi / 180
	g.cam.latitude = g.startLatitude * math.Pi / 180
	if g.startAltitude >= 0 {
		g.cam.clearance = g.startAltitude
	}
	g.cam.orbit(g.terrain)
	if g.startFlight {
		g.cam.flight = true
		north, east := northEast(g.cam.eye.Unit())
		heading := g.startHeading * math.Pi / 180
		g.cam.bearing = north.Mul(math.Cos(heading)).Add(east.Mul(math.Sin(heading)))
		g.cam.lookPitch = g.startPitch * math.Pi / 180
		g.cam.flightView()
	}
	g.camera(e)
	if g.lastError != nil {
		return g.lastError
	}
	log.Printf("seed %d, radius %.0f km; adaptive terrain to level %d; Tab toggles flight", g.world.Seed, g.world.Radius/1000, g.terrain.lod.MaxLevel)
	return nil
}
func (g *game) camera(e *glyph.Engine) {
	if g.cam.flight {
		g.cam.constrain(g.terrain)
		g.cam.flightView()
	} else {
		g.cam.orbit(g.terrain)
	}
	g.terrain.renderPositions(e, g.cam.eye)
	rocksStart := time.Now()
	if err := g.rocks.update(e, g.terrain, g.cam.eye, g.cam.clearance); err != nil {
		g.lastError = err
		e.Close()
		return
	}
	g.performance.rocksMS = milliseconds(rocksStart)
	grassStart := time.Now()
	if err := g.grass.update(e, g.terrain, g.cam.eye, g.cam.forward, g.cam.clearance, g.elapsed, g.grassEnabled); err != nil {
		g.lastError = err
		e.Close()
		return
	}
	g.performance.grassMS = milliseconds(grassStart)
	e.SetCamera(mgl32.Vec3{}, vec(g.cam.forward), vec(g.cam.up))
	parameters := atmosphere.FrameParameters(g.world.Radius, [3]float64(g.cam.eye), g.atmosphereEnabled)
	if g.shoreFoam && g.oceanEnabled {
		parameters.Features[2] = 1
	}
	if g.atmosphereCache {
		parameters.Planet[3] = 1
	}
	if g.oceanEnabled {
		parameters.SetOcean(g.world.Radius, g.seaLevel, [3]float64(g.cam.eye))
	}
	if g.causticsDebug {
		parameters.Features[3] = 1
	}
	if g.waveCache != nil {
		active := g.oceanEnabled && math.Abs(math.Sqrt(g.cam.eye.Dot(g.cam.eye))-g.world.Radius-g.seaLevel) < 4000
		g.waveCache.SetEnabled(active)
		if active {
			parameters.Mie[3] = 1
		}
	}
	if g.causticCache != nil {
		if g.causticTemporal {
			parameters.Rendering[2] = 1
		}
		g.causticCache.Update(&parameters, g.cam.eye, g.world.Radius+g.seaLevel, g.oceanEnabled, (space{}).State().SunDir)
	}
	parameters.Mie[2] = float32(math.Mod(g.elapsed, 62.83185307179586))
	if !g.materialsEnabled {
		parameters.Water[1] = 0
	}
	shadowStrength := g.shadowStrength()
	parameters.Features[0] = shadowStrength
	shadows := shadowStrength > 0
	// Explicit feature flags shared by every effect.
	if g.sunRaysEnabled {
		parameters.Features[1] = 1
	}
	if g.waterScatter != nil {
		active := !g.causticsDebug && g.oceanEnabled && g.sunRaysEnabled && g.cam.eye.Dot(g.cam.eye) < math.Pow(g.world.Radius+g.seaLevel, 2)
		g.waterScatter.SetEnabled(active)
		if active {
			parameters.Water[3] = 1
		}
	}
	if g.airPasses != nil {
		active := g.atmosphereEnabled && !(g.oceanEnabled && g.cam.eye.Dot(g.cam.eye) < math.Pow(g.world.Radius+g.seaLevel, 2)) && !g.causticsDebug
		g.airPasses.SetEnabled(active)
		if active {
			parameters.Rendering[0] = 1
		}
	}
	data := parameters.Bytes()
	if err := e.Renderer().SetShaderParameters(data[:]); err != nil {
		panic(fmt.Sprintf("upload planet shader parameters: %v", err))
	}
	var rays float32
	if g.sunRaysEnabled && g.atmosphereEnabled && (!g.oceanEnabled || math.Sqrt(g.cam.eye.Dot(g.cam.eye)) > g.world.Radius+g.seaLevel) {
		// Keep the existing screen-space shafts in space. Account for the
		// horizon dipping below the local tangent as the camera climbs.
		distance := math.Sqrt(g.cam.eye.Dot(g.cam.eye))
		solidRadius := math.Max(g.world.Radius-3000, g.world.Radius*0.95)
		ratio := clamp(solidRadius/distance, 0, 1)
		horizon := -math.Sqrt(1 - ratio*ratio)
		elevation := g.cam.eye.Unit().Dot((planet.Vec{-0.55, 0.45, 0.70}).Unit())
		visibility := clamp((elevation-horizon)/0.02, 0, 1)
		rays = float32(0.18 * visibility * visibility * (3 - 2*visibility))
	}
	e.Scene.Env = space{rays: rays, shadows: shadows}
}
func (g *game) Update(e *glyph.Engine, dt float32) {
	updateStart := time.Now()
	defer func() { g.performance.sample(g, e, updateStart) }()
	in := e.Input()
	if in.KeyPressed(input.KeyP) {
		g.performance.enabled = !g.performance.enabled
	}
	if in.KeyPressed(input.KeyF8) {
		g.performance.marker = true
	}
	if in.KeyPressed(input.KeyEscape) {
		e.Close()
	}
	if in.KeyPressed(input.KeyF) {
		g.atmosphereEnabled = !g.atmosphereEnabled
	}
	if in.KeyPressed(input.KeyR) {
		g.sunRaysEnabled = !g.sunRaysEnabled
	}
	if in.KeyPressed(input.KeyH) {
		g.shadowsEnabled = !g.shadowsEnabled
	}
	if in.KeyPressed(input.KeyM) {
		g.materialsEnabled = !g.materialsEnabled
	}
	if in.KeyPressed(input.KeyO) {
		g.oceanEnabled = !g.oceanEnabled
	}
	if in.KeyPressed(input.KeyG) {
		g.grassEnabled = !g.grassEnabled
	}
	g.elapsed += float64(dt)
	g.cam.update(g.terrain, in, float64(dt))
	if g.tour {
		t := g.elapsed
		phase := clamp(t/24, 0, 1)
		if t > 34 {
			phase = 1 - clamp((t-34)/24, 0, 1)
		}
		g.cam.flight = false
		g.cam.clearance = math.Exp((1-phase)*math.Log(g.world.Radius*1.8) + phase*math.Log(2))
		g.cam.orbit(g.terrain)
	}
	terrainStart := time.Now()
	if err := g.terrain.update(e, g.cam.eye); err != nil {
		g.lastError = err
		log.Printf("terrain: %v", err)
		e.Close()
		return
	}
	g.performance.terrainMS = milliseconds(terrainStart)
	g.camera(e)
	g.frameMS = g.frameMS*0.94 + float64(dt)*1000*0.06
	if !g.hud {
		return
	}
	leaves, level, triangles := g.terrain.stats()
	mode := "ORBIT / DESCENT"
	if g.cam.flight {
		mode = "FLIGHT"
	}
	e.Debugf("PROCEDURAL PLANET / %s", mode)
	e.Debugf("O: ocean %t | Sea level %.0f m | Height above sea %.1f m", g.oceanEnabled, g.seaLevel, math.Sqrt(g.cam.eye.Dot(g.cam.eye))-g.world.Radius-g.seaLevel)
	e.Debugf("Seed %d | Radius %.0f km | Ground clearance %.1f m", g.world.Seed, g.world.Radius/1000, g.cam.clearance)
	e.Debugf("%d patches | LOD %d | %d triangles | sim dt %.1f ms", leaves, level, triangles, g.frameMS)
	e.Debugf("Terrain %.1f ms | last build %.1f ms | GPU meshes %d | splits %d / merges %d", g.terrain.uploadMS, g.terrain.generationMS, g.terrain.allocated, g.terrain.splits, g.terrain.merges)
	e.Debugf("G: grass %t | %d tufts", g.grassEnabled, g.grass.count)
	e.Debugf("Foreground rocks: %d | H: shadows %t (%.0f%% active) | M: materials %t", g.rocks.count, g.shadowsEnabled, g.shadowStrength()*100, g.materialsEnabled)
	if g.cam.flight {
		e.Debugf("W/S: forward/back | A/D: yaw | Wheel: zoom | Drag: look | Q/E: down/up")
	} else {
		e.Debugf("Wheel: zoom | W/S: forward/back | A/D: yaw | Drag/arrows: orbit | Space: auto")
	}
	air := "on"
	if !g.atmosphereEnabled {
		air = "off"
	}
	rays := "off"
	if g.sunRaysEnabled {
		rays = "on"
	}
	e.Debugf("Shift: faster | Tab: camera | F: atmosphere %s | R: sun rays %s | Home: reset | Esc: quit", air, rays)
	g.performance.draw(e)
}
func (g *game) LateUpdate(e *glyph.Engine, _ float32) {
	if g.airPasses != nil {
		state := (space{}).State()
		if err := g.airPasses.Update(e.ViewProjection().Inv(), state.SunDir, state.SunColor); err != nil {
			g.lastError = err
			e.Close()
		}
	}
	if g.waterScatter != nil {
		state := (space{}).State()
		if err := g.waterScatter.Update(e.ViewProjection().Inv(), state.SunDir, state.SunColor); err != nil {
			g.lastError = err
			e.Close()
		}
	}
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	width := flag.Int("width", 1440, "initial window width")
	height := flag.Int("height", 900, "initial window height")
	seed := flag.Uint64("seed", 7, "deterministic planet seed")
	radius := flag.Float64("radius-km", 500, "planet radius in km (100 to 10000)")
	frames := flag.Int("frames", 0, "exit after N frames")
	shot := flag.String("screenshot", "", "save final frame as PNG")
	validate := flag.Bool("validate", false, "enable Vulkan validation")
	auto := flag.Bool("auto-orbit", false, "start with automatic orbit")
	altitude := flag.Float64("altitude", -1, "starting ground clearance in meters (-1 = orbit)")
	longitude := flag.Float64("longitude", 20, "starting longitude in degrees")
	latitude := flag.Float64("latitude", 12.6, "starting latitude in degrees (-88 to 88)")
	flight := flag.Bool("flight", false, "start in free flight facing the horizon")
	syncTerrain := flag.Bool("sync-terrain", false, "build terrain synchronously for repeatable validation runs")
	tour := flag.Bool("tour", false, "58-second scripted descent, surface hold, and ascent")
	perf := flag.Bool("perf", true, "show rolling CPU/GPU performance HUD (P toggles)")
	hud := flag.Bool("hud", true, "show debug text; disable for clean captures and geometry statistics")
	profile := flag.String("profile", "", "append rolling 120-frame performance reports to a JSONL file")
	profileFrameStep := flag.Int("profile-frame-step", 0, "record every N rendered frames (0: once per second; 120: non-overlapping windows)")
	vsync := flag.Bool("vsync", true, "enable display frame pacing")
	prepass := flag.String("depth-prepass", "auto", "opaque depth prepass: off, auto, or on (comparison only)")
	pipelineStats := flag.Bool("pipeline-stats", false, "record per-pass GPU fragment and clipping counters")
	air := flag.Bool("atmosphere", true, "enable spherical atmosphere and terrain haze")
	vegetation := flag.Bool("vegetation", true, "enable colored vegetation regions")
	grass := flag.Bool("grass", true, "enable nearby grass blades (G toggles)")
	grassDensity := flag.Float64("grass-density", 1, "grass density multiplier (0.1 to 2)")
	grassHeight := flag.Float64("grass-height-max", 2600, "maximum vegetation height above sea level in meters")
	grassSlope := flag.Float64("grass-slope-max", 38, "maximum grass slope in degrees (5 to 60)")
	ocean := flag.Bool("ocean", true, "enable spherical sea-level water (O toggles)")
	seaLevel := flag.Float64("sea-level", 250, "sea level in meters above planet reference radius")
	erosion := flag.Bool("erosion", false, "enable experimental drainage-guided valley and canyon incision")
	erosionStrength := flag.Float64("erosion-strength", 1, "incision strength, 0..2 (requires -erosion)")
	erosionDemo := flag.Bool("erosion-demo", false, "enable erosion and start above a nearby canyon")
	materials := flag.Bool("materials", true, "enable foreground surface detail (M toggles)")
	shadows := flag.Bool("shadows", true, "enable mountain, terrain and rock shadows (H toggles)")
	underwaterShadows := flag.Bool("underwater-shadows", false, "retain cast shadows underwater (default fades them out over the first 2 m)")
	rays := flag.Bool("sun-rays", true, "enable atmospheric screen-space sun shafts")
	heading := flag.Float64("heading", 0, "starting flight heading in degrees east of north")
	pitch := flag.Float64("pitch", -0.08*180/math.Pi, "starting flight pitch in degrees above horizon (-85 to 85)")
	waterPasses := flag.Bool("water-passes", true, "half-resolution underwater scattering (false uses full-resolution baseline)")
	causticsDebug := flag.Bool("caustics-debug", false, "show unattenuated seabed focusing: neutral light is gray, concentrated light is white")
	atmosphereCache := flag.Bool("atmosphere-cache", true, "cache spherical sun-path transmission in a GPU lookup table")
	causticCache := flag.Bool("caustic-cache", true, "forward-project and sum refracted sunlight (false: legacy inverse solver)")
	causticTemporal := flag.Bool("caustic-temporal", true, "stabilize caustic brightness over time (false: spatial filtering only)")
	waveTextures := flag.Bool("wave-cache", true, "use GPU-cached broad wave spectrum (false compares previous five-wave field)")
	asyncTerrainUploads := flag.Bool("async-terrain-uploads", true, "stream terrain into GPU-local buffers (false compares pooled dynamic buffers)")
	shoreFoam := flag.Bool("shore-foam", true, "enable animated shallow-water foam and wet shoreline sand")
	deferredAir := flag.Bool("air-passes", true, "separate view atmosphere with depth-aware reconstruction (false: inline reference)")
	airScale := flag.Float64("air-scale", 0.5, "atmosphere resolution scale (0.5 or 1 for quality comparisons)")
	flag.Parse()
	if math.IsNaN(*erosionStrength) || math.IsInf(*erosionStrength, 0) || *erosionStrength < 0 || *erosionStrength > 2 {
		return fmt.Errorf("erosion-strength must be between 0 and 2")
	}
	prepassMode := renderer.DepthPrepassOff
	switch *prepass {
	case "off":
	case "auto":
		prepassMode = renderer.DepthPrepassAuto
	case "on":
		prepassMode = renderer.DepthPrepassOn
	default:
		return fmt.Errorf("depth-prepass must be off, auto, or on")
	}
	if *profileFrameStep < 0 {
		return fmt.Errorf("profile-frame-step must be nonnegative")
	}
	if *airScale != 0.5 && *airScale != 1 {
		return fmt.Errorf("air-scale must be 0.5 or 1")
	}
	if *width < 320 || *width > 7680 || *height < 240 || *height > 4320 {
		return fmt.Errorf("window size must be between 320x240 and 7680x4320")
	}
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	if !finite(*grassDensity) || *grassDensity < 0.1 || *grassDensity > 2 {
		return fmt.Errorf("grass-density must be between 0.1 and 2")
	}
	if !finite(*grassHeight) || *grassHeight < 100 || *grassHeight > 10000 {
		return fmt.Errorf("grass-height-max must be between 100 and 10000")
	}
	if !finite(*grassSlope) || *grassSlope < 5 || *grassSlope > 60 {
		return fmt.Errorf("grass-slope-max must be between 5 and 60")
	}
	if !finite(*seaLevel) || *seaLevel < -2500 || *seaLevel > 10000 {
		return fmt.Errorf("sea-level must be between -2500 and 10000 meters")
	}
	if !finite(*radius) || *radius < 100 || *radius > 10000 {
		return fmt.Errorf("radius-km must be between 100 and 10000")
	}
	if !finite(*altitude) || (*altitude != -1 && (*altitude < 2 || *altitude > *radius*1000*8)) {
		return fmt.Errorf("altitude must be -1 or between 2 meters and 8 planet radii")
	}
	if !finite(*pitch) || math.Abs(*pitch) > 85 {
		return fmt.Errorf("pitch must be between -85 and 85 degrees")
	}
	if !finite(*heading) {
		return fmt.Errorf("heading must be finite")
	}
	if !finite(*longitude) || !finite(*latitude) || math.Abs(*latitude) > 88 {
		return fmt.Errorf("longitude must be finite; latitude must be between -88 and 88")
	}
	opts := []glyph.Option{glyph.WithTitle("Procedural Planet - Orbit to Surface"), glyph.WithWindowSize(*width, *height), glyph.WithMSAA(4), glyph.WithProjection(60, 0.1, float32(*radius*1000*20)), glyph.WithValidation(*validate), glyph.WithInterpolation(false), glyph.WithVSync(*vsync)}
	opts = append(opts, glyph.WithShaders(atmosphere.Shaders()))
	opts = append(opts, glyph.WithDepthPrepass(prepassMode))
	if *pipelineStats {
		opts = append(opts, glyph.WithPipelineStatistics())
	}
	if *frames > 0 {
		opts = append(opts, glyph.WithMaxFrames(*frames), glyph.WithFixedFrameTime(time.Second/60))
	}
	if *shot != "" {
		opts = append(opts, glyph.WithScreenshot(*shot))
	}
	g := &game{world: planet.Planet{Seed: *seed, Radius: *radius * 1000}, startAltitude: *altitude, startLongitude: *longitude, startLatitude: *latitude, startFlight: *flight, auto: *auto, syncTerrain: *syncTerrain, tour: *tour}
	if *vegetation {
		g.world.Vegetation = &planet.VegetationSettings{SeaLevel: *seaLevel, MaxHeight: *grassHeight, MaxSlope: *grassSlope}
	}
	g.performance.enabled, g.performance.vsync, g.performance.fixed = *perf, *vsync, *frames > 0
	g.hud = *hud
	g.performance.frameStep = *profileFrameStep
	if err := g.performance.open(*profile); err != nil {
		return fmt.Errorf("profile: %w", err)
	}
	defer g.performance.close()
	g.grassEnabled = *grass
	g.grassDensity = *grassDensity
	g.atmosphereEnabled = *air
	g.sunRaysEnabled = *rays
	g.shadowsEnabled = *shadows
	g.underwaterShadows = *underwaterShadows
	g.materialsEnabled = *materials
	g.oceanEnabled = *ocean
	g.seaLevel = *seaLevel
	g.startHeading = *heading
	g.startPitch = *pitch
	if *erosion || *erosionDemo {
		start := time.Now()
		log.Print("preparing terrain erosion (the default solve takes about ten seconds)...")
		world, err := g.world.WithErosion(planet.ErosionSettings{SeaLevel: *seaLevel, Strength: *erosionStrength})
		if err != nil {
			return err
		}
		g.world = world
		stats := world.ErosionStats()
		log.Printf("erosion: %d drainage nodes, %d channel samples, %d iterations, %.1f MiB cache; built in %s", stats.Nodes, stats.DrainageSamples, stats.Iterations, float64(stats.CacheBytes)/(1024*1024), time.Since(start).Round(time.Millisecond))
		if *erosionDemo {
			lon, lat := *longitude*math.Pi/180, *latitude*math.Pi/180
			near := planet.Vec{math.Sin(lon) * math.Cos(lat), math.Sin(lat), math.Cos(lon) * math.Cos(lat)}
			d, downstream, ok := world.ErosionPreview(near)
			if !ok {
				return fmt.Errorf("no erosion channel near this region; choose a different longitude/latitude or use -erosion without -erosion-demo")
			}
			north, east := northEast(d)
			g.startLongitude, g.startLatitude = math.Atan2(d[0], d[2])*180/math.Pi, math.Asin(d[1])*180/math.Pi
			g.startFlight, g.startAltitude, g.startPitch = true, 1600, -24
			g.startHeading = math.Atan2(downstream.Dot(east), downstream.Dot(north)) * 180 / math.Pi
			base := planet.Planet{Seed: *seed, Radius: g.world.Radius}
			log.Printf("erosion view: longitude %.6f latitude %.6f heading %.3f; incision %.0f m", g.startLongitude, g.startLatitude, g.startHeading, base.Elevation(d)-world.Elevation(d))
		}
	}
	g.waterPasses = *waterPasses
	g.causticsDebug = *causticsDebug
	g.atmosphereCache = *atmosphereCache
	g.waveTextures = *waveTextures
	g.causticCacheEnabled = *causticCache
	g.causticTemporal = *causticTemporal
	g.shoreFoam = *shoreFoam
	g.deferredAir, g.airScale = *deferredAir, *airScale
	g.asyncTerrainUploads = *asyncTerrainUploads
	e, err := glyph.New(g, opts...)
	if g.terrain != nil {
		defer g.terrain.close()
		defer g.grass.close()
	}
	if err != nil {
		return err
	}
	defer e.Destroy()
	e.Run()
	leaves, level, _ := g.terrain.stats()
	log.Printf("final: %d patches, level %d, %d splits / %d merges, %d GPU meshes, clearance %.2f m", leaves, level, g.terrain.splits, g.terrain.merges, g.terrain.allocated, g.cam.clearance)
	return g.lastError
}
