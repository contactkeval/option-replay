package tests

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var Update = flag.Bool(
	"update",
	false,
	"update golden files",
)

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

	if *Update {
		writeGolden(t, name, v)
		return
	}

	expected := loadGolden(t, name)

	if !bytes.Equal(expected, actual) {
		t.Fatalf("golden mismatch for %s\nexpected:\n%s\nactual:\n%s",
			name, string(expected), string(actual))
	}
}
