//go:build windows

package devrun

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
)

func TestConfigureProcessGroupWindows(t *testing.T) {
	command := exec.Command("flutter")
	configureProcessGroup(command)
	if command.SysProcAttr == nil || command.SysProcAttr.CreationFlags&syscall.CREATE_NEW_PROCESS_GROUP == 0 {
		t.Fatalf("process group flags were not configured: %#v", command.SysProcAttr)
	}
}

func TestIgnoreDeadProcessWindows(t *testing.T) {
	if err := ignoreDeadProcess(nil); err != nil {
		t.Fatalf("nil should be ignored, got %v", err)
	}
	if err := ignoreDeadProcess(os.ErrProcessDone); err != nil {
		t.Fatalf("ErrProcessDone should be ignored, got %v", err)
	}
	if err := ignoreDeadProcess(syscall.EINVAL); err != nil {
		t.Fatalf("EINVAL should be ignored on windows, got %v", err)
	}
	real := errors.New("real failure")
	if err := ignoreDeadProcess(real); !errors.Is(err, real) {
		t.Fatalf("non-dead errors must be returned, got %v", err)
	}
}
