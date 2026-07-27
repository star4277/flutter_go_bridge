package watcher

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRunRerunsOnChange ports FRB's test_run_with_watch: each run writes a new
// file into the watched tree, which must trigger the next run.
func TestRunRerunsOnChange(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "api.go"), "package api\n")

	runs := 0
	err := Run(Options{
		Roots:    []string{dir},
		Interval: 10 * time.Millisecond,
		MaxCount: 3,
	}, func() error {
		runs++
		write(t, filepath.Join(dir, fmt.Sprintf("%d.go", runs)), "package api\n")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if runs != 3 {
		t.Fatalf("runs = %d, want 3", runs)
	}
}

// TestRunKeepsWatchingAfterFailures mirrors FRB: a failing generator run is a
// warning, not the end of the loop.
func TestRunKeepsWatchingAfterFailures(t *testing.T) {
	dir := t.TempDir()

	runs := 0
	err := Run(Options{
		Roots:    []string{dir},
		Interval: 10 * time.Millisecond,
		MaxCount: 2,
	}, func() error {
		runs++
		write(t, filepath.Join(dir, fmt.Sprintf("%d.go", runs)), "package api\n")
		return fmt.Errorf("boom %d", runs)
	})
	if err != nil {
		t.Fatal(err)
	}
	if runs != 2 {
		t.Fatalf("runs = %d, want 2", runs)
	}
}

func TestRunIgnoresExcludedAndDotPaths(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "api.go"), "package api\n")
	excluded := filepath.Join(dir, "bridge_generated.go")

	runs := make(chan int, 16)
	done := make(chan error, 1)
	count := 0
	go func() {
		done <- Run(Options{
			Roots:    []string{dir},
			Exclude:  []string{excluded},
			Interval: 10 * time.Millisecond,
			MaxCount: 2,
		}, func() error {
			count++
			runs <- count
			return nil
		})
	}()

	if first := <-runs; first != 1 {
		t.Fatalf("first run = %d", first)
	}
	// Touch only ignored locations: the excluded output and a dot-directory.
	write(t, excluded, "package main // generated\n")
	write(t, filepath.Join(dir, ".git", "index"), "churn\n")
	select {
	case n := <-runs:
		t.Fatalf("ignored paths must not trigger run %d", n)
	case <-time.After(300 * time.Millisecond):
	}

	// A real input change still triggers the second (and final) run.
	write(t, filepath.Join(dir, "api.go"), "package api // changed\n")
	select {
	case <-runs:
	case <-time.After(3 * time.Second):
		t.Fatal("input change did not trigger a re-run")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestRunWatchesSingleFileRoot(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "api.go")
	write(t, input, "package api\n")

	runs := 0
	err := Run(Options{
		Roots:    []string{input},
		Interval: 10 * time.Millisecond,
		MaxCount: 2,
	}, func() error {
		runs++
		write(t, input, fmt.Sprintf("package api // rev %d\n", runs))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if runs != 2 {
		t.Fatalf("runs = %d, want 2", runs)
	}
}

func TestTakeSnapshotSkipsDotDirectoriesAndExcludes(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "api.go"), "package api\n")
	write(t, filepath.Join(dir, "nested", "more.go"), "package nested\n")
	write(t, filepath.Join(dir, ".git", "index"), "x\n")
	excluded := filepath.Join(dir, "bridge_generated.go")
	write(t, excluded, "package main\n")

	snap, err := takeSnapshot([]string{dir}, map[string]struct{}{filepath.Clean(excluded): {}})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap) != 2 {
		t.Fatalf("snapshot = %v, want exactly api.go and nested/more.go", snap)
	}
	for _, want := range []string{filepath.Join(dir, "api.go"), filepath.Join(dir, "nested", "more.go")} {
		if _, ok := snap[filepath.Clean(want)]; !ok {
			t.Fatalf("snapshot missing %s: %v", want, snap)
		}
	}
}

func TestTakeSnapshotToleratesMissingRoot(t *testing.T) {
	snap, err := takeSnapshot([]string{filepath.Join(t.TempDir(), "missing")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap) != 0 {
		t.Fatalf("snapshot = %v, want empty", snap)
	}
}
