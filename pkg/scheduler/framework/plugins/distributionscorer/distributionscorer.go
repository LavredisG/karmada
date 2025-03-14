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

	klog.Infof("DistributionScorer: Generating distributions for %d replicas across %d clusters",
		totalReplicas, len(clusterNames))

	// Generate all possible distributions
	distributions := GenerateAllDistributions(clusterNames, totalReplicas)
	klog.Infof("DistributionScorer: Generated %d possible distributions", len(distributions))

	// Estimate metrics for each distribution
	for i := range distributions {
		estimateDistributionMetrics(&distributions[i], clusterMetricsMap, r.cpuPerReplica, r.memoryPerReplica)
	}

	// Prepare AHP request
	request := DistributionAHPRequest{
		Distributions: distributions,
		Criteria: map[string]CriteriaConfig{
			// For distributions, higher CPU and Memory values are BETTER (indicating more availability)
			// "cpu":    {HigherIsBetter: true, Weight: 0.3},
			// "memory": {HigherIsBetter: true, Weight: 0.2},
			// Lower power and cost values are better
			"power": {HigherIsBetter: false, Weight: 0.5},
			"cost":  {HigherIsBetter: false, Weight: 0.5},
		},
	}

	// Evaluate distributions
	ahpResponse, err := EvaluateDistributions(request)
	if err != nil {
		klog.Errorf("DistributionScorer: Failed to evaluate distributions: %v", err)
		return framework.NewResult(framework.Error)
	}

	// Find best distribution
	bestDist := FindBestDistribution(distributions, ahpResponse)
	if bestDist == nil {
		klog.Errorf("DistributionScorer: Failed to find best distribution")
		return framework.NewResult(framework.Error)
	}

	klog.Infof("DistributionScorer: Selected best distribution: %s with allocation: %v",
		bestDist.ID, bestDist.Allocation)

	// Update cluster scores based on replica allocation in best distribution
	for i := range scores {
		clusterName := scores[i].Cluster.Name
		replicaCount := bestDist.Allocation[clusterName]

		// Set score based on replica count (simple linear scaling)
		scores[i].Score = int64(replicaCount)

		klog.Infof("DistributionScorer: Set score for cluster %s to %d based on %d replicas",
			clusterName, scores[i].Score, replicaCount)
	}

	// Send updated scores to the updater service asynchronously
	go UpdateClusterScores(bestDist)

	return framework.NewResult(framework.Success)
}
