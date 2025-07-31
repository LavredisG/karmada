package distributionscorer

import (
	"math"

	"k8s.io/klog/v2"
)

// estimateDistributionMetrics calculates metrics for comparing distributions
func EstimateDistributionMetrics(dist *Distribution, clusterMetrics map[string]ClusterMetrics,
	cpuPerReplica, memoryPerReplica int64) bool {

	klog.V(4).Infof("Estimating metrics for distribution %s", dist.ID)

	var totalPower, totalCost float64
	nodesByCluster := make(map[string]float64)

	// Fragmentation factor (20% overhead for worker nodes)
	// Example: node of capacity 4 CPU, if each replica needs 3 CPU and we have 4 replicas
	// then we would need 3 nodes (3+3+3+3) / 4 = 3.0. But a replica can't be split,
	// so we need 4 nodes. The fragmentation factor accounts for this.
	fragmentationFactor := 1.2
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
				klog.V(4).Infof("Cluster %s is idle (control plane only). Power: %.2f, Cost: %.2f",
					clusterName, controlPlanePower, controlPlaneCost)
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

			// Calculate resource requirements for replicas
			replicas := float64(replicaCount)
			totalCPURequired := float64(cpuPerReplica) * replicas
			totalMemoryRequired := float64(memoryPerReplica) * replicas

			// Calculate worker nodes needed (with fragmentation)
			cpuNodesRequired := totalCPURequired / workerCPUCapacity
			memNodesRequired := totalMemoryRequired / workerMemoryCapacity
			nodesRequired := math.Ceil(math.Max(cpuNodesRequired, memNodesRequired) * fragmentationFactor)

			// Enforce max_worker_nodes constraint
			if nodesRequired > maxWorkerNodes {
				klog.Warningf("Distribution %s is infeasible: Cluster %s cannot accommodate %.1f worker nodes (max: %.1f)",
					dist.ID, clusterName, nodesRequired, maxWorkerNodes)
				return false
			}

			// Store worker nodes needed for this cluster
			nodesByCluster[clusterName] = nodesRequired

			// Calculate worker node power and cost
			workerPower := metrics.Metrics["worker_power"]
			workerCost := metrics.Metrics["worker_cost"]
			totalPower += workerPower * nodesRequired
			totalCost += workerCost * nodesRequired

			klog.V(4).Infof("Cluster %s needs %d worker nodes, power: %.2f, cost: %.2f",
				clusterName, int(nodesRequired), workerPower*nodesRequired, workerCost*nodesRequired)
		} else {
			klog.Warningf("No metrics found for cluster %s", clusterName)
			return false
		}
	}

	// Store metrics for feasible distributions
	dist.Metrics["power"] = totalPower
	dist.Metrics["cost"] = totalCost

	// Calculate resource efficiency metric
	resourceEfficiency := calculateResourceEfficiency(dist, clusterMetrics, cpuPerReplica, memoryPerReplica, nodesByCluster)
	dist.Metrics["resource_efficiency"] = math.Floor(resourceEfficiency*1000) / 1000 // Round to 3 decimal places

	// Calculate load balance metric (standard deviation)
	loadBalanceStdDev := calculateLoadBalanceStdDev(dist, clusterMetrics, totalReplicas, totalMaxNodes)
	dist.Metrics["load_balance_std_dev"] = math.Floor(loadBalanceStdDev*1000) / 1000 // Round to 3 decimal places

	weightedLatency := calculateWeightedLatency(dist, clusterMetrics)
	dist.Metrics["weighted_latency"] = weightedLatency

	for cluster, nodes := range nodesByCluster {
		dist.Metrics["worker_nodes_"+cluster] = nodes // Use "worker_nodes" prefix for clarity
	}

	klog.V(4).Infof("\033[32mDistribution %s: Total Power=%.2f, Total Cost=%.2f, Resource Efficiency=%.3f, Load Balance StdDev=%.3f, WeightedLatency=%.2f\033[0m",
		dist.ID, totalPower, totalCost, dist.Metrics["resource_efficiency"], dist.Metrics["load_balance_std_dev"], weightedLatency)
	return true // Feasible distribution
}

// calculateResourceEfficiency calculates the resource efficiency score for a distribution
func calculateResourceEfficiency(dist *Distribution, clusterMetrics map[string]ClusterMetrics,
	cpuPerReplica, memoryPerReplica int64, nodesByCluster map[string]float64) float64 {

	resourceEfficiency := 0.0
	clusterCount := 0

	for clusterName, replicaCount := range dist.Allocation {
		if replicaCount == 0 {
			continue // Skip clusters with no allocation
		}

		metrics := clusterMetrics[clusterName]
		workerCPUCapacity := metrics.Metrics["worker_cpu_capacity"]
		workerMemoryCapacity := metrics.Metrics["worker_memory_capacity"]
		maxWorkerNodes := metrics.Metrics["max_worker_nodes"]
		nodesRequired := nodesByCluster[clusterName]

		// Calculate actual resource usage per node
		cpuUtil := float64(replicaCount) * float64(cpuPerReplica) / (nodesRequired * workerCPUCapacity)
		memUtil := float64(replicaCount) * float64(memoryPerReplica) / (nodesRequired * workerMemoryCapacity)

		// Packing efficiency (how well we use each node)
		packingEff := (cpuUtil + memUtil) / 2

		// Spare capacity (how much room we leave for future allocations)
		// spareCapacity := 1.0 - (nodesRequired / maxWorkerNodes)

		// Combined efficiency: balance packing and spare capacity
		// clusterEff := packingEff * (0.5 + 0.5*spareCapacity)
		clusterEff := packingEff 
		resourceEfficiency += clusterEff
		clusterCount++

		klog.V(4).Infof("Cluster %s: packing_eff=%.2f, combined_eff=%.2f",
			clusterName, packingEff, clusterEff)
		klog.V(4).Infof("Cluster %s: CPU Utilization=%.2f, Memory Utilization=%.2f",
			clusterName, cpuUtil, memUtil)
		klog.V(4).Infof("Cluster %s: Nodes Required=%.2f, Max Worker Nodes=%.2f",
			clusterName, nodesRequired, maxWorkerNodes)
	}

	// log
	klog.V(4).Infof("Total resource efficiency for distribution %s: %.3f", dist.ID, resourceEfficiency/float64(clusterCount))
	return resourceEfficiency / float64(clusterCount)
}

// calculateLoadBalanceStdDev calculates the load balance standard deviation for a distribution
func calculateLoadBalanceStdDev(dist *Distribution, clusterMetrics map[string]ClusterMetrics,
	totalReplicas int, totalMaxNodes float64) float64 {

	// If no replicas, return 0 (perfect balance), not expected as a case
	if totalReplicas == 0 {
		klog.V(5).Infof("No replicas in distribution %s, returning 0 for load balance std dev", dist.ID)
		return 0.0
	}

	loadRatios := make([]float64, 0, len(clusterMetrics))

	// Calculate load ratios for ALL clusters in the metrics
	for clusterName, metrics := range clusterMetrics {
		// Cluster capacity as a percentage of total capacity
		maxNodes := metrics.Metrics["max_worker_nodes"]
		capacityPercentage := maxNodes / totalMaxNodes

		replicaCount := dist.Allocation[clusterName]
		replicaPercentage := float64(replicaCount) / float64(totalReplicas)

		// Calculate normalized load ratio
		// A perfectly balanced distribution would have replicaPercentage == capacityPercentage
		// Deviation from this indicates imbalance
		loadRatio := replicaPercentage / capacityPercentage

		// Calculate load ratio - this is the portion of a cluster's capacity being used
		loadRatios = append(loadRatios, loadRatio)
		klog.V(5).Infof("Cluster %s load ratio: %.3f (replicas: %d/%d = %.2f%%, capacity: %.1f/%.1f = %.2f%%)",
			clusterName, loadRatio, replicaCount, totalReplicas,
			replicaPercentage*100, maxNodes, totalMaxNodes, capacityPercentage*100)
	}

	// The length of loadRatios equals the number of clusters you have (e.g., 3)
	// Each value is a ratio of how loaded that cluster is (nodesRequired/maxNodes)
	stdDev := calculateStandardDeviation(loadRatios)
	klog.V(5).Infof("Load balance std dev for distribution %s: %.3f", dist.ID, stdDev)
	return stdDev
}

// calculateStandardDeviation calculates the standard deviation of a slice of float values
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

// calculateWeightedLatency calculates the replica-weighted average latency for a distribution
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

	// Return weighted average latency
	avgLatency := totalLatencyWeight / float64(totalReplicas)
	klog.V(5).Infof("Weighted average latency for distribution %s: %.3f", dist.ID, avgLatency)
	return avgLatency
}
