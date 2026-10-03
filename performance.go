package main

import (
	"encoding/json"
	glyph "github.com/derekmwright/glyphengine"
	"github.com/derekmwright/glyphengine/renderer"
	"log"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"time"
)

type perfFrame struct {
	wall, update, terrain, rocks, grass float64
	gpu                                 renderer.GPUTimings
}
type perfReport struct {
	RunID                                                             string `json:"run_id"`
	ProfileFrameStep                                                  int    `json:"profile_frame_step"`
	AsyncTerrainUploads                                               bool
	AirPasses                                                         bool
	AirPassesActive                                                   bool
	AirScale                                                          float64
	CausticCache, WaveCache, WaterPasses, AtmosphereCache, ShoreFoam  bool
	UploadsSkipped, TerrainUploadsWaiting                             int
	Event                                                             string `json:"event"`
	UTC                                                               string `json:"utc"`
	Frame                                                             int    `json:"frame"`
	Samples                                                           int    `json:"samples"`
	GPUValidSamples                                                   int    `json:"gpu_valid_samples"`
	Width, Height                                                     int
	VSync                                                             bool
	FixedSimulation                                                   bool
	FPS, FrameMS, FrameP95MS, UpdateMS, TerrainMS, RocksMS, GrassMS   float64
	GPUMS, GPUP95MS                                                   float64
	GPUPasses                                                         map[string]float64
	CPUSessionMeans                                                   map[string]float32
	DrawCalls, Instances, Triangles                                   int
	Seed                                                              uint64
	Radius, SeaLevel, GroundClearance, SeaHeight, Longitude, Latitude float64
	Eye, Forward                                                      [3]float64
	Ocean, Underwater, Atmosphere, Materials, Grass, Shadows, SunRays bool
}
type performance struct {
	runID                            string
	frameStep                        int
	enabled, vsync, fixed, marker    bool
	file                             *os.File
	frames                           [120]perfFrame
	next, count                      int
	previous, lastRefresh, lastWrite time.Time
	report                           perfReport
	terrainMS, rocksMS, grassMS      float64
}

func (p *performance) open(path string) error {
	p.runID = time.Now().UTC().Format(time.RFC3339Nano)
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	p.file = f
	return nil
}
func (p *performance) close() {
	if p.file != nil {
		if err := p.file.Close(); err != nil {
			log.Printf("profile close: %v", err)
		}
	}
}
func milliseconds(start time.Time) float64 {
	return float64(time.Since(start)) / float64(time.Millisecond)
}
func percentile95(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	return values[int(math.Ceil(float64(len(values))*0.95))-1]
}
func (p *performance) sample(g *game, e *glyph.Engine, start time.Time) {
	if !p.previous.IsZero() {
		timing := e.GPUTimings()
		// The renderer reuses the backing array for application timestamps.
		timing.App = slices.Clone(timing.App)
		p.frames[p.next] = perfFrame{float64(start.Sub(p.previous)) / float64(time.Millisecond), milliseconds(start), p.terrainMS, p.rocksMS, p.grassMS, timing}
		p.next = (p.next + 1) % len(p.frames)
		p.count = min(p.count+1, len(p.frames))
	}
	p.previous = start
	now := time.Now()
	if p.count == 0 {
		return
	}
	write := p.file != nil && (p.lastWrite.IsZero() || now.Sub(p.lastWrite) >= time.Second)
	if p.frameStep > 0 {
		write = p.file != nil && p.count == len(p.frames) && e.FrameCount()%p.frameStep == 0
	}
	if now.Sub(p.lastRefresh) < 250*time.Millisecond && !write && !p.marker {
		return
	}
	p.lastRefresh = now
	r := perfReport{RunID: p.runID, ProfileFrameStep: p.frameStep, Event: "sample", UTC: now.UTC().Format(time.RFC3339Nano), Frame: e.FrameCount(), Samples: p.count, VSync: p.vsync, FixedSimulation: p.fixed, GPUPasses: map[string]float64{}, CPUSessionMeans: map[string]float32{}}
	var wall, gpu []float64
	for i := 0; i < p.count; i++ {
		f := p.frames[i]
		r.FrameMS += f.wall
		r.UpdateMS += f.update
		r.TerrainMS += f.terrain
		r.RocksMS += f.rocks
		r.GrassMS += f.grass
		wall = append(wall, f.wall)
		if f.gpu.Valid {
			r.GPUValidSamples++
			r.GPUMS += float64(f.gpu.Total)
			gpu = append(gpu, float64(f.gpu.Total))
			for _, pass := range f.gpu.App {
				r.GPUPasses[pass.Name] += float64(pass.Ms)
			}
			for pass, ms := range f.gpu.Pass {
				r.GPUPasses[renderer.Pass(pass).String()] += float64(ms)
			}
		}
	}
	n := float64(p.count)
	r.FrameMS /= n
	r.UpdateMS /= n
	r.TerrainMS /= n
	r.RocksMS /= n
	r.GrassMS /= n
	if r.FrameMS > 0 {
		r.FPS = 1000 / r.FrameMS
	}
	r.FrameP95MS = percentile95(wall)
	r.GPUP95MS = percentile95(gpu)
	if r.GPUValidSamples > 0 {
		r.GPUMS /= float64(r.GPUValidSamples)
		for name, ms := range r.GPUPasses {
			r.GPUPasses[name] = ms / float64(r.GPUValidSamples)
		}
	}
	cpu := e.CPUTimings()
	if cpu.Valid {
		r.CPUSessionMeans["total"] = cpu.Total
		for phase, ms := range cpu.Phase {
			r.CPUSessionMeans[glyph.CPUPhase(phase).String()] = ms
		}
	}
	r.Width, r.Height = e.Window().GetFramebufferSize()
	stats := e.Renderer().Stats()
	r.AsyncTerrainUploads = g.terrain.asyncUploads
	r.UploadsSkipped = stats.UploadsSkipped
	for _, child := range g.terrain.uploaded {
		if child.ticket != nil && !child.ticket.Ready() {
			r.TerrainUploadsWaiting++
		}
	}
	r.DrawCalls, r.Instances, r.Triangles = stats.DrawCalls, stats.Instances, stats.Triangles
	r.Seed, r.Radius, r.SeaLevel = g.world.Seed, g.world.Radius, g.seaLevel
	r.GroundClearance = g.cam.clearance
	r.SeaHeight = math.Sqrt(g.cam.eye.Dot(g.cam.eye)) - g.world.Radius - g.seaLevel
	d := g.cam.eye.Unit()
	r.Longitude = math.Atan2(d[0], d[2]) * 180 / math.Pi
	r.Latitude = math.Asin(d[1]) * 180 / math.Pi
	r.AirPasses, r.AirScale = g.deferredAir, g.airScale
	r.Eye, r.Forward = [3]float64(g.cam.eye), [3]float64(g.cam.forward)
	r.Ocean, r.Underwater = g.oceanEnabled, g.oceanEnabled && r.SeaHeight < 0
	r.Atmosphere, r.Materials, r.Grass, r.Shadows, r.SunRays = g.atmosphereEnabled, g.materialsEnabled, g.grassEnabled, g.shadowsEnabled, g.sunRaysEnabled
	r.AirPassesActive = r.AirPasses && r.Atmosphere && !r.Underwater && !g.causticsDebug
	r.CausticCache = g.causticCacheEnabled
	r.WaveCache, r.WaterPasses, r.AtmosphereCache, r.ShoreFoam = g.waveTextures, g.waterPasses, g.atmosphereCache, g.shoreFoam
	p.report = r
	if p.marker {
		r.Event = "marker"
		log.Printf("PERF %dx%d | underwater %t | frame %.2f ms p95 %.2f | GPU %.2f ms p95 %.2f | update %.2f ms | opaque %.2f sky %.2f shadow %.2f", r.Width, r.Height, r.Underwater, r.FrameMS, r.FrameP95MS, r.GPUMS, r.GPUP95MS, r.UpdateMS, r.GPUPasses["opaque"], r.GPUPasses["sky"], r.GPUPasses["shadow"])
	}
	if p.file != nil && (write || p.marker) {
		if err := json.NewEncoder(p.file).Encode(r); err != nil {
			log.Printf("profile write: %v", err)
			p.close()
			p.file = nil
		}
		p.lastWrite = now
	}
	p.marker = false
}
func (p *performance) draw(e *glyph.Engine) {
	if !p.enabled {
		return
	}
	r := p.report
	if r.Samples == 0 {
		e.Debugf("PERF: collecting timestamps...")
		return
	}
	e.Debugf("PERF [P hide / F8 mark] %dx%d | wall %.1f FPS / %.2f ms p95 %.2f | vsync %t", r.Width, r.Height, r.FPS, r.FrameMS, r.FrameP95MS, r.VSync)
	if r.GPUValidSamples == 0 {
		e.Debugf("GPU timestamps unavailable / warming up")
	} else {
		e.Debugf("GPU %.2f ms p95 %.2f | opaque %.2f | sky %.2f | shadows %.2f", r.GPUMS, r.GPUP95MS, r.GPUPasses["opaque"], r.GPUPasses["sky"], r.GPUPasses["shadow"])
	}
	if r.GPUPasses["air scattering"] > 0 {
		e.Debugf("GPU air: scatter %.2f | transmission %.2f | composite %.2f | present %.2f ms", r.GPUPasses["air scattering"], r.GPUPasses["air transmission"], r.GPUPasses["air composite"], r.GPUPasses["air present"])
	}
	if r.GPUPasses["wave field"] > 0 {
		e.Debugf("GPU wave cache %.2f | focusing %.2f ms", r.GPUPasses["wave field"], r.GPUPasses["water focusing"]+r.GPUPasses["water focusing resolve"]+r.GPUPasses["water focusing blur"]+r.GPUPasses["water focusing filter"])
	}
	if r.GPUPasses["sun transmission"] > 0 {
		e.Debugf("GPU atmosphere table %.2f ms", r.GPUPasses["sun transmission"])
	}
	if r.GPUPasses["water scattering"] > 0 {
		e.Debugf("GPU water: scatter %.2f | composite %.2f ms", r.GPUPasses["water scattering"], r.GPUPasses["water composite"])
	}
	e.Debugf("CPU update %.2f ms | terrain %.2f | rocks %.2f | grass %.2f", r.UpdateMS, r.TerrainMS, r.RocksMS, r.GrassMS)
	e.Debugf("CPU engine session avg: record %.2f | GPU wait %.2f | present %.2f ms", r.CPUSessionMeans["record"], r.CPUSessionMeans["gpuwait"], r.CPUSessionMeans["present"])
	e.Debugf("Submitted %d draws / %d instances / %d triangles | underwater %t", r.DrawCalls, r.Instances, r.Triangles, r.Underwater)
	if r.AsyncTerrainUploads {
		e.Debugf("Terrain GPU uploads: %d waiting | %d draws skipped", r.TerrainUploadsWaiting, r.UploadsSkipped)
	}
	if p.file != nil {
		if p.frameStep > 0 {
			e.Debugf("Profile: %s (every %d frames, window %d)", filepath.Base(p.file.Name()), p.frameStep, r.Samples)
		} else {
			e.Debugf("Profile: %s (1 Hz, last %d frames)", filepath.Base(p.file.Name()), r.Samples)
		}
	}
}
