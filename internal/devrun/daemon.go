// Package devrun drives a `flutter run` process from code generation: it
// regenerates the bridge when Go sources change and then restarts the app,
// because a native dynamic library cannot be swapped into a live process.
//
// Neither hot reload nor hot restart helps here. Both only rebuild the Dart
// isolate; the library opened via dlopen stays resident, and on Android the
// rebuilt .so has not even been pushed to the device. Only a process-level
// restart re-links the app against freshly generated Go code.
package devrun

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

// Event is a daemon event such as `app.started`.
type Event struct {
	Name   string
	Params map[string]any
}

// String returns a param as a string, or "" when absent or of another type.
func (e Event) String(key string) string {
	value, _ := e.Params[key].(string)
	return value
}

// wireMessage is one decoded daemon message. Requests carry Method, responses
// carry Result or Error against the request's ID, and events carry Event.
type wireMessage struct {
	ID     *int            `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`
	Event  string          `json:"event,omitempty"`
}

type response struct {
	result json.RawMessage
	err    error
}

// Client speaks the JSON protocol exposed by `flutter run --machine`.
//
// The framing is one message per line wrapped in a single-element array --
// `[{"event":"app.started",...}]`. Lines that do not match that shape are
// ordinary human-readable output and are forwarded verbatim to Passthrough.
type Client struct {
	writer io.Writer
	events chan Event

	writeMu sync.Mutex

	mu      sync.Mutex
	nextID  int
	pending map[int]chan response

	done      chan struct{}
	closing   chan struct{}
	closeOnce sync.Once
	readErr   error
}

// NewClient starts reading messages from reader and sends requests to writer.
// The transport is injected rather than owned so tests can drive it with
// in-memory pipes. Non-protocol lines are copied to passthrough.
func NewClient(reader io.Reader, writer io.Writer, passthrough io.Writer) *Client {
	client := &Client{
		writer:  writer,
		events:  make(chan Event, 256),
		pending: map[int]chan response{},
		done:    make(chan struct{}),
		closing: make(chan struct{}),
	}
	go client.readLoop(reader, passthrough)
	return client
}

// Close stops event delivery when the consumer is leaving. The underlying
// process/pipe owner is still responsible for closing the transport.
func (c *Client) Close() {
	c.closeOnce.Do(func() { close(c.closing) })
}

// Events yields decoded daemon events until the transport ends, at which point
// the channel is closed. It has a single consumer by design: the session hands
// the channel to the run loop once startup is complete.
func (c *Client) Events() <-chan Event {
	return c.events
}

// Done is closed once the reader has ended.
func (c *Client) Done() <-chan struct{} {
	return c.done
}

// Err reports why reading ended, or nil on a clean end of stream.
func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readErr
}

func (c *Client) readLoop(reader io.Reader, passthrough io.Writer) {
	defer close(c.events)
	defer close(c.done)

	scanner := bufio.NewScanner(reader)
	// Daemon lines carry whole stack traces and build logs; the 64KB default
	// would truncate them into undecodable fragments.
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		message, ok := decodeLine(line)
		if !ok {
			if passthrough != nil && strings.TrimSpace(line) != "" {
				fmt.Fprintln(passthrough, line)
			}
			continue
		}
		switch {
		case message.Event != "":
			params := map[string]any{}
			if len(message.Params) > 0 {
				_ = json.Unmarshal(message.Params, &params)
			}
			select {
			case c.events <- Event{Name: message.Event, Params: params}:
			case <-c.closing:
				return
			}
		case message.ID != nil:
			c.complete(*message.ID, message)
		}
	}
	err := scanner.Err()
	c.mu.Lock()
	c.readErr = err
	c.mu.Unlock()
	// Waiters are released by close(c.done) in the deferred cleanup rather than
	// from a snapshot of the pending map here: a request registered between
	// taking such a snapshot and closing done would be notified by neither, and
	// wait forever on a context that may never be cancelled.
}

func (c *Client) complete(id int, message wireMessage) {
	c.mu.Lock()
	waiter, found := c.pending[id]
	delete(c.pending, id)
	c.mu.Unlock()
	if !found {
		return
	}
	if len(message.Error) > 0 {
		waiter <- response{err: fmt.Errorf("flutter daemon error: %s", strings.TrimSpace(string(message.Error)))}
		return
	}
	waiter <- response{result: message.Result}
}

// Request sends a method call and waits for its response.
func (c *Client) Request(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	c.mu.Lock()
	select {
	case <-c.done:
		c.mu.Unlock()
		return nil, errors.New("flutter daemon is not running")
	default:
	}
	c.nextID++
	id := c.nextID
	waiter := make(chan response, 1)
	c.pending[id] = waiter
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	if params == nil {
		params = map[string]any{}
	}
	payload, err := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	c.writeMu.Lock()
	_, err = fmt.Fprintf(c.writer, "[%s]\n", payload)
	c.writeMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("send %s: %w", method, err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-waiter:
		return result.result, result.err
	case <-c.done:
		// The reader has stopped, so no further response can arrive -- but one
		// may already be buffered from just before it did.
		select {
		case result := <-waiter:
			return result.result, result.err
		default:
		}
		return nil, errors.New("flutter daemon exited before responding")
	}
}

// decodeLine recognises a protocol line and unwraps its single-element array.
func decodeLine(line string) (wireMessage, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "[{") || !strings.HasSuffix(trimmed, "}]") {
		return wireMessage{}, false
	}
	var message wireMessage
	if err := json.Unmarshal([]byte(trimmed[1:len(trimmed)-1]), &message); err != nil {
		return wireMessage{}, false
	}
	return message, true
}
