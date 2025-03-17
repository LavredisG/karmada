package distributionscorer	

import (
	"strconv"

	clusterv1alpha1 "github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
)

func CollectMetrics(cluster *clusterv1alpha1.Cluster) ClusterMetrics {
	metrics := make(map[string]float64)

	// Calculate CPU availability ratio.
	// cpuTotal := float64(cluster.Status.ResourceSummary.Allocatable.Cpu().MilliValue())
	// cpuUsed := float64(cluster.Status.ResourceSummary.Allocated.Cpu().MilliValue())
	// metrics["cpu"] = (cpuTotal - cpuUsed) / cpuTotal

	// Calculate memory availability ratio.
	// memTotal := float64(cluster.Status.ResourceSummary.Allocatable.Memory().Value())
	// memUsed := float64(cluster.Status.ResourceSummary.Allocated.Memory().Value())
	// metrics["memory"] = (memTotal - memUsed) / memTotal

	// Get power from cluster labels. If missing, default to 100.

	if cpu, exists := cluster.Labels["node_cpu_capacity"]; exists {
		if cpuf, err := strconv.ParseFloat(cpu, 64); err == nil {
			metrics["node_cpu_capacity"] = cpuf
		}
	}

	if memory, exists := cluster.Labels["node_memory_capacity"]; exists {
		if memoryf, err := strconv.ParseFloat(memory, 64); err == nil {
			metrics["node_memory_capacity"] = memoryf
		}
	}

	power := 100.0
	if p, exists := cluster.Labels["power"]; exists {
		if pf, err := strconv.ParseFloat(p, 64); err == nil {
			power = pf
		}
	}
	metrics["power"] = power

	
	cost := 1.0
	if c, exists := cluster.Labels["cost"]; exists {
		if cf, err := strconv.ParseFloat(c, 64); err == nil {
			cost = cf
		}
	}
	metrics["cost"] = cost

	maxNodes := 10000.0
	if maxNodesString, exists := cluster.Labels["max_nodes"]; exists {
		if maxNodesf, err := strconv.ParseFloat(maxNodesString, 64); err == nil {
			maxNodes = maxNodesf
		}
	}
	metrics["max_nodes"] = maxNodes

	return ClusterMetrics{
		Name:    cluster.Name,
		Metrics: metrics,
	}
}
