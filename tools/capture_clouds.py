"""Capture cloud validation views serially from the canonical executable.

Example: python tools/capture_clouds.py --scenes ground orbit --output captures/clouds
The window stays in the background. Images and validation/profile logs go to captures.
"""
import argparse
import os
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parent.parent
SCENES = {
    "ground": ["-longitude=20.0015", "-latitude=12.6048", "-altitude=2", "-flight", "-heading=16.73", "-pitch=12", "-ocean=false"],
    "mountains": ["-longitude=60", "-latitude=12.6", "-altitude=900", "-flight", "-heading=-72", "-pitch=6"],
    "coast": ["-sea-level=137.3", "-altitude=20", "-flight", "-heading=-72", "-pitch=8"],
    "cloud-horizon": ["-longitude=40", "-latitude=0", "-altitude=1500", "-flight", "-heading=-45", "-pitch=10"],
    "inside": ["-longitude=20", "-latitude=12.6", "-altitude=3800", "-flight", "-heading=16.73", "-pitch=0"],
    "above": ["-longitude=20", "-latitude=12.6", "-altitude=12000", "-flight", "-heading=16.73", "-pitch=-20"],
    "orbit": ["-longitude=20", "-latitude=12.6", "-altitude=800000"],
    "night": ["-longitude=140", "-latitude=-12.6", "-altitude=800000"],
    "underwater": ["-sea-level=150.4257", "-altitude=5", "-flight", "-heading=-72", "-pitch=12"],
    "water-window": ["-sea-level=150.4257", "-altitude=24", "-flight", "-heading=-72", "-pitch=65"],
    "water-distance": ["-sea-level=137.3", "-altitude=220", "-flight", "-heading=-72", "-pitch=-28"],
    "water-land": ["-longitude=44", "-latitude=12.6", "-altitude=125", "-flight", "-heading=90", "-pitch=0"],
    "water-mountain": ["-longitude=47.2", "-latitude=12.6", "-sea-level=1000", "-altitude=400", "-flight", "-heading=90", "-pitch=-3"],
    "sun-ridge": ["-longitude=54", "-latitude=0", "-altitude=100", "-flight", "-heading=-63.2", "-pitch=-4.58"],
    "sun-burst": ["-sea-level=137.3", "-altitude=20", "-flight", "-heading=-66.5", "-pitch=34"],
    "sun-high": ["-altitude=8300", "-flight", "-heading=-66.5", "-pitch=34"],
    "cloud-shade": ["-longitude=45", "-latitude=12.6", "-altitude=300", "-flight", "-heading=45", "-pitch=-12"],
}

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--scenes", nargs="+", choices=SCENES, default=["ground", "mountains", "orbit", "inside"])
    parser.add_argument("--output", type=Path, default=ROOT / "captures/clouds")
    parser.add_argument("--width", type=int, default=1280)
    parser.add_argument("--height", type=int, default=720)
    parser.add_argument("--frames", type=int, default=601)
    parser.add_argument("--scale", type=float, default=0.5)
    parser.add_argument("--coverage", type=float, default=0.52)
    parser.add_argument("--tonemap", choices=["filmic", "linear"], default="filmic")
    parser.add_argument("--exposure", type=float, default=1)
    parser.add_argument("--off", action="store_true")
    parser.add_argument("--no-shadows", action="store_true", help="cloud-shadow ablation; keep the visible clouds")
    parser.add_argument("--no-rays", action="store_true", help="disable sun glare, clear-air screen-space shafts, and underwater shafts")
    parser.add_argument("--no-reflections", action="store_true", help="use the inline clear-air water reference")
    parser.add_argument("--no-land-reflections", action="store_true", help="disable nearby terrain/rock water reflections")
    parser.add_argument("--no-wave-filtering", action="store_true", help="use the previous surface normal fade")
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=False)
    for name in args.scenes:
        command = [str(ROOT / "bin/universebuild.exe"), "-seed=7", f"-width={args.width}", f"-height={args.height}",
                   f"-frames={args.frames}", "-sync-terrain", "-vsync=false", "-hud=false", "-validate",
                   f"-clouds={str(not args.off).lower()}", f"-cloud-scale={args.scale}", f"-cloud-coverage={args.coverage}",
                   f"-tonemap={args.tonemap}", f"-exposure={args.exposure}",
                   "-profile-frame-step=120", f"-profile={args.output / (name + '.jsonl')}",
                   f"-screenshot={args.output / (name + '.png')}", *SCENES[name]]
        if args.no_rays:
            command.append("-sun-rays=false")
        if args.no_shadows:
            command.append("-cloud-shadows=false")
        if args.no_reflections:
            command.append("-water-reflections=false")
        if args.no_land_reflections:
            command.append("-water-land-reflections=false")
        if args.no_wave_filtering:
            command.append("-wave-filtering=false")
        log_path = args.output / (name + ".log")
        print(f"Capturing {name}", flush=True)
        with log_path.open("w") as log:
            subprocess.run(command, cwd=ROOT, env=dict(os.environ, GLYPHENGINE_BACKGROUND="1"),
                           stdout=log, stderr=subprocess.STDOUT, check=True, timeout=240,
                           creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0))
        text = log_path.read_text(errors="replace")
        if "VULKAN ERROR" in text or "VULKAN WARNING" in text:
            raise RuntimeError(f"Vulkan diagnostics in {log_path}")
    print(f"Saved to {args.output}")

if __name__ == "__main__":
    main()
