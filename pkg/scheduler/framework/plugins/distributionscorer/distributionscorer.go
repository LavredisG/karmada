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

	// Possible scenarios: power30, power50, cost30, cost50, latency30, latency50,
	// utilization30, utilization50, proportionality30, proportionality50, balance
	selectedProfile = "balance"
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

	klog.Infof("\033[32mWorkload requires %d replicas, CPU: %d millicores, Memory: %d bytes per replica\033[0m",
		r.totalReplicas, r.cpuPerReplica, r.memoryPerReplica)

	// Collect metrics for this cluster
	metrics := CollectMetrics(cluster)
	klog.Infof("\033[32mDistributionScorer: Collected metrics for cluster %s: %v\033[0m", cluster.Name, metrics.Metrics)

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

	// Calculate metrics for each distribution
	feasibleDistributions := []Distribution{}
	for i := range distributions {
		if CalculateDistributionMetrics(&distributions[i], clusterMetricsMap, r.cpuPerReplica, r.memoryPerReplica) {
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
		Criteria:      getCriteriaForProfile(selectedProfile),
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
	// Update cluster weights based on best distribution's replica allocation
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

	// Send updated scores to the updater service asynchronously
	// NOTICE: THIS CAUSES THE SCORES TO BE UPDATED TWICE
	// TOFIX
	go UpdateClusterWeights(bestDist)

	return framework.NewResult(framework.Success)
}

func getCriteriaForProfile(profile string) map[string]CriteriaConfig {
	// This function should return the criteria configuration based on the selected profile
	switch profile {
	// prioritizes power-efficient allocations
	case "power30":
		return map[string]CriteriaConfig{
			"power":                {HigherIsBetter: false, Weight: 0.300},
			"cost":                 {HigherIsBetter: false, Weight: 0.175},
			"utilization":         {HigherIsBetter: true, Weight: 0.175},
			"proportionality": {HigherIsBetter: false, Weight: 0.175},
			"weighted_latency":     {HigherIsBetter: false, Weight: 0.175},
		}
	case "power50":
		return map[string]CriteriaConfig{
			"power":                {HigherIsBetter: false, Weight: 0.500},
			"cost":                 {HigherIsBetter: false, Weight: 0.125},
			"utilization":         {HigherIsBetter: true, Weight: 0.125},
			"proportionality": {HigherIsBetter: false, Weight: 0.125},
			"weighted_latency":     {HigherIsBetter: false, Weight: 0.125},
		}
	// minimizes monetary cost
	case "cost30":
		return map[string]CriteriaConfig{
			"power":                {HigherIsBetter: false, Weight: 0.175},
			"cost":                 {HigherIsBetter: false, Weight: 0.300},
			"utilization":         {HigherIsBetter: true, Weight: 0.175},
			"proportionality": {HigherIsBetter: false, Weight: 0.175},
			"weighted_latency":     {HigherIsBetter: false, Weight: 0.175},
		}
	case "cost50":
		return map[string]CriteriaConfig{
			"power":                {HigherIsBetter: false, Weight: 0.125},
			"cost":                 {HigherIsBetter: false, Weight: 0.500},
			"utilization":         {HigherIsBetter: true, Weight: 0.125},
			"proportionality": {HigherIsBetter: false, Weight: 0.125},
			"weighted_latency":     {HigherIsBetter: false, Weight: 0.125},
		}
	// prioritizes low-latency clusters 
	case "latency30":
		return map[string]CriteriaConfig{
			"power":                {HigherIsBetter: false, Weight: 0.175},
			"cost":                 {HigherIsBetter: false, Weight: 0.175},
			"utilization":         {HigherIsBetter: true, Weight: 0.175},
			"proportionality": {HigherIsBetter: false, Weight: 0.175},
			"weighted_latency":     {HigherIsBetter: false, Weight: 0.300},
		}
	case "latency50":
		return map[string]CriteriaConfig{
			"power":                {HigherIsBetter: false, Weight: 0.125},
			"cost":                 {HigherIsBetter: false, Weight: 0.125},
			"utilization":  {HigherIsBetter: true, Weight: 0.125},
			"proportionality": {HigherIsBetter: false, Weight: 0.125},
			"weighted_latency":     {HigherIsBetter: false, Weight: 0.500},
		}
	// aims to maximize resource utilization across clusters
	case "utilization30":
		return map[string]CriteriaConfig{
			"power":                {HigherIsBetter: false, Weight: 0.175},
			"cost":                 {HigherIsBetter: false, Weight: 0.175},
			"utilization":         {HigherIsBetter: true, Weight: 0.300},
			"proportionality": {HigherIsBetter: false, Weight: 0.175},
			"weighted_latency":     {HigherIsBetter: false, Weight: 0.175},
		}
	case "utilization50":
		return map[string]CriteriaConfig{
			"power":                {HigherIsBetter: false, Weight: 0.125},
			"cost":                 {HigherIsBetter: false, Weight: 0.125},
			"utilization":         {HigherIsBetter: true, Weight: 0.500},
			"proportionality": {HigherIsBetter: false, Weight: 0.125},
			"weighted_latency":     {HigherIsBetter: false, Weight: 0.125},
		}
	// focuses on balancing load across clusters based on their CPU capacities
	case "proportionality30":
		return map[string]CriteriaConfig{
			"power":                {HigherIsBetter: false, Weight: 0.175},
			"cost":                 {HigherIsBetter: false, Weight: 0.175},
			"utilization":         {HigherIsBetter: true, Weight: 0.175},
			"proportionality": {HigherIsBetter: false, Weight: 0.300},
			"weighted_latency":     {HigherIsBetter: false, Weight: 0.175},
		}
	case "proportionality50":
		return map[string]CriteriaConfig{
			"power":                {HigherIsBetter: false, Weight: 0.125},
			"cost":                 {HigherIsBetter: false, Weight: 0.125},
			"utilization":         {HigherIsBetter: true, Weight: 0.125},
			"proportionality": {HigherIsBetter: false, Weight: 0.500},
			"weighted_latency":     {HigherIsBetter: false, Weight: 0.125},
		}
	// a balanced approach that doesn't overly prioritize any single criterion but
	// balances all criteria equally
	case "balance":
		fallthrough
	default:
		return map[string]CriteriaConfig{
			"power":                {HigherIsBetter: false, Weight: 0.20},
			"cost":                 {HigherIsBetter: false, Weight: 0.20},
			"utilization":         {HigherIsBetter: true, Weight: 0.20},
			"proportionality": {HigherIsBetter: false, Weight: 0.20},
			"weighted_latency":     {HigherIsBetter: false, Weight: 0.20},
		}
	}
}
