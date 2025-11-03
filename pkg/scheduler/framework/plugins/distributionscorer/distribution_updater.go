package distributionscorer

import (
	"bytes"
	"encoding/json"
	"net/http"

	"k8s.io/klog/v2"
)

const (
	distributionUpdaterEndpoint = "http://172.18.0.1:6001/weights"
)	

// UpdateClusterWeights sends the weights οφ the best distribution to the updater server
func UpdateClusterWeights(distribution *Distribution) {
	if distribution == nil {
		klog.Error("Cannot update cluster weights: nil distribution")
		return
	}

	// For each cluster in the distribution, send its weight (based on replica count)
	for clusterName, replicaCount := range distribution.Allocation {
		// Convert replica count to weight
		weight := int64(replicaCount)

		// Send to updater service
		sendWeight(clusterName, weight)
	}
}

// sendWeight sends a single cluster's weight to the updater service
func sendWeight(clusterName string, weight int64) {
	payload := map[string]interface{}{
		"cluster": clusterName,
		"weight":  weight,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		klog.Errorf("Failed to marshal weight for cluster %s: %v", clusterName, err)
		return
	}

	resp, err := http.Post(distributionUpdaterEndpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		klog.Errorf("Failed to send weight for cluster %s to update server: %v", clusterName, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		klog.Errorf("Update server returned non-200 status for cluster %s: %d", clusterName, resp.StatusCode)
	} else {
		klog.Infof("Successfully sent weight %d for cluster %s", weight, clusterName)
	}
}
