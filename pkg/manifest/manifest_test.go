package manifest

import (
	"testing"
	"time"
)

func TestToTasksCarriesEnergyMetadata(t *testing.T) {
	m, err := ParseManifest([]byte(`
name: demo
services:
  batch:
    image: busybox
    criticality: urgent
    flexible: true
    estimatedDuration: 120
    deadline: 2026-10-23T12:00:00Z
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	got := ToTasks(m, "demo")["batch"]
	if got.Criticality != "urgent" || !got.Flexible || got.EstimatedDuration != 120 {
		t.Fatalf("energy metadata not carried through: %+v", got)
	}
	if want := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC); !got.Deadline.Equal(want) {
		t.Fatalf("expected deadline %v, got %v", want, got.Deadline)
	}
}
