package device

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCSV(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "devices.csv")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing test csv: %v", err)
	}
	return path
}

func TestLoadCSV(t *testing.T) {
	path := writeCSV(t, "device_id\n60-6b-44-84-dc-64\nb4-45-52-a2-f1-3c\n")

	devices, err := LoadCSV(path)
	if err != nil {
		t.Fatalf("LoadCSV() error = %v", err)
	}

	want := []Device{{ID: "60-6b-44-84-dc-64"}, {ID: "b4-45-52-a2-f1-3c"}}
	if len(devices) != len(want) {
		t.Fatalf("got %d devices, want %d", len(devices), len(want))
	}
	for i := range want {
		if devices[i] != want[i] {
			t.Errorf("device[%d] = %+v, want %+v", i, devices[i], want[i])
		}
	}
}

func TestLoadCSV_MissingColumn(t *testing.T) {
	path := writeCSV(t, "not_device_id\nfoo\n")

	if _, err := LoadCSV(path); err == nil {
		t.Fatal("expected error for missing device_id column, got nil")
	}
}

func TestLoadCSV_MissingFile(t *testing.T) {
	if _, err := LoadCSV("/nonexistent/devices.csv"); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}
