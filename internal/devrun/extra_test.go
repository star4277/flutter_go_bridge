package devrun

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---- daemon.go extra coverage ----

func TestClientCloseDoneErr(t *testing.T) {
	reader, writer := io.Pipe()
	client := NewClient(reader, io.Discard, io.Discard)

	if client.Done() == nil {
		t.Fatal("Done() should return a channel")
	}
	client.Close()
	_ = writer.Close()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-client.Done():
			// Err may be nil on a clean EOF.
			_ = client.Err()
			return
		case <-deadline:
			t.Fatal("Done should close once the reader ends")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestClientRequestAfterDaemonGone(t *testing.T) {
	reader, writer := io.Pipe()
	client := NewClient(reader, io.Discard, io.Discard)
	_ = writer.Close()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-client.Done():
			goto done
		case <-deadline:
			t.Fatal("daemon did not finish")
		case <-time.After(5 * time.Millisecond):
		}
	}
done:
	if _, err := client.Request(context.Background(), "app.stop", nil); err == nil {
		t.Fatal("expected an error once the daemon is done")
	}
}

func TestClientRequestContextCancel(t *testing.T) {
	client, _ := fakeDaemon(t, func(string) string { return "" })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Request(ctx, "app.stop", map[string]any{"appId": "abc"}); err == nil {
		t.Fatal("expected a context cancellation error")
	}
}

func TestClientRequestMarshalError(t *testing.T) {
	reader, _ := io.Pipe()
	client := NewClient(reader, &bytes.Buffer{}, io.Discard)
	// params containing a channel is not JSON-marshalable
	if _, err := client.Request(context.Background(), "app.stop", map[string]any{"x": make(chan int)}); err == nil {
		t.Fatal("expected a marshal error")
	}
	client.Close()
}

// ---- session.go Handle / accessors / Restart ----

func TestSessionHandleCoversEventKinds(t *testing.T) {
	var output bytes.Buffer
	var logged []string
	session := &Session{
		options: SessionOptions{Output: &output, Logf: func(format string, args ...any) {
			logged = append(logged, fmt.Sprintf(format, args...))
		}},
	}

	session.Handle(Event{Name: "app.start", Params: map[string]any{"appId": "abc", "deviceId": "win"}})
	if session.appID != "abc" || session.deviceID != "win" {
		t.Fatalf("app.start not recorded: %#v %#v", session.appID, session.deviceID)
	}
	session.Handle(Event{Name: "app.debugPort", Params: map[string]any{"wsUri": "ws://x/ws"}})
	if session.vmService != "ws://x/ws" {
		t.Fatalf("vmService = %q", session.vmService)
	}
	session.Handle(Event{Name: "app.devTools", Params: map[string]any{"uri": "http://devtools"}})
	if len(logged) != 1 || !strings.Contains(logged[0], "DevTools") {
		t.Fatalf("devtools log missing: %#v", logged)
	}
	// uri empty -> nothing logged
	session.Handle(Event{Name: "app.devTools", Params: map[string]any{"uri": ""}})
	// progress with message
	session.Handle(Event{Name: "app.progress", Params: map[string]any{"message": "building"}})
	if len(logged) != 2 || !strings.Contains(logged[1], "building") {
		t.Fatalf("progress log missing: %#v", logged)
	}
	// finished=true suppresses the message
	session.Handle(Event{Name: "app.progress", Params: map[string]any{"finished": true, "message": "done"}})
	// empty message suppresses logging
	session.Handle(Event{Name: "app.progress", Params: map[string]any{"finished": false}})
	// app.log via "log"
	session.Handle(Event{Name: "app.log", Params: map[string]any{"log": "a log line\n"}})
	// daemon.logMessage via "message"
	session.Handle(Event{Name: "daemon.logMessage", Params: map[string]any{"message": "msg"}})
	if got := output.String(); !strings.Contains(got, "a log line") || !strings.Contains(got, "msg") {
		t.Fatalf("output missing log lines: %q", got)
	}
}

func TestSessionAccessors(t *testing.T) {
	exited := make(chan struct{})
	reader, _ := io.Pipe()
	session := &Session{client: NewClient(reader, io.Discard, io.Discard), exited: exited, appID: "6", vmService: "ws://v", deviceID: "d1"}
	session.client.Close()
	if session.Events() == nil {
		t.Fatal("Events() should return a channel")
	}
	if session.Exited() != exited {
		t.Fatal("Exited() should return the exited channel")
	}
	if session.VMServiceURI() != "ws://v" {
		t.Fatalf("vmService = %q", session.VMServiceURI())
	}
	if session.DeviceID() != "d1" {
		t.Fatalf("deviceID = %q", session.DeviceID())
	}
}

func TestSessionRestartNoApp(t *testing.T) {
	session := &Session{}
	if err := session.Restart(context.Background(), false); err == nil {
		t.Fatal("expected no-app error")
	}
}

func TestSessionRestartNonZeroCodeIsError(t *testing.T) {
	client, _ := fakeDaemon(t, func(string) string {
		return `[{"id":1,"result":{"code":1,"message":"in progress"}}]`
	})
	session := &Session{client: client, appID: "abc"}
	if err := session.Restart(context.Background(), true); err == nil || !strings.Contains(err.Error(), "in progress") {
		t.Fatalf("a non-zero restart code must surface as an error, got %v", err)
	}
}

func TestSessionRestartOperationResultError(t *testing.T) {
	client, _ := fakeDaemon(t, func(string) string {
		return `[{"id":1,"result":{"code":2,"message":"failed restart"}}]`
	})
	session := &Session{client: client, appID: "abc"}
	err := session.Restart(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "failed restart") {
		t.Fatalf("expected operation failure, got %v", err)
	}
}

func TestSessionRestartRequestError(t *testing.T) {
	client, _ := fakeDaemon(t, func(string) string {
		return `[{"id":1,"error":"daemon exploded"}]`
	})
	session := &Session{client: client, appID: "abc"}
	if err := session.Restart(context.Background(), false); err == nil || !strings.Contains(err.Error(), "daemon exploded") {
		t.Fatalf("expected daemon error, got %v", err)
	}
}

func TestDaemonSucceededVariants(t *testing.T) {
	if !daemonSucceeded(nil) {
		t.Fatal("nil raw should be success")
	}
	if !daemonSucceeded(json.RawMessage("not json")) {
		t.Fatal("unparseable raw should be success")
	}
	if !daemonSucceeded(json.RawMessage("true")) {
		t.Fatal("true should be success")
	}
	if daemonSucceeded(json.RawMessage("false")) {
		t.Fatal("false should be refusal")
	}
}

// ---- loop.go helpers ----

func TestJoinPaths(t *testing.T) {
	if got := joinPaths(nil); got != "(none)" {
		t.Fatalf("got %q", got)
	}
	if got := joinPaths([]string{"a", "b"}); got != "a, b" {
		t.Fatalf("got %q", got)
	}
}

func TestRelativeTo(t *testing.T) {
	base := filepath.Join("x", "base")
	inside := filepath.Join(base, "sub", "file.dart")
	if got := relativeTo(base, inside); filepath.Clean(got) != filepath.Clean(filepath.Join("sub", "file.dart")) {
		t.Fatalf("inside project = %q", got)
	}
	// The Go module directory may be "." from the test's cwd; force an absolute
	// base so an outside path cannot accidentally look relative.
	outside := relativeTo("/", "/abs/outside.dart")
	if outside == "" {
		t.Fatal("expected a non-empty absolute fallback")
	}
}

func TestSessionDetachSuccess(t *testing.T) {
	client, _ := fakeDaemon(t, func(string) string {
		return `[{"id":1,"result":true}]`
	})
	exited := make(chan struct{})
	session := &Session{
		options: SessionOptions{Output: io.Discard, Logf: func(string, ...any) {}},
		client:  client, appID: "abc",
		exited: exited,
	}
	go func() { close(exited) }()
	if err := session.detach(context.Background()); err != nil {
		t.Fatalf("detach success returned %v", err)
	}
}

func TestSessionStopSuccessPath(t *testing.T) {
	client, _ := fakeDaemon(t, func(string) string {
		return `[{"id":1,"result":true}]`
	})
	exited := make(chan struct{})
	session := &Session{
		options: SessionOptions{Output: io.Discard, Logf: func(string, ...any) {}},
		client:  client, appID: "abc",
		exited: exited,
	}
	go func() { close(exited) }()
	if err := session.stop(context.Background()); err != nil {
		t.Fatalf("stop returned %v", err)
	}
}
