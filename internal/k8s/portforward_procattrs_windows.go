//go:build windows

package k8s

import "os/exec"

func setDetachedProcessAttrs(cmd *exec.Cmd) {
	// No-op on Windows; this keeps cross-compilation working for release artifacts.
	_ = cmd
}
