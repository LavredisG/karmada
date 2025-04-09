package distributionscorer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"k8s.io/klog/v2"
)

const (
	distributionScorerAPIendpoint = "http://172.18.0.1:6000/distribution_score"
)

// EvaluateDistributions sends distribution metrics to AHP service for evaluation
func EvaluateDistributions(request DistributionAHPRequest) (*DistributionAHPResponse, error) {
	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal distribution AHP request: %v", err)
	}

	klog.V(4).Infof("Sending request to AHP service: %s", string(jsonData))
	resp, err := http.Post(distributionScorerAPIendpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to send request to AHP server: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AHP server returned non-200 status: %d", resp.StatusCode)
	}

	var response DistributionAHPResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse AHP response: %v", err)
	}

	return &response, nil
}

// FindBestDistribution finds the distribution with the highest score
func FindBestDistribution(distributions []Distribution, scores *DistributionAHPResponse) *Distribution {
	if scores == nil || len(scores.Scores) == 0 {
		return nil
	}

	var bestDist *Distribution
	var bestScore int64 = -1

	for _, distScore := range scores.Scores {
		klog.V(4).Infof("\033[32mDistribution %s scored %d\033[0m", distScore.ID, distScore.Score)
		if distScore.Score > bestScore {
			bestScore = distScore.Score
			// Find matching distribution
			for i := range distributions {
				if distributions[i].ID == distScore.ID {
					dist := distributions[i] // Create a copy
					bestDist = &dist
					break
				}
			}
		}
	}
	return bestDist
}
