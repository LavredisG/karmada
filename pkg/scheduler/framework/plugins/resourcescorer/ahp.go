package resourcescorer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	scorerAPIendpoint = "http://172.18.0.1:6000/score"
)

func sendToAHPService(request AHPRequest) (*AHPResponse, error) {
	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal AHP request: %v", err)
	}

	resp, err := http.Post(scorerAPIendpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to send request to AHP server: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AHP server returned non-200 status: %d", resp.StatusCode)
	}

	var response AHPResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse AHP response: %v", err)
	}

	return &response, nil
}
