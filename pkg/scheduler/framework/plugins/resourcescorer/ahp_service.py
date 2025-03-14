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
    for i in range(n):
        for j in range(n):
            # Avoid division by zero
            if values[j] == 0 or values[i] == 0:
                matrix[i, j] = 1.0
            else:
                if higher_is_better:
                    matrix[i, j] = values[i] / values[j]
                else:
                    matrix[i, j] = values[j] / values[i]
    # Compute row sums.
    row_sums = np.sum(matrix, axis=1)
    total = np.sum(row_sums)
    # Normalize row sums to get weights.
    weights = row_sums / total
    return weights

@app.route('/score', methods=['POST'])
def score():
    app.logger.info("RECEIVED POST REQUEST.")
    data = request.get_json()
    clusters = data.get("clusters", [])
    criteria = data.get("criteria", {})
    
    num_clusters = len(clusters)
    # Initialize an array to hold the final score per cluster.
    weighted_scores = np.zeros(num_clusters)
    
    # Process each criterion and update the weighted scores.
    for crit, config in criteria.items():
        higher_is_better = config.get("higher_is_better", True)
        weight = config.get("weight", 0.0)
        
        # Extract this criterion's values from each cluster.
        crit_values = []
        for cluster in clusters:
            metrics = cluster.get("metrics", {})
            crit_values.append(metrics.get(crit, 0.0))
        
        app.logger.info(f"Criterion '{crit}': values = {crit_values}, higher_is_better = {higher_is_better}, weight = {weight}")
        
        # Compute relative scores for this criterion.
        crit_scores = numeric_rsrv(crit_values, higher_is_better)
        app.logger.info(f"Criterion '{crit}': computed relative scores = {crit_scores}")
        
        # Update the weighted scores.
        weighted_scores += weight * crit_scores
        app.logger.info(f"Updated weighted scores after processing '{crit}': {weighted_scores}")

    # Normalize final scores to a 0-100 scale.
    max_score = np.max(weighted_scores)
    if max_score == 0:
        norm_scores = weighted_scores
        app.logger.info("Max score was 0, skipping normalization.")
    else:
        norm_scores = (weighted_scores / max_score) * 100
        app.logger.info(f"Normalized scores (0-100): {norm_scores}")
    
    # Build response JSON.
    response = {"scores": []}
    for i, cluster in enumerate(clusters):
        response["scores"].append({
            "name": cluster.get("name", ""),
            "score": int(norm_scores[i])
        })
    
    app.logger.info(f"Final response: {response}")
    return jsonify(response)

if __name__ == '__main__':
    app.run(host="172.18.0.1", port=6000)