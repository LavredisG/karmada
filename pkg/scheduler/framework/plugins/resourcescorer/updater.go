package resourcescorer

import (
	"bytes"
	"encoding/json"
	"net/http"

	"k8s.io/klog/v2"
)

const updaterAPIendpoint = "http://172.18.0.1:5000/score"

func sendNormalizedScore(clusterName string, score int64) {
	payload := map[string]interface{}{
		"cluster": clusterName,
		"score":   score,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		klog.Errorf("Failed to marshal normalized score for cluster %s: %v", clusterName, err)
		return
	}

	resp, err := http.Post(updaterAPIendpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		klog.Errorf("Failed to send normalized score for cluster %s to update server: %v", clusterName, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		klog.Errorf("Update server returned non-200 status for cluster %s: %d", clusterName, resp.StatusCode)
	}
}
