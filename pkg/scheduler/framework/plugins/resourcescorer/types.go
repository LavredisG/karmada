package resourcescorer

type ClusterMetrics struct {
	Name    string             `json:"name"`
	Metrics map[string]float64 `json:"metrics"`
}

type CriteriaConfig struct {
	HigherIsBetter bool    `json:"higher_is_better"`
	Weight         float64 `json:"weight"`
}

type AHPRequest struct {
	Clusters []ClusterMetrics          `json:"clusters"`
	Criteria map[string]CriteriaConfig `json:"criteria"`
}

type AHPResponse struct {
	Scores []ClusterScore `json:"scores"`
}

type ClusterScore struct {
	Name  string `json:"name"`
	Score int64  `json:"score"`
}
