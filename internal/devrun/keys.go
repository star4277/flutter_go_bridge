package devrun

import (
	"bufio"
	"bytes"
	"io"
	"os"

	"golang.org/x/term"
)

// Key is a keyboard command. Machine mode turns off flutter's own key
// handling, so we read the keys and translate them into daemon requests.
type Key rune

const (
	// KeyHotReload rebuilds the Dart isolate only.
	KeyHotReload Key = 'r'
	// KeyHotRestart restarts the Dart isolate only.
	KeyHotRestart Key = 'R'
	// KeyRegenerate regenerates the bridge and restarts the whole process,
	// which is the only way to pick up a rebuilt Go dynamic library.
	KeyRegenerate Key = 'g'
	// KeyQuit stops the app and exits.
	KeyQuit Key = 'q'
	// KeyDetach leaves the app running and exits.
	KeyDetach Key = 'd'
	// KeyHelp prints the key bindings.
	KeyHelp Key = 'h'
)

// HelpText documents the bindings. It states plainly that r and R do not
// refresh Go code, because that is the one thing a user coming from
// `flutter run` will assume works and it silently does not.
const HelpText = `Key commands:
  r  Hot reload      -- Dart only, does NOT reload the Go library
  R  Hot restart     -- Dart only, does NOT reload the Go library
  g  Regenerate the bridge and restart the app (reloads the Go library)
  q  Quit
  d  Detach, leaving the app running on the device
  h  Show this help

Editing Go sources triggers "g" automatically; editing Dart sources triggers
"r" automatically.`

// keyboard delivers keypresses from stdin.
//
// When stdin is a terminal it is switched to raw mode so a single keypress
// registers without Enter, matching `flutter run`. Otherwise -- piped input,
// CI -- it degrades to line mode, where the first character of each line is
// taken as the command.
type keyboard struct {
	keys    chan Key
	raw     bool
	restore func()
}

func newKeyboard() *keyboard {
	board := &keyboard{keys: make(chan Key, 16), restore: func() {}}
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		if state, err := term.MakeRaw(fd); err == nil {
			board.raw = true
			board.restore = func() { _ = term.Restore(fd, state) }
			go board.readRaw()
			return board
		}
	}
	go board.readLines()
	return board
}

// Keys yields keypresses.
func (k *keyboard) Keys() <-chan Key { return k.keys }

// Close restores the terminal. It must run on every exit path, including the
// signal path, or the shell is left in raw mode with no echo.
func (k *keyboard) Close() { k.restore() }

// Raw reports whether the terminal is in raw mode, which also means output
// needs explicit CRLF line endings.
func (k *keyboard) Raw() bool { return k.raw }

func (k *keyboard) readRaw() {
	defer close(k.keys)
	buffer := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buffer)
		if err != nil {
			return
		}
		if n == 0 {
			continue
		}
		// Raw mode suppresses signal generation, so Ctrl+C arrives as a byte
		// rather than SIGINT. Treat it as quit, otherwise it would do nothing.
		if buffer[0] == 3 {
			k.keys <- KeyQuit
			continue
		}
		k.keys <- Key(buffer[0])
	}
}

func (k *keyboard) readLines() {
	defer close(k.keys)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		k.keys <- Key([]rune(line)[0])
	}
}

// crlfWriter terminates lines with CRLF.
//
// Raw mode disables the terminal's own newline translation, so without this
// every line after the first would start at the previous line's end column and
// flutter's output would walk diagonally down the screen.
type crlfWriter struct{ inner io.Writer }

func (c crlfWriter) Write(data []byte) (int, error) {
	// Normalise first so input that already ends lines with CRLF does not
	// become CR CR LF.
	normalised := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	if _, err := c.inner.Write(bytes.ReplaceAll(normalised, []byte("\n"), []byte("\r\n"))); err != nil {
		return 0, err
	}
	// io.Writer must report progress against the caller's slice, not ours.
	return len(data), nil
}
