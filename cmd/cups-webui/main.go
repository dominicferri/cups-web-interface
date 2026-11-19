package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/PaperCutSoftware/cups-web-interface/internal/api"
	"github.com/PaperCutSoftware/cups-web-interface/internal/cups"
	"github.com/PaperCutSoftware/cups-web-interface/internal/macos"
	"github.com/PaperCutSoftware/cups-web-interface/internal/peercred"
	"github.com/PaperCutSoftware/cups-web-interface/internal/web"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

type SessionInfo struct {
	Port      int    `json:"port"`
	Host      string `json:"host"`
	URL       string `json:"url"`
	Token     string `json:"token"`
	PID       int    `json:"pid"`
	CreatedAt string `json:"created_at"`
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "open":
			handleOpenCommand(os.Args[2:])
			return
		case "launchagent":
			handleLaunchAgentCommand(os.Args[2:])
			return
		case "help", "--help", "-h":
			printUsage()
			return
		case "serve":
			// Strip "serve" subcommand argument and proceed with standard flag parsing
			os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
		}
	}

	runServer()
}

func printUsage() {
	fmt.Println(`CUPS webui - Web interface and diagnostics for CUPS

Usage:
  cups-webui [serve] [flags]          Start the CUPS webui server (default)
  cups-webui open [flags]             Open CUPS webui in the default browser with active session token
  cups-webui launchagent <command>    Manage the macOS background LaunchAgent service

LaunchAgent Commands:
  install [-port <int>]               Generate plist and register cups-webui to launch on login (default port 6310)
  uninstall                           Unload and remove the cups-webui LaunchAgent
  status                              Display current LaunchAgent installation and execution status

Flags:
  -port <int>        Port to listen on (default 6310)
  -host <string>     Host interface to bind to (default 127.0.0.1)
  -mock              Run with mock CUPS data (useful for dev and testing)
  -open              Open browser automatically upon server startup
  -help, -h          Show this help message`)
}

func runServer() {
	port := flag.Int("port", 6310, "Port to listen on (default 6310)")
	host := flag.String("host", "127.0.0.1", "Host interface to bind to (strictly loopback for security)")
	mockMode := flag.Bool("mock", false, "Run with mock CUPS data (useful for dev and testing)")
	autoOpen := flag.Bool("open", false, "Open browser automatically on start")
	flag.Parse()

	if os.Getenv("CUPS_WEBUI_MOCK") == "true" || os.Getenv("CUPS_WEBUI_MOCK") == "1" {
		*mockMode = true
	}

	var adapter cups.Adapter
	if *mockMode {
		log.Println("[INFO] Initializing in MOCK mode with simulated printers and queues")
		adapter = cups.NewMockAdapter()
	} else {
		log.Println("[INFO] Initializing with live CUPS CLI adapter")
		cli := cups.NewCLIAdapter()
		cli.ListenPort = *port
		adapter = cli
	}

	token := getOrGenerateSessionToken()
	serverFS := web.GetFS()
	srv := api.NewServer(adapter, serverFS, token)

	addr := fmt.Sprintf("%s:%d", *host, *port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      srv.Handler(),
		ConnContext:  srv.ConnContext,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Browser sign-in without the session URL relies on identifying which user
	// owns each connection. If that cannot be verified, it stays disabled.
	if err := peercred.SelfTest(); err != nil {
		log.Printf("[WARN] Cannot verify connection ownership (%v). Opening the address directly will not sign in; use the Session URL or 'cups-webui open'.", err)
	}

	// Verify listener can bind
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[ERROR] Failed to bind to %s: %v", addr, err)
	}

	targetURL := fmt.Sprintf("http://%s", addr)
	browserURL := fmt.Sprintf("%s/?token=%s", targetURL, token)

	// Save session info for 'cups-webui open' command
	saveSessionInfo(*host, *port, targetURL, token)

	fmt.Println()
	fmt.Println("==================================================")
	fmt.Printf("  🖨️  CUPS webui v0.1.0\n")
	fmt.Printf("  Serving at:    %s\n", targetURL)
	fmt.Printf("  Session URL:   %s\n", browserURL)
	if *mockMode {
		fmt.Printf("  Mode:          MOCK (Simulated CUPS data)\n")
	} else {
		fmt.Printf("  Mode:          LIVE CUPS (Connected to local cupsd)\n")
	}
	fmt.Println("==================================================")
	fmt.Println()

	if *autoOpen {
		go func() {
			time.Sleep(300 * time.Millisecond)
			openBrowser(browserURL)
		}()
	}

	// Graceful shutdown channel
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[ERROR] Server error: %v", err)
		}
	}()

	<-stopChan
	log.Println("[INFO] Shutting down CUPS webui server...")
	clearSessionInfo()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("[ERROR] Graceful shutdown error: %v", err)
	} else {
		log.Println("[INFO] Server stopped gracefully.")
	}
}

func handleOpenCommand(args []string) {
	fs := flag.NewFlagSet("open", flag.ExitOnError)
	port := fs.Int("port", 0, "Target CUPS webui port (defaults to active session)")
	_ = fs.Parse(args)

	session, err := loadSessionInfo()
	targetURL := ""
	if err == nil && session != nil && session.URL != "" {
		targetURL = session.URL
		if session.Token != "" {
			targetURL = fmt.Sprintf("%s/?token=%s", targetURL, session.Token)
		}
	} else {
		targetPort := 6310
		if *port > 0 {
			targetPort = *port
		}
		targetURL = fmt.Sprintf("http://127.0.0.1:%d", targetPort)
	}

	fmt.Printf("Opening CUPS webui in browser: %s\n", targetURL)
	openBrowser(targetURL)
}

func handleLaunchAgentCommand(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: cups-webui launchagent [install|uninstall|status]")
		os.Exit(1)
	}

	action := strings.ToLower(args[0])
	binPath, err := os.Executable()
	if err != nil {
		binPath = os.Args[0]
	}

	switch action {
	case "install":
		fs := flag.NewFlagSet("launchagent install", flag.ExitOnError)
		port := fs.Int("port", 6310, "Port the background service listens on")
		_ = fs.Parse(args[1:])
		if *port < 1 || *port > 65535 {
			fmt.Printf("Error installing LaunchAgent: invalid port %d (expected 1-65535)\n", *port)
			os.Exit(1)
		}
		if err := macos.Install(binPath, *port, "127.0.0.1"); err != nil {
			fmt.Printf("Error installing LaunchAgent: %v\n", err)
			os.Exit(1)
		}
		plistPath, _ := macos.GetPlistPath()
		fmt.Println("✅ CUPS webui macOS LaunchAgent successfully installed and registered!")
		fmt.Printf("   Plist location: %s\n", plistPath)
		fmt.Printf("   Address:        http://127.0.0.1:%d\n", *port)
		fmt.Println("   cups-webui will automatically start whenever you log into macOS.")

	case "uninstall":
		if err := macos.Uninstall(); err != nil {
			fmt.Printf("Error uninstalling LaunchAgent: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ CUPS webui macOS LaunchAgent successfully stopped and uninstalled.")

	case "status":
		status, err := macos.GetStatus(binPath)
		if err != nil {
			fmt.Printf("Error checking LaunchAgent status: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("CUPS webui macOS LaunchAgent Status:")
		fmt.Printf("  Supported:    %v\n", status.Supported)
		fmt.Printf("  Installed:    %v\n", status.Installed)
		fmt.Printf("  Running:      %v\n", status.Running)
		fmt.Printf("  Plist Path:   %s\n", status.PlistPath)
		fmt.Printf("  Status:       %s\n", status.StatusDetail)

	default:
		fmt.Printf("Unknown launchagent action: %q (supported: install, uninstall, status)\n", action)
		os.Exit(1)
	}
}

func getOrGenerateSessionToken() string {
	if info, err := loadSessionInfo(); err == nil && info != nil && len(info.Token) >= 16 {
		return info.Token
	}
	return generateSessionToken()
}

func generateSessionToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func getSessionFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/tmp"
	}
	return filepath.Join(home, ".cups-webui", "session.json")
}

func saveSessionInfo(host string, port int, url string, token string) {
	sessionPath := getSessionFilePath()
	_ = os.MkdirAll(filepath.Dir(sessionPath), 0700)

	info := SessionInfo{
		Port:      port,
		Host:      host,
		URL:       url,
		Token:     token,
		PID:       os.Getpid(),
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err == nil {
		_ = os.WriteFile(sessionPath, data, 0600)
	}
}

func loadSessionInfo() (*SessionInfo, error) {
	sessionPath := getSessionFilePath()
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		return nil, err
	}
	var info SessionInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func clearSessionInfo() {
	_ = os.Remove(getSessionFilePath())
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	}
	if cmd != nil {
		_ = cmd.Start()
	}
}
