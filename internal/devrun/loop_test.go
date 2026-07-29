package devrun

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const testInterval = 10 * time.Millisecond

// settleTimeout allows for the poll tick plus the quiet period the loop waits
// out before acting.
const settleTimeout = 5 * time.Second

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeApp stands in for a real `flutter run` session.
type fakeApp struct {
	events   chan Event
	restarts chan bool
	stopped  chan struct{}
	once     sync.Once
}

func newFakeApp() *fakeApp {
	return &fakeApp{
		events:   make(chan Event),
		restarts: make(chan bool, 8),
		stopped:  make(chan struct{}),
	}
}

func (f *fakeApp) Restart(_ context.Context, full bool) error {
	f.restarts <- full
	return nil
}

func (f *fakeApp) Stop(context.Context) error {
	// Closing the event stream is what a real session does on exit, and it is
	// how the loop's pump learns the process is gone.
	f.once.Do(func() {
		close(f.events)
		close(f.stopped)
	})
	return nil
}

func (f *fakeApp) Detach(ctx context.Context) error { return f.Stop(ctx) }
func (f *fakeApp) Events() <-chan Event             { return f.events }
func (f *fakeApp) Handle(Event)                     {}
func (f *fakeApp) VMServiceURI() string             { return "ws://127.0.0.1:1/ws" }
func (f *fakeApp) DeviceID() string                 { return "fake-device" }

// harness runs the loop against fake sessions and temporary watch trees.
type harness struct {
	goDir     string
	dartDir   string
	starts    chan *fakeApp
	keys      chan Key
	generated atomic.Int32
	generate  func() ([]string, error)
	cancel    context.CancelFunc
	done      chan error
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		goDir:   t.TempDir(),
		dartDir: t.TempDir(),
		starts:  make(chan *fakeApp, 8),
		keys:    make(chan Key, 4),
		done:    make(chan error, 1),
	}
	writeFile(t, filepath.Join(h.goDir, "api.go"), "package api\n")
	writeFile(t, filepath.Join(h.dartDir, "main.dart"), "void main() {}\n")
	return h
}

func (h *harness) start(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel

	generate := h.generate
	if generate == nil {
		generate = func() ([]string, error) { return nil, nil }
	}
	go func() {
		h.done <- Run(ctx, Options{
			GoRoots:   []string{h.goDir},
			DartRoots: []string{h.dartDir},
			Interval:  testInterval,
			Output:    io.Discard,
			Generate: func() ([]string, error) {
				h.generated.Add(1)
				return generate()
			},
			startApp: func(context.Context, SessionOptions) (app, error) {
				session := newFakeApp()
				h.starts <- session
				return session, nil
			},
			keys: h.keys,
		})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-h.done:
		case <-time.After(settleTimeout):
			t.Error("Run did not return after cancellation")
		}
	})
}

func (h *harness) awaitStart(t *testing.T, why string) *fakeApp {
	t.Helper()
	select {
	case session := <-h.starts:
		return session
	case <-time.After(settleTimeout):
		t.Fatalf("no session was started: %s", why)
		return nil
	}
}

func (h *harness) expectNoStart(t *testing.T, why string) {
	t.Helper()
	select {
	case <-h.starts:
		t.Fatalf("unexpected restart: %s", why)
	case <-time.After(20 * testInterval):
	}
}

// TestRunRestartsProcessWhenGoSourcesChange is the core promise of the
// command: a Go edit regenerates the bridge and replaces the process, because
// the rebuilt dynamic library cannot be swapped into the running one.
func TestRunRestartsProcessWhenGoSourcesChange(t *testing.T) {
	h := newHarness(t)
	h.start(t)

	first := h.awaitStart(t, "initial launch")
	generatedBefore := h.generated.Load()

	writeFile(t, filepath.Join(h.goDir, "api.go"), "package api // edited\n")

	second := h.awaitStart(t, "Go change must relaunch the process")
	select {
	case <-first.stopped:
	case <-time.After(settleTimeout):
		t.Fatal("the previous session was not stopped before relaunching")
	}
	if h.generated.Load() <= generatedBefore {
		t.Fatal("the bridge was not regenerated before restarting")
	}
	select {
	case full := <-second.restarts:
		t.Fatalf("a Go change must not use hot reload/restart (fullRestart=%v)", full)
	default:
	}
}

// TestRunHotReloadsWhenDartSourcesChange keeps Dart edits cheap: no rebuild,
// no reinstall, just a hot reload on the live session.
func TestRunHotReloadsWhenDartSourcesChange(t *testing.T) {
	h := newHarness(t)
	h.start(t)

	session := h.awaitStart(t, "initial launch")
	writeFile(t, filepath.Join(h.dartDir, "main.dart"), "void main() { print('edited'); }\n")

	select {
	case full := <-session.restarts:
		if full {
			t.Fatal("a Dart edit must hot reload, not hot restart")
		}
	case <-time.After(settleTimeout):
		t.Fatal("Dart change did not trigger a hot reload")
	}
	h.expectNoStart(t, "a Dart edit must not relaunch the process")
}

// TestRunKeepsAppAliveWhenGenerationFails matters because a half-typed Go API
// is the normal state while editing: tearing the session down would cost a
// full rebuild to get back, for nothing.
func TestRunKeepsAppAliveWhenGenerationFails(t *testing.T) {
	h := newHarness(t)
	var calls atomic.Int32
	h.generate = func() ([]string, error) {
		if calls.Add(1) == 1 {
			return nil, nil
		}
		return nil, errors.New("unsupported exported declaration")
	}
	h.start(t)

	session := h.awaitStart(t, "initial launch")
	writeFile(t, filepath.Join(h.goDir, "api.go"), "package api // broken\n")

	// Wait for the change to actually reach the generator, otherwise the
	// assertions below would hold trivially for a watcher that never fired.
	deadline := time.After(settleTimeout)
	for calls.Load() < 2 {
		select {
		case <-deadline:
			t.Fatal("the Go change never reached the generator")
		case <-time.After(testInterval):
		}
	}

	h.expectNoStart(t, "a failed generation must not restart the app")
	select {
	case <-session.stopped:
		t.Fatal("a failed generation must leave the running app alone")
	default:
	}
}

// TestRunFailsWhenTheFirstGenerationFails: there is nothing to run yet, so the
// error is worth surfacing instead of launching a stale app.
func TestRunFailsWhenTheFirstGenerationFails(t *testing.T) {
	h := newHarness(t)
	h.generate = func() ([]string, error) { return nil, errors.New("boom") }
	h.start(t)

	select {
	case err := <-h.done:
		if err == nil {
			t.Fatal("want an error from the first generation")
		}
	case <-time.After(settleTimeout):
		t.Fatal("Run did not return")
	}
	h.done <- nil // let the cleanup drain
}

func TestRunHandlesKeys(t *testing.T) {
	h := newHarness(t)
	h.start(t)
	session := h.awaitStart(t, "initial launch")

	h.keys <- KeyHotReload
	select {
	case full := <-session.restarts:
		if full {
			t.Fatal("r must hot reload")
		}
	case <-time.After(settleTimeout):
		t.Fatal("r did not hot reload")
	}

	h.keys <- KeyHotRestart
	select {
	case full := <-session.restarts:
		if !full {
			t.Fatal("R must hot restart")
		}
	case <-time.After(settleTimeout):
		t.Fatal("R did not hot restart")
	}

	// g is the manual form of the Go path: regenerate, then replace the
	// process so the rebuilt library is picked up.
	generatedBefore := h.generated.Load()
	h.keys <- KeyRegenerate
	h.awaitStart(t, "g must relaunch the process")
	if h.generated.Load() <= generatedBefore {
		t.Fatal("g did not regenerate the bridge")
	}
}

func TestRunQuitsOnKeyQuit(t *testing.T) {
	h := newHarness(t)
	h.start(t)
	session := h.awaitStart(t, "initial launch")

	h.keys <- KeyQuit
	select {
	case err := <-h.done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil", err)
		}
	case <-time.After(settleTimeout):
		t.Fatal("q did not end the loop")
	}
	select {
	case <-session.stopped:
	case <-time.After(settleTimeout):
		t.Fatal("q must stop the app")
	}
	h.done <- nil // let the cleanup drain
}

// TestRunReturnsWhenTheAppExitsOnItsOwn covers the user closing the app on the
// device: the loop has nothing left to drive and should end rather than poll a
// dead session.
func TestRunReturnsWhenTheAppExitsOnItsOwn(t *testing.T) {
	h := newHarness(t)
	h.start(t)
	session := h.awaitStart(t, "initial launch")

	_ = session.Stop(context.Background())

	select {
	case err := <-h.done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil", err)
		}
	case <-time.After(settleTimeout):
		t.Fatal("Run did not return after the app exited")
	}
	h.done <- nil // let the cleanup drain
}
