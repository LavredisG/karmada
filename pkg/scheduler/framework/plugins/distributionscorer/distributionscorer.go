package distributionscorer

import (
	"context"
	// "net/http"
	"sync"

	clusterv1alpha1 "github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	workv1alpha2 "github.com/karmada-io/karmada/pkg/apis/work/v1alpha2"
	"github.com/karmada-io/karmada/pkg/scheduler/framework"
	"k8s.io/klog/v2"
	// "github.com/prometheus/client_golang/prometheus"
	// "github.com/prometheus/client_golang/prometheus/promhttp"
)

// var (
// 	finalDistribution = prometheus.NewGaugeVec(
// 		prometheus.GaugeOpts{
// 			Name: "final_distribution_allocation",
// 			Help: "Final distribution allocation per cluster",
// 		},
// 		[]string{"cluster"},
// 	)
// 	cpuUsage = prometheus.NewGaugeVec(
// 		prometheus.GaugeOpts{
// 			Name: "cpu_usage_per_cluster",
// 			Help: "CPU usage per cluster in millicores",
// 		},
// 		[]string{"cluster"},
// 	)
// 	costMetrics = prometheus.NewGaugeVec(
// 		prometheus.GaugeOpts{
// 			Name: "cost_metrics_per_cluster",
// 			Help: "Cost metrics per cluster",
// 		},
// 		[]string{"cluster"},
// 	)
// )

// func init() {
// 	prometheus.MustRegister(finalDistribution)
// 	prometheus.MustRegister(cpuUsage)
// 	prometheus.MustRegister(costMetrics)

// 	// Start HTTP server for Prometheus metrics
// 	go func() {
// 		http.Handle("/metrics", promhttp.Handler())
// 		http.ListenAndServe(":8080", nil)
// 	}()
// }

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

	clusterNames := make([]string, len(scores))
	clusterMetricsMap := make(map[string]ClusterMetrics)

	for i, score := range scores {
		clusterName := score.Cluster.Name
		clusterNames[i] = clusterName

		if value, ok := r.metricsStore.Load(clusterName); ok {
			clusterMetricsMap[clusterName] = value.(ClusterMetrics)
		}
	}

	klog.Infof("\033[32mProcessing clusters in order: %v\033[0m", clusterNames)
	
	totalReplicas := int(r.totalReplicas)

	if totalReplicas <= 0 {
		klog.Warning("No replica count found in spec, skipping normalization")
		return framework.NewResult(framework.Success)
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
		if EstimateDistributionMetrics(&distributions[i], clusterMetricsMap, r.cpuPerReplica, r.memoryPerReplica) {
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
			"power":                {HigherIsBetter: false, Weight: 0.05},
			"cost":                 {HigherIsBetter: false, Weight: 0.05},
			"resource_efficiency":  {HigherIsBetter: true, Weight: 0.05},
			"load_balance_std_dev": {HigherIsBetter: false, Weight: 0.05},
			"weighted_latency":     {HigherIsBetter: false, Weight: 0.80},
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
	for _, replicaCount := range bestDist.Allocation {
		if replicaCount == 0 {
			hasZeroAllocations = true
		}
	}
	// Update cluster scores based on best distribution's replica allocation
	for i := range scores {
		clusterName := scores[i].Cluster.Name
		replicaCount := bestDist.Allocation[clusterName]

		if hasZeroAllocations {
			if replicaCount > 0 {
				const multiplier = 1000
				bestDist.Allocation[clusterName] = replicaCount * multiplier
			} else {
				bestDist.Allocation[clusterName] = 1
			}
		} else {
			bestDist.Allocation[clusterName] = replicaCount
		}

		klog.Infof("DistributionScorer: Set weight for cluster %s to %d based on %d replicas",
			clusterName, bestDist.Allocation[clusterName], replicaCount)
	}

	// Update Prometheus metrics
	// for clusterName, replicaCount := range bestDist.Allocation {
	// 	finalDistribution.WithLabelValues(clusterName).Set(float64(replicaCount))
	// 	if metrics, ok := clusterMetricsMap[clusterName]; ok {
	// 		cpuUsage.WithLabelValues(clusterName).Set(metrics.Metrics["worker_cpu_capacity"])
	// 		costMetrics.WithLabelValues(clusterName).Set(metrics.Metrics["worker_cost"])
	// 	}
	// }

	// Send updated scores to the updater service asynchronously
	// NOTICE: THIS CAUSES THE SCORES TO BE UPDATED TWICE
	// TOFIX
	go UpdateClusterScores(bestDist)

	return framework.NewResult(framework.Success)
}
