"""Fetch current GB regional carbon intensity and write a carbon_intensity.json.

The output is in the format `okube manager --carbon-intensity-file` already
loads (see pkg/carbon): {"default": f, "nodes": {worker: f}}, where f is the
unitless factor gCO2/kWh / 1000. A "source" block records provenance and is
ignored by the loader.

Data: Carbon Intensity API, operated by NESO with the University of Oxford,
EDF Europe and WWF, licensed CC BY 4.0 (https://carbonintensity.org.uk).
The /regional endpoint returns the current half-hour's *forecast* intensity
(an indicative value), not a metered measurement.

Usage:
  python3 -I scripts/fetch_uk_carbon.py \
      --map worker-1="North Scotland" --map worker-2="North West England" \
      --map worker-3="South Wales" --out carbon_intensity.json
"""
import argparse
import json
import sys
import urllib.request

URL = "https://api.carbonintensity.org.uk/regional"
USER_AGENT = "okube-research/0.1 (student project; aditip149209@gmail.com)"
DEFAULT_REGION = "GB"  # national figure, used for workers with no mapping


def fetch():
    req = urllib.request.Request(URL, headers={"User-Agent": USER_AGENT, "Accept": "application/json"})
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.load(resp)


def regions_by_name(payload):
    window = payload["data"][0]
    out = {}
    for r in window["regions"]:
        forecast = r["intensity"]["forecast"]
        if forecast is None:
            continue
        out[r["shortname"]] = float(forecast)
    return window["from"], window["to"], out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--map", action="append", default=[], metavar='WORKER="Region"',
                    help="map a worker ID or address to a GB region short name (repeatable)")
    ap.add_argument("--out", default="carbon_intensity.json")
    args = ap.parse_args()

    start, end, regions = regions_by_name(fetch())

    nodes = {}
    for item in args.map:
        worker, _, region = item.partition("=")
        if region not in regions:
            sys.exit(f"unknown region {region!r}; available: {sorted(regions)}")
        nodes[worker] = regions[region] / 1000.0

    if DEFAULT_REGION not in regions:
        sys.exit(f"API response has no {DEFAULT_REGION!r} region to use as the default")
    doc = {
        "default": regions[DEFAULT_REGION] / 1000.0,
        "nodes": nodes,
        "source": {
            "api": URL,
            "licence": "CC BY 4.0",
            "operator": "NESO, with University of Oxford, EDF Europe and WWF",
            "window_utc": [start, end],
            "kind": "forecast (indicative), gCO2/kWh divided by 1000",
            "regions_gco2_per_kwh": regions,
            "mapping": dict(m.split("=", 1) for m in args.map),
        },
    }
    with open(args.out, "w") as f:
        json.dump(doc, f, indent=2)
        f.write("\n")
    print(f"wrote {args.out}: {len(nodes)} workers, default (GB) {doc['default']:.3f}, window {start}..{end}", file=sys.stderr)


if __name__ == "__main__":
    main()
