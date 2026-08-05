package devrun

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestSessionRestartSuccess covers the nil-return path of Restart when the
// daemon answers with a zero exit code.
func TestSessionRestartSuccess(t *testing.T) {
	client, _ := fakeDaemon(t, func(string) string {
		return `[{"id":1,"result":{"code":0,"message":"ok"}}]`
	})
	session := &Session{client: client, appID: "abc"}
	if err := session.Restart(context.Background(), false); err != nil {
		t.Fatalf("restart with code 0 should succeed, got %v", err)
	}
}

// TestClientRequestBufferedBeforeDone drives the "response buffered as the
// daemon exits" path deterministically. Request registers its waiter and then
// blocks on the write lock, so we can deliver the response and end the reader
// first; by the time Request's select runs, both the buffered response and the
// closed done channel are ready, so the inner "just as it did" branch (rather
// than the outer normal return) is eventually selected.
func TestClientRequestBufferedBeforeDone(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		toClientReader, toClientWriter := io.Pipe()
		client := NewClient(toClientReader, io.Discard, io.Discard)
		client.writeMu.Lock()

		result := make(chan error, 1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := client.Request(context.Background(), "app.stop", map[string]any{"appId": "abc"})
			result <- err
		}()

		// Wait for the waiter to be registered rather than sleeping for it.
		// Request records pending[id] under mu and only then blocks on the
		// write lock, so seeing the entry proves it is past the guard that
		// rejects a request once done is closed. A fixed sleep let a loaded
		// machine run the rest of this iteration first, and Request then
		// failed with "flutter daemon is not running".
		waitForPendingRequest(t, client)

		// Deliver the response, then end the reader: the read loop completes
		// the waiter and only afterwards closes done, exactly as in flight.
		fmt.Fprintln(toClientWriter, `[{"id":1,"result":true}]`)
		_ = toClientWriter.Close()

		// Release Request so it reaches its select with both arms ready.
		client.writeMu.Unlock()

		if err := <-result; err != nil {
			t.Fatalf("iteration %d: Request returned %v, want success", i, err)
		}
		_ = toClientReader.Close()
	}
	wg.Wait()
}

// waitForPendingRequest blocks until a Request has registered its waiter.
func waitForPendingRequest(t *testing.T, client *Client) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		client.mu.Lock()
		pending := len(client.pending)
		client.mu.Unlock()
		if pending > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("Request never registered its waiter")
		}
		runtime.Gosched()
	}
}

// TestClientReadLoopStopsWhenClosing fills the event buffer so the read loop
// blocks delivering an event, then Close must let it return.
func TestClientReadLoopStopsWhenClosing(t *testing.T) {
	reader, writer := io.Pipe()
	client := NewClient(reader, io.Discard, io.Discard)
	go func() {
		for i := 0; i < 400; i++ {
			fmt.Fprintf(writer, "[{\"event\":\"e%d\",\"params\":{}}]\n", i)
		}
		_ = writer.Close()
	}()
	time.Sleep(150 * time.Millisecond)
	client.Close()
	select {
	case <-client.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("read loop did not stop after Close")
	}
}
