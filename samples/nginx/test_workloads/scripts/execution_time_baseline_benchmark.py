#!/usr/bin/env python3
"""
Measure Karmada default scheduler latency (first Begin → last End per ResourceBinding).

Flow per run:
1) Apply PropagationPolicy (from baseline directory)
2) Apply Deployment
3) Measure latency from first "Begin scheduling resource binding" to last "End ..."
4) Cleanup Deployment/PP

Outputs:
- scripts/results_baseline/<deployment>.csv with per-run rows (run, begin_ts, end_ts, latency_seconds)
- scripts/results_baseline/summary.csv with median per deployment

Notes:
- No AHP or weights updater services are started.
- Results directory is overwritten by default on each execution.
"""

import argparse
import csv
import os
import re
import shutil
import subprocess
import sys
import time
from pathlib import Path
from statistics import median
from typing import List, Optional, Tuple

# Contexts/namespaces
KARMADA_CTX_DEFAULT = "karmada-apiserver"
SCHED_CTX_DEFAULT = "kind-host"
SCHED_NS_DEFAULT = "karmada-system"

NS_DEFAULT = "default"
RUNS_DEFAULT = 10
TIMEOUT_SECONDS_DEFAULT = 120
SETTLE_SECONDS_DEFAULT = 2.0

TIME_RE = re.compile(r'^[IWEF]\d{4}\s+(\d{2}):(\d{2}):(\d{2}\.\d+)')
LOG_BEGIN_RE = re.compile(r'Begin scheduling resource binding')
LOG_END_RE = re.compile(r'End scheduling resource binding')

def sh(cmd: List[str], capture: bool = False, check: bool = True) -> str:
    if capture:
        return subprocess.check_output(cmd, text=True).strip()
    subprocess.run(cmd, check=check)
    return ""

def apply_yaml(ctx: str, path: Path) -> None:
    sh(["kubectl", "--context", ctx, "apply", "-f", str(path)])

def delete_yaml(ctx: str, path: Path) -> None:
    subprocess.run(["kubectl", "--context", ctx, "delete", "-f", str(path), "--ignore-not-found"], check=False)

def delete_rb(ctx: str, ns: str, rb_name: str) -> None:
    subprocess.run(["kubectl", "--context", ctx, "-n", ns, "delete", "resourcebinding", rb_name, "--ignore-not-found"], check=False)

def scale_scheduler(ctx: str, ns: str, deploy: str) -> None:
    sh(["kubectl", "--context", ctx, "-n", ns, "scale", "deployment", deploy, "--replicas=0"])
    time.sleep(2)
    sh(["kubectl", "--context", ctx, "-n", ns, "scale", "deployment", deploy, "--replicas=1"])
    sh(["kubectl", "--context", ctx, "-n", ns, "rollout", "status", "deployment", deploy])

def get_scheduler_pod_by_label(ctx: str, ns: str, label: str) -> str:
    pods = sh(["kubectl", "--context", ctx, "-n", ns, "get", "pods", "-l", label, "-o", "jsonpath={.items[*].metadata.name}"],
              capture=True, check=False)
    for name in pods.split():
        phase = sh(["kubectl", "--context", ctx, "-n", ns, "get", "pod", name, "-o", "jsonpath={.status.phase}"], capture=True, check=False)
        if phase == "Running":
            return name
    return ""

def parse_ts(line: str) -> Tuple[Optional[str], Optional[float]]:
    m = TIME_RE.match(line)
    if not m:
        return None, None
    hh, mm, ss = m.groups()
    ts_str = f"{hh}:{mm}:{ss}"
    ts_sec = int(hh) * 3600 + int(mm) * 60 + float(ss)
    return ts_str, ts_sec

def measure_latency(
    ns: str,
    dep_name: str,
    sched_ctx: str,
    sched_ns: str,
    sched_pod: str,
    timeout_s: int,
    settle_seconds: float,
) -> Optional[Tuple[float, str, str]]:
    rb_fq = f"{ns}/{dep_name}-deployment"
    deadline = time.time() + timeout_s
    first_begin_ts = None
    first_begin_ts_str = None
    last_end_ts = None
    last_end_ts_str = None
    last_end_seen = 0
    last_change = time.time()

    while time.time() < deadline:
        logs = sh(["kubectl", "--context", sched_ctx, "-n", sched_ns, "logs", sched_pod, "--since=15m"], capture=True, check=False)
        end_count_this_scan = 0

        for line in logs.splitlines():
            if rb_fq in line and LOG_BEGIN_RE.search(line):
                ts_str, ts = parse_ts(line)
                if ts is not None and (first_begin_ts is None or ts < first_begin_ts):
                    first_begin_ts, first_begin_ts_str = ts, ts_str
            if rb_fq in line and LOG_END_RE.search(line):
                ts_str, ts = parse_ts(line)
                if ts is not None:
                    end_count_this_scan += 1
                    if (last_end_ts is None) or (ts > last_end_ts):
                        last_end_ts, last_end_ts_str = ts, ts_str

        if end_count_this_scan > last_end_seen:
            last_end_seen = end_count_this_scan
            last_change = time.time()

        if (
            first_begin_ts is not None
            and last_end_ts is not None
            and last_end_ts >= first_begin_ts
            and (time.time() - last_change) >= settle_seconds
        ):
            return round(last_end_ts - first_begin_ts, 6), first_begin_ts_str, last_end_ts_str

        time.sleep(0.5)

    return None

def main() -> None:
    parser = argparse.ArgumentParser()
    scripts_dir = Path(__file__).resolve().parent
    test_root = scripts_dir.parent

    parser.add_argument("--deploy-dir", default=str(test_root / "deployment_workloads"))
    parser.add_argument("--pp-dir", default=str(test_root / "baseline/propagation_policies"))
    parser.add_argument("--output-dir", default=str(scripts_dir / "new_results_baseline"))
    parser.add_argument("--namespace", default=NS_DEFAULT)
    parser.add_argument("--karmada-context", default=KARMADA_CTX_DEFAULT)
    parser.add_argument("--scheduler-context", default=SCHED_CTX_DEFAULT)
    parser.add_argument("--scheduler-namespace", default=SCHED_NS_DEFAULT)
    parser.add_argument("--scheduler-deploy", default="karmada-scheduler")
    parser.add_argument("--runs", type=int, default=RUNS_DEFAULT)
    parser.add_argument("--timeout-seconds", type=int, default=TIMEOUT_SECONDS_DEFAULT)
    parser.add_argument("--settle-seconds", type=float, default=SETTLE_SECONDS_DEFAULT)
    parser.add_argument("--overwrite", action="store_true", default=True, help="Overwrite results output (default)")
    parser.add_argument("--no-overwrite", action="store_false", dest="overwrite", help="Append instead of overwrite")

    args = parser.parse_args()

    pp_dir = Path(args.pp_dir)
    deploy_dir = Path(args.deploy_dir)
    out_dir = Path(args.output_dir)

    sched_deploy = args.scheduler_deploy
    sched_label = f"app={sched_deploy}"

    if args.overwrite and out_dir.exists():
        shutil.rmtree(out_dir, ignore_errors=True)
    out_dir.mkdir(parents=True, exist_ok=True)

    summary_csv = out_dir / "summary.csv"
    if args.overwrite or not summary_csv.exists():
        with open(summary_csv, "w", newline="") as f:
            csv.writer(f).writerow(["deployment", "runs", "median_s"])

    if not pp_dir.exists():
        print(f"PP dir not found: {pp_dir}", file=sys.stderr)
        sys.exit(2)
    if not deploy_dir.exists():
        print(f"Deployment dir not found: {deploy_dir}", file=sys.stderr)
        sys.exit(2)

    scale_scheduler(args.scheduler_context, args.scheduler_namespace, sched_deploy)
    sched_pod = get_scheduler_pod_by_label(args.scheduler_context, args.scheduler_namespace, sched_label)
    if not sched_pod:
        print("Could not find a Running scheduler pod.", file=sys.stderr)
        sys.exit(2)

    pairs = []
    for pp in sorted(pp_dir.glob("pp-benchmark-*.yaml")):
        name = pp.stem[len("pp-"):]
        dep = deploy_dir / f"{name}.yaml"
        if dep.exists():
            pairs.append((pp, dep, name))

    if not pairs:
        print(f"No (PP, Deployment) pairs found in {pp_dir} and {deploy_dir}", file=sys.stderr)
        sys.exit(1)

    for pp_path, dep_path, name in pairs:
        print(f"\n=== Benchmark {name} ({args.runs} runs) ===")
        csv_path = out_dir / f"{name}.csv"
        if args.overwrite or not csv_path.exists():
            with open(csv_path, "w", newline="") as f:
                csv.writer(f).writerow(["run", "begin_ts", "end_ts", "latency_seconds"])

        latencies: List[float] = []

        for i in range(1, args.runs + 1):
            apply_yaml(args.karmada_context, pp_path)
            time.sleep(2)

            scale_scheduler(args.scheduler_context, args.scheduler_namespace, sched_deploy)
            sched_pod = get_scheduler_pod_by_label(args.scheduler_context, args.scheduler_namespace, sched_label)
            if not sched_pod:
                print("Could not find a Running scheduler pod after recycle.", file=sys.stderr)
                delete_yaml(args.karmada_context, pp_path)
                break

            apply_yaml(args.karmada_context, dep_path)

            m = measure_latency(
                ns=NS_DEFAULT,
                dep_name=name,
                sched_ctx=args.scheduler_context,
                sched_ns=args.scheduler_namespace,
                sched_pod=sched_pod,
                timeout_s=args.timeout_seconds,
                settle_seconds=args.settle_seconds,
            )
            if m is None:
                print(f"Run {i}: timeout (no Begin/End found)")
            else:
                latency_s, begin_ts_str, end_ts_str = m
                latencies.append(latency_s)
                print(f"Run {i}: {latency_s:.6f}s")
                with open(csv_path, "a", newline="") as f:
                    csv.writer(f).writerow([i, begin_ts_str, end_ts_str, f"{latency_s:.6f}"])

            delete_yaml(args.karmada_context, dep_path)
            delete_yaml(args.karmada_context, pp_path)
            delete_rb(args.karmada_context, NS_DEFAULT, f"{name}-deployment")
            time.sleep(2)

        if latencies:
            med = median(latencies)
            with open(summary_csv, "a", newline="") as f:
                csv.writer(f).writerow([name, len(latencies), f"{med:.6f}"])
            print(f"Result {name}: median={med:.6f}s (n={len(latencies)})")
        else:
            print(f"Result {name}: no valid measurements")

if __name__ == "__main__":
    main()