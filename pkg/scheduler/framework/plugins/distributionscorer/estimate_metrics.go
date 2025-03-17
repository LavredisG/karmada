package distributionscorer

import (
	"math"

	"k8s.io/klog/v2"
)

// estimateDistributionMetrics calculates metrics for comparing distributions
func estimateDistributionMetrics(dist *Distribution, clusterMetrics map[string]ClusterMetrics,
	cpuPerReplica, memoryPerReplica int64) bool {

	var totalPower, totalCost float64
	nodesByCluster := make(map[string]float64)

	// Fragmentation factor (20% overhead)
	fragmentationFactor := 1.2

	for clusterName, replicaCount := range dist.Allocation {
		if metrics, exists := clusterMetrics[clusterName]; exists {
			replicas := float64(replicaCount)

			// Handle clusters with no replicas assigned
			if replicaCount == 0 {
				// Include baseline power/cost for idle clusters
				basePower := metrics.Metrics["power"] * 0.3 // 30% base power when idle
				baseCost := metrics.Metrics["cost"] * 0.3   // 30% base cost when idle
				totalPower += basePower
				totalCost += baseCost
				klog.V(4).Infof("Cluster %s is idle (no replicas assigned). Base power: %.2f, Base cost: %.2f", clusterName, basePower, baseCost)
				continue
			}

			// Validate replica requirements against node capacity
			nodeCPUCapacity := metrics.Metrics["node_cpu_capacity"]
			nodeMemoryCapacity := metrics.Metrics["node_memory_capacity"]

			if cpuPerReplica > int64(nodeCPUCapacity) || memoryPerReplica > int64(nodeMemoryCapacity) {
				klog.Warningf("Replica requirements exceed node capacity in cluster %s", clusterName)
				return false // replica requirements exceed node capacity in at least 1 cluster
			}

			// Calculate resource requirements
			totalCPURequired := float64(cpuPerReplica) * replicas
			totalMemoryRequired := float64(memoryPerReplica) * replicas

			// Calculate nodes needed (with fragmentation)
			cpuNodesRequired := totalCPURequired / nodeCPUCapacity
			memNodesRequired := totalMemoryRequired / nodeMemoryCapacity
			nodesRequired := math.Ceil(math.Max(cpuNodesRequired, memNodesRequired) * fragmentationFactor)

			// Enforce maxNodes constraint
			if maxNodes, exists := metrics.Metrics["max_nodes"]; exists {
				if nodesRequired > maxNodes {
					klog.Warningf("Distribution %s is infeasible: Cluster %s cannot accommodate %.1f nodes (max: %.1f)",
						dist.ID, clusterName, nodesRequired, maxNodes)
					return false // Infeasible distribution
				}
			}

			// Store nodes needed per cluster
			nodesByCluster[clusterName] = nodesRequired

			// Calculate power and cost (based on worker nodes only)
			nodePower := metrics.Metrics["power"]
			nodeCost := metrics.Metrics["cost"]
			totalPower += nodePower * nodesRequired
			totalCost += nodeCost * nodesRequired

			klog.V(4).Infof("Cluster %s needs %.1f nodes, power: %.2f, cost: %.2f",
				clusterName, nodesRequired, nodePower*nodesRequired, nodeCost*nodesRequired)
		} else {
			klog.Warningf("No metrics found for cluster %s", clusterName)
			return false // Infeasible distribution
		}
	}

	// Store metrics for feasible distributions
	dist.Metrics["power"] = totalPower
	dist.Metrics["cost"] = totalCost

	for cluster, nodes := range nodesByCluster {
		dist.Metrics["nodes_"+cluster] = nodes
	}

	klog.V(4).Infof("Distribution %s: Power=%.2f, Cost=%.2f", dist.ID, totalPower, totalCost)
	return true // Feasible distribution 
}
