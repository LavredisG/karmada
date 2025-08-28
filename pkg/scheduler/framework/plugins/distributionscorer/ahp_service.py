from flask import Flask, request, jsonify
import numpy as np
import logging

app = Flask(__name__)
app.logger.setLevel(logging.INFO)

epsilon = 1e-9
RATIO_CAP = 9.0  # cap pairwise ratios (classic AHP bound)

def numeric_rsrv(values, higher_is_better):
    """Computes relative weights for a list of numeric values using pairwise comparisons."""
    n = len(values)
    if n == 0:
        return np.array([])

    # Add epsilon to all values to avoid division by zero
    v = np.array(values, dtype=float) + epsilon

    # Handle the degenerate case where all values are effectively equal
    if np.allclose(v, v[0], rtol=0.0, atol=1e-12):
        return np.ones(n, dtype=float) / n  # Uniform weights

    # Build the pairwise comparison matrix
    if higher_is_better:
        # Higher values are better: matrix[i, j] = v[i] / v[j]
        matrix = v[:, None] / v[None, :]
    else:
        # Lower values are better: matrix[i, j] = v[j] / v[i]
        matrix = v[None, :] / v[:, None]
    
    # Bound dominance: clip pairwise ratios to [1/RATIO_CAP, RATIO_CAP]
    matrix = np.clip(matrix, 1.0 / RATIO_CAP, RATIO_CAP)
    
    # Step 1: Normalize each column
    col_sums = np.sum(matrix, axis = 0)
    col_sums[col_sums == 0.0] = 1.0  # Avoid division by zero
    norm_matrix = matrix / col_sums

    # Step 2: Compute priority vector (row mean)
    weights = np.mean(norm_matrix, axis = 1)

    # Step 3: Normalize weights to sum to 1
    total = np.sum(weights)
    if total > 0:
        weights /= total
    else:
        weights = np.ones(n, dtype=float) / n  # Fallback to uniform weights if total is zero

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
    
    # Create and log tuples sorted by increasing score
    score_tuples = [(score["id"], score["score"]) for score in response["scores"]]
    score_tuples_sorted = sorted(score_tuples, key=lambda x: x[1])  # Sort by score in ascending order
    
    app.logger.info(f"\033[32mFinal distribution response (sorted by increasing score): {score_tuples_sorted}\033[0m")
    # app.logger.info(f"\033[32mFinal distribution response (original order): {response}\033[0m")
    
    return jsonify(response)

if __name__ == '__main__':
    # Listen on port 6000
    app.run(host="172.18.0.1", port=6000)