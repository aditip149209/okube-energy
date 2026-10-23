package sim

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
)

func parseCSV(t *testing.T, buf *bytes.Buffer) [][]string {
	t.Helper()
	recs, err := csv.NewReader(buf).ReadAll()
	if err != nil {
		t.Fatalf("output is not valid CSV: %v", err)
	}
	return recs
}

func TestReportCSVShapes(t *testing.T) {
	opts := smallOptions()
	rows := mustExperiment(t)
	agg := Aggregate(rows)

	var runs bytes.Buffer
	if err := WriteRunsCSV(&runs, rows); err != nil {
		t.Fatal(err)
	}
	recs := parseCSV(t, &runs)
	if len(recs) != len(rows)+1 {
		t.Fatalf("runs.csv has %d lines, want %d", len(recs), len(rows)+1)
	}
	for i, rec := range recs {
		if len(rec) != len(recs[0]) {
			t.Fatalf("runs.csv line %d has %d fields, header has %d", i, len(rec), len(recs[0]))
		}
	}

	var summary, sweep bytes.Buffer
	if err := WriteSummaryCSV(&summary, agg); err != nil {
		t.Fatal(err)
	}
	if err := WriteSweepCSV(&sweep, agg); err != nil {
		t.Fatal(err)
	}
	s := parseCSV(t, &summary)
	w := parseCSV(t, &sweep)
	if len(s)-1 != 4 {
		t.Errorf("summary.csv has %d data rows, want the 4 configs", len(s)-1)
	}
	if want := 2 * len(opts.SweepWeights); len(w)-1 != want {
		t.Errorf("sweep.csv has %d data rows, want %d", len(w)-1, want)
	}
	for _, recs := range [][][]string{s, w} {
		for i, rec := range recs {
			if len(rec) != len(aggHeader) {
				t.Fatalf("aggregate line %d has %d fields, header has %d", i, len(rec), len(aggHeader))
			}
		}
	}
}

func TestFormatTablesMentionsEveryConfig(t *testing.T) {
	out := FormatTables(Aggregate(mustExperiment(t)))
	for _, want := range []string{ConfigDefault, ConfigNetworkOnly, ConfigEnergyOnly, ConfigFused, "sweep", "paired"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q:\n%s", want, out)
		}
	}
}
