Karmada is an open source framework for orchestrating the lifecycle of Kubernetes resources across multiple clusters. It provides a unified control plane to manage and deploy applications in a multi-cluster environment, enabling high availability, disaster recovery, and resource optimization.

# Karmada Copilot Instructions
Welcome to the Karmada Copilot Instructions! This document provides guidance on how to effectively use GitHub Copilot to assist you in working with Karmada, an open-source project for multi-cluster Kubernetes management.

## Getting Started with Karmada
If you're new to Karmada, start by familiarizing yourself with the official documentation and installation guides. Understanding the basics of Karmada will help you make the most out of GitHub Copilot's suggestions.

- [Karmada Documentation](https://karmada.io/docs/)
- [Karmada GitHub Repository](https://github.com/karmada-io/karmada)
- [Karmada Installation Guide](https://karmada.io/docs/installation/)
- [Karmada Concepts](https://karmada.io/docs/concepts/)

## Using GitHub Copilot with Karmada
GitHub Copilot can assist you in writing code, configuration files, and documentation related to Karmada. Here are some tips to get the most out of Copilot:
1. **Contextual Prompts**: Provide clear and specific comments or code snippets to guide Copilot in generating relevant suggestions. For example, if you're writing a Karmada deployment configuration, start with a comment like `# Karmada deployment configuration for multi-cluster application`.
2. **Iterative Refinement**: Use Copilot's suggestions as a starting point and refine the code to fit your specific use case. Don't hesitate to modify or expand upon the generated code.
3. **Explore Examples**: Look at existing Karmada examples and configurations in the official repository. You can use these as references to guide Copilot in generating similar code.
4. **Leverage Documentation**: When writing documentation or comments, Copilot can help you draft clear explanations. Use it to generate initial drafts and then refine them for clarity and accuracy.
5. **Stay Updated**: Karmada is an evolving project. Keep an eye on the latest updates and features in the Karmada repository to ensure that your code and configurations are up-to-date.

## Example Prompts for Karmada
Here are some example prompts you can use with GitHub Copilot to get started with Karm
ada:
- `# Create a Karmada deployment for a multi-cluster application`
- `# Write a Karmada configuration file for cluster federation`
- `# Document the steps to install Karmada on a Kubernetes cluster`
- `# Generate a Karmada policy for resource scheduling across clusters`
- `# Create a Karmada controller for managing multi-cluster resources`


## Karmada Scheduler Framework
Karmada provides a flexible scheduler framework that allows users to customize the scheduling behavior of resources across multiple clusters. The scheduler framework is designed to be extensible, enabling users to implement their own scheduling algorithms and policies.
- [Karmada Scheduler Framework Documentation](https://karmada.io/docs/concepts/scheduler/)

Multiple plugins are available in Karmada to enhance the scheduling capabilities under the directory `pkg/scheduler/framework/plugins'. This project aims to introduce a new plugin to the Karmada scheduler framework. The new plugin will implement a custom scheduling algorithm that optimizes resource allocation across multiple clusters based on specific criteria. The plugin will be designed to be easily integrated into the existing Karmada scheduler framework, allowing users to leverage its functionality without significant modifications to their existing setup. The plugin will be developed using Go programming language and will follow the best practices and coding standards of the Karmada project. The development process will involve the following steps:
1. **Research and Analysis**: Conduct a thorough analysis of the existing Karmada scheduler framework and identify the key components and interfaces that need to be implemented for the new plugin.
2. **Design and Architecture**: Define the architecture and design of the new plugin, including the data structures, algorithms, and interfaces that will be used.
3. **Implementation**: Write the code for the new plugin, following the design and architecture defined in the previous step. Ensure that the code is well-documented and adheres to the coding standards of the Karmada project.
4. **Testing and Validation**: Develop a comprehensive test suite to validate the functionality and performance of the new plugin. Conduct unit tests, integration tests, and performance tests to ensure that the plugin works as expected.
5. **Documentation**: Create detailed documentation for the new plugin, including installation instructions, configuration options, and usage examples. Ensure that the documentation is clear and easy to understand for users of the Karmada project.

The implementation of the custom plugin is located in the directory `pkg/scheduler/framework/plugins/distributionscorer`. The main file there is `distributionscorer.go`, which contains the core logic for the plugin. It starts by collecting metrics from all clusters, such as CPU and memory usage, to understand the current resource distribution. The plugin then evaluates each cluster based on these metrics, scoring them according to how well they can accommodate new workloads while maintaining balance across the clusters.

The plugin is configurable via CriteriaConfig, which are essentially weighted criteria to be used by the AHP algorithm, implemented at `pkg/scheduler/framework/plugins/distributionscorer/ahp_service.py`. The AHP algorithm helps in making decisions based on multiple criteria, ensuring that the scheduling decisions are well-balanced and optimized for the overall system performance.


The testing environment cosists of three Kubernetes clusters, each with different resource capacities and workloads. The clusters are set up to simulate a real-world multi-cluster environment, allowing for comprehensive testing of the plugin's functionality. The collected metrics of each clusters are read as labels and their values are as follows:

Edge labels:

kubectl label cluster edge worker_cpu_capacity=2000
kubectl label cluster edge worker_memory_capacity=4294967296
kubectl label cluster edge control_plane_power=40
kubectl label cluster edge control_plane_cost=60
kubectl label cluster edge worker_power=40
kubectl label cluster edge worker_cost=60
kubectl label cluster edge max_worker_nodes=4
kubectl label cluster edge latency=10

Fog labels:

kubectl label cluster fog worker_cpu_capacity=4000
kubectl label cluster fog worker_memory_capacity=8589934592
kubectl label cluster fog control_plane_power=30
kubectl label cluster fog control_plane_cost=45
kubectl label cluster fog worker_power=70
kubectl label cluster fog worker_cost=100
kubectl label cluster fog max_worker_nodes=8
kubectl label cluster fog latency=25

Cloud labels:

kubectl label cluster cloud worker_cpu_capacity=8000
kubectl label cluster cloud worker_memory_capacity=17179869184
kubectl label cluster cloud control_plane_power=15
kubectl label cluster cloud control_plane_cost=30
kubectl label cluster cloud worker_power=100
kubectl label cluster cloud worker_cost=140
kubectl label cluster cloud max_worker_nodes=16
kubectl label cluster cloud latency=50

The plugin is tested by deploying a sample application across the three clusters and observing how the plugin distributes the workload based on the defined criteria. The results are analyzed to ensure that the plugin effectively balances the resource allocation and optimizes the overall performance of the multi-cluster environment.

On the `samples/nginx/test_workloads` directory under `deployment_workloads`, there are benchmark deployments ranging from small (low resource requirements per pod) to xlarge (high resource requirements per pod) and from 5 to 25 replicas. These deployments can be used to test the functionality of the plugin and observe how it distributes the workloads across the clusters based on their resource capacities and the defined criteria.