package distributionscorer

import (
	"bytes"
	"encoding/json"
	"net/http"

	"k8s.io/klog/v2"
)

const (
	distributionUpdaterEndpoint = "http://172.18.0.1:5000/score"
)

// UpdateClusterScores sends the scores from the best distribution to the updater server
func UpdateClusterScores(distribution *Distribution) {
	if distribution == nil {
		klog.Error("Cannot update cluster scores: nil distribution")
		return
	}

	// For each cluster in the distribution, send its score (based on replica count)
	for clusterName, replicaCount := range distribution.Allocation {
		// Convert replica count to score (simple multiplication)
		score := int64(replicaCount)

		// Send to updater service
		sendScore(clusterName, score)
	}
}

// sendScore sends a single cluster's score to the updater service
func sendScore(clusterName string, score int64) {
	payload := map[string]interface{}{
		"cluster": clusterName,
		"score":   score,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		klog.Errorf("Failed to marshal score for cluster %s: %v", clusterName, err)
		return
	}

	resp, err := http.Post(distributionUpdaterEndpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		klog.Errorf("Failed to send score for cluster %s to update server: %v", clusterName, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		klog.Errorf("Update server returned non-200 status for cluster %s: %d", clusterName, resp.StatusCode)
	} else {
		klog.Infof("Successfully sent score %d for cluster %s", score, clusterName)
	}
}
