//go:build windows

package devrun

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// configureProcessGroup puts the child in its own process group so a console
// Ctrl+C is delivered to us alone. We stop flutter deliberately via app.stop
// instead of letting the signal race us to it.
func configureProcessGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// killProcessTree terminates the child and every process it spawned.
//
// On Windows `flutter` is a batch wrapper that launches dart.exe, which in
// turn launches gradle, adb and the Go build tool. Killing the wrapper alone
// leaves that whole tree running, holding the device and the build lock, so
// the next `flutter run` fails in confusing ways. taskkill /T walks the tree.
func killProcessTree(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return nil
	}
	kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(command.Process.Pid))
	kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := kill.Run(); err == nil {
		return nil
	}
	// taskkill is missing or the pid is already gone; fall back to the direct
	// kill, which at least reaps the wrapper.
	return ignoreDeadProcess(command.Process.Kill())
}

// ignoreDeadProcess drops the errors that only mean "it already exited".
//
// Once cmd.Wait has reaped the process, Windows reports the released handle as
// EINVAL -- plain "invalid argument" -- rather than ErrProcessDone. Since the
// session always waits in the background, that is the normal state by the time
// a stop lands, and it surfaced as a warning on every clean shutdown.
func ignoreDeadProcess(err error) error {
	if err == nil || errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.EINVAL) {
		return nil
	}
	return err
}
