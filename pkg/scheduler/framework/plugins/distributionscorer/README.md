# Distribution Scorer Plugin for Karmada Scheduler

## Introduction
The DistributionScorer plugin is a custom scoring plugin for the Karmada scheduler. It is designed to optimize the distribution of workloads across clusters by evaluating various metrics such as power consumption, cost, resource efficiency, load balance, and latency. The plugin ensures that workloads are distributed in a way that meets user-defined criteria while maximizing efficiency and minimizing costs.

## Features
- **Custom Scoring**: Evaluates clusters based on multiple criteria, including power, cost, resource efficiency, load balance, and latency.
- **Prometheus Metrics Integration**: Exposes detailed metrics for monitoring and visualization in Grafana.
- **Dynamic Distribution**: Generates and evaluates all possible workload distributions to find the optimal allocation.
- **Feasibility Checks**: Ensures that clusters meet the resource requirements for workloads before scoring.

## Architecture
The plugin is integrated into the Karmada scheduler and operates during the scoring phase. It consists of the following components:

1. **Metrics Collection**:
   - Collects metrics such as CPU capacity, memory capacity, power, and cost from each cluster.
   - Validates that clusters can meet the resource requirements of workloads.

2. **Distribution Generation**:
   - Generates all possible workload distributions across clusters.
   - Evaluates the feasibility of each distribution based on resource constraints.

3. **Scoring and Normalization**:
   - Scores each feasible distribution using user-defined criteria.
   - Normalizes scores to ensure fair comparison across clusters.

4. **Prometheus Metrics**:
   - Exposes metrics such as final distribution allocation, CPU usage, and cost metrics via an HTTP endpoint.
   - Metrics are updated dynamically during the scoring process.

## Usage
### Enabling the Plugin
To enable the Distribution Scorer plugin, include it in the Karmada scheduler configuration:

```yaml
apiVersion: kubescheduler.config.k8s.io/v1
kind: KubeSchedulerConfiguration
...
plugins:
  score:
    enabled:
    - name: DistributionScorer
...
```

### Configuration
The plugin uses default criteria weights for scoring. These can be customized in the plugin's configuration:

- **Power**: Weight = 0.25
- **Cost**: Weight = 0.25
- **Resource Efficiency**: Weight = 0.15
- **Load Balance**: Weight = 0.15
- **Latency**: Weight = 0.20

## Prometheus Metrics
The plugin exposes the following metrics via Prometheus:

1. **Final Distribution Allocation**:
   - Metric: `final_distribution_allocation`
   - Labels: `cluster`
   - Description: Tracks the number of replicas allocated to each cluster.

2. **CPU Usage per Cluster**:
   - Metric: `cpu_usage_per_cluster`
   - Labels: `cluster`
   - Description: Monitors the CPU usage in millicores for each cluster.

3. **Cost Metrics per Cluster**:
   - Metric: `cost_metrics_per_cluster`
   - Labels: `cluster`
   - Description: Tracks the cost metrics for each cluster.

### Accessing Metrics
Metrics are exposed at the `/metrics` endpoint on port `8080`. Configure Prometheus to scrape this endpoint:

```yaml
scrape_configs:
  - job_name: 'karmada-distributionscorer'
    static_configs:
      - targets: ['<plugin-host>:8080']
```

## Grafana Dashboards
To visualize the metrics, create Grafana dashboards with panels for:
- Final distribution allocation.
- CPU usage per cluster.
- Cost metrics per cluster.

## Future Enhancements
- **Dynamic Weight Adjustment**: Allow dynamic adjustment of criteria weights based on workload requirements.
- **Additional Metrics**: Include metrics for memory usage, network latency, and storage utilization.
- **Advanced Visualization**: Provide pre-configured Grafana dashboards for easier setup.

## Conclusion
The DistributionScorer plugin enhances the Karmada scheduler by providing advanced scoring capabilities and detailed monitoring. It ensures optimal workload distribution while offering insights into cluster performance and resource utilization.