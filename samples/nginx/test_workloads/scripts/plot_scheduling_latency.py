import pandas as pd
import matplotlib.pyplot as plt
from scipy.stats import pearsonr
import re
import os

replicas_list = [5, 10, 15, 20, 25]
sizes = ["small", "medium", "large", "xlarge"]
custom_dir = "results_custom"
baseline_dir = "results_baseline"

def extract_median_arrow(val):
    m = re.search(r'->\s*(\d+)', str(val))
    return int(m.group(1)) if m else None

def load_summary(path):
    df = pd.read_csv(path)
    df['replicas'] = df['deployment'].str.extract(r'-(\d+)$').astype(int)
    df['size'] = df['deployment'].str.extract(r'benchmark-(\w+)-')
    df['ms'] = df['median_s'].apply(extract_median_arrow)
    return df

def load_all_runs(dir_path, size, prefix):
    all_replicas = []
    all_latencies = []
    for r in replicas_list:
        csv_path = f"{dir_path}/{prefix}benchmark-{size}-{r}.csv"
        if not os.path.exists(csv_path):
            print(f"Warning: {csv_path} not found.")
            continue
        df = pd.read_csv(csv_path)
        if 'latency_seconds' in df.columns:
            all_replicas.extend([r]*len(df))
            all_latencies.extend(df['latency_seconds'].astype(float).values * 1000)  # convert to ms
    return all_replicas, all_latencies

custom_summary = load_summary(f"{custom_dir}/custom_summary.csv")
baseline_summary = load_summary(f"{baseline_dir}/baseline_summary.csv")

for size in sizes:
    custom_medians = [custom_summary[(custom_summary['size'] == size) & (custom_summary['replicas'] == r)]['ms'].values[0]
                      if not custom_summary[(custom_summary['size'] == size) & (custom_summary['replicas'] == r)].empty else None
                      for r in replicas_list]
    baseline_medians = [baseline_summary[(baseline_summary['size'] == size) & (baseline_summary['replicas'] == r)]['ms'].values[0]
                        if not baseline_summary[(baseline_summary['size'] == size) & (baseline_summary['replicas'] == r)].empty else None
                        for r in replicas_list]

    custom_replicas, custom_latencies = load_all_runs(custom_dir, size, prefix="custom-")
    baseline_replicas, baseline_latencies = load_all_runs(baseline_dir, size, prefix="baseline-")

    plt.figure(figsize=(8, 6))
    plt.plot(replicas_list, custom_medians, marker='o', color='blue', label='Custom Median')
    plt.plot(replicas_list, baseline_medians, marker='o', color='gray', label='Baseline Median')
    plt.scatter(custom_replicas, custom_latencies, color='blue', alpha=0.5, label='Custom Data')
    plt.scatter(baseline_replicas, baseline_latencies, color='gray', alpha=0.5, label='Baseline Data')
    plt.xlabel("Number of Replicas")
    plt.ylabel("Scheduling Time (ms)")
    plt.title(f"Scheduling Latency for '{size}' Deployments")
    plt.legend()
    plt.grid(True, linestyle='--', alpha=0.5)
    plt.tight_layout()
    plt.savefig(f"{custom_dir}/scheduling_latency_{size}_full.pdf")
    plt.close()

    if len(custom_replicas) > 1 and len(custom_latencies) > 1:
        corr_custom, _ = pearsonr(custom_replicas, custom_latencies)
        print(f"Pearson correlation for custom ({size}): {corr_custom:.3f}")
    else:
        print(f"Not enough custom data for Pearson correlation ({size}).")
    if len(baseline_replicas) > 1 and len(baseline_latencies) > 1:
        corr_baseline, _ = pearsonr(baseline_replicas, baseline_latencies)
        print(f"Pearson correlation for baseline ({size}): {corr_baseline:.3f}")
    else:
        print(f"Not enough baseline data for Pearson correlation ({size}).")