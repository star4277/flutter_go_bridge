package devrun

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/star4277/flutter_go_bridge/internal/watcher"
)

func testLoop(output *bytes.Buffer) *loop {
	return &loop{output: output, interval: testInterval}
}

func TestLoopOnKeyDetach(t *testing.T) {
	var buf bytes.Buffer
	l := testLoop(&buf)
	session := newFakeApp()
	l.session = session

	done, err := l.onKey(context.Background(), KeyDetach)
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("KeyDetach should end the loop")
	}
	if l.session != nil {
		t.Fatal("session should be detached (nil)")
	}
	if !strings.Contains(buf.String(), "detaching") {
		t.Fatalf("expected detach message, got %q", buf.String())
	}
}

func TestLoopOnKeyDetachNilSession(t *testing.T) {
	var buf bytes.Buffer
	l := testLoop(&buf)
	// nil session: detaching should not panic.
	done, err := l.onKey(context.Background(), KeyDetach)
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("KeyDetach should still end the loop")
	}
}

func TestLoopOnKeyHelp(t *testing.T) {
	var buf bytes.Buffer
	l := testLoop(&buf)
	done, err := l.onKey(context.Background(), KeyHelp)
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Fatal("KeyHelp should not end the loop")
	}
	if !strings.Contains(buf.String(), "Key commands") || !strings.Contains(buf.String(), "Hot reload") {
		t.Fatalf("help output missing: %q", buf.String())
	}
}

func TestLoopStopSessionNil(t *testing.T) {
	l := testLoop(&bytes.Buffer{})
	l.stopSession(context.Background()) // must not panic
}

func TestLoopHotReloadLogsWarningOnError(t *testing.T) {
	var buf bytes.Buffer
	l := testLoop(&buf)
	session := &alwaysFailsApp{err: errors.New("reload boom")}
	l.session = session
	tr, err := watcher.NewTracker(watcher.Options{Roots: []string{filepath.Join(t.TempDir(), "missing")}})
	if err != nil {
		t.Fatal(err)
	}
	l.dartTracker = tr
	l.hotReload(context.Background())
	if !strings.Contains(buf.String(), "hot reload failed") {
		t.Fatalf("expected hot reload warning, got %q", buf.String())
	}
}

func TestLoopLogf(t *testing.T) {
	var buf bytes.Buffer
	l := testLoop(&buf)
	l.logf("hello %d", 5)
	if !strings.Contains(buf.String(), "[fgb] hello 5") {
		t.Fatalf("got %q", buf.String())
	}
}

type alwaysFailsApp struct{ err error }

func (f *alwaysFailsApp) Restart(context.Context, bool) error { return f.err }
func (f *alwaysFailsApp) Stop(context.Context) error          { return nil }
func (f *alwaysFailsApp) Detach(context.Context) error        { return nil }
func (f *alwaysFailsApp) Events() <-chan Event                { return nil }
func (f *alwaysFailsApp) Handle(Event)                        {}
func (f *alwaysFailsApp) VMServiceURI() string                { return "" }
func (f *alwaysFailsApp) DeviceID() string                    { return "" }

func TestSessionDetachAlreadyExited(t *testing.T) {
	client, _ := fakeDaemon(t, func(string) string { return `[{"id":1,"result":true}]` })
	exited := make(chan struct{})
	close(exited)
	session := &Session{
		options: SessionOptions{Output: &bytes.Buffer{}, Logf: func(string, ...any) {}},
		client:  client, appID: "abc", exited: exited,
	}
	if err := session.detach(context.Background()); err != nil {
		t.Fatalf("detach with exited process should return nil, got %v", err)
	}
}

var _ = errors.New
