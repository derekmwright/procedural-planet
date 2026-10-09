"""Capture the README gallery from the canonical executable, without the HUD.

Build first: go build -o bin/universebuild.exe .
Runs serially in the background. PNGs go to docs/images; validation logs stay
in the ignored captures directory. Python's standard library is sufficient.
"""

import os
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parent.parent
SCENES = {
    "orbit": ["-longitude=20", "-latitude=12.6", "-altitude=800000"],
    "mountains": ["-longitude=60", "-latitude=12.6", "-altitude=900",
                  "-flight", "-heading=-72", "-pitch=-8"],
    "shoreline": ["-sea-level=137.3", "-altitude=20", "-flight", "-heading=-72", "-pitch=-8"],
    "underwater": ["-sea-level=150.4257", "-altitude=5", "-flight", "-heading=-72", "-pitch=12"],
}


def main():
    output = ROOT / "docs/images"
    logs = ROOT / "captures/readme"
    output.mkdir(parents=True, exist_ok=True)
    logs.mkdir(parents=True, exist_ok=True)
    for name, pose in SCENES.items():
        print(f"Capturing {name}...", flush=True)
        args = [str(ROOT / "bin/universebuild.exe"), "-seed=7", "-width=1600", "-height=900",
                "-frames=1201", "-sync-terrain", "-vsync=false", "-hud=false", "-validate", "-clouds=false",
                *pose, f"-screenshot={output / (name + '.png')}"]
        log_path = logs / (name + ".log")
        with log_path.open("w") as log:
            subprocess.run(args, cwd=ROOT, env=dict(os.environ, GLYPHENGINE_BACKGROUND="1"),
                           stdout=log, stderr=subprocess.STDOUT, check=True, timeout=300,
                           creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0))
        text = log_path.read_text(errors="replace")
        if "VULKAN ERROR" in text or "VULKAN WARNING" in text:
            raise RuntimeError(f"Validation diagnostics in {log_path}")
    print(f"Gallery saved to {output}")


if __name__ == "__main__":
    main()
