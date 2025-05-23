from flask import Flask, request, jsonify
import numpy as np
import logging

app = Flask(__name__)
app.logger.setLevel(logging.INFO)

def numeric_rsrv(values, higher_is_better):
    """Computes relative weights for a list of numeric values using pairwise comparisons."""
    n = len(values)
    if n == 0:
        return np.array([])
    # Build a pairwise comparison matrix.
    matrix = np.ones((n, n), dtype=float)
    
    # Define a large number to represent "much better" when comparing against zero
    # for criteria where lower is better
    LARGE_VALUE = 1000000.0
    
    for i in range(n):
        for j in range(n):
            # Special handling for zero values
            if higher_is_better:
                if values[i] == 0 and values[j] > 0:
                    # When higher is better, zero is worst case
                    matrix[i, j] = 1.0 / LARGE_VALUE
                elif values[j] == 0 and values[i] > 0:
                    matrix[i, j] = LARGE_VALUE
                elif values[i] == 0 and values[j] == 0:
                    matrix[i, j] = 1.0  # Equal
                else:
                    matrix[i, j] = values[i] / values[j]
            else:
                # When lower is better, zero is the best case
                if values[i] == 0 and values[j] > 0:
                    matrix[i, j] = LARGE_VALUE
                elif values[j] == 0 and values[i] > 0:
                    matrix[i, j] = 1.0 / LARGE_VALUE
                elif values[i] == 0 and values[j] == 0:
                    matrix[i, j] = 1.0  # Equal
                else:
                    matrix[i, j] = values[j] / values[i]
    
    # Step 1: Normalize each column
    col_sums = np.sum(matrix, axis = 0)
    norm_matrix = matrix / col_sums

    # Step 2: Compute priority vector (row mean)
    weights = np.mean(norm_matrix, axis = 1)

    # Step 3: Normalize weights to sum to 1
    weights /= np.sum(weights)

    return weights



@app.route('/distribution_score', methods=['POST'])
def score_distributions():
    app.logger.info("RECEIVED DISTRIBUTION SCORING REQUEST")
    data = request.get_json()
    distributions = data.get("distributions", [])
    criteria = data.get("criteria", {})
    
    num_distributions = len(distributions)
    # Initialize an array to hold the final score per distribution
    weighted_scores = np.zeros(num_distributions)
    
    # Process each criterion and update the weighted scores
    for crit, config in criteria.items():
        higher_is_better = config.get("higher_is_better", True)
        weight = config.get("weight", 0.0)
        
        # Extract this criterion's values from each distribution
        crit_values = []
        for dist in distributions:
            metrics = dist.get("metrics", {})
            crit_values.append(metrics.get(crit, 0.0))
        
        app.logger.info(f"\033[32mCriterion '{crit}': values = {crit_values}, higher_is_better = {higher_is_better}, weight = {weight}\033[0m")
        
        # Compute relative scores for this criterion
        crit_scores = numeric_rsrv(crit_values, higher_is_better)
        app.logger.info(f"\033[32mCriterion '{crit}': computed priority vector = {crit_scores}\033[0m")
        
        # Update the weighted scores
        weighted_scores += weight * crit_scores
        app.logger.info(f"Updated weighted scores after processing '{crit}': {weighted_scores}")

    # Normalize final scores to a 0-100 scale
    max_score = np.max(weighted_scores)
    if max_score == 0:
        norm_scores = weighted_scores
        app.logger.info("Max score was 0, skipping normalization")
    else:
        norm_scores = (weighted_scores / max_score) * 100
        app.logger.info(f"Normalized scores (0-100): {norm_scores}")
    
    # Build response JSON
    response = {"scores": []}
    for i, dist in enumerate(distributions):
        response["scores"].append({
            "id": dist.get("id", ""),
            "score": int(norm_scores[i])
        })
    
    app.logger.info(f"\033[32mFinal distribution response: {response}\033[0m")
    return jsonify(response)


if __name__ == '__main__':
    # Listen on port 6000
    app.run(host="172.18.0.1", port=6000)
