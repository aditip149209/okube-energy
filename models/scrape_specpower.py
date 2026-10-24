"""Scrape the SPECpower_ssj2008 published-results summary table into a CSV.

One page, one request. spec.org's robots.txt asks for a 10 s crawl delay and
does not disallow this path; fetching each result's detail page (for the full
11-level load curve) would be hundreds of requests, so this only reads the
summary: per-system watts at 100% load and at active idle.

Usage:
  python3 -I models/scrape_specpower.py                 # fetch, then parse
  python3 -I models/scrape_specpower.py --html FILE     # parse a saved copy
"""
import argparse
import csv
import re
import sys
import urllib.request
from html.parser import HTMLParser

URL = "https://www.spec.org/power_ssj2008/results/power_ssj2008/"
BASE = "https://www.spec.org"
USER_AGENT = "okube-research/0.1 (student project; aditip149209@gmail.com)"

# CSS class on each <td> -> output column.
CELL_CLASSES = {
    "wkld_ssj_global_config_hw_vendor": "vendor",
    "wkld_ssj_global_config_hw_model": "model",
    "hwnodes": "nodes",
    "wkld_ssj_global_config_sw_jvm_vendor": "jvm_vendor",
    "wkld_ssj_global_config_hw_cpu": "cpu",
    "wkld_ssj_global_config_hw_cpu_mhz": "mhz",
    "aggregate_config_cpu_chips": "chips",
    "aggregate_config_cpu_cores": "cores",
    "hwtotalthreads": "threads",
    "aggregate_config_memory_gb": "memory_gb",
    "submetric_ssjops_0": "ssj_ops_100",
    "submetric_power_0": "watts_100",
    "submetric_power_10": "watts_idle",
    "metric_performance_power_ratio": "result_ssj_ops_per_watt",
}
NUMERIC = {"nodes", "mhz", "chips", "cores", "threads", "memory_gb",
           "ssj_ops_100", "watts_100", "watts_idle", "result_ssj_ops_per_watt"}
COLUMNS = ["vendor", "model", "date", "html_url", "nodes", "jvm_vendor", "cpu", "mhz",
           "chips", "cores", "threads", "memory_gb", "ssj_ops_100", "watts_100",
           "watts_idle", "result_ssj_ops_per_watt"]
DATE_RE = re.compile(r"[A-Z][a-z]{2}\s+\d{1,2},\s+\d{4}")


class TableParser(HTMLParser):
    def __init__(self):
        super().__init__()
        self.rows = []
        self._in_body = False
        self._row = None
        self._cell = None  # (column, [text chunks])
        self._links = []

    def handle_starttag(self, tag, attrs):
        a = dict(attrs)
        if tag == "tbody":
            self._in_body = True
        elif tag == "tr" and self._in_body:
            self._row = {}
            self._links = []
        elif tag == "td" and self._row is not None:
            col = CELL_CLASSES.get(a.get("class", "").split()[0] if a.get("class") else "")
            self._cell = (col, [])
        elif tag == "a" and self._cell is not None:
            href = a.get("href", "")
            if href.endswith(".html"):
                self._links.append(href)
        elif tag == "br" and self._cell is not None:
            self._cell[1].append("\n")

    def handle_data(self, data):
        if self._cell is not None:
            self._cell[1].append(data)

    def handle_endtag(self, tag):
        if tag == "td" and self._cell is not None:
            col, chunks = self._cell
            if col:
                self._row[col] = "".join(chunks)
            self._cell = None
        elif tag == "tr" and self._row is not None:
            if self._row:
                self._row["_links"] = self._links
                self.rows.append(self._row)
            self._row = None
        elif tag == "tbody":
            self._in_body = False


def clean_text(s):
    return re.sub(r"\s+", " ", s).strip()


def to_number(s):
    s = s.replace(",", "").strip()
    return s if re.fullmatch(r"-?\d+(\.\d+)?", s) else ""


def normalise(raw):
    model_cell = raw.get("model", "")
    date = DATE_RE.search(model_cell)
    # The model cell is "<model>\n<enclosure>\n<date> | HTML | Text"; keep the
    # first line as the model name.
    model = clean_text(model_cell.split("\n")[0]) if model_cell else ""
    row = {
        "model": model,
        "date": date.group(0) if date else "",
        "html_url": BASE + raw["_links"][0] if raw.get("_links") else "",
    }
    for col in COLUMNS:
        if col in row:
            continue
        v = clean_text(raw.get(col, ""))
        row[col] = to_number(v) if col in NUMERIC else v
    return row


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--html", help="parse this saved file instead of fetching")
    ap.add_argument("--out", default="models/data/specpower_summary.csv")
    args = ap.parse_args()

    if args.html:
        html = open(args.html, encoding="utf-8", errors="replace").read()
    else:
        req = urllib.request.Request(URL, headers={"User-Agent": USER_AGENT})
        with urllib.request.urlopen(req, timeout=60) as resp:
            html = resp.read().decode("utf-8", errors="replace")

    parser = TableParser()
    parser.feed(html)
    rows = [normalise(r) for r in parser.rows]

    with open(args.out, "w", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(f, fieldnames=COLUMNS)
        w.writeheader()
        w.writerows(rows)
    print(f"wrote {len(rows)} rows to {args.out}", file=sys.stderr)


if __name__ == "__main__":
    main()
