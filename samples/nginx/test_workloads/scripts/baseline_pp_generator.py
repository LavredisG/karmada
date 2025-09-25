#!/usr/bin/env python3
import os
from pathlib import Path
import argparse
import sys
import textwrap

def gen_pp(size: str, replicas: str, ns: str) -> str:
    name = f"pp-benchmark-{size}-{replicas}"
    dep = f"benchmark-{size}-{replicas}"
    return textwrap.dedent(f"""\
    apiVersion: policy.karmada.io/v1alpha1
    kind: PropagationPolicy
    metadata:
      name: {name}
      namespace: {ns}
    spec:
      resourceSelectors:
      - apiVersion: apps/v1
        kind: Deployment
        name: {dep}
        namespace: {ns}
      placement:
        clusterAffinity:
          clusterNames:
          - edge
          - fog
          - cloud
        replicaScheduling:
          replicaDivisionPreference: Weighted
          replicaSchedulingType: Divided
          weightPreference:
            dynamicWeight: AvailableReplicas
    """)

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--deploy-dir", default=str(Path(__file__).resolve().parents[1] / "deployment_workloads"))
    parser.add_argument("--out-dir", default=str(Path(__file__).resolve().parents[1] / "baseline" / "propagation_policies"))
    parser.add_argument("--namespace", default="default")
    args = parser.parse_args()

    deploy_dir = Path(args.deploy_dir)
    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)

    files = sorted(deploy_dir.glob("benchmark-*.yaml"))
    if not files:
        print(f"No deployments found under {deploy_dir}", file=sys.stderr)
        sys.exit(1)

    for f in files:
        base = f.stem  # benchmark-<size>-<replicas>
        try:
            _, size, replicas = base.split("-")
        except ValueError:
            print(f"Skip unrecognized filename: {f.name}", file=sys.stderr)
            continue
        pp_yaml = gen_pp(size, replicas, args.namespace)
        out_path = out_dir / f"pp-baseline-{size}-{replicas}.yaml"
        out_path.write_text(pp_yaml)
        print(f"Wrote {out_path}")

if __name__ == "__main__":
    main()