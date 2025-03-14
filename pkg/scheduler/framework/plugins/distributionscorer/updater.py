from flask import Flask, request, jsonify
from kubernetes import client, config
from threading import Lock
import time
import logging
import os
import signal

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = Flask(__name__)
scores_lock = Lock()
cluster_scores = {}
last_update_time = 0
UPDATE_THRESHOLD = int(os.getenv('UPDATE_THRESHOLD', '30'))
SCORE_TIMEOUT = int(os.getenv('SCORE_TIMEOUT', '60'))  # seconds
last_score_time = time.time()

# Kubernetes configuration
KARMADA_CONTEXT = os.getenv('KARMADA_CONTEXT', 'karmada-apiserver')  # Replace with your Karmada context
POLICY_GROUP = "policy.karmada.io"
POLICY_VERSION = "v1alpha1"
POLICY_PLURAL = "propagationpolicies"
NAMESPACE = "default"
POLICY_NAME = "nginx-propagation"

# Configure Kubernetes client
try:
    contexts, active_context = config.list_kube_config_contexts()
    if not contexts:
        logger.error("No Kubernetes contexts found")
        raise RuntimeError("No Kubernetes contexts found")

    # Log available contexts
    logger.info("  - Available contexts:")
    for ctx in contexts:
        logger.info(f"  - {ctx['name']}")

    # Load specific context
    config.load_kube_config(
        config_file="/home/lavredis/.kube/config",
        context=KARMADA_CONTEXT
    )
    custom_api = client.CustomObjectsApi()
    
    # Verify connection
    policy = custom_api.get_namespaced_custom_object(
        group=POLICY_GROUP,
        version=POLICY_VERSION,
        namespace=NAMESPACE,
        plural=POLICY_PLURAL,
        name=POLICY_NAME
    )
    logger.info(f"    Successfully connected to Karmada API using context: {KARMADA_CONTEXT}")

except Exception as e:
    logger.error(f"Failed to configure Kubernetes client: {e}")
    raise


def signal_handler(sig, frame):
    """Handles graceful shutdown."""
    logger.info("Shutting down server...")
    # Could add cleanup code here if needed
    exit(0)

signal.signal(signal.SIGINT, signal_handler)
signal.signal(signal.SIGTERM, signal_handler)

def update_propagation_policy():
    """Updates the PropagationPolicy with new weights based on collected scores."""
    try:
        if not cluster_scores:
            logger.warning("No scores available to update policy")
            return

        # Prepare new weights list with correct format
        weight_list = [
            {
                "targetCluster": {
                    "clusterNames": [cluster]
                },
                "weight": int(score)
            }
            for cluster, score in cluster_scores.items()
        ]
        
        # Create patch for the policy
        patch = {
            "spec": {
                "placement": {
                    "replicaScheduling": {
                        "replicaDivisionPreference": "Weighted",
                        "replicaSchedulingType": "Divided",
                        "weightPreference": {
                            "staticWeightList": weight_list
                        }
                    }
                }
            }
        }
        
        logger.info(f"Attempting to patch policy with weights: {weight_list}")
        
        # Apply the patch
        custom_api.patch_namespaced_custom_object(
            group=POLICY_GROUP,
            version=POLICY_VERSION,
            namespace=NAMESPACE,
            plural=POLICY_PLURAL,
            name=POLICY_NAME,
            body=patch
        )
        logger.info(f"    Successfully updated policy weights: {weight_list}")
        
    except Exception as e:
        logger.error(f"Error updating PropagationPolicy: {e}")
        if hasattr(e, 'status'):
            logger.error(f"Status code: {e.status}")
            if e.status == 404:
                logger.error(f"PropagationPolicy '{POLICY_NAME}' not found in namespace '{NAMESPACE}'")
            elif e.status == 422:
                logger.error("Invalid weight values in patch")


@app.route('/score', methods=['POST'])
def update_score():
    """Receives normalized scores from the ResourceScorer plugin."""
    global last_update_time, last_score_time  # Add last_score_time to global declaration

    data = request.get_json()
    if not data or 'cluster' not in data or 'score' not in data:
        return jsonify({'error': 'Missing cluster or score'}), 400

    cluster = data['cluster']
    score = data['score']

    with scores_lock:
        current_time = time.time()
        if current_time - last_score_time > SCORE_TIMEOUT:
            # Reset scores if too old
            cluster_scores.clear()
            last_score_time = current_time  # Now correctly updates global variable
        cluster_scores[cluster] = score
        app.logger.info(f"    Received score for {cluster}: {score}")

        # Only update policy when we have scores for all clusters
        expected_clusters = {'edge', 'fog', 'cloud'}
        
        if (set(cluster_scores.keys()) == expected_clusters and 
            current_time - last_update_time > UPDATE_THRESHOLD):
            update_propagation_policy()
            last_update_time = current_time
        else:
            app.logger.info(f"    Waiting for all cluster scores. Current scores: {cluster_scores}")
        
    return jsonify({'status': 'success'})


@app.route('/scores', methods=['GET'])
def get_scores():
    """Returns the current scores for each cluster."""
    with scores_lock:
        return jsonify(cluster_scores)


@app.route('/health', methods=['GET'])
def health_check():
    """Returns health status of the API and Karmada connection."""
    try:
        # Verify Karmada connection
        policy = custom_api.get_namespaced_custom_object(
            group=POLICY_GROUP,
            version=POLICY_VERSION,
            namespace=NAMESPACE,
            plural=POLICY_PLURAL,
            name=POLICY_NAME
        )
        return jsonify({
            'status': 'healthy',
            'karmada_connected': True,
            'last_update_time': last_update_time,
            'scores_age': time.time() - last_score_time
        })
    except Exception as e:
        return jsonify({
            'status': 'unhealthy',
            'error': str(e)
        }), 500

if __name__ == '__main__':
    app.run(host='172.18.0.1', port=5000)
e