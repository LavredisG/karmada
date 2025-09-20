# pp_generator_simple.py

import os
import yaml

# List your deployment names here (without .yaml extension)
deployment_names = [
    "benchmark-small-5", "benchmark-small-10", "benchmark-small-15", "benchmark-small-20", "benchmark-small-25",
    "benchmark-medium-5", "benchmark-medium-10", "benchmark-medium-15", "benchmark-medium-20", "benchmark-medium-25",
    "benchmark-large-5", "benchmark-large-10", "benchmark-large-15", "benchmark-large-20", "benchmark-large-25",
    "benchmark-xlarge-5", "benchmark-xlarge-10", "benchmark-xlarge-15", "benchmark-xlarge-20", "benchmark-xlarge-25"
]

CRITERIA = ["power", "cost", "latency", "efficiency", "balanced", "fairness"]
# Set your scheduler name (change per criterion)
# SCHEDULER_NAME = "custom-latency-scheduler"

for criterion in CRITERIA:
    for dep_name in deployment_names:
        SCHEDULER_NAME = f"custom-{criterion}-scheduler"
        pp = {
            "apiVersion": "policy.karmada.io/v1alpha1",
            "kind": "PropagationPolicy",
        "metadata": {
            "name": f"pp-{dep_name}",
        },
        "spec": {
            "schedulerName": SCHEDULER_NAME,
            "resourceSelectors": [
                {
                    "apiVersion": "apps/v1",
                    "kind": "Deployment",
                    "name": dep_name,
                }
            ],
            "placement": {
                "clusterAffinity": {
                    "clusterNames": ["edge", "fog", "cloud"]
                },
                "replicaScheduling": {
                    "replicaDivisionPreference": "Weighted",
                    "replicaSchedulingType": "Divided",
                    "weightPreference": {
                        "staticWeightList": [
                            {"targetCluster": {"clusterNames": ["edge"]}, "weight": 1},
                            {"targetCluster": {"clusterNames": ["fog"]}, "weight": 1},
                            {"targetCluster": {"clusterNames": ["cloud"]}, "weight": 1},
                        ]
                    }
                }
            }
        }
    }


        output_dir = f"./propagation_policies/{criterion}"
        os.makedirs(output_dir, exist_ok=True)

        out_file = os.path.join(output_dir, f"pp-{dep_name}.yaml")
        with open(out_file, "w") as f:
            yaml.dump(pp, f, sort_keys=False)
        print(f"Generated {out_file}")

print("Done.")