#!/usr/bin/env python3
"""
Test Karmada default scheduler allocations and capture specific scheduler logs.

- Recycles the scheduler pod to get clean logs for each run.
- Applies a Deployment and its corresponding baseline PropagationPolicy.
- Waits for the ResourceBinding to be scheduled.
- Records the final replica allocation and three key log lines.
- Cleans up all resources.
- Saves results to a .log file, overwriting on each run.
"""

import argparse
import json
import re
import shutil
import subprocess
import sys
import time
from pathlib import Path
from typing import Dict, List, Optional, Tuple

# Default values
KARMADA_CTX_DEFAULT = "karmada-apiserver"
SCHED_CTX_DEFAULT = "kind-host"
SCHED_NS_DEFAULT = "karmada-system"
SCHED_DEPLOY_DEFAULT = "karmada-scheduler"
NS_DEFAULT = "default"
TIMEOUT_SECONDS_DEFAULT = 120

# Regex to find the specific log lines
LOG_TARGET_RE = re.compile(r'Target cluster calculated by estimators')
LOG_ASSIGNED_RE = re.compile(r'Assigned Replicas:')
LOG_SCHEDULED_RE = re.compile(r'scheduled to clusters')


def sh(cmd: List[str], capture: bool = False, check: bool = True) -> str:
    """Execute a shell command."""
    try:
        if capture:
            return subprocess.check_output(cmd, text=True, stderr=subprocess.PIPE).strip()
        subprocess.run(cmd, check=check, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        return ""
    except subprocess.CalledProcessError as e:
        if not check:
            return ""
        print(f"Command failed: {' '.join(cmd)}\nStderr: {e.stderr}", file=sys.stderr)
        raise


def apply_yaml(ctx: str, path: Path) -> None:
    """Apply a Kubernetes YAML file."""
    sh(["kubectl", "--context", ctx, "apply", "-f", str(path)])


def delete_yaml(ctx: str, path: Path) -> None:
    """Delete resources from a Kubernetes YAML file."""
    sh(["kubectl", "--context", ctx, "delete", "-f", str(path), "--ignore-not-found"], check=False)


def scale_scheduler(ctx: str, ns: str, deploy: str) -> None:
    """Scale the scheduler to 0 then 1 to ensure a clean state and logs."""
    print("  Recycling scheduler pod for clean logs...")
    sh(["kubectl", "--context", ctx, "-n", ns, "scale", "deployment", deploy, "--replicas=0"])
    time.sleep(3)  # Wait for termination
    sh(["kubectl", "--context", ctx, "-n", ns, "scale", "deployment", deploy, "--replicas=1"])
    # Wait for the new pod to be ready
    sh(["kubectl", "--context", ctx, "-n", ns, "rollout", "status", "deployment", deploy, "--timeout=90s"])
    print("  Scheduler is running. Pausing for 5 seconds to ensure it's fully initialized...")
    time.sleep(5) # Extra grace period for the scheduler process to be fully operational


def get_scheduler_pod(ctx: str, ns: str, label: str) -> Optional[str]:
    """Get the name of the running scheduler pod."""
    deadline = time.time() + 60
    while time.time() < deadline:
        pods_str = sh(["kubectl", "--context", ctx, "-n", ns, "get", "pods", "-l", label, "-o", "jsonpath={.items[*].metadata.name}"], capture=True, check=False)
        if not pods_str:
            time.sleep(1)
            continue
        for name in pods_str.split():
            phase = sh(["kubectl", "--context", ctx, "-n", ns, "get", "pod", name, "-o", "jsonpath={.status.phase}"], capture=True, check=False)
            if phase == "Running":
                return name
        time.sleep(1)
    return None


def get_allocation_and_logs(
    karmada_ctx: str,
    sched_ctx: str,
    sched_ns: str,
    sched_pod: str,
    ns: str,
    rb_name: str,
    timeout_s: int
) -> Tuple[Optional[Dict[str, int]], List[str]]:
    """
    Wait for a ResourceBinding and get the final allocation and specific scheduler logs.
    """
    deadline = time.time() + timeout_s
    allocation = None
    
    while time.time() < deadline:
        try:
            # 1. Check if allocation is complete
            clusters_json = sh(
                [
                    "kubectl", "--context", karmada_ctx, "-n", ns, "get", "resourcebinding", rb_name,
                    "-o", "jsonpath={.spec.clusters}",
                ],
                capture=True,
                check=False,
            )
            if clusters_json:
                clusters = json.loads(clusters_json)
                allocation = {item["name"]: item["replicas"] for item in clusters}

                # 2. If allocation is found, grab the logs and find the specific lines
                log_output = sh(["kubectl", "--context", sched_ctx, "-n", sched_ns, "logs", sched_pod, "--since=2m"], capture=True, check=False)
                all_lines = log_output.splitlines()
                
                # Find the index of the final "scheduled to clusters" line for our specific RB
                log_rb_name = f"{ns}/{rb_name}"
                scheduled_line_index = -1
                for i, line in enumerate(all_lines):
                    if log_rb_name in line and LOG_SCHEDULED_RE.search(line):
                        scheduled_line_index = i
                        break
                
                # If found, work backwards to get the preceding two lines
                if scheduled_line_index > 1:
                    line3 = all_lines[scheduled_line_index]
                    line2 = all_lines[scheduled_line_index - 1]
                    line1 = all_lines[scheduled_line_index - 2]

                    # Verify that we have the correct preceding lines
                    if LOG_ASSIGNED_RE.search(line2) and LOG_TARGET_RE.search(line1):
                        return allocation, [line1, line2, line3]

        except (subprocess.CalledProcessError, json.JSONDecodeError):
            pass  # Ignore errors and retry
        time.sleep(1.0)

    print(f"ERROR: Timeout or could not find the complete log sequence for '{rb_name}'.", file=sys.stderr)
    return allocation, []


def main() -> None:
    """Main execution function."""
    parser = argparse.ArgumentParser(description="Test default Karmada scheduler allocations.")
    scripts_dir = Path(__file__).resolve().parent
    test_root = scripts_dir.parent

    parser.add_argument("--deploy-dir", default=str(test_root / "deployment_workloads"))
    parser.add_argument("--pp-dir", default=str(test_root / "baseline" / "propagation_policies"))
    parser.add_argument("--output-dir", default=str(scripts_dir / "baseline_allocations"))
    parser.add_argument("--namespace", default=NS_DEFAULT)
    parser.add_argument("--karmada-context", default=KARMADA_CTX_DEFAULT)
    parser.add_argument("--scheduler-context", default=SCHED_CTX_DEFAULT)
    parser.add_argument("--scheduler-namespace", default=SCHED_NS_DEFAULT)
    parser.add_argument("--scheduler-deploy", default=SCHED_DEPLOY_DEFAULT)
    parser.add_argument("--timeout-seconds", type=int, default=TIMEOUT_SECONDS_DEFAULT)
    args = parser.parse_args()

    pp_dir = Path(args.pp_dir)
    deploy_dir = Path(args.deploy_dir)
    out_dir = Path(args.output_dir)

    if out_dir.exists():
        shutil.rmtree(out_dir)
    out_dir.mkdir(parents=True)

    results_log = out_dir / "allocations.log"
    sched_label = f"app={args.scheduler_deploy}"

    pairs = []
    for pp_path in sorted(pp_dir.glob("pp-benchmark-*.yaml")):
        name = pp_path.stem[len("pp-"):]
        dep_path = deploy_dir / f"{name}.yaml"
        if dep_path.exists():
            pairs.append((pp_path, dep_path, name))

    if not pairs:
        print(f"ERROR: No (PP, Deployment) pairs found in {pp_dir} and {deploy_dir}", file=sys.stderr)
        sys.exit(1)

    for pp_path, dep_path, name in pairs:
        print(f"--- Testing: {name} ---")
        try:
            scale_scheduler(args.scheduler_context, args.scheduler_namespace, args.scheduler_deploy)
            sched_pod = get_scheduler_pod(args.scheduler_context, args.scheduler_namespace, sched_label)
            if not sched_pod:
                print(f"ERROR: Could not find running scheduler pod for {args.scheduler_deploy}", file=sys.stderr)
                continue
            
            print(f"  Using scheduler pod: {sched_pod}")
            # Apply PP first, then the Deployment
            apply_yaml(args.karmada_context, pp_path)
            time.sleep(1) # Small pause between applying resources
            apply_yaml(args.karmada_context, dep_path)

            rb_name = f"{name}-deployment"
            allocation, logs = get_allocation_and_logs(
                args.karmada_context, args.scheduler_context, args.scheduler_namespace,
                sched_pod, args.namespace, rb_name, args.timeout_seconds
            )

            with open(results_log, "a") as f:
                if allocation:
                    alloc_str = ", ".join([f"{k}={v}" for k, v in sorted(allocation.items())])
                    f.write(f"{name}: {alloc_str}\n")
                    print(f"  Allocation: {allocation}")
                    if logs:
                        for log_line in logs:
                            f.write(f"{log_line}\n")
                    else:
                        f.write("--> Could not retrieve specific log lines for this run.\n")
                else:
                    f.write(f"{name}: FAILED to get allocation\n")
                    print(f"  Failed to get allocation for {name}.")

        finally:
            print("  Cleaning up resources...")
            delete_yaml(args.karmada_context, dep_path)
            delete_yaml(args.karmada_context, pp_path)
            time.sleep(3)

    print(f"\n✅ Done. Results saved to {results_log}")


if __name__ == "__main__":
    main()