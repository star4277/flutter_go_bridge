// Package watcher re-runs code generation whenever watched inputs change,
// mirroring flutter_rust_bridge's `generate --watch` controller. It polls
// file metadata instead of using OS file events: the watched trees are small
// Go packages, and polling behaves identically across platforms, editors that
// save via atomic renames, and network file systems.
package watcher

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Options configure a watch loop.
type Options struct {
	// Roots are files or directories (watched recursively) to observe.
	Roots []string
	// Exclude lists paths that never trigger a re-run, typically the
	// generated outputs living inside a watched root.
	Exclude []string
	// Interval is the polling period; 0 means 400ms (close to FRB's 300ms
	// debounce window).
	Interval time.Duration
	// MaxCount limits how many times runInner executes; <=0 means unlimited.
	// FRB has the same internal knob, used by tests.
	MaxCount int
	// ConfirmContent compares a content hash instead of (size, mtime), so
	// rewriting a file with identical bytes is not reported as a change. The
	// generator rewrites every output unconditionally, so a consumer whose
	// reaction is expensive -- `run` restarts the whole app -- needs this to
	// avoid rebuilding for a no-op write. Hashes are cached per file and only
	// recomputed when (size, mtime) moves, so steady-state polling still costs
	// one stat per file.
	ConfirmContent bool
}

// Run executes runInner once, then again after every observed change. A
// failing runInner is reported as a warning and watching continues; only
// watcher errors (e.g. an unreadable root) abort the loop.
func Run(options Options, runInner func() error) error {
	return RunContext(context.Background(), options, runInner)
}

func RunContext(ctx context.Context, options Options, runInner func() error) error {
	interval := options.Interval
	if interval <= 0 {
		interval = 400 * time.Millisecond
	}
	// The baseline is taken before the run so edits made while the generator
	// is busy still trigger a re-run; generated outputs are kept out via
	// Exclude instead.
	tracker, err := NewTracker(options)
	if err != nil {
		return err
	}
	for count := 1; ; count++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := runInner(); err != nil {
			log.Printf("warning: code generator failed: %v", err)
		}
		if options.MaxCount > 0 && count >= options.MaxCount {
			return nil
		}
		log.Printf("watching for file changes on %s ...", strings.Join(options.Roots, ", "))
		for {
			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			changed, err := tracker.Changed()
			if err != nil {
				return err
			}
			if changed {
				break
			}
		}
	}
}

// Tracker observes a set of roots and reports whether they changed since the
// last observation. It is the reusable half of Run, for callers that drive
// their own event loop instead of blocking on a callback.
type Tracker struct {
	roots          []string
	exclude        map[string]struct{}
	confirmContent bool
	current        snapshot
}

// NewTracker takes the initial baseline. Only Roots, Exclude and
// ConfirmContent are read; the loop-related options are ignored.
func NewTracker(options Options) (*Tracker, error) {
	tracker := &Tracker{
		roots:          append([]string(nil), options.Roots...),
		confirmContent: options.ConfirmContent,
	}
	tracker.setExclude(options.Exclude)
	if err := tracker.Reset(); err != nil {
		return nil, err
	}
	return tracker, nil
}

func (t *Tracker) setExclude(paths []string) {
	exclude := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		exclude[filepath.Clean(path)] = struct{}{}
	}
	t.exclude = exclude
}

// SetExclude replaces the excluded paths and re-baselines, so a caller that
// only learns the generated file set after running the generator can drop
// those files from the next comparison.
func (t *Tracker) SetExclude(paths []string) error {
	t.setExclude(paths)
	return t.Reset()
}

// Reset re-baselines without reporting a change. Callers use it after acting
// on one tracker in a way that also touches another's files.
func (t *Tracker) Reset() error {
	next, err := takeSnapshotWith(t.roots, t.exclude, t.current, t.confirmContent)
	if err != nil {
		return err
	}
	t.current = next
	return nil
}

// Changed re-observes the roots and reports whether they differ from the last
// baseline, advancing the baseline either way.
func (t *Tracker) Changed() (bool, error) {
	next, err := takeSnapshotWith(t.roots, t.exclude, t.current, t.confirmContent)
	if err != nil {
		return false, err
	}
	changed := !t.current.equal(next, t.confirmContent)
	t.current = next
	return changed, nil
}

// Roots reports the watched roots, for logging.
func (t *Tracker) Roots() []string {
	return append([]string(nil), t.roots...)
}

type fileState struct {
	size    int64
	modTime int64
	// hash is only populated in ConfirmContent mode, where it -- not
	// (size, modTime) -- decides equality.
	hash uint64
}

type snapshot map[string]fileState

func (s snapshot) equal(other snapshot, useHash bool) bool {
	if len(s) != len(other) {
		return false
	}
	for path, state := range s {
		otherState, found := other[path]
		if !found {
			return false
		}
		if useHash {
			if state.hash != otherState.hash {
				return false
			}
			continue
		}
		if state.size != otherState.size || state.modTime != otherState.modTime {
			return false
		}
	}
	return true
}

// takeSnapshot records (size, mtime) for every watched file.
func takeSnapshot(roots []string, exclude map[string]struct{}) (snapshot, error) {
	return takeSnapshotWith(roots, exclude, nil, false)
}

// takeSnapshotWith walks the roots and records each file's state. Dot-
// directories such as .git are skipped: the Go toolchain ignores them as
// inputs and their churn would cause spurious re-runs. Files vanishing
// mid-walk are tolerated.
//
// In confirmContent mode a file's hash is reused from previous whenever
// (size, mtime) is unchanged, so only genuinely touched files are re-read.
func takeSnapshotWith(roots []string, exclude map[string]struct{}, previous snapshot, confirmContent bool) (snapshot, error) {
	result := snapshot{}
	record := func(path string, info fs.FileInfo) {
		path = filepath.Clean(path)
		if _, excluded := exclude[path]; excluded {
			return
		}
		state := fileState{size: info.Size(), modTime: info.ModTime().UnixNano()}
		if confirmContent {
			if before, found := previous[path]; found && before.size == state.size && before.modTime == state.modTime {
				state.hash = before.hash
			} else {
				state.hash = hashFile(path)
			}
		}
		result[path] = state
	}

	for _, root := range roots {
		root = filepath.Clean(root)
		info, err := os.Stat(root)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		if !info.IsDir() {
			record(root, info)
			continue
		}
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			if entry.IsDir() {
				if path != root && strings.HasPrefix(entry.Name(), ".") {
					return fs.SkipDir
				}
				return nil
			}
			fileInfo, err := entry.Info()
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			record(path, fileInfo)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// hashFile streams file contents and uses metadata as a failure sentinel so a
// persistently unreadable file can still be observed changing.
func hashFile(path string) uint64 {
	file, err := os.Open(path)
	if err != nil {
		if info, statErr := os.Stat(path); statErr == nil {
			digest := fnv.New64a()
			_, _ = fmt.Fprintf(digest, "%d:%d", info.Size(), info.ModTime().UnixNano())
			return digest.Sum64()
		}
		return ^uint64(0)
	}
	defer file.Close()
	digest := fnv.New64a()
	if _, err := io.Copy(digest, file); err != nil {
		return ^uint64(0)
	}
	return digest.Sum64()
}
