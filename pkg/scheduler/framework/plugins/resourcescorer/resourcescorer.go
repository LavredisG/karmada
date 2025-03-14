package resourcescorer

import (
	"context"
	"sync"

	clusterv1alpha1 "github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	workv1alpha2 "github.com/karmada-io/karmada/pkg/apis/work/v1alpha2"
	"github.com/karmada-io/karmada/pkg/scheduler/framework"
	"k8s.io/klog/v2"
)

const (
	Name       = "ResourceScorer"
	bytesPerMi = 1024 * 1024
)

type ResourceScorer struct {
	metricsStore sync.Map
}

var _ framework.ScorePlugin = &ResourceScorer{}

func New() (framework.Plugin, error) {
	return &ResourceScorer{
		metricsStore: sync.Map{},
	}, nil
}

func (r *ResourceScorer) Name() string {
	return Name
}

func (r *ResourceScorer) Score(ctx context.Context, spec *workv1alpha2.ResourceBindingSpec,
	cluster *clusterv1alpha1.Cluster) (int64, *framework.Result) {

	metrics := CollectMetrics(cluster)
	klog.Infof("Evaluating cluster %s, collected metrics: %v", cluster.Name, metrics.Metrics)

	r.metricsStore.Store(cluster.Name, metrics)

	// Return preliminary score (MinClusterScore) because final score comes from AHP normalization.
	return framework.MinClusterScore, framework.NewResult(framework.Success)
}

func (r *ResourceScorer) ScoreExtensions() framework.ScoreExtensions {
	return r
}

func (r *ResourceScorer) NormalizeScore(ctx context.Context, scores framework.ClusterScoreList) *framework.Result {
	// Collect all metrics stored
	clusters := make([]ClusterMetrics, 0, len(scores))
	r.metricsStore.Range(func(key, value any) bool {
		clusterMetrics := value.(ClusterMetrics)
		clusters = append(clusters, clusterMetrics)
		// klog.Infof("Collected metrics for cluster %s: %v", key, clusterMetrics.Metrics)
		return true
	})

	// Prepare AHP request with criteria
	request := AHPRequest{
		Clusters: clusters,
		Criteria: map[string]CriteriaConfig{
			"cpu":    {HigherIsBetter: true, Weight: 0.3},
			"memory": {HigherIsBetter: true, Weight: 0.2},
			"power":  {HigherIsBetter: false, Weight: 0.25},
			"cost":   {HigherIsBetter: false, Weight: 0.25},
		},
	}
	klog.Infof("Sending AHP request with criteria: %v, for clusters: %v", request.Criteria, request.Clusters)

	// Send request to AHP server and get back scores
	ahpScores, err := sendToAHPService(request)
	if err != nil {
		klog.Errorf("Failed to send AHP request: %v", err)
		return framework.NewResult(framework.Error)
	}
	klog.Infof("Received AHP scores: %v", ahpScores.Scores)

	// Update cluster scores with AHP results and send normalized scores asynchronously
	for i := range scores {
		for _, s := range ahpScores.Scores {
			if s.Name == scores[i].Cluster.Name {
				klog.Infof("Updating cluster %s with normalized score %d", s.Name, s.Score)
				scores[i].Score = s.Score
				go sendNormalizedScore(s.Name, s.Score)
				break
			}
		}
	}

	return framework.NewResult(framework.Success)
}
