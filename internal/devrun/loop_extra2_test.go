package devrun

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/star4277/flutter_go_bridge/internal/watcher"
)

func TestRunRequiresGenerate(t *testing.T) {
	err := Run(context.Background(), Options{})
	if err == nil || !strings.Contains(err.Error(), "Generate is required") {
		t.Fatalf("got %v, want Generate required error", err)
	}
}

func TestRunDefaultsIntervalOutput(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "api.go"), "package api\n")
	keys := make(chan Key, 4)
	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), Options{
			GoRoots:   []string{dir},
			DartRoots: []string{dir},
			Generate:  func() ([]string, error) { return nil, nil },
			startApp:  func(context.Context, SessionOptions) (app, error) { return &quietApp{}, nil },
			keys:      keys,
		})
	}()
	time.Sleep(50 * time.Millisecond)
	keys <- KeyQuit
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned err: %v", err)
		}
	case <-time.After(settleTimeout):
		t.Fatal("Run did not return")
	}
}

func TestRunUsesDefaultKeyboard(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "api.go"), "package api\n")
	rd, wr, _ := os.Pipe()
	orig := os.Stdin
	os.Stdin = rd
	t.Cleanup(func() { os.Stdin = orig })
	defer func() { _ = rd.Close() }()
	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), Options{
			GoRoots:   []string{dir},
			DartRoots: []string{dir},
			Interval:  testInterval,
			Output:    io.Discard,
			Generate:  func() ([]string, error) { return nil, nil },
			startApp:  func(context.Context, SessionOptions) (app, error) { return &quietApp{}, nil },
		})
	}()
	time.Sleep(80 * time.Millisecond)
	_, _ = wr.Write([]byte("q\n"))
	_ = wr.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned err: %v", err)
		}
	case <-time.After(settleTimeout):
		t.Fatal("Run did not return after quit")
	}
}

func TestRunNewTrackersError(t *testing.T) {
	nul := filepath.Join(t.TempDir(), string(rune(0)), "x")
	err := Run(context.Background(), Options{
		GoRoots:  []string{nul},
		Generate: func() ([]string, error) { return nil, nil },
		keys:     make(chan Key),
	})
	if err == nil {
		t.Fatal("expected NewTracker error")
	}
}

func TestRunStartSessionError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "api.go"), "package api\n")
	keys := make(chan Key)
	err := Run(context.Background(), Options{
		GoRoots:   []string{dir},
		DartRoots: []string{dir},
		Generate:  func() ([]string, error) { return nil, nil },
		startApp:  func(context.Context, SessionOptions) (app, error) { return nil, errors.New("boom") },
		keys:      keys,
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("got %v, want startApp error", err)
	}
}

func TestRunKeysClosed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "api.go"), "package api\n")
	keys := make(chan Key, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{
			GoRoots:   []string{dir},
			DartRoots: []string{dir},
			Interval:  testInterval,
			Output:    io.Discard,
			Generate:  func() ([]string, error) { return nil, nil },
			startApp:  func(context.Context, SessionOptions) (app, error) { return &quietApp{}, nil },
			keys:      keys,
		})
	}()
	time.Sleep(80 * time.Millisecond)
	close(keys) // closes incoming -> pending nil branch
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned err: %v", err)
		}
	case <-time.After(settleTimeout):
		t.Fatal("Run did not return")
	}
}

// quietApp has an empty VMServiceURI so the else branch of the startup log is
// reached, and its event stream never sends so the pump idles safely.
type quietApp struct{ events chan Event }

func (q *quietApp) Restart(context.Context, bool) error { return nil }
func (q *quietApp) Stop(context.Context) error {
	if q.events != nil {
		close(q.events)
	}
	return nil
}
func (q *quietApp) Detach(context.Context) error        { return nil }
func (q *quietApp) Events() <-chan Event                { return q.events }
func (q *quietApp) Handle(Event)                        {}
func (q *quietApp) VMServiceURI() string                { return "" }
func (q *quietApp) DeviceID() string                    { return "quiet-device" }

func TestLoopNewTrackersGoError(t *testing.T) {
	l := testLoop(&bytes.Buffer{})
	valid := t.TempDir()
	nul := filepath.Join(valid, string(rune(0)), "x")
	l.options.GoRoots = []string{nul}
	l.options.DartRoots = []string{valid}
	if err := l.newTrackers(nil); err == nil {
		t.Fatal("expected Go tracker error")
	}
}

func TestLoopNewTrackersDartError(t *testing.T) {
	l := testLoop(&bytes.Buffer{})
	valid := t.TempDir()
	nul := filepath.Join(valid, string(rune(0)), "x")
	l.options.GoRoots = []string{valid}
	l.options.DartRoots = []string{nul}
	if err := l.newTrackers(nil); err == nil {
		t.Fatal("expected Dart tracker error")
	}
}

func TestLoopSettleCanceled(t *testing.T) {
	tr, err := watcher.NewTracker(watcher.Options{Roots: []string{t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	l := testLoop(&bytes.Buffer{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	l.settle(ctx, tr) // returns immediately via ctx.Done
}

func TestLoopSettleWaitsQuiet(t *testing.T) {
	dir := t.TempDir()
	tr, err := watcher.NewTracker(watcher.Options{Roots: []string{dir}})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "a.go"), "package a\n")
	l := testLoop(&bytes.Buffer{})
	l.settle(context.Background(), tr) // first Changed true -> reset timer, then false -> return
}

func TestLoopRegenerateAndRestartGenerateError(t *testing.T) {
	dir := t.TempDir()
	l := testLoop(&bytes.Buffer{})
	l.options.Generate = func() ([]string, error) { return nil, errors.New("gen boom") }
	gr, _ := watcher.NewTracker(watcher.Options{Roots: []string{dir}})
	dr, _ := watcher.NewTracker(watcher.Options{Roots: []string{dir}})
	l.goTracker, l.dartTracker = gr, dr
	if err := l.regenerateAndRestart(context.Background(), "gen"); err != nil {
		t.Fatalf("generate error should keep the app running (nil error), got %v", err)
	}
}

func TestLoopRegenerateAndRestartRuns(t *testing.T) {
	dir := t.TempDir()
	l := testLoop(&bytes.Buffer{})
	l.options.WorkDir = dir
	l.options.Generate = func() ([]string, error) { return []string{filepath.Join(dir, "gen1.go")}, nil }
	l.options.GoRoots = []string{dir}
	l.options.DartRoots = []string{dir}
	l.options.startApp = func(context.Context, SessionOptions) (app, error) { return &quietApp{}, nil }
	if err := l.regenerateAndRestart(context.Background(), "Go changed"); err != nil {
		t.Fatal(err)
	}
	if l.session == nil {
		t.Fatal("session should be running")
	}
}

func TestLoopRegenerateAndRestartNewTrackersError(t *testing.T) {
	l := testLoop(&bytes.Buffer{})
	l.options.Generate = func() ([]string, error) { return nil, nil }
	l.options.GoRoots = []string{filepath.Join(t.TempDir(), string(rune(0)), "x")}
	if err := l.regenerateAndRestart(context.Background(), "x"); err == nil {
		t.Fatal("expected newTrackers error")
	}
}

type errorStopApp struct{ err error }

func (r *errorStopApp) Restart(context.Context, bool) error { return nil }
func (r *errorStopApp) Stop(context.Context) error          { return r.err }
func (r *errorStopApp) Detach(context.Context) error        { return nil }
func (r *errorStopApp) Events() <-chan Event                { return nil }
func (r *errorStopApp) Handle(Event)                        {}
func (r *errorStopApp) VMServiceURI() string                { return "" }
func (r *errorStopApp) DeviceID() string                    { return "" }

func TestLoopStopSessionError(t *testing.T) {
	var buf bytes.Buffer
	l := testLoop(&buf)
	l.session = &errorStopApp{err: errors.New("stop boom")}
	l.stopSession(context.Background())
	if !strings.Contains(buf.String(), "stopping the app failed") {
		t.Fatalf("expected stop warning, got %q", buf.String())
	}
}

func TestLoopOnKeyHotReloadError(t *testing.T) {
	var buf bytes.Buffer
	l := testLoop(&buf)
	l.session = &alwaysFailsApp{err: errors.New("reload boom")}
	tr, _ := watcher.NewTracker(watcher.Options{Roots: []string{t.TempDir()}})
	l.dartTracker = tr
	if _, err := l.onKey(context.Background(), KeyHotReload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "hot reload failed") {
		t.Fatalf("expected hot reload warning, got %q", buf.String())
	}
}

func TestLoopOnKeyHotRestartError(t *testing.T) {
	var buf bytes.Buffer
	l := testLoop(&buf)
	l.session = &alwaysFailsApp{err: errors.New("restart boom")}
	tr, _ := watcher.NewTracker(watcher.Options{Roots: []string{t.TempDir()}})
	l.dartTracker = tr
	if _, err := l.onKey(context.Background(), KeyHotRestart); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "hot restart failed") {
		t.Fatalf("expected hot restart warning, got %q", buf.String())
	}
}

type errorDetachApp struct{}

func (e *errorDetachApp) Restart(context.Context, bool) error { return nil }
func (e *errorDetachApp) Stop(context.Context) error          { return nil }
func (e *errorDetachApp) Detach(context.Context) error        { return errors.New("detach boom") }
func (e *errorDetachApp) Events() <-chan Event                { return nil }
func (e *errorDetachApp) Handle(Event)                        {}
func (e *errorDetachApp) VMServiceURI() string                { return "" }
func (e *errorDetachApp) DeviceID() string                    { return "" }

func TestLoopOnKeyDetachError(t *testing.T) {
	var buf bytes.Buffer
	l := testLoop(&buf)
	l.session = &errorDetachApp{}
	if _, err := l.onKey(context.Background(), KeyDetach); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "detach failed") {
		t.Fatalf("expected detach warning, got %q", buf.String())
	}
}

func TestLoopOnKeyRegenerateError(t *testing.T) {
	var buf bytes.Buffer
	l := testLoop(&buf)
	l.options.Generate = func() ([]string, error) { return nil, nil }
	l.options.GoRoots = []string{filepath.Join(t.TempDir(), string(rune(0)), "x")}
	if _, err := l.onKey(context.Background(), KeyRegenerate); err == nil {
		t.Fatal("expected regenerate error")
	}
}

func TestRelativeToOutside(t *testing.T) {
	base := filepath.Join(t.TempDir(), "proj")
	outside := filepath.Dir(base)
	if got := relativeTo(base, outside); got != outside {
		t.Fatalf("got %q, want absolute fallback %q", got, outside)
	}
}

func TestKeyboardReadRaw(t *testing.T) {
	rd, wr, _ := os.Pipe()
	orig := os.Stdin
	os.Stdin = rd
	t.Cleanup(func() { os.Stdin = orig })
	defer func() { _ = rd.Close() }()
	board := &keyboard{keys: make(chan Key, 4), restore: func() {}, closed: make(chan struct{})}
	done := make(chan struct{})
	go func() { board.readRaw(); close(done) }()
	if _, err := wr.Write([]byte{'r'}); err != nil {
		t.Fatal(err)
	}
	select {
	case k := <-board.keys:
		if k != KeyHotReload {
			t.Fatalf("got %v, want r", k)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for r")
	}
	if _, err := wr.Write([]byte{3}); err != nil {
		t.Fatal(err)
	}
	select {
	case k := <-board.keys:
		if k != KeyQuit {
			t.Fatalf("got %v, want quit", k)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Ctrl+C")
	}
	_ = wr.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("readRaw did not return on EOF")
	}
}

func TestKeyboardReadRawSendFails(t *testing.T) {
	rd, wr, _ := os.Pipe()
	orig := os.Stdin
	os.Stdin = rd
	t.Cleanup(func() { os.Stdin = orig })
	defer func() { _ = rd.Close() }()
	board := &keyboard{keys: make(chan Key, 1), restore: func() {}, closed: make(chan struct{})}
	board.keys <- KeyHelp // fill buffer
	close(board.closed)   // closed is ready so send fails
	done := make(chan struct{})
	go func() { board.readRaw(); close(done) }()
	if _, err := wr.Write([]byte{'x'}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("readRaw did not return on send failure")
	}
}

func TestKeyboardReadLinesSendFails(t *testing.T) {
	rd, wr, _ := os.Pipe()
	orig := os.Stdin
	os.Stdin = rd
	t.Cleanup(func() { os.Stdin = orig })
	defer func() { _ = rd.Close() }()
	board := &keyboard{keys: make(chan Key, 1), restore: func() {}, closed: make(chan struct{})}
	board.keys <- KeyHelp
	close(board.closed)
	done := make(chan struct{})
	go func() { board.readLines(); close(done) }()
	if _, err := wr.Write([]byte("r\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("readLines did not return on send failure")
	}
}

func TestSessionDetachNoAppIDFallsBack(t *testing.T) {
	session := &Session{
		options: SessionOptions{Output: io.Discard, Logf: func(string, ...any) {}},
		exited:  make(chan struct{}),
	}
	if err := session.detach(context.Background()); err != nil {
		t.Fatalf("detach with no app id should fall back to killing nothing, got %v", err)
	}
}

func TestSessionDetachNoAppIDAlreadyExited(t *testing.T) {
	exited := make(chan struct{})
	close(exited)
	session := &Session{
		options: SessionOptions{Output: io.Discard, Logf: func(string, ...any) {}},
		exited:  exited,
	}
	if err := session.detach(context.Background()); err != nil {
		t.Fatalf("detach of an already-exited process should return nil, got %v", err)
	}
}

func TestStartSessionFlutterNotFound(t *testing.T) {
	t.Setenv("PATH", "")
	_, err := StartSession(context.Background(), SessionOptions{})
	if err == nil || !strings.Contains(err.Error(), "flutter not found") {
		t.Fatalf("got %v, want flutter-not-found error", err)
	}
}

func TestStartSessionLaunchError(t *testing.T) {
	if _, err := exec.LookPath("flutter"); err != nil {
		t.Skip("flutter not in PATH")
	}
	bad := filepath.Join(t.TempDir(), "nodir", "sub")
	_, err := StartSession(context.Background(), SessionOptions{WorkDir: bad, Output: io.Discard, Logf: func(string, ...any) {}})
	if err == nil || !strings.Contains(err.Error(), "start `flutter") {
		t.Fatalf("got %v, want start `flutter error", err)
	}
}
func TestRunDetachEndsLoop(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "api.go"), "package api\n")
	keys := make(chan Key, 4)
	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), Options{
			GoRoots:   []string{dir},
			DartRoots: []string{dir},
			Interval:  testInterval,
			Output:    io.Discard,
			Generate:  func() ([]string, error) { return nil, nil },
			startApp:  func(context.Context, SessionOptions) (app, error) { return &quietApp{}, nil },
			keys:      keys,
		})
	}()
	time.Sleep(80 * time.Millisecond)
	keys <- KeyDetach
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned err: %v", err)
		}
	case <-time.After(settleTimeout):
		t.Fatal("Run did not return after detach")
	}
}

func TestKeyboardReadRawCtrlCSendFails(t *testing.T) {
	rd, wr, _ := os.Pipe()
	orig := os.Stdin
	os.Stdin = rd
	t.Cleanup(func() { os.Stdin = orig })
	defer func() { _ = rd.Close() }()
	board := &keyboard{keys: make(chan Key, 1), restore: func() {}, closed: make(chan struct{})}
	board.keys <- KeyHelp // fill buffer
	close(board.closed)   // send will fail
	done := make(chan struct{})
	go func() { board.readRaw(); close(done) }()
	if _, err := wr.Write([]byte{3}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("readRaw did not return on Ctrl+C send failure")
	}
}
