import matplotlib.pyplot as plt
import pandas as pd
import numpy as np

# ======================
# CONFIGURABLE PARAMETERS
# ======================
CRITERION = "utilization80"  # ← Change only this!

# Data for utilization80 (obtain these data from karmada/samples/nginx/test_workloads/allocations)
data = [
    # small
    ("small", 5, 5, 0, 0),
    ("small", 10, 8, 2, 0),
    ("small", 15, 15, 0, 0),
    ("small", 20, 4, 16, 0),
    ("small", 25, 24, 1, 0),

    # medium
    ("medium", 5, 4, 1, 0),
    ("medium", 10, 2, 8, 0),
    ("medium", 15, 0, 0, 15),
    ("medium", 20, 1, 4, 15),
    ("medium", 25, 1, 8, 16),

    # large
    ("large", 5, 1, 4, 0),
    ("large", 10, 2, 8, 0),
    ("large", 15, 0, 0, 15),
    ("large", 20, 1, 4, 15),
    ("large", 25, 1, 4, 20),

    # xlarge
    ("xlarge", 5, 0, 1, 4),
    ("xlarge", 10, 0, 2, 8),
    ("xlarge", 15, 1, 2, 12),
    ("xlarge", 20, 1, 4, 15),
    ("xlarge", 25, 1, 4, 20),
]

# ======================
# Auto-generated Labels (Greek)
# ======================
title_map = {
    "latency80": "Latency80",
    "latency60": "Latency60",
    "cost80": "Cost80",
    "cost60": "Cost60",
    "power80": "Power80",
    "power60": "Power60",
    "utilization80": "Utilization80",
    "utilization60": "Utilization60",
    "fairness80": "Fairness80",
    "fairness60": "Fairness60",
    "balance": "Balance"
}

PLOT_TITLE = title_map.get(CRITERION, CRITERION.capitalize())

# File names
pdf_filename = f"{CRITERION}_allocation.pdf"
png_filename = f"{CRITERION}_allocation.png"

# Criterion text (italic Greek)
criterion_text = f"Βασικό\\ κριτήριο: {CRITERION}"

# ======================
# Data Processing
# ======================
df = pd.DataFrame(data, columns=["benchmark", "total_replicas", "edge", "fog", "cloud"])
replica_counts = sorted(df["total_replicas"].unique())
benchmarks = ["small", "medium", "large", "xlarge"]

# Colors
colors = {
    'edge': '#1f77b4',   # blue
    'fog': '#ff7f0e',    # orange
    'cloud': '#2ca02c'   # green
}

# ======================
# Create Subplots
# ======================
fig, axes = plt.subplots(5, 1, figsize=(8, 14), sharex=False)
fig.suptitle(
    f"Κατανομή αντιγράφων μεταξύ Edge/Fog/Cloud - {PLOT_TITLE}",
    fontsize=16, y=0.98
)

for idx, total in enumerate(replica_counts):
    ax = axes[idx]
    subset = df[df["total_replicas"] == total].set_index("benchmark").loc[benchmarks]

    x = np.arange(len(benchmarks))
    edge_vals = subset["edge"].astype(int).values
    fog_vals = subset["fog"].astype(int).values
    cloud_vals = subset["cloud"].astype(int).values

    # Stacked bars
    ax.bar(x, edge_vals, color=colors['edge'], alpha=0.9)
    ax.bar(x, fog_vals, bottom=edge_vals, color=colors['fog'], alpha=0.9)
    ax.bar(x, cloud_vals, bottom=edge_vals + fog_vals, color=colors['cloud'], alpha=0.9)

    # Annotate each segment
    for i in range(len(benchmarks)):
        height = 0
        if edge_vals[i] > 0:
            ax.text(i, height + edge_vals[i]/2, str(edge_vals[i]),
                    ha='center', va='center', color='white', fontweight='bold', fontsize=9)
            height += edge_vals[i]
        if fog_vals[i] > 0:
            ax.text(i, height + fog_vals[i]/2, str(fog_vals[i]),
                    ha='center', va='center', color='white', fontweight='bold', fontsize=9)
            height += fog_vals[i]
        if cloud_vals[i] > 0:
            ax.text(i, height + cloud_vals[i]/2, str(cloud_vals[i]),
                    ha='center', va='center', color='white', fontweight='bold', fontsize=9)

    # Title and formatting
    ax.set_ylabel("# αντιγράφων", fontsize=11)
    ax.set_title(f"{total} αντίγραφα", fontsize=12, pad=10)
    ax.set_ylim(0, total + 0.5)
    ax.grid(axis='y', linestyle='--', alpha=0.4, zorder=0)
    ax.set_xticks(x)
    ax.set_xticklabels(benchmarks)

# Shared X label
axes[-1].set_xlabel("Μέγεθος αντιγράφων", fontsize=12)

# Legend
handles = [
    plt.Rectangle((0,0),1,1, color=colors[tier], alpha=0.9) for tier in ['edge', 'fog', 'cloud']
]
labels = ['Edge', 'Fog', 'Cloud']
fig.legend(
    handles, labels,
    loc='lower center',
    ncol=3,
    bbox_to_anchor=(0.5, 0.02),
    title="Clusters",
    fontsize=11,
    title_fontsize=12
)

# Add main criterion text
fig.text(
    0.5, 0.005,
    rf"$\mathit{{{criterion_text}}}$",
    ha='center',
    fontsize=10,
    style='italic',
    bbox=dict(boxstyle="round,pad=0.3", facecolor="lightgray", alpha=0.5)
)

# Adjust layout
plt.tight_layout(rect=[0, 0.06, 1, 0.96])
plt.subplots_adjust(hspace=0.4)

# Save high-res versions
plt.savefig(pdf_filename, format='pdf', bbox_inches='tight', dpi=300)
# plt.savefig(png_filename, format='png', bbox_inches='tight', dpi=300)

plt.show()