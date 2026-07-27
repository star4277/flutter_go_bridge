// Package watcher re-runs code generation whenever watched inputs change,
// mirroring flutter_rust_bridge's `generate --watch` controller. It polls
// file metadata instead of using OS file events: the watched trees are small
// Go packages, and polling behaves identically across platforms, editors that
// save via atomic renames, and network file systems.
package watcher

import (
	"errors"
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
}

// Run executes runInner once, then again after every observed change. A
// failing runInner is reported as a warning and watching continues; only
// watcher errors (e.g. an unreadable root) abort the loop.
func Run(options Options, runInner func() error) error {
	interval := options.Interval
	if interval <= 0 {
		interval = 400 * time.Millisecond
	}
	exclude := make(map[string]struct{}, len(options.Exclude))
	for _, path := range options.Exclude {
		exclude[filepath.Clean(path)] = struct{}{}
	}

	// The baseline is taken before the run so edits made while the generator
	// is busy still trigger a re-run; generated outputs are kept out via
	// Exclude instead.
	current, err := takeSnapshot(options.Roots, exclude)
	if err != nil {
		return err
	}
	for count := 1; ; count++ {
		if err := runInner(); err != nil {
			log.Printf("warning: code generator failed: %v", err)
		}
		if options.MaxCount > 0 && count >= options.MaxCount {
			return nil
		}
		log.Printf("watching for file changes on %s ...", strings.Join(options.Roots, ", "))
		for {
			time.Sleep(interval)
			next, err := takeSnapshot(options.Roots, exclude)
			if err != nil {
				return err
			}
			if !current.equal(next) {
				current = next
				break
			}
		}
	}
}

type fileState struct {
	size    int64
	modTime int64
}

type snapshot map[string]fileState

func (s snapshot) equal(other snapshot) bool {
	if len(s) != len(other) {
		return false
	}
	for path, state := range s {
		if other[path] != state {
			return false
		}
	}
	return true
}

// takeSnapshot records (size, mtime) for every watched file. Dot-directories
// such as .git are skipped: the Go toolchain ignores them as inputs and their
// churn would cause spurious re-runs. Files vanishing mid-walk are tolerated.
func takeSnapshot(roots []string, exclude map[string]struct{}) (snapshot, error) {
	result := snapshot{}
	record := func(path string, info fs.FileInfo) {
		path = filepath.Clean(path)
		if _, excluded := exclude[path]; excluded {
			return
		}
		result[path] = fileState{size: info.Size(), modTime: info.ModTime().UnixNano()}
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
