package devrun

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuffer collects passthrough output, which the read loop writes from its
// own goroutine while the test inspects it.
type syncBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (s *syncBuffer) Write(data []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buffer.Write(data)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buffer.String()
}

// TestClientDecodesEventsAndForwardsPlainLines covers the framing rule the
// whole command rests on: a protocol message is a single-element JSON array on
// its own line, and anything else is human-readable output to be shown as-is.
func TestClientDecodesEventsAndForwardsPlainLines(t *testing.T) {
	reader, writer := io.Pipe()
	passthrough := &syncBuffer{}
	client := NewClient(reader, io.Discard, passthrough)

	go func() {
		fmt.Fprintln(writer, "Launching lib/main.dart on Windows in debug mode...")
		fmt.Fprintln(writer, `[{"event":"app.start","params":{"appId":"abc","deviceId":"windows"}}]`)
		fmt.Fprintln(writer, `[{"event":"app.debugPort","params":{"appId":"abc","wsUri":"ws://127.0.0.1:53576/xx=/ws"}}]`)
		fmt.Fprintln(writer, `[{"event":"app.started","params":{"appId":"abc"}}]`)
		_ = writer.Close()
	}()

	var names []string
	events := map[string]Event{}
	for event := range client.Events() {
		names = append(names, event.Name)
		events[event.Name] = event
	}

	want := []string{"app.start", "app.debugPort", "app.started"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", names, want)
	}
	if got := events["app.start"].String("appId"); got != "abc" {
		t.Fatalf("appId = %q, want abc", got)
	}
	if got := events["app.debugPort"].String("wsUri"); got != "ws://127.0.0.1:53576/xx=/ws" {
		t.Fatalf("wsUri = %q", got)
	}
	if out := passthrough.String(); !strings.Contains(out, "Launching lib/main.dart") {
		t.Fatalf("plain line was not forwarded: %q", out)
	}
	if strings.Contains(passthrough.String(), "app.started") {
		t.Fatal("protocol lines must not leak into the human-readable output")
	}
}

// fakeDaemon wires a Client to a scripted peer, returning the client and a
// channel of the raw lines the client sent.
func fakeDaemon(t *testing.T, reply func(line string) string) (*Client, <-chan string) {
	t.Helper()
	toClientReader, toClientWriter := io.Pipe()
	fromClientReader, fromClientWriter := io.Pipe()
	client := NewClient(toClientReader, fromClientWriter, io.Discard)

	sent := make(chan string, 4)
	go func() {
		scanner := bufio.NewScanner(fromClientReader)
		for scanner.Scan() {
			line := scanner.Text()
			sent <- line
			if response := reply(line); response != "" {
				fmt.Fprintln(toClientWriter, response)
			}
		}
	}()
	t.Cleanup(func() {
		_ = toClientWriter.Close()
		_ = fromClientWriter.Close()
	})
	return client, sent
}

func TestClientRequestFramesMessageAndReturnsResult(t *testing.T) {
	client, sent := fakeDaemon(t, func(string) string {
		return `[{"id":1,"result":true}]`
	})

	raw, err := client.Request(context.Background(), "app.stop", map[string]any{"appId": "abc"})
	if err != nil {
		t.Fatal(err)
	}

	line := <-sent
	const want = `[{"id":1,"method":"app.stop","params":{"appId":"abc"}}]`
	if line != want {
		t.Fatalf("sent %s, want %s", line, want)
	}
	var result bool
	if err := json.Unmarshal(raw, &result); err != nil || !result {
		t.Fatalf("result = %s (%v), want true", raw, err)
	}
}

func TestClientRequestSurfacesDaemonError(t *testing.T) {
	client, _ := fakeDaemon(t, func(string) string {
		return `[{"id":1,"error":"app 'abc' not found"}]`
	})

	_, err := client.Request(context.Background(), "app.restart", map[string]any{"appId": "abc"})
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v, want it to carry the daemon message", err)
	}
}

// TestClientRequestFailsWhenDaemonExits keeps a restart from hanging forever
// when flutter dies mid-request.
func TestClientRequestFailsWhenDaemonExits(t *testing.T) {
	reader, writer := io.Pipe()
	client := NewClient(reader, io.Discard, io.Discard)

	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = writer.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.Request(ctx, "app.stop", map[string]any{"appId": "abc"}); err == nil {
		t.Fatal("want an error once the daemon is gone")
	}
}

func TestDecodeLineRejectsNonProtocolLines(t *testing.T) {
	for _, line := range []string{
		"",
		"Syncing files to device...",
		`{"event":"app.started"}`,
		`[{"event":"app.started"}`,
		`[not json]`,
	} {
		if _, ok := decodeLine(line); ok {
			t.Fatalf("decodeLine(%q) was accepted as a protocol message", line)
		}
	}
	if _, ok := decodeLine(`  [{"event":"app.started","params":{}}]  `); !ok {
		t.Fatal("a padded protocol line must still decode")
	}
}
