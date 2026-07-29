package devrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// stopGracePeriod bounds how long we wait for `app.stop` to unwind the app,
// the Dart Development Service and the device connection. Past it we kill the
// process tree: a wedged flutter would otherwise block the restart forever.
const stopGracePeriod = 15 * time.Second

// SessionOptions configure one `flutter run` process.
type SessionOptions struct {
	// WorkDir is the Flutter project directory.
	WorkDir string
	// FlutterArgs are appended after `run --machine`, e.g. -d, --flavor.
	FlutterArgs []string
	// Output receives flutter's human-readable output.
	Output io.Writer
	// Logf reports session progress; nil silences it.
	Logf func(format string, args ...any)
}

// Session is one running app: the flutter process plus the daemon connection
// that controls it.
type Session struct {
	options SessionOptions
	command *exec.Cmd
	client  *Client

	appID     string
	deviceID  string
	vmService string

	exited   chan struct{}
	exitErr  error
	stopOnce sync.Once
}

// StartSession launches `flutter run --machine` and returns once the app
// reports app.started.
//
// Machine mode is used rather than the interactive runner because it exposes a
// structured protocol: a definite app.started signal, the VM Service URI as an
// event, and stop/restart as requests. Scraping the human-readable log would
// couple us to English strings that vary across Flutter forks.
func StartSession(ctx context.Context, options SessionOptions) (*Session, error) {
	if options.Output == nil {
		options.Output = os.Stdout
	}
	if options.Logf == nil {
		options.Logf = func(string, ...any) {}
	}

	flutter, err := exec.LookPath("flutter")
	if err != nil {
		return nil, fmt.Errorf("flutter not found in PATH: %w", err)
	}
	args := append([]string{"run", "--machine"}, options.FlutterArgs...)
	command := exec.Command(flutter, args...)
	command.Dir = options.WorkDir
	command.Env = append(os.Environ(), "DART_SUPPRESS_ANALYTICS=true")
	command.Stderr = options.Output
	configureProcessGroup(command)

	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start `flutter %s`: %w", strings.Join(args, " "), err)
	}

	session := &Session{
		options: options,
		command: command,
		client:  NewClient(stdout, stdin, options.Output),
		exited:  make(chan struct{}),
	}
	// Wait only after the reader has drained the pipe: cmd.Wait closes it, so
	// calling it concurrently with the read loop would truncate the stream.
	go func() {
		<-session.client.Done()
		session.exitErr = command.Wait()
		close(session.exited)
	}()

	if err := session.awaitStarted(ctx); err != nil {
		// Leave nothing behind when startup fails half-way.
		_ = killProcessTree(command)
		<-session.exited
		return nil, err
	}
	return session, nil
}

// awaitStarted consumes events until the app is up, recording the app id and
// VM Service URI on the way. There is deliberately no timeout: a cold Android
// build legitimately takes minutes, and ctx already carries the user's Ctrl+C.
func (s *Session) awaitStarted(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-s.client.Events():
			if !ok {
				return s.startupFailure()
			}
			s.Handle(event)
			if event.Name == "app.started" {
				return nil
			}
		}
	}
}

// startupFailure explains an exit that happened before app.started.
func (s *Session) startupFailure() error {
	<-s.exited
	if s.exitErr != nil {
		return fmt.Errorf("flutter run exited before the app started: %w", s.exitErr)
	}
	return errors.New("flutter run exited before the app started")
}

// Handle records state carried by an event and prints the parts worth showing.
// The run loop keeps calling it for the life of the session.
func (s *Session) Handle(event Event) {
	switch event.Name {
	case "app.start":
		s.appID = event.String("appId")
		s.deviceID = event.String("deviceId")
	case "app.debugPort":
		// The daemon hands us the VM Service URI directly, so there is no need
		// to scrape "The Dart VM service is listening on ..." from the log.
		s.vmService = event.String("wsUri")
	case "app.devTools":
		if uri := event.String("uri"); uri != "" {
			s.options.Logf("DevTools: %s", uri)
		}
	case "app.progress":
		if finished, _ := event.Params["finished"].(bool); !finished {
			if message := event.String("message"); message != "" {
				s.options.Logf("%s", message)
			}
		}
	case "app.log", "daemon.logMessage":
		text := event.String("log")
		if text == "" {
			text = event.String("message")
		}
		if text != "" {
			fmt.Fprintln(s.options.Output, strings.TrimRight(text, "\n"))
		}
	}
}

// Events exposes the daemon event stream so the run loop can keep draining it
// after startup.
func (s *Session) Events() <-chan Event { return s.client.Events() }

// Exited is closed when the flutter process ends, whether we stopped it or the
// user closed the app on the device.
func (s *Session) Exited() <-chan struct{} { return s.exited }

// VMServiceURI is the Dart VM Service WebSocket URI, empty in release mode.
func (s *Session) VMServiceURI() string { return s.vmService }

// DeviceID is the device the app was launched on.
func (s *Session) DeviceID() string { return s.deviceID }

// operationResult mirrors flutter's OperationResult for app.restart.
type operationResult struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Restart performs a hot reload (full=false) or hot restart (full=true).
//
// Neither refreshes the Go dynamic library: both rebuild only the Dart
// isolate, leaving the already-loaded native library in place. Use the run
// loop's full restart for that.
func (s *Session) Restart(ctx context.Context, full bool) error {
	if s.appID == "" {
		return errors.New("no running app")
	}
	reason := "hot reload"
	if full {
		reason = "hot restart"
	}
	raw, err := s.client.Request(ctx, "app.restart", map[string]any{
		"appId":       s.appID,
		"fullRestart": full,
		"pause":       false,
		"reason":      reason,
	})
	if err != nil {
		return err
	}
	var result operationResult
	if len(raw) > 0 && json.Unmarshal(raw, &result) == nil && result.Code != 0 {
		return fmt.Errorf("%s failed: %s", reason, result.Message)
	}
	return nil
}

// Stop shuts the app down, preferring the graceful path so flutter can detach
// from the device and tear down DDS. It falls back to killing the process tree
// and always waits for the process to be reaped, so the caller can start the
// next run without racing the old one for the device.
func (s *Session) Stop(ctx context.Context) error {
	var err error
	s.stopOnce.Do(func() {
		err = s.stop(ctx)
	})
	<-s.exited
	return err
}

func (s *Session) stop(ctx context.Context) error {
	if s.appID != "" {
		stopCtx, cancel := context.WithTimeout(ctx, stopGracePeriod)
		_, err := s.client.Request(stopCtx, "app.stop", map[string]any{"appId": s.appID})
		cancel()
		if err == nil {
			select {
			case <-s.exited:
				return nil
			case <-time.After(stopGracePeriod):
				s.options.Logf("app.stop did not finish in %s, killing the process tree", stopGracePeriod)
			}
		}
	}
	return killProcessTree(s.command)
}

// Detach leaves the app running on the device and disconnects from it.
func (s *Session) Detach(ctx context.Context) error {
	var err error
	s.stopOnce.Do(func() {
		if s.appID != "" {
			detachCtx, cancel := context.WithTimeout(ctx, stopGracePeriod)
			_, err = s.client.Request(detachCtx, "app.detach", map[string]any{"appId": s.appID})
			cancel()
		}
		if err != nil {
			err = killProcessTree(s.command)
		}
	})
	<-s.exited
	return err
}
