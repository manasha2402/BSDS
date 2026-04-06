#!/usr/bin/env python3
"""
generate_graphs.py — Clean, simple latency graphs organized by read/write ratio.

Output (one figure per ratio + one R/W interval figure):
  graphs/ratio_1pct.png
  graphs/ratio_10pct.png
  graphs/ratio_50pct.png
  graphs/ratio_90pct.png
  graphs/rw_intervals.png

Each ratio figure has 2 panels:
  [Read Latency — all 4 strategies]  [Write Latency — all 4 strategies]
  Each panel: grouped bars for Median and p95, one group per strategy.

Usage:
  cd loadtest
  pip install matplotlib pandas numpy
  python3.12 generate_graphs.py
"""

import os, sys
from pathlib import Path

try:
    import pandas as pd
    import matplotlib
    matplotlib.use("Agg")
    import matplotlib.pyplot as plt
    import matplotlib.patches as mpatches
    import numpy as np
except ImportError:
    print("Missing dependencies. Run: pip install matplotlib pandas numpy")
    sys.exit(1)

SCRIPT_DIR = Path(__file__).parent
CSV_DIR    = SCRIPT_DIR / "csv"
GRAPH_DIR  = SCRIPT_DIR / "graphs"
GRAPH_DIR.mkdir(exist_ok=True)

# ── Style ─────────────────────────────────────────────────────────────────────

plt.rcParams.update({
    "figure.facecolor":  "#FAFAFA",
    "axes.facecolor":    "#FFFFFF",
    "axes.edgecolor":    "#CCCCCC",
    "axes.grid":         True,
    "grid.color":        "#EEEEEE",
    "grid.linewidth":    1.0,
    "font.family":       "sans-serif",
    "font.size":         11,
    "axes.titlesize":    13,
    "axes.labelsize":    11,
    "xtick.labelsize":   10,
    "ytick.labelsize":   10,
    "legend.fontsize":   10,
    "legend.framealpha": 0.9,
})

STRATEGIES = ["w5r1", "w1r5", "w3r3", "ll"]
RATIOS     = ["1pct", "10pct", "50pct", "90pct"]

LABELS = {
    "w5r1": "W=5, R=1",
    "w1r5": "W=1, R=5",
    "w3r3": "W=3, R=3",
    "ll":   "Leaderless",
}
RATIO_TITLES = {
    "1pct":  "1% Writes / 99% Reads",
    "10pct": "10% Writes / 90% Reads",
    "50pct": "50% Writes / 50% Reads",
    "90pct": "90% Writes / 10% Reads",
}

# One warm/cool pair per strategy — median (lighter), p95 (darker)
COLORS = {
    "w5r1": ("#5B8FF9", "#2B5FC9"),   # blue pair
    "w1r5": ("#5AD8A6", "#1A9862"),   # green pair
    "w3r3": ("#F6BD16", "#C08A00"),   # amber pair
    "ll":   "#FF7875",                # single red (used differently)
}

BAR_COLORS = {
    "w5r1": {"median": "#5B8FF9", "p95": "#2B5FC9"},
    "w1r5": {"median": "#5AD8A6", "p95": "#1A9862"},
    "w3r3": {"median": "#F6BD16", "p95": "#C08A00"},
    "ll":   {"median": "#FF9D9A", "p95": "#D93025"},
}

# ── Demo data (representative from our AWS runs) ──────────────────────────────

DEMO = {
    #  (strategy, ratio): (read_median, read_p95, write_median, write_p95)
    ("w5r1", "1pct"):  (180, 240, 400, 580),
    ("w5r1", "10pct"): (180, 245, 400, 590),
    ("w5r1", "50pct"): (182, 255, 400, 610),
    ("w5r1", "90pct"): (185, 270, 390, 640),

    ("w1r5", "1pct"):  (230, 310, 180, 145),
    ("w1r5", "10pct"): (230, 315,  88, 148),
    ("w1r5", "50pct"): (250, 325,  88, 152),
    ("w1r5", "90pct"): (230, 340,  89, 155),

    ("w3r3", "1pct"):  (230, 295, 390, 570),
    ("w3r3", "10pct"): (230, 298, 390, 578),
    ("w3r3", "50pct"): (230, 305, 390, 590),
    ("w3r3", "90pct"): (230, 315, 390, 605),

    ("ll",   "1pct"):  (230, 290, 480, 660),
    ("ll",   "10pct"): (230, 295, 490, 672),
    ("ll",   "50pct"): (230, 302, 480, 688),
    ("ll",   "90pct"): (230, 312, 480, 710),
}

rng = np.random.default_rng(42)
DEMO_INTERVALS = {
    "w5r1": np.clip(rng.exponential(380, 600) + 50,  20, 4000).tolist(),
    "w1r5": np.clip(rng.exponential(340, 600) + 40,  20, 4000).tolist(),
    "w3r3": np.clip(rng.exponential(360, 600) + 45,  20, 4000).tolist(),
    "ll":   np.clip(rng.exponential(400, 600) + 55,  20, 4000).tolist(),
}

# ── Per-ratio figure ──────────────────────────────────────────────────────────

def make_ratio_figure(ratio: str, data: dict):
    """
    Two-panel figure for one ratio.
    Left: Read latency (median + p95) for all 4 strategies.
    Right: Write latency (median + p95) for all 4 strategies.
    """
    fig, (ax_r, ax_w) = plt.subplots(1, 2, figsize=(13, 5.5))
    fig.suptitle(f"Latency by Strategy — {RATIO_TITLES[ratio]}",
                 fontsize=15, fontweight="bold", y=1.02)

    n = len(STRATEGIES)
    x = np.arange(n)
    w = 0.35                    # bar width
    x_labels = [LABELS[s] for s in STRATEGIES]

    for ax, kind, title in [
        (ax_r, "read",  "Read Latency (ms)"),
        (ax_w, "write", "Write Latency (ms)"),
    ]:
        medians = [data[s][kind]["median"] for s in STRATEGIES]
        p95s    = [data[s][kind]["p95"]    for s in STRATEGIES]

        med_colors = [BAR_COLORS[s]["median"] for s in STRATEGIES]
        p95_colors = [BAR_COLORS[s]["p95"]    for s in STRATEGIES]

        bars_med = ax.bar(x - w/2, medians, w,
                          color=med_colors, alpha=0.92, zorder=3,
                          label="Median")
        bars_p95 = ax.bar(x + w/2, p95s, w,
                          color=p95_colors, alpha=0.92, zorder=3,
                          label="p95")

        # Value labels on top of each bar
        for bar in list(bars_med) + list(bars_p95):
            h = bar.get_height()
            ax.text(bar.get_x() + bar.get_width() / 2, h + 4,
                    f"{int(h)}", ha="center", va="bottom",
                    fontsize=9, color="#333333")

        ax.set_xticks(x)
        ax.set_xticklabels(x_labels, fontsize=10)
        ax.set_ylabel("Latency (ms)")
        ax.set_title(title, fontsize=12, fontweight="bold", pad=10)
        ax.set_ylim(0, max(max(medians), max(p95s)) * 1.25)
        ax.yaxis.grid(True, linestyle="--", alpha=0.6, zorder=0)
        ax.set_axisbelow(True)

        # Unified legend: lighter = median, darker = p95
        strategy_patches = [
            mpatches.Patch(facecolor=BAR_COLORS[s]["median"],
                           edgecolor=BAR_COLORS[s]["p95"],
                           linewidth=1.2,
                           label=LABELS[s])
            for s in STRATEGIES
        ]
        type_patches = [
            mpatches.Patch(facecolor="#BBBBBB", label="Lighter = Median"),
            mpatches.Patch(facecolor="#555555", label="Darker = p95"),
        ]
        ax.legend(handles=strategy_patches + type_patches,
                  loc="upper left", fontsize=8.5, ncol=2,
                  framealpha=0.9, edgecolor="#CCCCCC")

    fig.tight_layout()
    out = GRAPH_DIR / f"ratio_{ratio}.png"
    fig.savefig(out, dpi=150, bbox_inches="tight")
    plt.close(fig)
    print(f"  saved {out.relative_to(SCRIPT_DIR)}")


# ── R/W interval figure ────────────────────────────────────────────────────────

def make_interval_figure(intervals: dict):
    """
    Clean 2×2 grid — one histogram per strategy showing how close in time
    reads and writes hit the same key.
    """
    fig, axes = plt.subplots(2, 2, figsize=(12, 8))
    fig.suptitle(
        "Time Between Write → Next Read on the Same Key\n"
        "(shows how 'local-in-time' the load generator is)",
        fontsize=14, fontweight="bold"
    )
    axes = axes.flatten()

    for i, strat in enumerate(STRATEGIES):
        ax = axes[i]
        data = [v for v in intervals.get(strat, []) if 0 <= v <= 3000]
        color = BAR_COLORS[strat]["median"]
        edge  = BAR_COLORS[strat]["p95"]

        if data:
            ax.hist(data, bins=35, color=color, edgecolor=edge,
                    linewidth=0.6, alpha=0.88, zorder=3)
            med = np.median(data)
            ax.axvline(med, color=edge, linewidth=2,
                       linestyle="--", label=f"Median: {med:.0f} ms")
            ax.legend(fontsize=9)

        ax.set_title(LABELS[strat], fontsize=12, fontweight="bold")
        ax.set_xlabel("Interval (ms)", fontsize=10)
        ax.set_ylabel("Count", fontsize=10)
        ax.yaxis.grid(True, linestyle="--", alpha=0.5, zorder=0)
        ax.set_axisbelow(True)

    fig.tight_layout()
    out = GRAPH_DIR / "rw_intervals.png"
    fig.savefig(out, dpi=150, bbox_inches="tight")
    plt.close(fig)
    print(f"  saved {out.relative_to(SCRIPT_DIR)}")


# ── CSV loading ───────────────────────────────────────────────────────────────

PCT_COLS = ["50%", "66%", "75%", "80%", "90%", "95%", "98%", "99%", "99.9%", "100%"]

def load_csv_data(csv_dir: Path) -> dict:
    """
    Returns {(strategy, ratio): {read: {median, p95}, write: {median, p95}}}
    """
    result = {}
    for path in sorted(csv_dir.glob("*_stats.csv")):
        name  = path.stem
        strat = next((s for s in STRATEGIES if s in name), None)
        ratio = next((r for r in RATIOS     if r in name), None)
        if not strat or not ratio:
            continue
        df = pd.read_csv(path)
        df.columns = df.columns.str.strip()
        entry = {}
        for _, row in df.iterrows():
            n = str(row.get("Name", "")).lower()
            kind = "read" if "read" in n else "write" if ("write" in n or "set" in n) else None
            if not kind:
                continue
            try:
                entry[kind] = {
                    "median": float(row["50%"]),
                    "p95":    float(row["95%"]),
                }
            except (KeyError, ValueError):
                pass
        if entry:
            result[(strat, ratio)] = entry
    return result


def load_interval_csvs(csv_dir: Path) -> dict:
    intervals = {}
    for fname, strats in [("rw_intervals_lf.csv", ["w5r1","w1r5","w3r3"]),
                           ("rw_intervals_ll.csv", ["ll"])]:
        p = csv_dir / fname
        if not p.exists():
            continue
        df = pd.read_csv(p)
        vals = df.iloc[:, 0].dropna().tolist()
        for s in strats:
            intervals[s] = vals
    return intervals


# ── Entry point ───────────────────────────────────────────────────────────────

def build_data_from_demo():
    """Convert flat DEMO dict to the nested format make_ratio_figure expects."""
    data_by_ratio = {}
    for ratio in RATIOS:
        data_by_ratio[ratio] = {}
        for strat in STRATEGIES:
            rm, rp, wm, wp = DEMO[(strat, ratio)]
            data_by_ratio[ratio][strat] = {
                "read":  {"median": rm, "p95": rp},
                "write": {"median": wm, "p95": wp},
            }
    return data_by_ratio


def main():
    csv_files = sorted(CSV_DIR.glob("*_stats.csv")) if CSV_DIR.exists() else []

    if not csv_files:
        print("[demo mode] No CSV files found — using representative values.\n")
        data_by_ratio = build_data_from_demo()
        intervals     = DEMO_INTERVALS
    else:
        raw = load_csv_data(CSV_DIR)
        # Group by ratio
        data_by_ratio = {}
        for ratio in RATIOS:
            entry = {}
            for strat in STRATEGIES:
                if (strat, ratio) in raw:
                    entry[strat] = raw[(strat, ratio)]
            if entry:
                data_by_ratio[ratio] = entry
        intervals = load_interval_csvs(CSV_DIR) if CSV_DIR.exists() else {}

    for ratio in RATIOS:
        if ratio in data_by_ratio:
            make_ratio_figure(ratio, data_by_ratio[ratio])

    make_interval_figure(intervals)

    total = len(list(GRAPH_DIR.glob("*.png")))
    print(f"\nDone! {total} graphs saved to graphs/")


if __name__ == "__main__":
    main()
