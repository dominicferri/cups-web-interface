package macos

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

const (
	LaunchAgentLabel = "com.papercut.cups-webui"
)

// LaunchAgentStatus describes the state of the macOS LaunchAgent.
type LaunchAgentStatus struct {
	Supported    bool   `json:"supported"`
	Installed    bool   `json:"installed"`
	Running      bool   `json:"running"`
	PlistPath    string `json:"plist_path"`
	Executable   string `json:"executable"`
	StatusDetail string `json:"status_detail"`
}

// PlistData holds template parameters for the launchd agent plist.
type PlistData struct {
	Label      string
	BinaryPath string
	WorkingDir string
	Port       int
	Host       string
	StdOutPath string
	StdErrPath string
}

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>{{.Label}}</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{.BinaryPath}}</string>
        <string>serve</string>
        <string>-port</string>
        <string>{{.Port}}</string>
        <string>-host</string>
        <string>{{.Host}}</string>
    </array>
    <key>WorkingDirectory</key>
    <string>{{.WorkingDir}}</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>{{.StdOutPath}}</string>
    <key>StandardErrorPath</key>
    <string>{{.StdErrPath}}</string>
</dict>
</plist>
`

// GetPlistPath returns the standard user LaunchAgent path for CUPS webui.
func GetPlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("unable to determine user home directory: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents", fmt.Sprintf("%s.plist", LaunchAgentLabel)), nil
}

// GetInstalledBinaryPath returns the standard persistent user binary path:
// ~/Library/Application Support/cups-webui/cups-webui
func GetInstalledBinaryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("unable to determine user home directory: %w", err)
	}
	return filepath.Join(home, "Library", "Application Support", "cups-webui", "cups-webui"), nil
}

// GeneratePlist creates the XML string for the launch agent.
func GeneratePlist(binaryPath string, port int, host string) (string, error) {
	if port <= 0 || port > 65535 {
		port = 6310
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" && host != "[::1]" {
		host = "127.0.0.1"
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = "/tmp"
	}

	logsDir := filepath.Join(home, "Library", "Logs")
	_ = os.MkdirAll(logsDir, 0755)

	data := PlistData{
		Label:      LaunchAgentLabel,
		BinaryPath: binaryPath,
		WorkingDir: filepath.Dir(binaryPath),
		Port:       port,
		Host:       host,
		StdOutPath: filepath.Join(logsDir, fmt.Sprintf("%s.stdout.log", LaunchAgentLabel)),
		StdErrPath: filepath.Join(logsDir, fmt.Sprintf("%s.stderr.log", LaunchAgentLabel)),
	}

	tmpl, err := template.New("launchagent").Parse(plistTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// Install writes the LaunchAgent plist and loads it via launchctl.
func Install(binaryPath string, port int, host string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("macOS LaunchAgent is only supported on macOS (darwin)")
	}

	absBinary, err := filepath.Abs(binaryPath)
	if err != nil {
		absBinary = binaryPath
	}

	plistPath, err := GetPlistPath()
	if err != nil {
		return err
	}

	// Unload any previous running instance before replacing the binary on disk
	_ = exec.Command("launchctl", "unload", plistPath).Run()

	// Copy binary to ~/Library/Application Support/cups-webui/cups-webui.
	// This ensures:
	// 1. launchd executes outside of ~/Documents or ~/Downloads, preventing macOS TCC permission dialogs.
	// 2. The background service persists independently of development or temporary build folders.
	// 3. Writing to a temp file and renaming (new inode) avoids macOS arm64 vnode code-signature cache invalidation (SIGKILL) on upgrade.
	targetBinary, err := GetInstalledBinaryPath()
	if err == nil {
		if filepath.Clean(absBinary) != filepath.Clean(targetBinary) {
			targetDir := filepath.Dir(targetBinary)
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				return fmt.Errorf("failed to create application support directory: %w", err)
			}
			data, err := os.ReadFile(absBinary)
			if err != nil {
				return fmt.Errorf("failed to read source binary %s: %w", absBinary, err)
			}
			tmpBinary := targetBinary + ".tmp"
			_ = os.Remove(tmpBinary)
			if err := os.WriteFile(tmpBinary, data, 0755); err != nil {
				return fmt.Errorf("failed to stage binary to %s: %w", tmpBinary, err)
			}
			_ = os.Remove(targetBinary)
			if err := os.Rename(tmpBinary, targetBinary); err != nil {
				_ = os.Remove(tmpBinary)
				return fmt.Errorf("failed to replace installed binary at %s: %w", targetBinary, err)
			}
		}
		absBinary = targetBinary
	}

	plistContent, err := GeneratePlist(absBinary, port, host)
	if err != nil {
		return fmt.Errorf("failed to generate LaunchAgent plist: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(plistPath), 0755); err != nil {
		return fmt.Errorf("failed to create LaunchAgents directory: %w", err)
	}

	if err := os.WriteFile(plistPath, []byte(plistContent), 0644); err != nil {
		return fmt.Errorf("failed to write LaunchAgent plist to %s: %w", plistPath, err)
	}

	// Load the newly installed LaunchAgent
	cmd := exec.Command("launchctl", "load", plistPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl load failed: %v (output: %s)", err, string(out))
	}

	// On modern macOS, ensure the service is immediately spawned
	uid := os.Getuid()
	_ = exec.Command("launchctl", "kickstart", "-k", fmt.Sprintf("gui/%d/%s", uid, LaunchAgentLabel)).Run()
	_ = exec.Command("launchctl", "start", LaunchAgentLabel).Run()

	return nil
}

// Uninstall unloads and deletes the LaunchAgent plist.
func Uninstall() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("macOS LaunchAgent is only supported on macOS (darwin)")
	}

	plistPath, err := GetPlistPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		return nil // Already uninstalled
	}

	// Unload from launchctl
	_ = exec.Command("launchctl", "unload", plistPath).Run()

	// Remove plist file
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete LaunchAgent plist: %w", err)
	}

	// Clean up installed binary in Application Support
	if targetBinary, err := GetInstalledBinaryPath(); err == nil {
		_ = os.Remove(targetBinary)
	}

	return nil
}

// GetStatus checks the current status of the LaunchAgent.
func GetStatus(binaryPath string) (*LaunchAgentStatus, error) {
	if runtime.GOOS != "darwin" {
		return &LaunchAgentStatus{
			Supported:    false,
			Installed:    false,
			Running:      false,
			StatusDetail: "LaunchAgent is only supported on macOS",
		}, nil
	}

	plistPath, err := GetPlistPath()
	if err != nil {
		return nil, err
	}

	installed := false
	if _, err := os.Stat(plistPath); err == nil {
		installed = true
	}

	running := false
	detail := "Not installed"
	if installed {
		detail = "Installed"
		out, err := exec.Command("launchctl", "list", LaunchAgentLabel).CombinedOutput()
		if err == nil && !strings.Contains(string(out), "Could not find service") {
			running = true
			detail = "Installed and running under launchd"
		} else {
			detail = "Installed (inactive / stopped)"
		}
	}

	absBinary, _ := filepath.Abs(binaryPath)

	return &LaunchAgentStatus{
		Supported:    true,
		Installed:    installed,
		Running:      running,
		PlistPath:    plistPath,
		Executable:   absBinary,
		StatusDetail: detail,
	}, nil
}
