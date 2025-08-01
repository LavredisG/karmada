package distributionscorer

import (
	"strconv"

	clusterv1alpha1 "github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
)

func CollectMetrics(cluster *clusterv1alpha1.Cluster) ClusterMetrics {
	metrics := make(map[string]float64)

	if cpu, exists := cluster.Labels["worker_cpu_capacity"]; exists {
		if cpuf, err := strconv.ParseFloat(cpu, 64); err == nil {
			metrics["worker_cpu_capacity"] = cpuf
		}
	}

	if memory, exists := cluster.Labels["worker_memory_capacity"]; exists {
		if memoryf, err := strconv.ParseFloat(memory, 64); err == nil {
			metrics["worker_memory_capacity"] = memoryf
		}
	}

	if power, exists := cluster.Labels["control_plane_power"]; exists {
		if powerf, err := strconv.ParseFloat(power, 64); err == nil {
			metrics["control_plane_power"] = powerf
		}
	}

	if cost, exists := cluster.Labels["control_plane_cost"]; exists {
		if costf, err := strconv.ParseFloat(cost, 64); err == nil {
			metrics["control_plane_cost"] = costf
		}
	}

	if power, exists := cluster.Labels["worker_power"]; exists {
		if powerf, err := strconv.ParseFloat(power, 64); err == nil {
			metrics["worker_power"] = powerf
		}
	}

	if cost, exists := cluster.Labels["worker_cost"]; exists {
		if costf, err := strconv.ParseFloat(cost, 64); err == nil {
			metrics["worker_cost"] = costf
		}
	}

	if maxNodes, exists := cluster.Labels["max_worker_nodes"]; exists {
		if maxNodesf, err := strconv.ParseFloat(maxNodes, 64); err == nil {
			metrics["max_worker_nodes"] = maxNodesf
		}
	}

	if latency, exists := cluster.Labels["latency"]; exists {
		if latencyf, err := strconv.ParseFloat(latency, 64); err == nil {
			metrics["latency"] = latencyf
		}
	}

	return ClusterMetrics{
		Name:    cluster.Name,
		Metrics: metrics,
	}
}
