//go:build !unix

package macos

import "fmt"

// RunDetached is only needed under launchd, which exists on macOS.
func RunDetached(binaryPath, action string, port int) error {
	return fmt.Errorf("macOS LaunchAgent is only supported on macOS (darwin)")
}
