#!/usr/bin/env python3
"""
Custom Karmada scheduler latency benchmark (balance criterion).

- Overwrites results_custom and per-deployment CSVs on each run.
- Starts plugin Python services (AHP + Weights Updater) in temp dirs, deletes temp dirs after each run.
- No service_logs pollution in repo.
- Cleans up all resources and processes between runs.
"""

import argparse
import csv
import os
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import time
from pathlib import Path
from statistics import median
from typing import List, Optional, Tuple

CRITERION_DEFAULT = "balance"
KARMADA_CTX_DEFAULT = "karmada-apiserver"
SCHED_CTX_DEFAULT = "kind-host"
SCHED_NS_DEFAULT = "karmada-system"
CUSTOM_SCHED_DEPLOY_DEFAULT = "custom-balance-scheduler"
DEFAULT_SCHED_DEPLOY = "karmada-scheduler"
NS_DEFAULT = "default"
RUNS_DEFAULT = 10
TIMEOUT_BEGIN_DEFAULT = 60
TIMEOUT_END_DEFAULT = 180


TIME_RE = re.compile(r'^[IWEF]\d{4}\s+(\d{2}):(\d{2}):(\d{2}\.\d+)')
LOG_BEGIN_RE = re.compile(r'Begin scheduling resource binding')
LOG_END_RE = re.compile(r'End scheduling resource binding')

REPO_ROOT = Path(__file__).resolve().parents[4]
PLUGIN_DIR = REPO_ROOT / "pkg" / "scheduler" / "framework" / "plugins" / "distributionscorer"
AHP_SERVICE = PLUGIN_DIR / "ahp_service.py"
WEIGHTS_UPDATER = PLUGIN_DIR / "weights_updater_service.py"

_started_procs: List[subprocess.Popen] = []
_temp_dirs: List[str] = []

def sh(cmd: List[str], capture: bool = False, check: bool = True) -> str:
    if capture:
        return subprocess.check_output(cmd, text=True).strip()
    subprocess.run(cmd, check=check)
    return ""

def scale_deploy(ctx: str, ns: str, deploy: str, replicas: int) -> None:
    sh(["kubectl", "--context", ctx, "-n", ns, "scale", "deploy", deploy, f"--replicas={replicas}"], check=False)
    sh(["kubectl", "--context", ctx, "-n", ns, "rollout", "status", "deploy", deploy], check=False)

def ensure_single_custom_scheduler(ctx: str, ns: str, deploy: str) -> None:
    scale_deploy(ctx, ns, deploy, 1)

def restart_scheduler_fresh_logs(ctx: str, ns: str, deploy: str, pod_label: str) -> None:
    subprocess.run(
        ["kubectl", "--context", ctx, "-n", ns, "delete", "pod", "-l", pod_label, "--wait=true"],
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False
    )
    sh(["kubectl", "--context", ctx, "-n", ns, "rollout", "status", "deploy", deploy], check=False)

def get_scheduler_pod(ctx: str, ns: str, pod_label: str) -> str:
    pods = sh(
        ["kubectl", "--context", ctx, "-n", ns, "get", "pods", "-l", pod_label, "-o", "name"],
        capture=True, check=False
    )
    return pods.splitlines()[0] if pods else ""

def apply_yaml(ctx: str, path: Path) -> None:
    sh(["kubectl", "--context", ctx, "apply", "-f", str(path)], check=True)

def delete_yaml_async(ctx: str, path: Path) -> None:
    subprocess.Popen(
        ["kubectl", "--context", ctx, "delete", "-f", str(path), "--ignore-not-found"],
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL
    )

def delete_resourcebinding_async(ctx: str, ns: str, rb_name: str) -> None:
    subprocess.Popen(
        ["kubectl", "--context", ctx, "-n", ns, "delete", "resourcebinding", rb_name, "--ignore-not-found"],
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL
    )

def parse_ts(line: str) -> Tuple[Optional[str], Optional[float]]:
    m = TIME_RE.match(line)
    if not m:
        return None, None
    hh, mm, ss = m.groups()
    ts_str = f"{hh}:{mm}:{ss}"
    ts_sec = int(hh) * 3600 + int(mm) * 60 + float(ss)
    return ts_str, ts_sec

def measure_latency(ns: str, dep_name: str, sched_ctx: str, sched_ns: str, pod_label: str,
                    timeout_begin: int, timeout_end: int) -> Optional[Tuple[float, str, str]]:
    rb_fq = f"{ns}/{dep_name}-deployment"
    pod = get_scheduler_pod(sched_ctx, sched_ns, pod_label)
    if not pod:
        print(f"ERROR: No scheduler pod found with selector: {pod_label}", file=sys.stderr)
        return None

    # Wait for Begin
    deadline = time.time() + timeout_begin
    begin_ts_str = None
    begin_ts = None
    while time.time() < deadline and begin_ts is None:
        logs = sh(["kubectl", "--context", sched_ctx, "-n", sched_ns, "logs", pod, "--since=10m"], capture=True, check=False)
        for line in logs.splitlines():
            if rb_fq in line and LOG_BEGIN_RE.search(line):
                ts_str, ts = parse_ts(line)
                if ts is not None:
                    begin_ts_str, begin_ts = ts_str, ts
                    break
        if begin_ts is None:
            time.sleep(1.0)
    if begin_ts is None:
        print("Timeout waiting for Begin.", file=sys.stderr)
        return None

    # Wait for End
    deadline = time.time() + timeout_end
    end_ts_str = None
    end_ts = None
    while time.time() < deadline and end_ts is None:
        logs = sh(["kubectl", "--context", sched_ctx, "-n", sched_ns, "logs", pod, "--since=10m"], capture=True, check=False)
        for line in logs.splitlines():
            if rb_fq in line and LOG_END_RE.search(line):
                ts_str, ts = parse_ts(line)
                if ts is not None:
                    end_ts_str, end_ts = ts_str, ts
                    break
        if end_ts is None:
            time.sleep(1.0)
    if end_ts is None:
        print("Timeout waiting for End.", file=sys.stderr)
        return None

    return round(end_ts - begin_ts, 6), begin_ts_str, end_ts_str

def start_service(py_path: Path, name: str, run_idx: int, extra_env: Optional[dict] = None) -> subprocess.Popen:
    """
    Start a Python service in a temp dir (no service_logs pollution).
    """
    env = os.environ.copy()
    if extra_env:
        env.update(extra_env)
    temp_dir = tempfile.mkdtemp(prefix=f"{name}_run_")
    _temp_dirs.append(temp_dir)
    proc = subprocess.Popen(
        ["python3", str(py_path)],
        cwd=temp_dir,
        env=env,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        start_new_session=True,
    )
    _started_procs.append(proc)
    time.sleep(2.0)
    return proc

def stop_all_services() -> None:
    while _started_procs:
        p = _started_procs.pop()
        try:
            p.terminate()
            p.wait(timeout=8)
        except Exception:
            try:
                p.kill()
            except Exception:
                pass
    while _temp_dirs:
        d = _temp_dirs.pop()
        shutil.rmtree(d, ignore_errors=True)

def ensure_csv_with_header(path: Path, headers: List[str]) -> None:
    with open(path, "w", newline="") as f:
        csv.writer(f).writerow(headers)

def handle_signal(signum, frame):
    stop_all_services()
    sys.exit(1)

def main():
    signal.signal(signal.SIGINT, handle_signal)
    signal.signal(signal.SIGTERM, handle_signal)

    root = Path(__file__).resolve().parents[1]
    parser = argparse.ArgumentParser()
    parser.add_argument("--criterion", default=CRITERION_DEFAULT)
    parser.add_argument("--deploy-dir", default=str(root / "deployment_workloads"))
    parser.add_argument("--pp-dir", default=None, help="Defaults to propagation_policies/<criterion>")
    parser.add_argument("--output-dir", default=str(root / "scripts" / "results_custom"))
    parser.add_argument("--namespace", default=NS_DEFAULT)
    parser.add_argument("--karmada-context", default=KARMADA_CTX_DEFAULT)
    parser.add_argument("--scheduler-context", default=SCHED_CTX_DEFAULT)
    parser.add_argument("--scheduler-namespace", default=SCHED_NS_DEFAULT)
    parser.add_argument("--scheduler-deploy", default=CUSTOM_SCHED_DEPLOY_DEFAULT)
    parser.add_argument("--scheduler-pod-label", default=None, help="Pod selector, defaults to app=<scheduler-deploy>")
    parser.add_argument("--scale-down-default", action="store_true", help="Scale down the default scheduler")
    parser.add_argument("--runs", type=int, default=RUNS_DEFAULT)
    parser.add_argument("--timeout-begin", type=int, default=TIMEOUT_BEGIN_DEFAULT)
    parser.add_argument("--timeout-end", type=int, default=TIMEOUT_END_DEFAULT)
    args = parser.parse_args()

    deploy_dir = Path(args.deploy_dir)
    pp_dir = Path(args.pp_dir) if args.pp_dir else (root / "propagation_policies" / args.criterion)
    out_dir = Path(args.output_dir)
    if out_dir.exists():
        shutil.rmtree(out_dir, ignore_errors=True)
    out_dir.mkdir(parents=True, exist_ok=True)
    per_suite_summary = out_dir / f"summary_{args.criterion}.csv"
    ensure_csv_with_header(per_suite_summary, ["deployment", "runs", "median_s"])

    pod_label = args.scheduler_pod_label or f"app={args.scheduler_deploy}"

    # Scheduler readiness
    if args.scale_down_default:
        scale_deploy(args.scheduler_context, args.scheduler_namespace, DEFAULT_SCHED_DEPLOY, 0)
    ensure_single_custom_scheduler(args.scheduler_context, args.scheduler_namespace, args.scheduler_deploy)

    # Collect PP files
    # pp_files = sorted(pp_dir.glob("pp-*.yaml"))
    pp_files = sorted([pp for pp in pp_dir.glob("pp-benchmark-xlarge-*.yaml")])
    if not pp_files:
        print(f"No PPs under {pp_dir}.", file=sys.stderr)
        sys.exit(1)

    for pp in pp_files:
        # Infer deployment name from PP filename: pp-benchmark-<profile>-<replicas>.yaml
        stem = pp.stem
        parts = stem.split("-")
        if len(parts) >= 4 and parts[0] == "pp" and parts[1] == "benchmark":
            dep_name = "benchmark-" + "-".join(parts[2:4])
        else:
            print(f"Skip PP (cannot resolve deployment): {pp.name}", file=sys.stderr)
            continue

        deploy_path = deploy_dir / f"{dep_name}.yaml"
        if not deploy_path.exists():
            print(f"Missing deployment for {dep_name}: {deploy_path}", file=sys.stderr)
            continue

        dep_csv = out_dir / f"{dep_name}.csv"
        ensure_csv_with_header(dep_csv, ["run", "begin_ts", "end_ts", "latency_seconds"])

        print(f"=== Benchmark (custom {args.criterion}) {dep_name} ({args.runs} runs) ===")
        latencies: List[float] = []

        for i in range(1, args.runs + 1):
            stop_all_services()  # ensure clean state

            # Non-blocking cleanup
            delete_yaml_async(args.karmada_context, pp)
            delete_yaml_async(args.karmada_context, deploy_path)
            delete_resourcebinding_async(args.karmada_context, args.namespace, f"{dep_name}-deployment")

            # Fresh scheduler logs
            restart_scheduler_fresh_logs(args.scheduler_context, args.scheduler_namespace, args.scheduler_deploy, pod_label)
            time.sleep(2.0)

            # Start plugin services (in temp dirs)
            start_service(AHP_SERVICE, "ahp_service", i)
            start_service(WEIGHTS_UPDATER, "weights_updater", i, extra_env={"POLICY_NAME": f"pp-{dep_name}"})

            # Apply PP then Deployment
            apply_yaml(args.karmada_context, pp)
            time.sleep(1.0)
            apply_yaml(args.karmada_context, deploy_path)

            # Measure latency
            m = measure_latency(
                args.namespace, dep_name,
                args.scheduler_context, args.scheduler_namespace, pod_label,
                timeout_begin=args.timeout_begin, timeout_end=args.timeout_end
            )
            if m is None:
                print(f"Run {i}: timeout (no complete Begin/End window).")
            else:
                latency_s, begin_ts_str, end_ts_str = m
                latencies.append(latency_s)
                print(f"Run {i}: {latency_s:.6f}s")
                with open(dep_csv, "a", newline="") as f:
                    csv.writer(f).writerow([i, begin_ts_str, end_ts_str, f"{latency_s:.6f}"])

            # Cleanup for next run
            stop_all_services()
            delete_yaml_async(args.karmada_context, deploy_path)
            delete_yaml_async(args.karmada_context, pp)
            delete_resourcebinding_async(args.karmada_context, args.namespace, f"{dep_name}-deployment")
            time.sleep(1.5)

        if latencies:
            med = median(latencies)
            print(f"Result (custom {args.criterion}) {dep_name}: median={med:.6f}s (n={len(latencies)})")
            with open(per_suite_summary, "a", newline="") as f:
                csv.writer(f).writerow([dep_name, len(latencies), f"{med:.6f}"])
        else:
            print(f"Result (custom {args.criterion}) {dep_name}: no valid measurements")

    stop_all_services()

if __name__ == "__main__":
    main()