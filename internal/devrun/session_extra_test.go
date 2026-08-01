package devrun

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func TestSessionAwaitStartedSuccess(t *testing.T) {
	reader, writer := io.Pipe()
	client := NewClient(reader, io.Discard, io.Discard)
	var logged []string
	session := &Session{
		options: SessionOptions{Output: io.Discard, Logf: func(format string, args ...any) {
			logged = append(logged, fmt.Sprintf(format, args...))
		}},
		client: client,
		exited: make(chan struct{}),
	}
	go func() {
		fmt.Fprintln(writer, `[{"event":"app.start","params":{"appId":"abc","deviceId":"windows"}}]`)
		fmt.Fprintln(writer, `[{"event":"app.debugPort","params":{"appId":"abc","wsUri":"ws://127.0.0.1:1/xx=/ws"}}]`)
		fmt.Fprintln(writer, `[{"event":"app.devTools","params":{"uri":"http://devtools"}}]`)
		fmt.Fprintln(writer, `[{"event":"app.progress","params":{"finished":true,"message":"building"}}]`)
		fmt.Fprintln(writer, `[{"event":"app.started","params":{"appId":"abc"}}]`)
	}()
	if err := session.awaitStarted(context.Background()); err != nil {
		t.Fatalf("awaitStarted: %v", err)
	}
	if session.appID != "abc" || session.deviceID != "windows" {
		t.Fatalf("appID=%q deviceID=%q", session.appID, session.deviceID)
	}
	if session.vmService != "ws://127.0.0.1:1/xx=/ws" {
		t.Fatalf("vmService=%q", session.vmService)
	}
	if len(logged) == 0 || !strings.Contains(strings.Join(logged, " "), "DevTools") {
		t.Fatalf("devTools was not logged: %v", logged)
	}
}

func TestSessionAwaitStartedCanceled(t *testing.T) {
	reader, writer := io.Pipe()
	client := NewClient(reader, io.Discard, io.Discard)
	session := &Session{client: client, exited: make(chan struct{}),
		options: SessionOptions{Output: io.Discard, Logf: func(string, ...any) {}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = writer
	if err := session.awaitStarted(ctx); err == nil {
		t.Fatal("want a context error")
	}
	_ = reader.Close()
}

func TestSessionStartupFailureNoExitErr(t *testing.T) {
	reader, writer := io.Pipe()
	client := NewClient(reader, io.Discard, io.Discard)
	exited := make(chan struct{})
	close(exited)
	session := &Session{client: client, exited: exited,
		options: SessionOptions{Output: io.Discard, Logf: func(string, ...any) {}}}
	// Writer closed -> reader gets EOF -> Events() channel closes.
	go func() { time.Sleep(20 * time.Millisecond); _ = writer.Close() }()
	if err := session.awaitStarted(context.Background()); err == nil || !strings.Contains(err.Error(), "before the app started") {
		t.Fatalf("got %v", err)
	}
}

func TestSessionStartupFailureWithExitErr(t *testing.T) {
	reader, writer := io.Pipe()
	client := NewClient(reader, io.Discard, io.Discard)
	exited := make(chan struct{})
	close(exited)
	session := &Session{client: client, exited: exited, exitErr: errors.New("boom"),
		options: SessionOptions{Output: io.Discard, Logf: func(string, ...any) {}}}
	go func() { time.Sleep(20 * time.Millisecond); _ = writer.Close() }()
	if err := session.awaitStarted(context.Background()); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("got %v", err)
	}
}

func TestSessionDetachSucceeds(t *testing.T) {
	client, _ := fakeDaemon(t, func(string) string {
		return `[{"id":1,"result":true}]`
	})
	exited := make(chan struct{})
	session := &Session{
		client:  client,
		appID:   "abc",
		options: SessionOptions{Output: io.Discard, Logf: func(string, ...any) {}},
		exited:  exited,
	}
	go func() { time.Sleep(50 * time.Millisecond); close(exited) }()
	if err := session.Detach(context.Background()); err != nil {
		t.Fatalf("Detach: %v", err)
	}
}
