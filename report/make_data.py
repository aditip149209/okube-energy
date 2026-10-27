"""Generate the LaTeX tables, plot data and headline-number macros for the
report and slides from the experiment CSVs and the power-model JSON, so the
prose cannot drift from the data.

  python3 -I report/make_data.py

Reads : results/sim/{summary,sweep}.csv, models/artifacts/{metrics,cleaning_report}.json
Writes: report/generated/*
"""
import csv
import json
import math
import os

ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..")
OUT = os.path.join(ROOT, "report", "generated")
HEADLINE_W = "1.5"  # chosen on seeds 1-30; see results/sim/README.md


def rows(name):
    with open(os.path.join(ROOT, "results", "sim", name), newline="") as f:
        return list(csv.DictReader(f))


def fnum(r, k):
    return float(r[k])


def find(rs, workload, config, weight):
    m = [r for r in rs if r["workload"] == workload and r["config"] == config and r["energy_weight"] == weight]
    assert len(m) == 1, (workload, config, weight, len(m))
    return m[0]


def ci95(r, k):
    """Half-width of the 95% CI on a paired mean: 1.96 * std / sqrt(n)."""
    return 1.96 * fnum(r, k) / math.sqrt(int(r["runs"]))


def write(name, text):
    with open(os.path.join(OUT, name), "w") as f:
        f.write(text)


summary, sweep = rows("summary.csv"), rows("sweep.csv")
allrows = summary + sweep
metrics = json.load(open(os.path.join(ROOT, "models", "artifacts", "metrics.json")))
clean = json.load(open(os.path.join(ROOT, "models", "artifacts", "cleaning_report.json")))

NAMES = {"default": "Default", "network-only": "Network-only", "energy-only": "Energy-only", "fused": "Fused"}

# ---- results tables ---------------------------------------------------------
def result_line(r, label, weight_label):
    co2, d_co2 = fnum(r, "dynamic_co2_g_mean"), fnum(r, "co2_vs_default_pct_mean")
    lat, d_lat = fnum(r, "mean_latency_ms_mean"), fnum(r, "latency_vs_default_pct_mean")
    if r["config"] == "default":
        delta_co2 = delta_lat = "--"
    else:
        delta_co2 = f"${d_co2:+.1f}\\ \\pm{ci95(r, 'co2_vs_default_pct_std'):.1f}$"
        delta_lat = f"${d_lat:+.1f}$"
    return f"{label} & {weight_label} & {co2:.2f} & {delta_co2} & {lat:.2f} & {delta_lat} \\\\\n"

for wl in ("default", "stress"):
    lines = []
    for cfg in ("default", "network-only", "energy-only", "fused"):
        lines.append(result_line(find(summary, wl, cfg, "default"), NAMES[cfg], "0.2" if cfg in ("energy-only", "fused") else "--"))
    lines.append("\\midrule\n")
    for cfg in ("energy-only", "fused"):
        lines.append(result_line(find(sweep, wl, cfg, HEADLINE_W), NAMES[cfg], HEADLINE_W))
    head = ("\\begin{tabular}{@{}llrrrr@{}}\n\\toprule\n"
            "Config & $w$ & CO$_2$ & $\\Delta$CO$_2$ (\\%) & Lat.\\ (ms) & $\\Delta$Lat.\\ (\\%) \\\\\n\\midrule\n")
    write(f"table_results_{wl}.tex", head + "".join(lines) + "\\bottomrule\n\\end{tabular}\n")

# ---- grid intensity by class ------------------------------------------------
def intensity_line(label, r):
    return f"{label} & {fnum(r, 'flexible_intensity_mean'):.0f} & {fnum(r, 'critical_intensity_mean'):.0f} \\\\\n"
write("table_class.tex",
      "\\begin{tabular}{@{}lrr@{}}\n\\toprule\nConfig & Flexible & Critical \\\\\n\\midrule\n" +
      intensity_line("Default", find(summary, "default", "default", "default")) +
      intensity_line("Network-only", find(summary, "default", "network-only", "default")) +
      intensity_line(f"Energy-only ($w{{=}}{HEADLINE_W}$)", find(sweep, "default", "energy-only", HEADLINE_W)) +
      intensity_line(f"Fused ($w{{=}}{HEADLINE_W}$)", find(sweep, "default", "fused", HEADLINE_W)) +
      "\\bottomrule\n\\end{tabular}\n")

# ---- power model table -------------------------------------------------------
cv, tm = metrics["grouped_cv"], metrics["temporal_holdout"]
MODELS = [("placeholder_linear", "Placeholder line (100--250\\,W)"), ("oracle_linear", "Line through true idle/max (oracle)"),
          ("ridge", "Ridge regression"), ("xgboost", "XGBoost (ours)")]
write("table_power.tex",
      "\\begin{tabular}{@{}lrrrr@{}}\n\\toprule\n"
      " & \\multicolumn{2}{c}{Grouped CV} & \\multicolumn{2}{c}{Temporal split} \\\\\n"
      "\\cmidrule(lr){2-3}\\cmidrule(lr){4-5}\n"
      "Model & MAE & MAPE & MAE & MAPE \\\\\n\\midrule\n" +
      "".join(f"{label} & {cv[k]['mae_w']:.1f} & {cv[k]['mape_pct']:.1f} & {tm[k]['mae_w']:.1f} & {tm[k]['mape_pct']:.1f} \\\\\n"
              for k, label in MODELS) +
      "\\bottomrule\n\\end{tabular}\n")

# ---- plot data: trade-off curves --------------------------------------------
WEIGHTS = ["0", "0.1", "0.2", "0.5", "1", "1.5", "2", "3", "5", "8", "10"]
LABELLED = {"fused": ["0.2", "1.5", "5"], "energy-only": ["1.5", "5"]}
for cfg in ("fused", "energy-only"):
    tag = cfg.replace("-", "")
    for wl in ("default", "stress"):
        # red = CO2 reduction vs default in percent (positive is better).
        out = ["w lat red flex crit"]
        for w in WEIGHTS:
            r = find(sweep, wl, cfg, w)
            out.append(f"{w} {fnum(r, 'mean_latency_ms_mean'):.3f} {-fnum(r, 'co2_vs_default_pct_mean'):.3f} "
                       f"{fnum(r, 'flexible_intensity_mean'):.1f} {fnum(r, 'critical_intensity_mean'):.1f}")
        write(f"sweep_{tag}_{wl}.dat", "\n".join(out) + "\n")
        lab = ["lat red label"]
        for w in LABELLED[cfg]:
            r = find(sweep, wl, cfg, w)
            lab.append(f"{fnum(r, 'mean_latency_ms_mean'):.3f} {-fnum(r, 'co2_vs_default_pct_mean'):.3f} {w}")
        write(f"labels_{tag}_{wl}.dat", "\n".join(lab) + "\n")
for wl in ("default", "stress"):
    out = ["name lat red"]
    for cfg in ("default", "network-only"):
        r = find(summary, wl, cfg, "default")
        out.append(f"{cfg.replace('-', '')} {fnum(r, 'mean_latency_ms_mean'):.3f} {-fnum(r, 'co2_vs_default_pct_mean'):.3f}")
    write(f"points_{wl}.dat", "\n".join(out) + "\n")

# ---- plot data: predicted power curves ---------------------------------------
curves = json.load(open(os.path.join(ROOT, "models", "artifacts", "power_curves.json")))["node_types"]
out = ["u small mid large placeholder"]
for i, u in enumerate(curves["small-8c"]["utilisation"]):
    out.append(f"{u} {curves['small-8c']['watts'][i]} {curves['mid-32c']['watts'][i]} "
               f"{curves['large-64c']['watts'][i]} {100 + 150 * u:.1f}")
write("power_curves.dat", "\n".join(out) + "\n")
small = curves["small-8c"]["watts"]

# ---- headline macros -----------------------------------------------------------
fd = find(sweep, "default", "fused", HEADLINE_W)
fs = find(sweep, "stress", "fused", HEADLINE_W)
nd = find(summary, "default", "network-only", "default")
ed = find(sweep, "default", "energy-only", HEADLINE_W)
f02 = find(summary, "default", "fused", "default")
def idle_share(workload):
    rs = [r for r in csv.DictReader(open(os.path.join(ROOT, "results", "sim", "runs.csv")))
          if r["workload"] == workload and r["config"] == "default" and r["energy_weight"] == "default"]
    dyn = sum(float(r["dynamic_co2_g"]) for r in rs)
    idle = sum(float(r["idle_co2_g"]) for r in rs)
    return 100 * idle / (idle + dyn)

macros = {
    "IdleShare": f"{idle_share('default'):.0f}",
    "IdleShareStress": f"{idle_share('stress'):.0f}",
    "NSeeds": fd["runs"],
    "HeadlineW": HEADLINE_W,
    "FusedCO": f"{-fnum(fd, 'co2_vs_default_pct_mean'):.1f}",
    "FusedCOci": f"{ci95(fd, 'co2_vs_default_pct_std'):.1f}",
    "FusedCOStress": f"{-fnum(fs, 'co2_vs_default_pct_mean'):.1f}",
    "FusedLatCut": f"{-fnum(fd, 'latency_vs_default_pct_mean'):.0f}",
    "NetLatCut": f"{-fnum(nd, 'latency_vs_default_pct_mean'):.0f}",
    "NetCO": f"{-fnum(nd, 'co2_vs_default_pct_mean'):.1f}",
    "FusedLatCutStress": f"{-fnum(find(sweep, 'stress', 'fused', HEADLINE_W), 'latency_vs_default_pct_mean'):.0f}",
    "NetLatCutStress": f"{-fnum(find(summary, 'stress', 'network-only', 'default'), 'latency_vs_default_pct_mean'):.0f}",
    "FusedLowW": f"{-fnum(f02, 'co2_vs_default_pct_mean'):.1f}",
    "EnergyOnlyCO": f"{-fnum(ed, 'co2_vs_default_pct_mean'):.1f}",
    "FusedDecisionUs": f"{fnum(fd, 'decision_us_mean'):.1f}",
    "NSystemsScraped": str(clean["detail_systems"]),
    "NSystemsKept": str(clean["systems_kept"]),
    "NRowsKept": f"{clean['rows_kept']:,}".replace(",", "{,}"),
    "YearLo": str(clean["years"][0]),
    "YearHi": str(clean["years"][1]),
    "MapeXgb": f"{cv['xgboost']['mape_pct']:.1f}",
    "MapeOracle": f"{cv['oracle_linear']['mape_pct']:.1f}",
    "MapePlaceholder": f"{cv['placeholder_linear']['mape_pct']:.0f}",
    "MapeTemporal": f"{tm['xgboost']['mape_pct']:.0f}",
    "TrainSystems": str(tm["train_systems"]),
    "TestSystems": str(tm["test_systems"]),
    "SmallIdle": f"{small[0]:.0f}",
    "SmallHalf": f"{small[5]:.0f}",
    "SmallPeak": f"{small[10]:.0f}",
}
write("numbers.tex", "".join(f"\\newcommand{{\\{k}}}{{{v}}}\n" for k, v in macros.items()))
print("wrote", len(os.listdir(OUT)), "files to", os.path.relpath(OUT, ROOT))
