#!/usr/bin/env python3
"""Measure local CLI --version process launches with warm OS caches.

This is deliberately not a cold-start or model-quality benchmark. Candidates
are interleaved to reduce ordering bias. Python's subprocess overhead is
included in every sample. No provider credentials or model calls are used.
"""

import argparse
import datetime
import json
import math
import platform
import statistics
import subprocess
import time
from pathlib import Path


def assignment(value):
    name, separator, path = value.partition("=")
    if not separator or not name or not path:
        raise argparse.ArgumentTypeError("expected NAME=PATH")
    return name, Path(path).resolve()


def tree_size(path):
    if path.is_file():
        return path.stat().st_size
    return sum(item.stat().st_size for item in path.rglob("*") if item.is_file())


def run_version(executable):
    result = subprocess.run(
        [str(executable), "--version"], stdin=subprocess.DEVNULL,
        capture_output=True, timeout=20,
    )
    if result.returncode != 0:
        raise RuntimeError(f"{executable} --version exited {result.returncode}: {result.stderr!r}")
    return result.stdout.decode("utf-8", "replace").strip()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--candidate", action="append", type=assignment, required=True,
                        metavar="NAME=EXECUTABLE", help="repeat for each CLI")
    parser.add_argument("--footprint", action="append", type=assignment, default=[],
                        metavar="NAME=PATH", help="file or install tree to measure, excluding shared runtimes")
    parser.add_argument("--runs", type=int, default=100, help="measured launches per candidate")
    parser.add_argument("--warmups", type=int, default=5, help="unmeasured launches per candidate")
    parser.add_argument("--output", type=Path, help="also write the JSON report here")
    args = parser.parse_args()
    if args.runs < 1 or args.warmups < 0:
        parser.error("--runs must be positive and --warmups cannot be negative")
    footprints = dict(args.footprint)
    candidates = []
    for name, executable in args.candidate:
        if not executable.is_file():
            parser.error(f"candidate not found: {executable}")
        if name in footprints and not footprints[name].exists():
            parser.error(f"footprint not found: {footprints[name]}")
        candidates.append({
            "name": name,
            "executable": str(executable),
            "version": run_version(executable),
            "executable_bytes": executable.stat().st_size,
            "footprint_bytes": tree_size(footprints.get(name, executable)),
            "footprint_path": str(footprints.get(name, executable)),
        })

    samples = {candidate["name"]: [] for candidate in candidates}
    for candidate in candidates:
        for _ in range(args.warmups):
            subprocess.run([candidate["executable"], "--version"], stdin=subprocess.DEVNULL,
                           stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                           check=True, timeout=20)
    for iteration in range(args.runs):
        # Rotate rather than always giving the first candidate the coolest run.
        ordered = candidates[iteration % len(candidates):] + candidates[:iteration % len(candidates)]
        for candidate in ordered:
            start = time.perf_counter_ns()
            subprocess.run([candidate["executable"], "--version"], stdin=subprocess.DEVNULL,
                           stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                           check=True, timeout=20)
            elapsed_ms = (time.perf_counter_ns() - start) / 1_000_000
            samples[candidate["name"]].append(elapsed_ms)

    results = []
    for candidate in candidates:
        values = sorted(samples[candidate["name"]])
        results.append({
            **candidate,
            "runs": len(values),
            "warmups": args.warmups,
            "min_ms": round(values[0], 3),
            "median_ms": round(statistics.median(values), 3),
            "p95_ms": round(values[math.ceil(0.95 * len(values)) - 1], 3),
            "max_ms": round(values[-1], 3),
        })
    report = {
        "suite": "uncut-startup-v1",
        "measured_at_utc": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "method": "wall time of --version subprocess; warm OS caches; interleaved candidates",
        "platform": platform.platform(),
        "machine": platform.machine(),
        "processor": platform.processor(),
        "logical_cpus": __import__("os").cpu_count(),
        "python": platform.python_version(),
        "results": results,
    }
    rendered = json.dumps(report, indent=2, ensure_ascii=False) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(rendered, encoding="utf-8")
    print(rendered, end="")


if __name__ == "__main__":
    main()
