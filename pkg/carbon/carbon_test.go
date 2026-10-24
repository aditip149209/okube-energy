package carbon

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "carbon_intensity.json")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return path
}

func TestLoadAndFor(t *testing.T) {
	path := writeConfig(t, `{
		"default": 0.5,
		"nodes": {
			"worker-1": 0.82,
			"10.0.0.5:5555": 0.1
		}
	}`)

	intensity, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if got := intensity.For("worker-1", "unused"); got != 0.82 {
		t.Fatalf("expected lookup by ID to return 0.82, got %v", got)
	}
	if got := intensity.For("unknown-id", "10.0.0.5:5555"); got != 0.1 {
		t.Fatalf("expected lookup by address to return 0.1, got %v", got)
	}
	if got := intensity.For("unknown-id", "unknown-address"); got != 0.5 {
		t.Fatalf("expected unmatched node to fall back to default 0.5, got %v", got)
	}
}

func TestForOnNilIntensity(t *testing.T) {
	var i *Intensity
	if got := i.For("worker-1", "1.2.3.4:5555"); got != 0 {
		t.Fatalf("expected nil *Intensity to return 0, got %v", got)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "does-not-exist.json")); err == nil {
		t.Fatal("expected an error loading a missing file")
	}
}

func TestLookupDistinguishesZeroFromUnknown(t *testing.T) {
	path := writeConfig(t, `{"default": 0.5, "nodes": {"clean": 0}}`)
	intensity, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if v, ok := intensity.Lookup("clean", ""); !ok || v != 0 {
		t.Fatalf("expected a configured zero to be known and 0, got %v (known=%v)", v, ok)
	}
	if v, ok := intensity.Lookup("other", ""); !ok || v != 0.5 {
		t.Fatalf("expected default 0.5 to be known, got %v (known=%v)", v, ok)
	}

	var none *Intensity
	if _, ok := none.Lookup("clean", ""); ok {
		t.Fatal("expected a nil *Intensity to report unknown")
	}
}

// The snapshot written by scripts/fetch_uk_carbon.py must load with the same
// loader the manager uses, with its extra "source" block ignored and a real
// intensity of 0 (North Scotland) kept as a known zero.
func TestLoadsUKSnapshotFromFetchScript(t *testing.T) {
	intensity, err := Load("../../results/carbon_intensity_uk_snapshot.json")
	if err != nil {
		t.Skipf("no snapshot present: %v", err)
	}
	if v, ok := intensity.Lookup("worker-1", ""); !ok || v != 0 {
		t.Fatalf("expected worker-1 (North Scotland) to be a known 0, got %v (known=%v)", v, ok)
	}
	if v, ok := intensity.Lookup("worker-3", ""); !ok || v <= 0 || v >= 1 {
		t.Fatalf("expected worker-3 (South Wales) to be a factor in (0,1), got %v (known=%v)", v, ok)
	}
	if intensity.Default <= 0 {
		t.Fatalf("expected a positive GB default, got %v", intensity.Default)
	}
}
