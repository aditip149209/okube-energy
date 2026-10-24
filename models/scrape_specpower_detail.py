"""Fetch each SPECpower_ssj2008 result's detail page and extract its load curve.

Reads models/data/specpower_summary.csv (from scrape_specpower.py) and writes
one row per (system, target load) to models/data/specpower_detail.csv:
target load 100%..10% plus 0 for "Active Idle", with actual load, ssj_ops and
average active power in watts. Power is for the whole system under test (all
nodes), like the summary; join on html_url.

Politeness: spec.org's robots.txt asks for a 10 s crawl delay, so requests are
spaced at least that far apart (~3 hours for the full set). The job is
resumable: systems already in the output are skipped, so it can be stopped and
restarted. Failures and summary/detail mismatches go to
specpower_detail_issues.csv instead of being guessed at.

Usage:
  python3 -I models/scrape_specpower_detail.py                 # full, resumable
  python3 -I models/scrape_specpower_detail.py --limit 5       # first N systems
  python3 -I models/scrape_specpower_detail.py --parse FILE    # parse a saved page, no network
"""
import argparse
import csv
import os
import re
import sys
import time
import urllib.request

SUMMARY = "models/data/specpower_summary.csv"
OUT = "models/data/specpower_detail.csv"
ISSUES = "models/data/specpower_detail_issues.csv"
USER_AGENT = "okube-research/0.1 (student project; aditip149209@gmail.com)"
CRAWL_DELAY = 10.0
TIMEOUT = 60
RETRIES = 3

OUT_COLUMNS = ["html_url", "target_load_pct", "actual_load_pct", "ssj_ops", "avg_power_w"]
ISSUE_COLUMNS = ["html_url", "issue"]


def strip_tags(s):
    return re.sub(r"\s+", " ", re.sub(r"<[^>]+>", " ", s)).strip()


def num(s):
    s = s.replace(",", "").replace("%", "").strip()
    return float(s) if re.fullmatch(r"-?\d+(\.\d+)?", s) else None


def parse_load_table(html):
    """Return [(target_pct, actual_pct, ssj_ops, watts), ...] with idle as target 0.

    Raises ValueError if the benchmark-results table is missing or malformed.
    """
    m = re.search(r"<div class='resultsTable'>\s*(<table>.*?</table>)", html, re.S)
    if not m:
        raise ValueError("results table not found")

    rows = []
    for tr in re.findall(r"<tr[^>]*>(.*?)</tr>", m.group(1), re.S):
        cells = [strip_tags(c) for c in re.findall(r"<td[^>]*>(.*?)</td>", tr, re.S)]
        if len(cells) == 5 and re.fullmatch(r"\d+%", cells[0]):
            target, actual, ops, watts = num(cells[0]), num(cells[1]), num(cells[2]), num(cells[3])
        elif len(cells) == 5 and cells[0].lower().startswith("active idle"):
            # Idle rows have a colspan label cell, so cells are label, ops, watts, ratio
            target, actual, ops, watts = 0.0, 0.0, num(cells[1]), num(cells[2])
        elif len(cells) == 4 and cells[0].lower().startswith("active idle"):
            target, actual, ops, watts = 0.0, 0.0, num(cells[1]), num(cells[2])
        else:
            continue
        if watts is None:
            raise ValueError(f"unparseable power in row {cells}")
        rows.append((target, actual, ops, watts))

    targets = sorted(r[0] for r in rows)
    if targets != [0.0] + [float(x) for x in range(10, 101, 10)]:
        raise ValueError(f"expected idle + 10..100% load levels, got {targets}")
    return rows


def fetch(url):
    last = None
    for attempt in range(1, RETRIES + 1):
        try:
            req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
            with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
                return resp.read().decode("utf-8", errors="replace")
        except Exception as e:  # network errors are logged, not fatal
            last = e
            time.sleep(CRAWL_DELAY * attempt)
    raise RuntimeError(f"fetch failed after {RETRIES} attempts: {last}")


def done_urls():
    if not os.path.exists(OUT):
        return set()
    with open(OUT, newline="", encoding="utf-8") as f:
        return {r["html_url"] for r in csv.DictReader(f)}


def append_rows(path, columns, rows):
    new = not os.path.exists(path)
    with open(path, "a", newline="", encoding="utf-8") as f:
        w = csv.writer(f)
        if new:
            w.writerow(columns)
        w.writerows(rows)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--limit", type=int, help="only the first N pending systems")
    ap.add_argument("--parse", help="parse a saved detail page and print it; no network")
    args = ap.parse_args()

    if args.parse:
        html = open(args.parse, encoding="utf-8", errors="replace").read()
        for r in parse_load_table(html):
            print(r)
        return

    with open(SUMMARY, newline="", encoding="utf-8") as f:
        systems = [r for r in csv.DictReader(f) if r["html_url"] and r["watts_100"]]
    done = done_urls()
    pending = [s for s in systems if s["html_url"] not in done]
    if args.limit:
        pending = pending[: args.limit]
    print(f"{len(systems)} systems with power data, {len(done)} done, {len(pending)} to fetch", file=sys.stderr, flush=True)

    last_request = 0.0
    for i, s in enumerate(pending, 1):
        wait = CRAWL_DELAY - (time.monotonic() - last_request)
        if wait > 0:
            time.sleep(wait)
        url = s["html_url"]
        last_request = time.monotonic()
        try:
            rows = parse_load_table(fetch(url))
        except Exception as e:
            append_rows(ISSUES, ISSUE_COLUMNS, [(url, f"error: {e}")])
            print(f"[{i}/{len(pending)}] FAIL {url}: {e}", file=sys.stderr, flush=True)
            continue

        by_target = {r[0]: r for r in rows}
        issues = []
        # Cross-check against the summary table: 100% and idle watts must agree.
        if abs(by_target[100.0][3] - float(s["watts_100"])) > 0.5:
            issues.append((url, f"watts@100% detail={by_target[100.0][3]} summary={s['watts_100']}"))
        if s["watts_idle"] and abs(by_target[0.0][3] - float(s["watts_idle"])) > 0.5:
            issues.append((url, f"watts@idle detail={by_target[0.0][3]} summary={s['watts_idle']}"))
        if issues:
            append_rows(ISSUES, ISSUE_COLUMNS, issues)

        append_rows(OUT, OUT_COLUMNS, [(url, t, a, o, w) for t, a, o, w in sorted(rows, reverse=True)])
        print(f"[{i}/{len(pending)}] ok {url}", file=sys.stderr, flush=True)


if __name__ == "__main__":
    main()
