"""Clean the scraped SPECpower data and train a node power model.

Predicts per-node average active power (watts) from CPU utilisation and
hardware features, so the simulator's made-up idle/max watts can be replaced by
data-backed power curves.

Inputs (from scrape_specpower.py and scrape_specpower_detail.py):
  models/data/specpower_summary.csv   hardware per system
  models/data/specpower_detail.csv    watts at each target load per system

Outputs (models/artifacts/):
  cleaning_report.json   rows kept/dropped per rule
  metrics.json           held-out error for each model and split
  power_curves.json      predicted watts vs utilisation for each node type
  power_model.json       the trained XGBoost model

Usage:
  python -I models/train_power_model.py
  (needs numpy, pandas, scikit-learn, xgboost: see models/requirements.txt)
"""
import argparse
import json
import os
import re

import numpy as np
import pandas as pd
import xgboost as xgb
from sklearn.linear_model import Ridge
from sklearn.metrics import mean_absolute_error
from sklearn.model_selection import GroupKFold

SUMMARY = "models/data/specpower_summary.csv"
DETAIL = "models/data/specpower_detail.csv"
OUT_DIR = "models/artifacts"

# Per-system columns that scale with the number of nodes in the system under test.
PER_NODE = ["chips", "cores", "threads", "memory_gb"]

FEATURES = ["utilisation", "chips", "cores", "threads", "mhz", "memory_gb", "year",
            "cores_per_chip", "threads_per_core", "mem_per_core"]
UTIL_IDX = FEATURES.index("utilisation")

# Temporal holdout: train on systems published up to this year, test on later.
TEMPORAL_SPLIT_YEAR = 2022

# Hardware profiles to export curves for. Illustrative node types for the
# simulator; the model interpolates between real systems, so these should be
# within the range of the data (the script warns if they are not).
NODE_TYPES = {
    "small-8c": dict(chips=1, cores=8, threads=16, mhz=2400, memory_gb=32, year=2024),
    "mid-32c": dict(chips=2, cores=32, threads=64, mhz=2400, memory_gb=128, year=2024),
    "large-64c": dict(chips=2, cores=64, threads=128, mhz=2300, memory_gb=256, year=2024),
}
UTIL_GRID = [round(0.1 * i, 1) for i in range(11)]

# The simulator's current placeholder: 100 W idle, 250 W at full load.
PLACEHOLDER_IDLE_W, PLACEHOLDER_MAX_W = 100.0, 250.0


def clean(summary_path, detail_path):
    """Join summary and detail, normalise per node, and drop bad systems.

    Returns (long dataframe, cleaning report dict).
    """
    summary = pd.read_csv(summary_path)
    detail = pd.read_csv(detail_path)
    report = {"summary_rows": len(summary), "detail_systems": int(detail["html_url"].nunique())}

    # Complete curves only: idle plus 10..100%.
    counts = detail.groupby("html_url")["target_load_pct"].nunique()
    complete = counts[counts == 11].index
    report["incomplete_curves_dropped"] = int(detail["html_url"].nunique() - len(complete))
    detail = detail[detail["html_url"].isin(complete)]

    df = detail.merge(summary, on="html_url", how="inner", validate="many_to_one")
    systems = df["html_url"].nunique()

    def drop(mask, name):
        nonlocal df, systems
        bad = set(df.loc[mask, "html_url"])
        df = df[~df["html_url"].isin(bad)]
        report[name] = len(bad)
        systems = df["html_url"].nunique()

    df["year"] = df["date"].astype(str).str.extract(r"(\d{4})", expand=False).astype(float)

    needed = ["nodes", "mhz", "chips", "cores", "threads", "memory_gb", "year", "avg_power_w"]
    drop(df[needed].isna().any(axis=1), "dropped_missing_values")
    drop((df[["nodes", "cores", "chips", "threads", "memory_gb"]] <= 0).any(axis=1), "dropped_nonpositive_hardware")
    drop(~df["mhz"].between(500, 6000), "dropped_implausible_mhz")

    # Whole-system values to per-node values.
    for col in PER_NODE:
        df[col] = df[col] / df["nodes"]
    df["watts"] = df["avg_power_w"] / df["nodes"]
    df["utilisation"] = df["target_load_pct"] / 100.0

    # Power should rise with load: idle below max, and 100% above 50%.
    pivot = df.pivot_table(index="html_url", columns="target_load_pct", values="watts")
    bad_idle = set(pivot.index[pivot[0.0] >= pivot[100.0]])
    bad_shape = set(pivot.index[pivot[100.0] < pivot[50.0]])
    drop(df["html_url"].isin(bad_idle), "dropped_idle_not_below_max")
    drop(df["html_url"].isin(bad_shape), "dropped_power_not_rising")
    drop(df["watts"] <= 0, "dropped_nonpositive_power")

    df["cores_per_chip"] = df["cores"] / df["chips"]
    df["threads_per_core"] = df["threads"] / df["cores"]
    df["mem_per_core"] = df["memory_gb"] / df["cores"]

    report["systems_kept"] = int(df["html_url"].nunique())
    report["rows_kept"] = len(df)
    report["years"] = [int(df["year"].min()), int(df["year"].max())]
    return df.reset_index(drop=True), report


def to_xgb(seed):
    constraints = tuple(1 if i == UTIL_IDX else 0 for i in range(len(FEATURES)))
    return xgb.XGBRegressor(
        n_estimators=400, max_depth=4, learning_rate=0.05, subsample=0.8,
        colsample_bytree=0.9, monotone_constraints=constraints,
        random_state=seed, n_jobs=1,
    )


def fit_predict(kind, train, test, seed):
    """Train `kind` on `train` and return predicted watts for `test`.

    Both models fit log(watts), so error is relative across the 17 W to 1 kW range.
    """
    y = np.log(train["watts"].to_numpy())
    if kind == "xgboost":
        model = to_xgb(seed).fit(train[FEATURES].to_numpy(), y)
        return np.exp(model.predict(test[FEATURES].to_numpy()))
    if kind == "ridge":
        def design(d):
            return np.column_stack([
                d["utilisation"], np.log(d["cores"]), np.log(d["memory_gb"]),
                d["chips"], d["mhz"] / 1000.0, d["year"] - 2007,
            ])
        mean, std = design(train).mean(0), design(train).std(0) + 1e-9
        model = Ridge(alpha=1.0).fit((design(train) - mean) / std, y)
        return np.exp(model.predict((design(test) - mean) / std))
    raise ValueError(kind)


def baseline_predict(kind, test):
    u = test["utilisation"].to_numpy()
    if kind == "placeholder_linear":
        return PLACEHOLDER_IDLE_W + (PLACEHOLDER_MAX_W - PLACEHOLDER_IDLE_W) * u
    if kind == "oracle_linear":
        # Knows each system's true idle and 100% watts, interpolates between.
        idle = test.groupby("html_url")["watts"].transform(lambda w: w[test.loc[w.index, "utilisation"] == 0].iloc[0])
        peak = test.groupby("html_url")["watts"].transform(lambda w: w[test.loc[w.index, "utilisation"] == 1].iloc[0])
        return (idle + (peak - idle) * u).to_numpy()
    raise ValueError(kind)


def score(test, pred):
    y = test["watts"].to_numpy()
    idle = (test["utilisation"] == 0).to_numpy()
    peak = (test["utilisation"] == 1).to_numpy()
    return {
        "mae_w": float(mean_absolute_error(y, pred)),
        "mape_pct": float(np.mean(np.abs(pred - y) / y) * 100),
        "mae_idle_w": float(mean_absolute_error(y[idle], pred[idle])),
        "mae_peak_w": float(mean_absolute_error(y[peak], pred[peak])),
        "rows": int(len(y)),
    }


def evaluate(df, n_splits, seed):
    """Held-out error for each model, split by system and by time."""
    results = {}
    kinds = ["placeholder_linear", "oracle_linear", "ridge", "xgboost"]

    # Grouped k-fold: no system appears in both train and test.
    folds = max(2, min(n_splits, df["html_url"].nunique()))
    preds = {k: np.zeros(len(df)) for k in kinds}
    for train_idx, test_idx in GroupKFold(n_splits=folds).split(df, groups=df["html_url"]):
        train, test = df.iloc[train_idx], df.iloc[test_idx]
        for k in kinds:
            preds[k][test_idx] = baseline_predict(k, test) if k in ("placeholder_linear", "oracle_linear") \
                else fit_predict(k, train, test, seed)
    results["grouped_cv"] = {"folds": folds, **{k: score(df, preds[k]) for k in kinds}}

    # Temporal holdout: older systems train, newer systems test.
    train = df[df["year"] <= TEMPORAL_SPLIT_YEAR]
    test = df[df["year"] > TEMPORAL_SPLIT_YEAR]
    if train["html_url"].nunique() >= 5 and test["html_url"].nunique() >= 2:
        results["temporal_holdout"] = {
            "train_systems": int(train["html_url"].nunique()),
            "test_systems": int(test["html_url"].nunique()),
            "split_year": TEMPORAL_SPLIT_YEAR,
            **{k: score(test, baseline_predict(k, test) if k in ("placeholder_linear", "oracle_linear")
                        else fit_predict(k, train, test, seed)) for k in kinds},
        }
    else:
        results["temporal_holdout"] = "skipped: not enough systems on each side of the split yet"
    return results


def export_curves(df, seed):
    """Fit on all data and predict a power curve for each node type."""
    model = to_xgb(seed).fit(df[FEATURES].to_numpy(), np.log(df["watts"].to_numpy()))

    ranges = {c: (float(df[c].min()), float(df[c].max())) for c in ("cores", "memory_gb", "mhz", "year")}
    curves, warnings = {}, []
    for name, hw in NODE_TYPES.items():
        for c, (lo, hi) in ranges.items():
            if not lo <= hw[c] <= hi:
                warnings.append(f"{name}: {c}={hw[c]} is outside the data range [{lo:g}, {hi:g}]")
        rows = []
        for u in UTIL_GRID:
            rows.append({**hw, "utilisation": u,
                         "cores_per_chip": hw["cores"] / hw["chips"],
                         "threads_per_core": hw["threads"] / hw["cores"],
                         "mem_per_core": hw["memory_gb"] / hw["cores"]})
        pred = np.exp(model.predict(pd.DataFrame(rows)[FEATURES].to_numpy()))
        curves[name] = {"hardware": hw, "utilisation": UTIL_GRID,
                        "watts": [round(float(w), 2) for w in pred]}
    return model, curves, warnings


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--summary", default=SUMMARY)
    ap.add_argument("--detail", default=DETAIL)
    ap.add_argument("--out-dir", default=OUT_DIR)
    ap.add_argument("--folds", type=int, default=5)
    ap.add_argument("--seed", type=int, default=0)
    args = ap.parse_args()

    os.makedirs(args.out_dir, exist_ok=True)
    df, report = clean(args.summary, args.detail)
    print(json.dumps(report, indent=2))
    if report["systems_kept"] < 10:
        raise SystemExit("too few systems after cleaning to train a model")

    metrics = evaluate(df, args.folds, args.seed)
    model, curves, warnings = export_curves(df, args.seed)

    def dump(name, obj):
        with open(os.path.join(args.out_dir, name), "w") as f:
            json.dump(obj, f, indent=2)
            f.write("\n")

    report["warnings"] = warnings
    dump("cleaning_report.json", report)
    dump("metrics.json", metrics)
    dump("power_curves.json", {"utilisation_unit": "fraction of CPU", "power_unit": "watts per node",
                               "node_types": curves})
    model.save_model(os.path.join(args.out_dir, "power_model.json"))

    print("\n== held-out error (watts per node) ==")
    for split in ("grouped_cv", "temporal_holdout"):
        res = metrics[split]
        print(f"[{split}]", res if isinstance(res, str) else "")
        if isinstance(res, dict):
            for k, v in res.items():
                if isinstance(v, dict):
                    print(f"  {k:19s} MAE {v['mae_w']:7.1f} W  MAPE {v['mape_pct']:5.1f}%  "
                          f"idle MAE {v['mae_idle_w']:6.1f}  peak MAE {v['mae_peak_w']:6.1f}")
    print("\n== predicted curves (W at 0%, 50%, 100% utilisation) ==")
    for name, c in curves.items():
        print(f"  {name:10s} {c['watts'][0]:7.1f} {c['watts'][5]:7.1f} {c['watts'][10]:7.1f}")
    for w in warnings:
        print("WARNING:", w)


if __name__ == "__main__":
    main()
