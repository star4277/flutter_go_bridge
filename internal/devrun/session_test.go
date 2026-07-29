package devrun

import (
	"context"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestKillProcessTreeToleratesAnAlreadyReapedProcess reproduces the state that
// produced a spurious "invalid argument" warning on every clean shutdown: the
// session waits for the process in the background, and once that Wait returns
// the OS handle is released, which Windows reports as EINVAL rather than
// ErrProcessDone.
func TestKillProcessTreeToleratesAnAlreadyReapedProcess(t *testing.T) {
	// The test binary itself is the only executable guaranteed to be here.
	// Filtering to a test that does not exist makes it exit 0 immediately.
	command := exec.Command(os.Args[0], "-test.run=TestKillProcessTreeNoSuchTest")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()

	if err := killProcessTree(command); err != nil {
		t.Fatalf("killing an already-reaped process must not report an error, got %v", err)
	}
}

func TestKillProcessTreeIgnoresAnUnstartedCommand(t *testing.T) {
	if err := killProcessTree(exec.Command(os.Args[0])); err != nil {
		t.Fatalf("an unstarted command must not report an error, got %v", err)
	}
	if err := killProcessTree(nil); err != nil {
		t.Fatalf("a nil command must not report an error, got %v", err)
	}
}

// TestSessionDetachFallsBackWhenTheDaemonRefuses covers the boolean the app
// domain answers with. A false detach can leave the runner unable to finish, so
// treating it as success would mean waiting on a process that never exits.
func TestSessionDetachFallsBackWhenTheDaemonRefuses(t *testing.T) {
	client, _ := fakeDaemon(t, func(string) string {
		return `[{"id":1,"result":false}]`
	})
	session := &Session{
		options: SessionOptions{Output: io.Discard, Logf: func(string, ...any) {}},
		client:  client,
		appID:   "abc",
		// Never closed: the runner is wedged, which is the case that used to
		// hang. command is nil, so the fallback kill is a no-op.
		exited: make(chan struct{}),
	}

	started := time.Now()
	if err := session.detach(context.Background()); err != nil {
		t.Fatalf("detach should fall back to the kill, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > stopGracePeriod/2 {
		t.Fatalf("a refused detach waited %s instead of falling back promptly", elapsed)
	}
}

// TestSessionStopFallsBackWhenTheDaemonRefuses is the same contract for stop.
func TestSessionStopFallsBackWhenTheDaemonRefuses(t *testing.T) {
	client, _ := fakeDaemon(t, func(string) string {
		return `[{"id":1,"result":false}]`
	})
	session := &Session{
		options: SessionOptions{Output: io.Discard, Logf: func(string, ...any) {}},
		client:  client,
		appID:   "abc",
		exited:  make(chan struct{}),
	}

	started := time.Now()
	if err := session.stop(context.Background()); err != nil {
		t.Fatalf("stop should fall back to the kill, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > stopGracePeriod/2 {
		t.Fatalf("a refused stop waited %s instead of falling back promptly", elapsed)
	}
}

// TestSessionStopIsQuietWhenTheProcessAlreadyExited covers the other half of
// the spurious warning: flutter usually exits before it can answer app.stop,
// and a gone process is the outcome we asked for, not a failure.
func TestSessionStopIsQuietWhenTheProcessAlreadyExited(t *testing.T) {
	exited := make(chan struct{})
	close(exited)
	session := &Session{
		options: SessionOptions{Output: io.Discard, Logf: func(string, ...any) {}},
		exited:  exited,
	}
	if err := session.Stop(context.Background()); err != nil {
		t.Fatalf("stopping an already-exited session must not report an error, got %v", err)
	}
}
