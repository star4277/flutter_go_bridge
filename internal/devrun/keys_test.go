package devrun

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

type failWriter struct {
	count atomic.Int32
}

func (f *failWriter) Write(data []byte) (int, error) {
	f.count.Add(1)
	return 0, errors.New("boom")
}

func TestCRLFWriterNormalizes(t *testing.T) {
	var buf bytes.Buffer
	w := crlfWriter{inner: &buf}
	n, err := w.Write([]byte("hello\nworld\r\nagain"))
	if err != nil {
		t.Fatal(err)
	}
	if n != len("hello\nworld\r\nagain") {
		t.Fatalf("reported %d bytes, want %d", n, len("hello\nworld\r\nagain"))
	}
	if got := buf.String(); got != "hello\r\nworld\r\nagain" {
		t.Fatalf("got %q", got)
	}
}

func TestCRLFWriterPropagatesInnerError(t *testing.T) {
	inner := &failWriter{}
	w := crlfWriter{inner: inner}
	if _, err := w.Write([]byte("a\nb")); err == nil {
		t.Fatal("expected the inner error to propagate")
	}
}

func TestKeyboardSendAndClose(t *testing.T) {
	board := &keyboard{keys: make(chan Key, 1), restore: func() {}, closed: make(chan struct{})}
	if !board.send(KeyHotReload) {
		t.Fatal("first send should succeed")
	}
	if got := <-board.keys; got != KeyHotReload {
		t.Fatalf("got %#v", got)
	}
	if board.Raw() {
		t.Fatal("default should not be raw")
	}
	if board.Keys() != board.keys {
		t.Fatal("Keys() should return the channel")
	}
	// Fill the buffer so the closed branch of send wins deterministically.
	if !board.send(KeyQuit) {
		t.Fatal("buffer fill must succeed")
	}
	board.Close()
	if board.send(KeyQuit) {
		t.Fatal("send after close must fail")
	}
}

func TestNewKeyboardReadsLinesFromStdin(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(input, []byte("r\n\nq\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(input)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })

	original := os.Stdin
	os.Stdin = file
	t.Cleanup(func() { os.Stdin = original })

	board := newKeyboard()
	var received []Key
	for key := range board.Keys() {
		received = append(received, key)
		// Stop after the first line so the test does not rely on q semantics.
		break
	}
	board.Close()
	if len(received) != 1 || received[0] != KeyHotReload {
		t.Fatalf("got %v, want [r]", received)
	}
}

func TestHelpTextDocumentsGoReload(t *testing.T) {
	if !strings.Contains(HelpText, "does NOT reload the Go library") {
		t.Fatalf("HelpText must warn that r/R do not reload Go: %q", HelpText)
	}
}



