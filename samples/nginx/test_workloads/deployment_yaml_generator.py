# yaml_generator.py

import os
import yaml

# Define resource profiles
resource_profiles = {
    "small":      {"cpu": "50m",   "memory": "64Mi"},
    "medium":     {"cpu": "200m",  "memory": "128Mi"},
    "large":      {"cpu": "1000m", "memory": "1Gi"},
    "xlarge":     {"cpu": "2000m", "memory": "2Gi"},
}

replica_counts = [5, 10, 15, 20, 25]

def make_deployment(name, cpu, memory, replicas):
    return {
        "apiVersion": "apps/v1",
        "kind": "Deployment",
        "metadata": {
            "name": f"benchmark-{name}-{replicas}",
            "namespace": "default",
            "labels": {
                "app": name
            }
        },
        "spec": {
            "replicas": replicas,
            "selector": {
                "matchLabels": {
                    "app": name
                }
            },
            "template": {
                "metadata": {
                    "labels": {
                        "app": name
                    }
                },
                "spec": {
                    "containers": [
                        {
                            "name": "app",
                            "image": "nginx:stable",
                            "resources": {
                                "requests": {
                                    "cpu": cpu,
                                    "memory": memory
                                }
                            }
                        }
                    ]
                }
            }
        }
    }

def main():
    # Set your output directory and app/criterion name here
    output_dir = "./deployment_workloads"
    app_name = "benchmark"  # Change per criterion, e.g., "latency-bench"
    os.makedirs(output_dir, exist_ok=True)

    for profile, res in resource_profiles.items():
        for replicas in replica_counts:
            dep = make_deployment(profile, res["cpu"], res["memory"], replicas)
            filename = f"{output_dir}/{app_name}-{profile}-{replicas}.yaml"
            with open(filename, "w") as f:
                yaml.dump(dep, f, sort_keys=False)
            print(f"Generated {filename}")

if __name__ == "__main__":
    main()