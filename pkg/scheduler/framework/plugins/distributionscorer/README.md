# Distribution Scorer Plugin for Karmada Scheduler

## Introduction

The DistributionScorer plugin is an advanced scoring plugin for the Karmada scheduler that optimizes workload distribution across clusters. It evaluates multiple dimensions including power consumption, cost, resource efficiency, load balance, and latency to find the optimal distribution of workload replicas.

This plugin uses the Analytic Hierarchy Process (AHP) to make multi-criteria decisions, ensuring a balanced consideration of all relevant factors when deciding where to place workloads.

## Features

- **Multi-Criteria Decision Making**: Uses AHP to evaluate distributions based on multiple weighted criteria.
- **Dynamic Weight Adjustment**: Automatically adjusts criteria weights based on workload characteristics.
- **Comprehensive Metrics Collection**: Collects and analyzes metrics such as CPU capacity, memory capacity, power, and cost.
- **Distribution Generation**: Creates and evaluates all possible workload distributions across clusters.
- **Feasibility Checks**: Validates resource requirements to ensure practical distributions.
- **Prometheus Integration**: Exposes detailed metrics for monitoring and visualization.
- **Robust Error Handling**: Implements retry logic and comprehensive error reporting.

## Architecture

The plugin architecture consists of several components:

1. **Metrics Collection**
   - Collects metrics from each cluster including CPU, memory, power, cost, and latency.
   - These metrics form the basis for evaluating different distribution options.

2. **Distribution Generation**
   - Generates all possible ways to distribute workload replicas across available clusters.
   - Eliminates distributions that don't meet resource requirements.

3. **Dynamic Weight Determination**
   - Analyzes workload characteristics to determine appropriate weights for each criterion.
   - Supports customization through annotations.

4. **AHP-Based Scoring**
   - Uses AHP to compare distributions across multiple criteria.
   - Normalizes scores for fair comparison.

5. **Distribution Selection**
   - Selects the highest-scoring distribution as the optimal solution.
   - Ensures proper handling of zero-allocation cases.

## Usage

### Configuration

To enable the DistributionScorer plugin in Karmada, add it to your scheduler configuration:

```yaml
apiVersion: kubescheduler.config.k8s.io/v1
kind: KubeSchedulerConfiguration
...
plugins:
  score:
    enabled:
    - name: DistributionScorer
```

### Dynamic Weight Control

You can control criterion weights through the following annotations on your workload:

- `karmada.io/distribution-power-weight`: Weight for power efficiency (default: 0.25)
- `karmada.io/distribution-cost-weight`: Weight for cost optimization (default: 0.25)
- `karmada.io/distribution-efficiency-weight`: Weight for resource efficiency (default: 0.15)
- `karmada.io/distribution-balance-weight`: Weight for load balancing (default: 0.15)
- `karmada.io/distribution-latency-weight`: Weight for latency optimization (default: 0.20)

Example:
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