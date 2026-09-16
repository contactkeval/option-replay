package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	stage2 "github.com/contactkeval/option-replay/internal/pipeline/stage2_dxfeeddatadownloader"
)

func TestRecControlLifecycle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	rc := newRecControl("SPX", "SPXW")

	// Nothing active.
	if st := rc.state(); st.Recording {
		t.Fatalf("expected idle state, got %+v", st)
	}
	if _, _, err := rc.stop(); err == nil {
		t.Fatalf("stop on idle should error")
	}

	// Start immediately and append an event.
	r, err := rc.start(path, true)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if r.startAt.After(time.Now().UTC()) {
		t.Fatalf("immediate start should not be future-scheduled: %s", r.startAt)
	}
	ev := stage2.LiveEvent{Symbol: "SPX", Kind: "Quote", Bid: 1, Ask: 2, Time: time.Now().UTC()}
	if err := rc.append(ev); err != nil {
		t.Fatalf("append: %v", err)
	}
	if st := rc.state(); !st.Recording || st.Paused || st.Events != 1 || st.Path != path {
		t.Fatalf("unexpected state after append: %+v", st)
	}

	// Pause -> appends are dropped.
	if err := rc.pause(); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if err := rc.append(ev); err != nil {
		t.Fatalf("append while paused: %v", err)
	}
	if st := rc.state(); !st.Paused || st.Events != 1 {
		t.Fatalf("paused append should be dropped: %+v", st)
	}

	// Resume -> appends count again.
	if err := rc.resume(); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if err := rc.append(ev); err != nil {
		t.Fatalf("append after resume: %v", err)
	}
	if st := rc.state(); st.Paused || st.Events != 2 {
		t.Fatalf("resumed append should count: %+v", st)
	}

	// Stop closes the file; a second start is allowed afterwards.
	outPath, n, err := rc.stop()
	if err != nil || n != 2 || outPath != path {
		t.Fatalf("stop: path=%q n=%d err=%v", outPath, n, err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read recording: %v", err)
	}
	lines := 0
	for _, c := range b {
		if c == '\n' {
			lines++
		}
	}
	if lines != 3 { // meta header + 2 events
		t.Fatalf("expected 3 JSONL lines, got %d", lines)
	}

	if _, err := rc.start(path, false); err != nil {
		t.Fatalf("restart: %v", err)
	}
	rc.close()
	if st := rc.state(); st.Recording {
		t.Fatalf("close should stop recording: %+v", st)
	}
}

func TestRecControlErrorPaths(t *testing.T) {
	rc := newRecControl("SPX", "SPXW")
	if err := rc.pause(); err == nil {
		t.Fatalf("pause on idle should error")
	}
	if err := rc.resume(); err == nil {
		t.Fatalf("resume on idle should error")
	}
	_, err := rc.start(filepath.Join(t.TempDir(), "a.json"), true)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := rc.start(filepath.Join(t.TempDir(), "b.json"), true); err == nil {
		t.Fatalf("double start should error")
	}
	rc.close()
}