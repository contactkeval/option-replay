package stage2_dxfeeddatadownloader

import (
	"testing"
	"time"

	"github.com/contactkeval/option-replay/internal/db"
)

func TestResolveDownloadTarget_LatestRunAllBatches(t *testing.T) {
	database := openTestDB(t)
	insertRun(t, database, 1, 2)
	insertRun(t, database, 2, 3)

	runNo, batches, err := ResolveDownloadTarget(database, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if runNo != 2 {
		t.Fatalf("want latest run 2, got %d", runNo)
	}
	if len(batches) != 3 || batches[0] != 1 || batches[2] != 3 {
		t.Fatalf("want batches 1..3, got %v", batches)
	}
}

func TestResolveDownloadTarget_SpecificBatch(t *testing.T) {
	database := openTestDB(t)
	insertRun(t, database, 4, 5)

	runNo, batches, err := ResolveDownloadTarget(database, 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	if runNo != 4 {
		t.Fatalf("want run 4, got %d", runNo)
	}
	if len(batches) != 1 || batches[0] != 2 {
		t.Fatalf("want batch [2], got %v", batches)
	}
}

func TestResolveDownloadTarget_BatchOutOfRange(t *testing.T) {
	database := openTestDB(t)
	insertRun(t, database, 1, 2)

	_, _, err := ResolveDownloadTarget(database, 1, 9)
	if err == nil {
		t.Fatal("expected error for batch out of range")
	}
}

func TestResolveDownloadTarget_NoRuns(t *testing.T) {
	database := openTestDB(t)

	_, _, err := ResolveDownloadTarget(database, 0, 0)
	if err == nil {
		t.Fatal("expected error when no runs exist")
	}
}

func insertRun(t *testing.T, database *db.DB, runNo int64, batchCount int) {
	t.Helper()
	mustExec(t, database, `
		INSERT INTO runs (runNo, groupNo, runDateTime, contractCount, batchCount)
		VALUES (?, -1, ?, 0, ?)
	`, runNo, time.Now().Format(time.RFC3339), batchCount)
}
