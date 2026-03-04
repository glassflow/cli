//go:build !windows

package k8s

import (
	"os/exec"
	"syscall"
)

func setDetachedProcessAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
