"""Run serial, repeatable GPU comparisons with the canonical showcase executable.

Build first: go build -o bin/universebuild.exe .
Example: python tools/profile_scene.py --scenes ground coast --repeats 2
Requires only Python's standard library. Outputs JSONL, logs, and summary.json.
"""

import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import statistics
import subprocess

ROOT = Path(__file__).resolve().parent.parent
SCENES = {
    "seabed-oblique": ["-longitude=20", "-latitude=12.6", "-sea-level=150.4257", "-altitude=5", "-flight", "-heading=-72", "-pitch=-40"],
    "seabed-overhead": ["-longitude=-38.1572", "-latitude=26.8141", "-sea-level=-422.3118", "-altitude=5", "-flight", "-heading=-72", "-pitch=-40"],
    "caustic-oblique": ["-longitude=20", "-latitude=12.6", "-sea-level=150.4257", "-altitude=5", "-flight", "-heading=-72", "-pitch=-12"],
    "caustic-overhead": ["-longitude=-38.1572", "-latitude=26.8141", "-sea-level=-422.3118", "-altitude=5", "-flight", "-heading=-72", "-pitch=-12"],
    "ground": ["-longitude=20.0015", "-latitude=12.6048", "-altitude=2",
               "-flight", "-heading=16.73", "-pitch=-8", "-ocean=false"],
    "mountains": ["-longitude=60", "-latitude=12.6", "-altitude=900",
                  "-flight", "-heading=-72", "-pitch=-8", "-ocean=false"],
    "coast": ["-sea-level=137.3", "-altitude=20", "-flight", "-heading=-72", "-pitch=-2"],
    "underwater": ["-sea-level=137.3", "-altitude=2", "-flight", "-heading=-72", "-pitch=-20"],
    "orbit": ["-longitude=100", "-latitude=12.6", "-altitude=300000"],
}
VARIANTS = {
    "caustic-spatial": ["-caustic-temporal=false"],
    "caustic-temporal": ["-caustic-temporal=true"],
    "prepass-off": ["-depth-prepass=off"],
    "prepass-auto": ["-depth-prepass=auto"],
    "prepass-on": ["-depth-prepass=on"],
    "caustic-forward": ["-caustic-cache=true"],
    "caustic-legacy": ["-caustic-cache=false"],
    "half": ["-air-passes=true", "-air-scale=0.5"],
    "inline": ["-air-passes=false"],
    "full": ["-air-passes=true", "-air-scale=1"],
}


def collect(path, warmup):
    rows = [json.loads(line) for line in path.read_text().splitlines() if line.strip()]
    if len({r["run_id"] for r in rows}) != 1:
        raise RuntimeError(f"Expected one run in {path}")
    # Each report covers the preceding 120 frames. Exclude the entire warmup.
    rows = [r for r in rows if r["event"] == "sample" and r["frame"] >= warmup + 120
            and r["samples"] == 120 and r["gpu_valid_samples"] == 120]
    if not rows:
        raise RuntimeError(f"No complete, settled GPU windows in {path}")
    return rows


def summarize(rows):
    passes = sorted({k for r in rows for k in r["GPUPasses"]})
    return {
        "windows": len(rows),
        "median_window_gpu_ms": statistics.median(r["GPUMS"] for r in rows),
        # This is explicitly NOT a pooled per-frame percentile.
        "median_window_gpu_p95_ms": statistics.median(r["GPUP95MS"] for r in rows),
        "median_window_frame_ms": statistics.median(r["FrameMS"] for r in rows),
        "gpu_pass_ms": {k: statistics.median(r["GPUPasses"].get(k, 0) for r in rows) for k in passes},
        "air_active": sorted({r["AirPassesActive"] for r in rows}),
        "prepass_active_share": sum(r.get("PrepassActiveSamples", 0) for r in rows) / sum(r["samples"] for r in rows),
        "median_prepass_estimate": statistics.median(r.get("PrepassEstimate", 0) for r in rows),
        "pipeline_valid_samples": sum(r.get("PipelineValidSamples", 0) for r in rows),
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--scenes", nargs="+", choices=SCENES, default=["ground", "mountains", "coast", "underwater"])
    parser.add_argument("--variants", nargs="+", choices=VARIANTS, default=["half", "inline"])
    parser.add_argument("--repeats", type=int, default=2)
    parser.add_argument("--width", type=int, default=3840)
    parser.add_argument("--height", type=int, default=2054)
    parser.add_argument("--frames", type=int, default=1201)
    parser.add_argument("--warmup", type=int, default=480)
    parser.add_argument("--validate", action="store_true", help="enable validation; adds CPU overhead")
    parser.add_argument("--screenshots", action="store_true", help="capture the first repetition of each variant")
    parser.add_argument("--pipeline-stats", action="store_true", help="enable fragment/primitive diagnostic queries (leave off for timing baselines)")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    if args.repeats < 1 or args.warmup < 0 or args.frames <= args.warmup + 240:
        parser.error("need repeats >= 1, warmup >= 0 and at least 240 measured frames")
    exe = ROOT / "bin/universebuild.exe"
    if not exe.is_file():
        parser.error("Build first with: go build -o bin/universebuild.exe .")
    output = (args.output or ROOT / "captures" / ("profile-" + datetime.datetime.now().strftime("%Y%m%d-%H%M%S"))).resolve()
    output.mkdir(parents=True, exist_ok=False)
    env = dict(os.environ, GLYPHENGINE_BACKGROUND="1")
    common = [str(exe), f"-width={args.width}", f"-height={args.height}", f"-frames={args.frames}",
              "-sync-terrain", "-vsync=false", "-profile-frame-step=120", "-hud=false"]
    if args.validate:
        common.append("-validate")
    if args.pipeline_stats:
        common.append("-pipeline-stats")
    result = {"executable_sha256": hashlib.sha256(exe.read_bytes()).hexdigest(),
              "settings": {k: str(v) if isinstance(v, Path) else v for k, v in vars(args).items()},
              "runs": [], "comparisons": {}}
    combined = {}
    expected_frames = None
    for scene in args.scenes:
        for repeat in range(args.repeats):
            # Alternate execution order to reduce consistent warm/cold ordering bias.
            variants = args.variants if repeat % 2 == 0 else args.variants[::-1]
            for variant in variants:
                name = f"{scene}-{variant}-{repeat + 1}"
                profile = output / (name + ".jsonl")
                command = common + SCENES[scene] + VARIANTS[variant] + [f"-profile={profile}"]
                if args.screenshots and repeat == 0:
                    command.append(f"-screenshot={output / (name + '.png')}")
                print(f"Running {name}...", flush=True)
                with (output / (name + ".log")).open("w") as log:
                    subprocess.run(command, cwd=ROOT, env=env, stdout=log, stderr=subprocess.STDOUT,
                                   check=True, timeout=300, creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0))
                log_text = (output / (name + ".log")).read_text(errors="replace")
                if "VULKAN ERROR" in log_text or "VULKAN WARNING" in log_text:
                    raise RuntimeError(f"Validation diagnostics in {name}.log")
                rows = collect(profile, args.warmup)
                frame_ids = [r["frame"] for r in rows]
                if expected_frames is None:
                    expected_frames = frame_ids
                if frame_ids != expected_frames:
                    raise RuntimeError(f"Unequal sample windows in {name}: {frame_ids} != {expected_frames}")
                run = dict(name=name, command=command, frames=frame_ids, **summarize(rows))
                result["runs"].append(run)
                combined.setdefault((scene, variant), []).extend(rows)
                print(f"  GPU {run['median_window_gpu_ms']:.2f} ms; {len(rows)} complete windows", flush=True)
                (output / "summary.json").write_text(json.dumps(result, indent=2) + "\n")
        # Geometry/camera parity matters more than a fast timing from a different view.
        reference = combined[(scene, args.variants[0])][0]
        for variant in args.variants:
            rows = combined[(scene, variant)]
            for row in rows:
                # A prepass deliberately resubmits geometry. Check the source
                # scene instead of demanding equal total submission counts.
                keys = ["Width", "Height", "Eye", "Forward", "TerrainLeaves", "TerrainLevel", "TerrainTriangles", "RockCount", "GrassCount"]
                for key in keys:
                    if row[key] != reference[key]:
                        raise RuntimeError(f"Unsettled or mismatched scene {scene}/{variant}: {key}")
                if not any(v.startswith("prepass-") for v in args.variants):
                    # Effect variants can deliberately add full-screen draws.
                    # Keep checking scene submissions after removing app work.
                    for key in ("DrawCalls", "Instances", "Triangles"):
                        actual = row[key] - row["AppWork"][key]
                        expected = reference[key] - reference["AppWork"][key]
                        if actual != expected:
                            raise RuntimeError(f"Unsettled or mismatched scene {scene}/{variant}: scene {key}")
            result["comparisons"][f"{scene}/{variant}"] = summarize(rows)
        (output / "summary.json").write_text(json.dumps(result, indent=2) + "\n")
    print(f"Results: {output / 'summary.json'}", flush=True)


if __name__ == "__main__":
    main()
