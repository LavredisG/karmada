package distributionscorer

import (
	"math"

	"k8s.io/klog/v2"
)

// estimateDistributionMetrics calculates metrics for comparing distributions
func estimateDistributionMetrics(dist *Distribution, clusterMetrics map[string]ClusterMetrics,
	cpuPerReplica, memoryPerReplica int64) {

	// Initialize metrics for this distribution
	var totalPower, totalCost float64
	nodesByCluster := make(map[string]float64)

	// For each cluster in this distribution
	for clusterName, replicaCount := range dist.Allocation {
		if metrics, exists := clusterMetrics[clusterName]; exists {
			replicas := float64(replicaCount)

			// Skip further calculations if no replicas assigned
			if replicaCount == 0 {
				// Consider baseline power for idle clusters
				// totalPower += metrics.Metrics["power"] * 0.3 // 30% base power when idle
				continue
			}

			// Calculate resource requirements
			totalCPURequired := float64(cpuPerReplica) * replicas
			totalMemoryRequired := float64(memoryPerReplica) * replicas

			// Get node capacity for this cluster type
			nodeCPUCapacity := metrics.Metrics["node_cpu_capacity"]
			nodeMemoryCapacity := metrics.Metrics["node_memory_capacity"]

			// Calculate nodes needed for this cluster
			cpuNodesRequired := totalCPURequired / nodeCPUCapacity
			memNodesRequired := totalMemoryRequired / nodeMemoryCapacity
			nodesRequired := math.Ceil(math.Max(cpuNodesRequired, memNodesRequired))

			// Store nodes needed per cluster
			nodesByCluster[clusterName] = nodesRequired

			// Calculate power for this cluster
			// Base power (30%) + active power (70% * nodes required)
			// clusterPower := (metrics.Metrics["power"] * 0.3) +
			//    (metrics.Metrics["power"] * 0.7 * nodesRequired)
			nodePower := metrics.Metrics["power"]
			clusterPower := nodePower * nodesRequired
			totalPower += clusterPower

			// Calculate cost for this cluster (based on nodes)
			nodeCost := metrics.Metrics["cost"]
			clusterCost := nodeCost * nodesRequired
			totalCost += clusterCost

			klog.V(4).Infof("Cluster %s needs %.1f nodes, power: %.2f, cost: %.2f",
				clusterName, nodesRequired, clusterPower, clusterCost)
		} else {
			klog.Warningf("No metrics found for cluster %s", clusterName)
		}
	}

	// Store only the metrics we need for comparison
	dist.Metrics["power"] = totalPower
	dist.Metrics["cost"] = totalCost

	// Store nodes by cluster (useful for reporting)
	for cluster, nodes := range nodesByCluster {
		dist.Metrics["nodes_"+cluster] = nodes
	}

	klog.V(4).Infof("Distribution %s: Power=%.2f, Cost=%.2f",
		dist.ID, totalPower, totalCost)
}
