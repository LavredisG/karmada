# DistributionScorer Plugin

The `DistributionScorer` plugin is a custom scheduling extension for the Karmada multi-cluster Scheduler Framework. It enables advanced, criteria-driven workload distribution across multiple Kubernetes clusters, optimizing for resource usage, cost, latency, power, efficiency, and load balancing.

## Overview

This plugin evaluates all possible ways to distribute a workload's replicas across available clusters. For each candidate distribution, it calculates a set of metrics (such as cost, power, resource efficiency, load balance standard deviation, and weighted latency) using cluster labels and workload requirements. The plugin then ranks distributions using the Analytic Hierarchy Process (AHP), with configurable weights for each criterion, and selects the best feasible allocation.

## Key Features

- **Multi-criteria Optimization:** Supports weighted criteria for cost, power, latency, efficiency, and load balance.
- **AHP-based Scoring:** Integrates with an external Python AHP service ([`ahp_service.py`](ahp_service.py)) for multi-criteria decision making.
- **Feasibility Checks:** Ensures allocations respect cluster resource capacities and node limits..
- **Detailed Metrics:** Logs and exposes metrics for each evaluated distribution for benchmarking and analysis.

## How It Works

1. **Metrics Collection:** For each cluster, the plugin collects resource and cost metrics from cluster labels.
2. **Distribution Generation:** Generates all possible ways to assign replicas across clusters.
3. **Feasibility Filtering:** Filters out distributions that exceed cluster capacities or node limits.
4. **Metric Calculation:** Computes metrics for each feasible distribution (see [`calculate_metrics.go`](calculate_metrics.go)).
5. **AHP Scoring:** Sends metrics and criteria weights to the AHP service to score each distribution.
6. **Best Allocation Selection:** Picks the highest-scoring feasible distribution and updates cluster weights.

## Configuration

- **Criteria Profiles:** Profiles (e.g., `cost80`, `latency60`, `efficiency80`, `balance`) define the weights for each criterion. See [`distributionscorer.go`](distributionscorer.go) for available profiles.
- **Cluster Labels:** Each cluster must be labeled with resource and cost metrics (e.g., `worker_cpu_capacity`, `worker_memory_capacity`, `worker_cost`, `latency`, `max_worker_nodes`).
- **External Services:** Requires [`ahp_service.py`](ahp_service.py) and [`weights_updater_service.py`](weights_updater_service.py) to be running for scoring and dynamic weight updates.

## Usage

1. **Enable the Plugin:** Add `DistributionScorer` to your Karmada scheduler configuration.
2. **Label Clusters:** Ensure all clusters have the required labels.
3. **Configure PropagationPolicy:** Set up policies with the desired criteria profile.
4. **Start External Services:** Run the AHP and weights updater services.
5. **Deploy Workloads:** Apply deployments and propagation policies. The plugin will distribute replicas according to the selected optimization criteria.

## Example

```yaml
apiVersion: policy.karmada.io/v1alpha1
kind: PropagationPolicy
metadata:
  name: pp-cost-optimized
  namespace: default
spec:
  resourceSelectors:
    - apiVersion: apps/v1
      kind: Deployment
      name: my-app
      namespace: default
  placement:
    clusterAffinity:
      clusterNames: [edge, fog, cloud]
    replicaScheduling:
      replicaDivisionPreference: Weighted
      replicaSchedulingType: Divided
```

## References

- [distributionscorer.go](distributionscorer.go)
- [calculate_metrics.go](calculate_metrics.go)
- [ahp_service.py](ahp_service.py)
- [weights_updater_service.py](weights_updater_service.py)

## License

This plugin is part of the Karmada project and is licensed under the Apache License 2.0.