package watcher

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTrackerRootsReturnsCopy(t *testing.T) {
	tracker, err := NewTracker(Options{Roots: []string{"a", "b"}})
	if err != nil {
		t.Fatal(err)
	}
	roots := tracker.Roots()
	if len(roots) != 2 || roots[0] != "a" || roots[1] != "b" {
		t.Fatalf("roots = %v", roots)
	}
	// Mutating the returned slice must not affect the tracker.
	roots[0] = "mutated"
	if got := tracker.Roots()[0]; got != "a" {
		t.Fatalf("Roots() did not return a copy: %q", got)
	}
}

func TestHashFileRealFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := hashFile(path); got == 0 || got == ^uint64(0) {
		t.Fatalf("hashFile returned sentinel %d", got)
	}
}

func TestHashFileDirectoryCopyError(t *testing.T) {
	// Opening a directory succeeds on most platforms but io.Copy from it
	// errors, exercising the io.Copy failure sentinel.
	if got := hashFile(t.TempDir()); got != ^uint64(0) {
		t.Fatalf("hashFile of a directory = %d, want sentinel", got)
	}
}

func TestHashFileMissingBothSentinels(t *testing.T) {
	if got := hashFile(filepath.Join(t.TempDir(), "nope")); got != ^uint64(0) {
		t.Fatalf("hashFile of a missing path = %d, want sentinel", got)
	}
}

func TestEqualUsesHashWhenSet(t *testing.T) {
	left := snapshot{"/a": {size: 1, modTime: 1, hash: 100}}
	rightDifferentHash := snapshot{"/a": {size: 1, modTime: 1, hash: 200}}
	if left.equal(rightDifferentHash, true) {
		t.Fatal("different hashes must compare unequal in hash mode")
	}
	rightSameHash := snapshot{"/a": {size: 1, modTime: 2, hash: 100}}
	if !left.equal(rightSameHash, true) {
		t.Fatal("same hash must compare equal in hash mode even with a different mtime")
	}
	// Size/mode mode must reject a different mtime but equal size.
	if left.equal(rightSameHash, false) {
		t.Fatal("same-size different-mtime must compare unequal outside hash mode")
	}
	if !left.equal(snapshot{"/a": {size: 1, modTime: 1}}, false) {
		t.Fatal("identical size/mtime must compare equal")
	}
}

func TestRunContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := RunContext(ctx, Options{MaxCount: 5, Interval: 10 * time.Millisecond, Roots: nil}, func() error { return nil })
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("RunContext with a cancelled context should return context.Canceled, got %v", err)
	}
}

func TestTakeSnapshotWithUnreadableRootError(t *testing.T) {
	// A directory that cannot be stat'd returns an error; point at a
	// nonexistent parent path used as a file that exists as a dir is not
	// possible portably, so use a root that is a file that we later make
	// unreadable is not reliable - instead assert a missing root is ignored.
	snap, err := takeSnapshotWith([]string{filepath.Join(t.TempDir(), "ghost")}, map[string]struct{}{}, snapshot{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap) != 0 {
		t.Fatalf("snapshot = %v, want empty", snap)
	}
}

func TestRunContextIntervalDefaultAndMaxCount(t *testing.T) {
	// Interval 0 and no Roots exercises the default interval branch in RunContext.
	err := RunContext(context.Background(), Options{MaxCount: 1, Interval: 0}, func() error { return nil })
	if err != nil {
		t.Fatalf("RunContext with MaxCount=1 should return nil, got %v", err)
	}
}

func TestRunContextNewTrackerError(t *testing.T) {
	nul := filepath.Join(t.TempDir(), string(rune(0)), "x")
	err := RunContext(context.Background(), Options{Roots: []string{nul}, MaxCount: 1}, func() error { return nil })
	if err == nil {
		t.Fatal("expected NewTracker error for an invalid root")
	}
}

func TestNewTrackerInvalidRootError(t *testing.T) {
	nul := filepath.Join(t.TempDir(), string(rune(0)), "x")
	if _, err := NewTracker(Options{Roots: []string{nul}}); err == nil {
		t.Fatal("expected Reset to fail on an invalid root")
	}
}

func TestRunContextCancelledWhileWaiting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ranCh := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- RunContext(ctx, Options{Interval: time.Hour}, func() error {
			select {
			case ranCh <- struct{}{}:
			default:
			}
			return nil
		})
	}()
	<-ranCh
	cancel()
	select {
	case err := <-done:
		if err == nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunContext did not return after cancel")
	}
}

func TestSnapshotEqualMissingKey(t *testing.T) {
	one := snapshot{}
	one["/a"] = fileState{size: 1}
	if one.equal(snapshot{}, false) {
		t.Fatal("a snapshot with an extra key must not be equal")
	}
	if (snapshot{}).equal(one, false) {
		t.Fatal("a snapshot missing a key must not be equal")
	}
	// Equal lengths but a missing key exercises the per-key lookup branch.
	other := snapshot{}
	other["/b"] = fileState{size: 1}
	if one.equal(other, false) {
		t.Fatal("equal-length snapshots with different keys must not be equal")
	}
}

func TestTakeSnapshotWithStatErrorNonNotExist(t *testing.T) {
	// A path with an embedded NUL byte makes os.Stat fail with an error that
	// is not fs.ErrNotExist, exercising the non-tolerated root error branch.
	nul := filepath.Join(t.TempDir(), string(rune(0)), "x")
	if _, err := takeSnapshotWith([]string{nul}, nil, nil, false); err == nil {
		t.Fatal("expected an error for a root that cannot be stat'd")
	}
}
