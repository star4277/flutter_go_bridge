package devrun

import (
	"context"
	"io"
	"os"
	"os/exec"
	"testing"
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

// TestSessionStopIsQuietWhenTheProcessAlreadyExited covers the other half of
// the same warning: flutter usually exits before it can answer app.stop, and a
// gone process is the outcome we asked for, not a failure.
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
