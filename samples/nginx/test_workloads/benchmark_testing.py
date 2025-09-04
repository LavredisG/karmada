import subprocess
import os
import time
import yaml
import re

# --- CONFIGURATION ---
criterion = "fairness"  # change as needed
pp_dir = f"./propagation_policies/{criterion}/"
deploy_dir = "./deployment_workloads/"
home = os.environ["HOME"]
ahp_service_path = f"{home}/workspace/karmada/pkg/scheduler/framework/plugins/distributionscorer/ahp_service.py"
weights_updater_path = f"{home}/workspace/karmada/pkg/scheduler/framework/plugins/distributionscorer/weights_updater_service.py"
allocation_log = f"allocation_{criterion}.log"

# Only up to large-25, not xlarge
profiles = ["small", "medium", "large", "xlarge"]
replicas = [5, 10, 15, 20, 25]

def get_policy_and_deployment_names():
    pairs = []
    for profile in profiles:
        for rep in replicas:
            name = f"benchmark-{profile}-{rep}"
            pp = f"pp-{name}.yaml"
            dep = f"{name}.yaml"
            pairs.append((pp, dep, name))
    return pairs

def apply_yaml(path):
    subprocess.run(["kubectl", "apply", "-f", path], check=True)

def delete_yaml(path):
    subprocess.run(["kubectl", "delete", "-f", path], check=True)

def start_service(cmd, env=None):
    return subprocess.Popen(["python3", cmd], env=env)

def stop_service(proc):
    proc.terminate()
    try:
        proc.wait(timeout=10)
    except subprocess.TimeoutExpired:
        proc.kill()

def scale_scheduler(criterion):
    deployment_name = f"custom-{criterion}-scheduler"
    # Switch to kind-host context
    subprocess.run(["kubectx", "kind-host"], check=True)

    # Scale down
    subprocess.run([
        "kubectl", "scale", "deployment", deployment_name, "--replicas=0"
    ], check=True)
    print(f"Scaled down {deployment_name} to 0 replicas.")
    time.sleep(2)  # Give time for pod to terminate

    # Scale up
    subprocess.run([
        "kubectl", "scale", "deployment", deployment_name, "--replicas=1"
    ], check=True)
    print(f"Scaled up {deployment_name} to 1 replica.")

    # Wait for pod to be ready
    while True:
        result = subprocess.run([
            "kubectl", "get", "pods", "-l", f"app={deployment_name}",
            "-o", "jsonpath={.items[0].status.phase}"
        ], capture_output=True, text=True)
        if result.stdout.strip() == "Running":
            print(f"{deployment_name} pod is running.")
            break
        print("Waiting for scheduler pod to be ready...")
        time.sleep(2)

    # Switch back to karmada-apiserver context
    subprocess.run(["kubectx", "karmada-apiserver"], check=True)

def get_distribution_metrics(scheduler_name):
    # Switch to kind-host context
    subprocess.run(["kubectx", "kind-host"], check=True)
    try:
        logs = subprocess.check_output(
            ["kubectl", "logs", scheduler_name, "-n", "karmada-system"],
            text=True
        )
    except Exception as e:
        subprocess.run(["kubectx", "karmada-apiserver"], check=True)
        return f"Error fetching scheduler logs: {e}"
    subprocess.run(["kubectx", "karmada-apiserver"], check=True)

    lines = logs.splitlines()
    # 1. Find the latest "DistributionScorer: Selected best distribution: (triplet)" and its index
    selected_pattern = re.compile(r"Selected best distribution: (\([^)]+\))")
    triplet = None
    selected_idx = None
    for idx in range(len(lines)-1, -1, -1):
        m = selected_pattern.search(lines[idx])
        if m:
            triplet = m.group(1)
            selected_idx = idx
            break
    if not triplet or selected_idx is None:
        return "Best distribution triplet not found in logs."

    # 2. Search backwards from selected_idx for the last "Estimating metrics for distribution (triplet)"
    metrics_pattern = f"Estimating metrics for distribution {triplet}"
    start_idx = None
    for idx in range(selected_idx, -1, -1):
        if metrics_pattern in lines[idx]:
            start_idx = idx
            break
    if start_idx is None:
        return f"Metrics for distribution {triplet} not found."

    # 3. Collect lines from start_idx up to and including the summary line for this triplet
    metrics_lines = []
    summary_pattern = re.compile(rf"Distribution {re.escape(triplet)}: ")
    found_summary = False
    for line in lines[start_idx:]:
        metrics_lines.append(line)
        if summary_pattern.search(line):
            found_summary = True
            break
    if not found_summary:
        return f"Summary for distribution {triplet} not found."
    return "\n".join(metrics_lines)

def get_allocation(deployment_name):
    try:
        rb_list = subprocess.check_output(
            ["kubectl", "get", "resourcebindings", "-n", "default", "-o", "yaml"]
        )
        rbs = yaml.safe_load(rb_list)
        found = False
        for rb in rbs.get("items", []):
            print("DEBUG ResourceBinding:", rb["spec"]["resource"]["name"])
            if rb["spec"]["resource"]["name"] == deployment_name:
                found = True
                alloc = []
                for c in rb.get("spec", {}).get("clusters", []):
                    alloc.append(f"{c['name']}={c['replicas']}")
                return ", ".join(alloc) if alloc else "No clusters assigned yet"
        if not found:
            print("DEBUG: Deployment name not found in ResourceBindings")
    except Exception as e:
        return f"Error getting allocation: {e}"
    return "Not found"

def get_scheduler_pod_name(criterion):
    deployment_name = f"custom-{criterion}-scheduler"
    subprocess.run(["kubectx", "kind-host"], check=True)
    pods = subprocess.check_output([
        "kubectl", "get", "pods", "-n", "karmada-system", "--no-headers"
    ], text=True)
    subprocess.run(["kubectx", "karmada-apiserver"], check=True)
    for line in pods.splitlines():
        if deployment_name in line and "Running" in line:
            return line.split()[0]
    return None

def wait_for_allocation(deployment_name, retries=20, delay=5):
    for attempt in range(retries):
        try:
            rb_list = subprocess.check_output(
                ["kubectl", "get", "resourcebindings", "-n", "default", "-o", "yaml"]
            )
            rbs = yaml.safe_load(rb_list)
            for rb in rbs.get("items", []):
                print(f"DEBUG: Found ResourceBinding for {rb['spec']['resource']['name']}")
                if rb["spec"]["resource"]["name"] == deployment_name:
                    clusters = rb.get("spec", {}).get("clusters", [])
                    if clusters:
                        alloc = [f"{c['name']}={c['replicas']}" for c in clusters]
                        return ", ".join(alloc)
                    else:
                        print("DEBUG: No clusters assigned yet, waiting...")
        except Exception as e:
            print(f"DEBUG: Error getting allocation: {e}")
        time.sleep(delay)
    return "No clusters assigned yet"

def wait_for_metrics(scheduler_pod_name, retries=10, delay=3):
    for _ in range(retries):
        metrics = get_distribution_metrics(scheduler_pod_name)
        if "not found" not in metrics and "Error" not in metrics:
            return metrics
        time.sleep(delay)
    return metrics

def main():
    pairs = get_policy_and_deployment_names()
    with open(allocation_log, "w") as log:
        for pp_file, dep_file, name in pairs:
            print(f"\n=== Testing {name} ===")

            # Scale scheduler to clear logs for next run
            scale_scheduler(criterion)
            scheduler_pod_name = get_scheduler_pod_name(criterion)
            if not scheduler_pod_name:
                print("Could not find scheduler pod name!")
                continue
            
            # 1. Apply PropagationPolicy
            apply_yaml(os.path.join(pp_dir, pp_file))
            time.sleep(2)
            
            # 2. Start ahp_service.py
            ahp_proc = start_service(ahp_service_path)
            time.sleep(2)
            
            # 3. Start weights_updater_service.py with envvar
            env = os.environ.copy()
            env["POLICY_NAME"] = f"pp-{name}"
            wu_proc = start_service(weights_updater_path, env=env)
            time.sleep(3)
            
            # 4. Apply Deployment
            apply_yaml(os.path.join(deploy_dir, dep_file))
            
            # Wait for scheduling to complete (tune as needed)
            print("Waiting for scheduling...")
            time.sleep(4)

            # Record allocation
            alloc = wait_for_allocation(name)
            log.write(f"{name}: {alloc}\n")
            metrics = wait_for_metrics(scheduler_pod_name)
            log.write(metrics + "\n")
            log.flush()
            print(f"Allocation: {alloc}")
            print(metrics)

            # Cleanup
            delete_yaml(os.path.join(deploy_dir, dep_file))
            delete_yaml(os.path.join(pp_dir, pp_file))
            stop_service(wu_proc)
            stop_service(ahp_proc)
            time.sleep(3)

if __name__ == "__main__":
    main()