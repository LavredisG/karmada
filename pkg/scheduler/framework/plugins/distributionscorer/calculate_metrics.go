package distributionscorer

import (
	"math"

 	"k8s.io/klog/v2"
)

// binPackNodes calculates the number of nodes required to fit the given replicas
// using a simple first-fit decreasing bin packing algorithm, efficient for identical replicas and nodes.
func binPackNodes(replicaCount int, perReplicaCPU, perReplicaMem, nodeCPU, nodeMem float64) int {
	nodes := 0
	remaining := replicaCount
	for remaining > 0 {
		usedCPU := 0.0
		usedMem := 0.0
		fit := 0
		for fit < remaining {
			if usedCPU+perReplicaCPU > nodeCPU || usedMem+perReplicaMem > nodeMem {
				break
			}
			usedCPU += perReplicaCPU
			usedMem += perReplicaMem
			fit++
		}
		nodes++
		remaining -= fit
	}
	return nodes
}

// CalculateDistributionMetrics calculates metrics for comparing distributions
// Returns true if the distribution is feasible, false otherwise. On error, logs the reason.
func CalculateDistributionMetrics(dist *Distribution, clusterMetrics map[string]ClusterMetrics,
	cpuPerReplica, memoryPerReplica int64) bool {

	// klog.V(4).Infof("Calculating metrics for distribution %s", dist.ID)

	var totalPower, totalCost float64
	nodesByCluster := make(map[string]float64)

	totalReplicas := 0
	totalMaxNodes := 0.0

	for clusterName, replicaCount := range dist.Allocation {

		totalReplicas += replicaCount
		totalMaxNodes += float64(clusterMetrics[clusterName].Metrics["max_worker_nodes"])

		if metrics, exists := clusterMetrics[clusterName]; exists {
			// Always add control plane's fixed power/cost (even if no replicas are assigned)
			controlPlanePower := metrics.Metrics["control_plane_power"]
			controlPlaneCost := metrics.Metrics["control_plane_cost"]
			totalPower += controlPlanePower
			totalCost += controlPlaneCost

			// Handle clusters with no replicas assigned
			if replicaCount == 0 {
				// klog.V(4).Infof("Cluster %s is idle (control plane only). Power: %.2f, Cost: %.2f",
					// clusterName, controlPlanePower, controlPlaneCost)
				continue
			}

			// Validate replica requirements against worker node capacity
			workerCPUCapacity := metrics.Metrics["worker_cpu_capacity"]
			workerMemoryCapacity := metrics.Metrics["worker_memory_capacity"]
			maxWorkerNodes := metrics.Metrics["max_worker_nodes"]

			// Check if a single replica exceeds worker node capacity
			if cpuPerReplica > int64(workerCPUCapacity) || memoryPerReplica > int64(workerMemoryCapacity) {
				klog.Warningf("Replica requirements exceed worker node capacity in cluster %s", clusterName)
				return false // Reject distribution
			}

			// Bin-packing calculation for nodes required
			nodesRequired := float64(binPackNodes(
				int(replicaCount),
				float64(cpuPerReplica),
				float64(memoryPerReplica),
				workerCPUCapacity,
				workerMemoryCapacity,
			))

			// Enforce max_worker_nodes constraint
			if nodesRequired > maxWorkerNodes {
				// klog.Warningf("Distribution %s is infeasible: Cluster %s cannot accommodate %.1f worker nodes (max: %.1f)",
					// dist.ID, clusterName, nodesRequired, maxWorkerNodes)
				return false
			}

			// Store worker nodes needed for this cluster
			nodesByCluster[clusterName] = nodesRequired

			// Calculate worker node power and cost
			workerPower := metrics.Metrics["worker_power"]
			workerCost := metrics.Metrics["worker_cost"]
			totalPower += workerPower * nodesRequired
			totalCost += workerCost * nodesRequired

			// klog.V(4).Infof("Cluster %s needs %d worker nodes, power: %.2f, cost: %.2f",
				// clusterName, int(nodesRequired), workerPower*nodesRequired, workerCost*nodesRequired)
		} else {
			klog.Warningf("No metrics found for cluster %s", clusterName)
			return false
		}
	}

	// Store metrics for feasible distributions
	dist.Metrics["power"] = totalPower
	dist.Metrics["cost"] = totalCost

	// Utilization: measures how well resources are packed into nodes.
	// We use the average of CPU and memory utilization per node, which balances both bottlenecks.
	utilization := calculateUtilization(dist, clusterMetrics, cpuPerReplica, memoryPerReplica, nodesByCluster)
	dist.Metrics["utilization"] = math.Floor(utilization*1000) / 1000 // Round to 3 decimal places

	// Load balance: measures how evenly replicas are distributed relative to cluster resource capacity.
	// Uses standard deviation of normalized load ratios (replica% / capacity%)
	loadBalanceStdDev := calculateLoadBalanceStdDev(dist, clusterMetrics, totalReplicas)
	dist.Metrics["load_balance_std_dev"] = math.Floor(loadBalanceStdDev*1000) / 1000 // Round to 3 decimal places

	// Weighted latency: average latency weighted by replica count.
	weightedLatency := calculateWeightedLatency(dist, clusterMetrics)
	dist.Metrics["weighted_latency"] = weightedLatency

	for cluster, nodes := range nodesByCluster {
		dist.Metrics["worker_nodes_"+cluster] = nodes // Use "worker_nodes" prefix for clarity
	}

	// klog.V(4).Infof("\033[32mDistribution %s: Total Power=%.2f, Total Cost=%.2f, Utilization=%.3f, Load Balance StdDev=%.3f, WeightedLatency=%.2f\033[0m",
		// dist.ID, totalPower, totalCost, dist.Metrics["utilization"], dist.Metrics["load_balance_std_dev"], weightedLatency)
	return true // Feasible distribution
}

// calculateUtilization calculates the resource utilization for a distribution.
func calculateUtilization(dist *Distribution, clusterMetrics map[string]ClusterMetrics,
    cpuPerReplica, memoryPerReplica int64, nodesByCluster map[string]float64) float64 {

    totalWeightedUtilization := 0.0
    totalReplicas := 0

    for clusterName, replicaCount := range dist.Allocation {
        if replicaCount == 0 {
            continue
        }

        metrics := clusterMetrics[clusterName]
        workerCPUCapacity := metrics.Metrics["worker_cpu_capacity"]
        workerMemoryCapacity := metrics.Metrics["worker_memory_capacity"]
        nodesRequired := nodesByCluster[clusterName]

        // Calculate resource utilization per node
        cpuUtil := float64(replicaCount) * float64(cpuPerReplica) / (nodesRequired * workerCPUCapacity)
        memUtil := float64(replicaCount) * float64(memoryPerReplica) / (nodesRequired * workerMemoryCapacity)

        // Packing utilization: average of CPU and memory utilization
        packingUtil := (cpuUtil + memUtil) / 2

        // Weight utilization by replica count
        totalWeightedUtilization += packingUtil * float64(replicaCount)
        totalReplicas += replicaCount
    }

    if totalReplicas == 0 {
        return 0.0
    }

    utilization := totalWeightedUtilization / float64(totalReplicas)
    return math.Floor(utilization*1000) / 1000 // Round to 3 decimal places
}

// calculateLoadBalanceStdDev calculates the load balance standard deviation for a distribution.
// This measures how evenly replicas are distributed relative to each cluster's total CPU capacity.
// Lower stddev means more balanced allocation.
func calculateLoadBalanceStdDev(dist *Distribution, clusterMetrics map[string]ClusterMetrics,
	totalReplicas int) float64 {

	// If no replicas, return 0 (perfect balance), not expected as a case
	if totalReplicas == 0 {
		klog.V(5).Infof("No replicas in distribution %s, returning 0 for load balance std dev", dist.ID)
		return 0.0
	}

	// Calculate total CPU capacity across all clusters
	// Memory ratios are the same as CPU ratios for load balancing, so we focus on one of them
	totalCPUCapacity := 0.0
	clusterCPUCapacities := make(map[string]float64)
	for clusterName, metrics := range clusterMetrics {
		maxNodes := metrics.Metrics["max_worker_nodes"]
		workerCPU := metrics.Metrics["worker_cpu_capacity"]
		clusterCPUCapacity := maxNodes * workerCPU
		clusterCPUCapacities[clusterName] = clusterCPUCapacity
		totalCPUCapacity += clusterCPUCapacity
	}

	loadRatios := make([]float64, 0, len(clusterMetrics))

	// Calculate load ratios for ALL clusters in the metrics
	for clusterName := range clusterMetrics {
		// Cluster capacity as a percentage of total capacity
		clusterCPUCapacity := clusterCPUCapacities[clusterName]
		capacityPercentage := clusterCPUCapacity / totalCPUCapacity

		replicaCount := dist.Allocation[clusterName]
		replicaPercentage := float64(replicaCount) / float64(totalReplicas)

		// Calculate normalized load ratio
		// A perfectly balanced distribution would have replicaPercentage == capacityPercentage
		// Deviation from this indicates imbalance
		loadRatio := replicaPercentage / capacityPercentage

		// Calculate load ratio - this is the portion of a cluster's capacity being used
		loadRatios = append(loadRatios, loadRatio)
		klog.V(5).Infof("Cluster %s load ratio: %.3f (replicas: %d/%d = %.2f%%, Resources capacity: %.1f/%.1f = %.2f%%)",
			clusterName, loadRatio, replicaCount, totalReplicas,
			replicaPercentage*100, clusterCPUCapacity, totalCPUCapacity, capacityPercentage*100)
	}

	// The length of loadRatios equals the number of clusters you have (e.g., 3)
	// Each value is a ratio of how loaded that cluster is (nodesRequired/maxNodes)
	stdDev := calculateStandardDeviation(loadRatios)
	klog.V(5).Infof("Load balance std dev for distribution %s: %.3f", dist.ID, stdDev)
	return stdDev
}

// calculateStandardDeviation calculates the standard deviation of a slice of float values.
// Uses population standard deviation, appropriate since all clusters are considered.
func calculateStandardDeviation(loadRatios []float64) float64 {
	if len(loadRatios) <= 1 {
		return 0.0
	}

	// Calculate mean
	sum := 0.0
	for _, v := range loadRatios {
		sum += v
	}
	meanLoadRatio := sum / float64(len(loadRatios))

	// Calculate variance
	sumSquaredDiff := 0.0
	for _, v := range loadRatios {
		diff := v - meanLoadRatio
		sumSquaredDiff += diff * diff
	}
	variance := sumSquaredDiff / float64(len(loadRatios))

	// Return standard deviation
	return math.Sqrt(variance)
}

// calculateWeightedLatency calculates the replica-weighted average latency for a distribution.
// This reflects the average latency experienced by all replicas.
func calculateWeightedLatency(dist *Distribution, clusterMetrics map[string]ClusterMetrics) float64 {
	totalLatencyWeight := 0.0

	// Get total replicas once from the distribution
	totalReplicas := 0
	for _, replicaCount := range dist.Allocation {
		totalReplicas += replicaCount
	}

	// Calculate total latency weight
	for clusterName, replicaCount := range dist.Allocation {
		if replicaCount == 0 {
			continue // Skip clusters with no replicas
		}

		metrics := clusterMetrics[clusterName]
		// Read static latency value from cluster metrics
		latency := metrics.Metrics["latency"]

		// Weight the latency by replica count
		totalLatencyWeight += float64(replicaCount) * latency
	}

	avgLatency := totalLatencyWeight / float64(totalReplicas)
	klog.V(5).Infof("Weighted average latency for distribution %s: %.3f", dist.ID, avgLatency)
	return avgLatency
}
