package distributionscorer

import (
	"context"
	"sync"

	clusterv1alpha1 "github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	workv1alpha2 "github.com/karmada-io/karmada/pkg/apis/work/v1alpha2"
	"github.com/karmada-io/karmada/pkg/scheduler/framework"
	"k8s.io/klog/v2"
)

const (
	Name = "DistributionScorer"
)

var _ framework.ScorePlugin = &DistributionScorer{}

type DistributionScorer struct {
	metricsStore     sync.Map
	totalReplicas    int32
	cpuPerReplica    int64 // in millicores
	memoryPerReplica int64 // in bytes
}

// New creates a new DistributionScorer plugin
func New() (framework.Plugin, error) {
	return &DistributionScorer{
		metricsStore: sync.Map{},
	}, nil
}

func (r *DistributionScorer) Name() string {
	return Name
}

// Score collects metrics for each cluster but returns a minimum score
// The real scoring happens in NormalizeScore
func (r *DistributionScorer) Score(ctx context.Context, spec *workv1alpha2.ResourceBindingSpec,
	cluster *clusterv1alpha1.Cluster) (int64, *framework.Result) {

	r.totalReplicas = spec.Replicas

	// Extract CPU and memory requirements per replica
	if spec.ReplicaRequirements != nil {
		if cpu, ok := spec.ReplicaRequirements.ResourceRequest["cpu"]; ok {
			r.cpuPerReplica = cpu.MilliValue()
		}
		if memory, ok := spec.ReplicaRequirements.ResourceRequest["memory"]; ok {
			r.memoryPerReplica = memory.Value()
		}
	}

	klog.Infof("Workload requires %d replicas, CPU: %d millicores, Memory: %d bytes per replica",
		r.totalReplicas, r.cpuPerReplica, r.memoryPerReplica)

	// Collect metrics for this cluster
	metrics := CollectMetrics(cluster)
	klog.Infof("DistributionScorer: Collected metrics for cluster %s: %v", cluster.Name, metrics.Metrics)

	// Store metrics for later use in normalization phase
	r.metricsStore.Store(cluster.Name, metrics)

	// Return minimum score - will be updated during normalization
	return framework.MinClusterScore, framework.NewResult(framework.Success)
}

// ScoreExtensions returns the scorer extension interface
func (r *DistributionScorer) ScoreExtensions() framework.ScoreExtensions {
	return r
}

// NormalizeScore evaluates all possible distributions and assigns scores
func (r *DistributionScorer) NormalizeScore(ctx context.Context, scores framework.ClusterScoreList) *framework.Result {

	klog.Infof("Starting NormalizeScore for %d clusters",
		len(scores))

	clusterNameList := make([]string, len(scores))
	for i, score := range scores {
		clusterNameList[i] = score.Cluster.Name
	}
	klog.Infof("Processing clusters in order: %v", clusterNameList)

	totalReplicas := int(r.totalReplicas)

	if totalReplicas <= 0 {
		klog.Warning("No replica count found in spec, skipping normalization")
		return framework.NewResult(framework.Success)
	}

	// Get cluster names and build cluster metrics map (NOTE: (cloud,edge,fog))
	clusterNames := make([]string, 0, len(scores))
	clusterMetricsMap := make(map[string]ClusterMetrics)

	for _, score := range scores {
		clusterName := score.Cluster.Name
		clusterNames = append(clusterNames, clusterName)

		if value, ok := r.metricsStore.Load(clusterName); ok {
			clusterMetricsMap[clusterName] = value.(ClusterMetrics)
		}
	}

	pluralSuffix := "s"
	if totalReplicas == 1 {
		pluralSuffix = ""
	}

	klog.Infof("DistributionScorer: Generating distributions for %d replica%s across %d clusters",
		totalReplicas, pluralSuffix, len(clusterNames))

	// Generate all possible distributions
	distributions := GenerateAllDistributions(clusterNames, totalReplicas)
	klog.Infof("DistributionScorer: Generated %d possible distributions", len(distributions))

	// Estimate metrics for each distribution
	feasibleDistributions := []Distribution{}
	for i := range distributions {
		if estimateDistributionMetrics(&distributions[i], clusterMetricsMap, r.cpuPerReplica, r.memoryPerReplica) {
			feasibleDistributions = append(feasibleDistributions, distributions[i])
		}
	}

	if len(feasibleDistributions) == 0 {
		klog.Warning("No feasible distributions found")
		return framework.NewResult(framework.Error)
	}

	// Prepare AHP request
	request := DistributionAHPRequest{
		Distributions: feasibleDistributions,
		Criteria: map[string]CriteriaConfig{
			"power":                {HigherIsBetter: false, Weight: 0.25},
			"cost":                 {HigherIsBetter: false, Weight: 0.25},
			"resource_efficiency":  {HigherIsBetter: true, Weight: 0.15},
			"load_balance_std_dev": {HigherIsBetter: false, Weight: 0.15},
			"weighted_latency":     {HigherIsBetter: false, Weight: 0.20},
		},
	}

	// Evaluate distributions
	ahpResponse, err := EvaluateDistributions(request)
	if err != nil {
		klog.Errorf("DistributionScorer: Failed to evaluate distributions: %v", err)
		return framework.NewResult(framework.Error)
	}

	// Find best distribution
	bestDist := FindBestDistribution(feasibleDistributions, ahpResponse)
	if bestDist == nil {
		klog.Errorf("DistributionScorer: Failed to find best distribution")
		return framework.NewResult(framework.Error)
	}

	klog.Infof("DistributionScorer: Selected best distribution: %s with allocation: %v",
		bestDist.ID, bestDist.Allocation)

	// For Allocations where a cluster would get weight of 0 (no replicas)
	// we instead assign it a weight of 1, but multiply the rest by a big constant
	hasZeroAllocations := false
	maxReplicas := 0

	for _, replicaCount := range bestDist.Allocation {
		if replicaCount == 0 {
			hasZeroAllocations = true
		}
		if replicaCount > maxReplicas {
			maxReplicas = replicaCount
		}
	}

	// Update cluster scores based on replica allocation in best distribution
	for i := range scores {
		clusterName := scores[i].Cluster.Name
		replicaCount := bestDist.Allocation[clusterName]

		if hasZeroAllocations {
			if replicaCount > 0 {
				const multiplier = 1000
				// scores[i].Score = int64(maxReplicas * multiplier)
				bestDist.Allocation[clusterName] = replicaCount * multiplier
			} else {
				// scores[i].Score = 1
				bestDist.Allocation[clusterName] = 1
			}
		} else {
			// scores[i].Score = int64(replicaCount)
			bestDist.Allocation[clusterName] = replicaCount
		}

		// klog.Infof("DistributionScorer: Set score for cluster %s to %d based on %d replicas",
		// 	clusterName, scores[i].Score, replicaCount)

		klog.Infof("DistributionScorer: Set allocation for cluster %s to %d based on %d replicas",
			clusterName, bestDist.Allocation[clusterName], replicaCount)
	}

	// Send updated scores to the updater service asynchronously
	// NOTICE: THIS CAUSES THE SCORES TO BE UPDATED TWICE
	// TOFIX
	go UpdateClusterScores(bestDist)

	return framework.NewResult(framework.Success)
}
