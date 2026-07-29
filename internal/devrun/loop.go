package devrun

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/star4277/flutter_go_bridge/internal/watcher"
)

// Generate regenerates the bridge and returns every file it wrote, so those
// files can be excluded from the watch and not trigger themselves.
type Generate func() ([]string, error)

// Options configure the run loop.
type Options struct {
	// WorkDir is the Flutter project directory.
	WorkDir string
	// FlutterArgs are passed through to `flutter run`.
	FlutterArgs []string
	// GoRoots are watched for changes that require a full process restart.
	GoRoots []string
	// DartRoots are watched for changes that only need a hot reload.
	DartRoots []string
	// Exclude lists paths that never trigger anything, on top of the
	// generated files discovered on each run.
	Exclude []string
	// Interval is the polling period; 0 means 400ms.
	Interval time.Duration
	// Generate regenerates the bridge.
	Generate Generate
	// Output receives flutter's output; defaults to os.Stdout.
	Output io.Writer
	// startApp overrides how a session is created. Tests inject a fake here;
	// nil means a real `flutter run` process.
	startApp func(context.Context, SessionOptions) (app, error)
	// keys overrides the key source, so tests do not touch the real terminal.
	keys <-chan Key
}

// app is the part of Session the run loop drives. It exists so the loop's
// decisions can be tested without launching a real Flutter toolchain.
type app interface {
	Restart(ctx context.Context, full bool) error
	Stop(ctx context.Context) error
	Detach(ctx context.Context) error
	Events() <-chan Event
	Handle(event Event)
	VMServiceURI() string
	DeviceID() string
}

// loop holds the mutable state of one Run call.
type loop struct {
	options     Options
	interval    time.Duration
	output      io.Writer
	session     app
	goTracker   *watcher.Tracker
	dartTracker *watcher.Tracker
	// quiet silences the current session's pump once we have decided to stop
	// it. The pump must keep draining -- app.stop needs the daemon reader to
	// stay responsive -- but its buffered output is no longer worth printing.
	quiet *atomic.Bool
	// ended receives a signal when the current session's process exits,
	// whether we asked for it or the app died on the device.
	ended chan struct{}
}

// Run generates the bridge, launches `flutter run`, and keeps both in sync
// until the context is cancelled or the user quits.
//
// Go changes force a process-level restart because a rebuilt dynamic library
// cannot be swapped into a live process; Dart changes only need a hot reload.
func Run(ctx context.Context, options Options) error {
	if options.Generate == nil {
		return errors.New("devrun: Generate is required")
	}
	interval := options.Interval
	if interval <= 0 {
		interval = 400 * time.Millisecond
	}
	output := options.Output
	if output == nil {
		output = os.Stdout
	}

	// Tests inject their own key source; otherwise take over the terminal.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	keys := options.keys
	if keys == nil {
		keyboard := newKeyboard()
		defer keyboard.Close()
		if keyboard.Raw() {
			// Raw mode stops the terminal from expanding newlines, so
			// everything we and flutter print needs its own carriage return.
			output = crlfWriter{inner: output}
		}
		keys = keyboard.Keys()
	}

	// Quitting has to work from the moment the keyboard exists, not from the
	// moment the main loop starts reading. Raw mode takes Ctrl+C away from the
	// terminal -- it arrives as a byte rather than a signal -- and the first
	// generation and app launch run before the select below, which for a cold
	// Android build is minutes of an otherwise uninterruptible wait.
	incoming := make(chan Key, 16)
	go func() {
		defer close(incoming)
		for key := range keys {
			if key == KeyQuit {
				cancel()
				return
			}
			select {
			case incoming <- key:
			default:
				// Startup is still running and nobody is reading. A hot reload
				// requested against an app that is not up yet is not worth
				// queueing.
			}
		}
	}()
	var pending <-chan Key = incoming


	// The generator reports progress through the standard logger. Route it
	// through the same writer so it picks up the newline translation, and drop
	// the timestamps, which are noise in an interactive dev loop.
	previousWriter, previousFlags := log.Writer(), log.Flags()
	log.SetOutput(output)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	}()

	runner := &loop{
		options:  options,
		interval: interval,
		output:   output,
		ended:    make(chan struct{}, 1),
	}

	// Generate before taking the watch baseline, so the freshly written
	// outputs are part of the baseline instead of looking like a change.
	generated, err := runner.options.Generate()
	if err != nil {
		return err
	}
	if err := runner.newTrackers(generated); err != nil {
		return err
	}
	if err := runner.startSession(ctx); err != nil {
		// Quitting during a long first build cancels the context, which
		// surfaces here as a startup failure. The user asked for it, so it is
		// not an error to report.
		if ctx.Err() != nil {
			runner.logf("stopping")
			return nil
		}
		return err
	}
	defer runner.stopSession(context.WithoutCancel(ctx))

	runner.logf("watching %s for Go changes, %s for Dart changes",
		joinPaths(options.GoRoots), joinPaths(options.DartRoots))
	runner.logf("press h for help, q to quit")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			runner.logf("stopping")
			return nil
		case <-runner.ended:
			runner.logf("flutter run exited")
			return nil
		case key, ok := <-pending:
			if !ok {
				// stdin closed; keep reacting to file changes alone. A nil
				// channel blocks forever, so this arm stops being selected.
				pending = nil
				continue
			}
			done, err := runner.onKey(ctx, key)
			if err != nil {
				return err
			}
			if done {
				return nil
			}
		case <-ticker.C:
			if err := runner.poll(ctx); err != nil {
				return err
			}
		}
	}
}

// newTrackers rebuilds both watchers with the generated files excluded.
//
// Generated Dart sources live under the Dart root, so without excluding them
// every regeneration would also look like a hand edit and queue a redundant
// hot reload straight after the restart that already covered it.
func (l *loop) newTrackers(generated []string) error {
	exclude := append(append([]string(nil), l.options.Exclude...), generated...)
	goTracker, err := watcher.NewTracker(watcher.Options{
		Roots:   l.options.GoRoots,
		Exclude: exclude,
		// A restart costs a full rebuild and reinstall, so content hashing
		// earns its keep: the generator rewrites its outputs unconditionally
		// and must not trigger a restart when the bytes are unchanged.
		ConfirmContent: true,
	})
	if err != nil {
		return err
	}
	dartTracker, err := watcher.NewTracker(watcher.Options{
		Roots:          l.options.DartRoots,
		Exclude:        exclude,
		ConfirmContent: true,
	})
	if err != nil {
		return err
	}
	l.goTracker, l.dartTracker = goTracker, dartTracker
	return nil
}

func (l *loop) startSession(ctx context.Context) error {
	l.logf("starting flutter run ...")
	start := l.options.startApp
	if start == nil {
		start = func(ctx context.Context, options SessionOptions) (app, error) {
			return StartSession(ctx, options)
		}
	}
	session, err := start(ctx, SessionOptions{
		WorkDir:     l.options.WorkDir,
		FlutterArgs: l.options.FlutterArgs,
		Output:      l.output,
		Logf:        l.logf,
	})
	if err != nil {
		return err
	}
	l.session = session

	// One pump owns the event stream for the whole session, so the main select
	// never competes with it and a slow restart cannot stall the daemon reader
	// by letting its event buffer fill.
	quiet := &atomic.Bool{}
	l.quiet = quiet
	go func() {
		for event := range session.Events() {
			if quiet.Load() {
				continue
			}
			session.Handle(event)
		}
		select {
		case l.ended <- struct{}{}:
		default:
		}
	}()

	if uri := session.VMServiceURI(); uri != "" {
		l.logf("app started on %s (Dart VM Service: %s)", session.DeviceID(), uri)
	} else {
		l.logf("app started on %s", session.DeviceID())
	}
	return nil
}

func (l *loop) stopSession(ctx context.Context) {
	if l.session == nil {
		return
	}
	// Stop printing before stopping: whatever is still buffered belongs to a
	// session the user has already dismissed.
	if l.quiet != nil {
		l.quiet.Store(true)
	}
	if err := l.session.Stop(ctx); err != nil {
		l.logf("warning: stopping the app failed: %v", err)
	}
	l.session = nil
	// Stop only returns after the process is reaped, so the pump has already
	// signalled; clear it so the next session does not inherit a stale exit.
	select {
	case <-l.ended:
	default:
	}
}

// poll checks both trees. Go wins when both changed: the restart it triggers
// already covers the Dart edit.
func (l *loop) poll(ctx context.Context) error {
	goChanged, err := l.goTracker.Changed()
	if err != nil {
		return err
	}
	if goChanged {
		l.settle(l.goTracker)
		return l.regenerateAndRestart(ctx, "Go sources changed")
	}
	dartChanged, err := l.dartTracker.Changed()
	if err != nil {
		return err
	}
	if dartChanged {
		l.settle(l.dartTracker)
		return l.hotReload(ctx)
	}
	return nil
}

// settle waits for the tree to go quiet before acting, so a save-all or a
// gofmt-on-save burst produces one restart instead of several.
func (l *loop) settle(tracker *watcher.Tracker) {
	for {
		time.Sleep(l.interval)
		changed, err := tracker.Changed()
		if err != nil || !changed {
			return
		}
	}
}

// regenerateAndRestart is the whole point of the command: regenerate the
// bridge, then replace the process so it links against the rebuilt library.
func (l *loop) regenerateAndRestart(ctx context.Context, reason string) error {
	l.logf("%s -> regenerating", reason)
	generated, err := l.options.Generate()
	if err != nil {
		// Keep the running app: a broken edit is usually mid-thought, and
		// tearing down a working session would cost a full rebuild to undo.
		l.logf("warning: code generation failed, keeping the running app: %v", err)
		l.resetTrackers()
		return nil
	}
	for _, path := range generated {
		l.logf("generated %s", relativeTo(l.options.WorkDir, path))
	}

	l.logf("restarting the app -- a rebuilt Go library cannot be hot reloaded")
	l.stopSession(ctx)
	if err := l.newTrackers(generated); err != nil {
		return err
	}
	if err := l.startSession(ctx); err != nil {
		return err
	}
	// Startup can take minutes; anything edited meanwhile belongs to the next
	// round, not to the run that just launched.
	l.resetTrackers()
	return nil
}

func (l *loop) hotReload(ctx context.Context) error {
	l.logf("Dart sources changed -> hot reload")
	if err := l.session.Restart(ctx, false); err != nil {
		l.logf("warning: hot reload failed: %v", err)
	}
	return l.dartTracker.Reset()
}

func (l *loop) resetTrackers() {
	if err := l.goTracker.Reset(); err != nil {
		l.logf("warning: %v", err)
	}
	if err := l.dartTracker.Reset(); err != nil {
		l.logf("warning: %v", err)
	}
}

// onKey handles one keypress and reports whether the loop should end.
func (l *loop) onKey(ctx context.Context, key Key) (bool, error) {
	switch key {
	case KeyHotReload:
		l.logf("hot reload (Dart only -- the Go library is unchanged)")
		if err := l.session.Restart(ctx, false); err != nil {
			l.logf("warning: hot reload failed: %v", err)
		}
		l.resetTrackers()
	case KeyHotRestart:
		l.logf("hot restart (Dart only -- the Go library is unchanged)")
		if err := l.session.Restart(ctx, true); err != nil {
			l.logf("warning: hot restart failed: %v", err)
		}
		l.resetTrackers()
	case KeyRegenerate:
		return false, l.regenerateAndRestart(ctx, "requested")
	// KeyQuit never arrives here: the key pump intercepts it and cancels the
	// context, so quitting works during startup too.
	case KeyDetach:
		l.logf("detaching, the app keeps running")
		if l.session != nil {
			if err := l.session.Detach(context.WithoutCancel(ctx)); err != nil {
				l.logf("warning: detach failed: %v", err)
			}
			l.session = nil
		}
		return true, nil
	case KeyHelp:
		for line := range strings.SplitSeq(HelpText, "\n") {
			fmt.Fprintln(l.output, line)
		}
	}
	return false, nil
}

func (l *loop) logf(format string, args ...any) {
	fmt.Fprintf(l.output, "[fgb] "+format+"\n", args...)
}

func joinPaths(paths []string) string {
	if len(paths) == 0 {
		return "(none)"
	}
	return strings.Join(paths, ", ")
}

// relativeTo shortens a generated path for logging, falling back to the
// absolute path when it lies outside the project.
func relativeTo(base, path string) string {
	relative, err := filepath.Rel(base, path)
	if err != nil || strings.HasPrefix(relative, "..") {
		return path
	}
	return relative
}
