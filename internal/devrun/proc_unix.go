//go:build !windows

package devrun

import (
	"os/exec"
	"syscall"
)

// configureProcessGroup gives the child its own process group, so Ctrl+C in
// our terminal does not reach flutter directly and we can stop it in order.
// It also makes the whole tree addressable as a single group in killProcessTree.
func configureProcessGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessTree terminates the child and every process it spawned. `flutter`
// is a shell wrapper around dart, which spawns gradle, adb and the Go build
// tool; killing the wrapper alone would leave that tree holding the device.
func killProcessTree(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return nil
	}
	// A negative pid signals the entire process group created by Setpgid.
	if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil {
		return command.Process.Kill()
	}
	return nil
}
