package distributionscorer

import (
	"fmt"
)

// GenerateAllDistributions creates all possible ways to distribute replicas across clusters
func GenerateAllDistributions(clusterNames []string, totalReplicas int) []Distribution {
	distributions := []Distribution{}

	// Special case handling
	if totalReplicas < 0 {
		return distributions
	}

	for i := 0; i <= totalReplicas; i++ {
		for j := 0; j <= totalReplicas-i; j++ {
			k := totalReplicas - i - j

			allocation := map[string]int{
				clusterNames[0]: i,
				clusterNames[1]: j,
				clusterNames[2]: k,
			}

			dist := Distribution{
				ID:         fmt.Sprintf("(%d,%d,%d)", i, j, k),
				Allocation: allocation,
				Metrics:    make(map[string]float64),
			}

			distributions = append(distributions, dist)
		}
	}
	return distributions

}
