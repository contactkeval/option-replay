package tests

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/contactkeval/option-replay/internal/data"
)

var (
	locNY  *time.Location
	start  time.Time
	end    time.Time
	update *bool

	dataProv data.Provider
)

func init() {
	var err error
	locNY, err = time.LoadLocation("America/New_York")
	if err != nil {
		panic(err)
	}

	start = time.Date(2025, 1, 1, 0, 0, 0, 0, locNY)
	end = time.Date(2026, 1, 1, 0, 0, 0, 0, locNY)

	update = flag.Bool("update", false, "update golden files")
}

func getLocalFileDataProvider() data.Provider {
	dataProv = data.NewMassiveDataProvider(os.Getenv("POLYGON_API_KEY"))
	dataProv = data.NewLocalFileDataProvider("dir", dataProv) // Massive data provider as secondary
	return dataProv
}

func getMassiveDataProvider() data.Provider {
	return data.NewMassiveDataProvider(os.Getenv("POLYGON_API_KEY"))
}

//
// --- Golden file helpers ---
//

func writeGolden(t *testing.T, name string, v any) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")

	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}

	err = os.WriteFile(path, b, 0644)
	if err != nil {
		t.Fatalf("failed to write golden file: %v", err)
	}
}

func loadGolden(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}
	return b
}

func CompareWithGolden(t *testing.T, name string, v any) {
	t.Helper()

	actual, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal actual JSON: %v", err)
	}

	if *update {
		writeGolden(t, name, v)
		return
	}

	expected := loadGolden(t, name)

	if !bytes.Equal(expected, actual) {
		t.Fatalf("golden mismatch for %s\nexpected:\n%s\nactual:\n%s",
			name, string(expected), string(actual))
	}
}
