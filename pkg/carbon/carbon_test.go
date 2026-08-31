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
