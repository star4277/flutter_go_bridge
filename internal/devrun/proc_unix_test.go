//go:build !windows

package devrun

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
)

func TestConfigureProcessGroupUnix(t *testing.T) {
	command := exec.Command("flutter")
	configureProcessGroup(command)
	if command.SysProcAttr == nil || !command.SysProcAttr.Setpgid {
		t.Fatalf("process group was not configured: %#v", command.SysProcAttr)
	}
}

func TestIgnoreDeadProcessUnix(t *testing.T) {
	if err := ignoreDeadProcess(nil); err != nil {
		t.Fatalf("nil should be ignored, got %v", err)
	}
	if err := ignoreDeadProcess(os.ErrProcessDone); err != nil {
		t.Fatalf("ErrProcessDone should be ignored, got %v", err)
	}
	if err := ignoreDeadProcess(syscall.ESRCH); err != nil {
		t.Fatalf("ESRCH should be ignored, got %v", err)
	}
	real := errors.New("real failure")
	if err := ignoreDeadProcess(real); !errors.Is(err, real) {
		t.Fatalf("non-dead errors must be returned, got %v", err)
	}
}
