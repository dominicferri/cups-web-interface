//go:build unix

package macos

import (
	"os/exec"
	"strconv"
	"syscall"
)

// RunDetached starts `<binaryPath> launchagent <action>` in its own session and
// returns without waiting for it. launchd stops the processes of a job it
// unloads; a new session keeps the helper outside the job, so it can finish
// the install or uninstall after the calling service has been stopped.
func RunDetached(binaryPath, action string, port int) error {
	args := []string{"launchagent", action}
	if action == "install" {
		args = append(args, "-port", strconv.Itoa(port))
	}
	cmd := exec.Command(binaryPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
