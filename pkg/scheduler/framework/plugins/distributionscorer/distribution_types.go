package distributionscorer

// Distribution represents one possible way to distribute replicas

type Distribution struct {
	ID			string				`json:"id"` // Identifier like "(1,2,0)"
	Allocation 	map[string]int	    `json:"allocation"`// Maps cluster name to replica count
	Metrics		map[string]float64	`json:"metrics"`// Estimated metrics for this distribution
}

// DistributionAHPRequest is the request format for AHP service
type DistributionAHPRequest struct {
	Distributions []Distribution			`json:"distributions"`
	Criteria	  map[string]CriteriaConfig `json:"criteria"`
}

// DistributionAHPResponse is the response format from AHP service
type DistributionAHPResponse struct {
	Scores []DistributionScore 				`json:"scores"`
}

// DistributionScore represents the score of a distribution
type DistributionScore struct {
	ID		string		`json:"id"`
	Score	int64		`json:"score"`
}

type CriteriaConfig struct {
	HigherIsBetter bool    `json:"higher_is_better"`
	Weight         float64 `json:"weight"`
}

type ClusterMetrics struct {
	Name    string             `json:"name"`
	Metrics map[string]float64 `json:"metrics"`
}
