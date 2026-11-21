package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	"strconv"
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
  cups-webui open [flags]             Open CUPS webui in the default browser, starting the installed service if it is stopped
  cups-webui launchagent <command>    Manage the macOS background LaunchAgent service

LaunchAgent Commands:
  install [-port <int>]               Generate plist and register cups-webui as an on-demand service (default port 6310)
  uninstall                           Unload and remove the cups-webui LaunchAgent
  status                              Display current LaunchAgent installation and execution status

Flags:
  -port <int>        Port to listen on (default 6310)
  -host <string>     Host interface to bind to (default 127.0.0.1)
  -mock              Run with mock CUPS data (useful for dev and testing)
  -open              Open browser automatically upon server startup
  -keep-running      Keep the server running after the browser closes
  -help, -h          Show this help message`)
}

func runServer() {
	port := flag.Int("port", 6310, "Port to listen on (default 6310)")
	host := flag.String("host", "127.0.0.1", "Host interface to bind to (strictly loopback for security)")
	mockMode := flag.Bool("mock", false, "Run with mock CUPS data (useful for dev and testing)")
	autoOpen := flag.Bool("open", false, "Open browser automatically on start")
	keepRunning := flag.Bool("keep-running", false, "Keep the server running after the browser closes")
	flag.Parse()

	if os.Getenv("CUPS_WEBUI_MOCK") == "true" || os.Getenv("CUPS_WEBUI_MOCK") == "1" {
		*mockMode = true
	}

	if !api.IsLoopbackHost(*host) {
		log.Fatalf("[ERROR] -host %q is not a loopback address. Use 127.0.0.1, ::1 or localhost.", *host)
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

	// A new token on every start, so one left in session.json by a crash is never reused.
	token := generateSessionToken()
	serverFS := web.GetFS()
	srv := api.NewServer(adapter, serverFS, token)

	addr := net.JoinHostPort(*host, strconv.Itoa(*port))
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
	// The Session URL holds the token. Show it on a terminal only, so it does
	// not end up in the LaunchAgent log file or any other redirected output.
	if info, err := os.Stdout.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
		fmt.Printf("  Session URL:   %s\n", browserURL)
	}
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
			if err := openBrowser(browserURL); err != nil {
				log.Printf("[WARN] Could not open the browser: %v", err)
			}
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

	// Without -keep-running the server stops when the last browser tab closes.
	// 15 s covers a page reload. The server runs for at least 2 min, which
	// covers a browser that is still opening.
	idle := make(chan struct{})
	if !*keepRunning {
		go func() {
			srv.WaitIdle(15*time.Second, 2*time.Minute, time.Second)
			close(idle)
		}()
	}

	select {
	case <-stopChan:
	case <-idle:
		log.Println("[INFO] No browser is using the server.")
	}
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

	// The page of another user's process must never open as CUPS webui.
	answers := func(host string, port int) bool {
		ok, err := serverAnswers(host, port)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v. Not opening the browser.\n", err)
			os.Exit(1)
		}
		return ok
	}
	liveSession := func() *SessionInfo {
		session, err := loadSessionInfo()
		if err != nil || !answers(session.Host, session.Port) {
			return nil
		}
		return session
	}

	session := liveSession()
	// The server stops when the browser closes. Start the installed service
	// again and wait for the session of the new process.
	if session == nil && macos.IsInstalled() && !macos.IsRunning() {
		if err := macos.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not start the CUPS webui service: %v\n", err)
		} else {
			for deadline := time.Now().Add(5 * time.Second); session == nil && time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
				session = liveSession()
			}
			if session == nil {
				fmt.Fprintln(os.Stderr, "Warning: the CUPS webui service did not start. Check ~/Library/Logs for com.papercut.cups-webui logs.")
			}
		}
	}
	targetURL := ""
	if session != nil && session.URL != "" {
		targetURL = session.URL
		if session.Token != "" {
			targetURL = fmt.Sprintf("%s/?token=%s", targetURL, session.Token)
		}
	} else {
		// No live session file. A server that runs at the default address
		// still signs this user in by connection owner.
		targetPort := 6310
		if *port > 0 {
			targetPort = *port
		}
		answers("127.0.0.1", targetPort)
		targetURL = fmt.Sprintf("http://127.0.0.1:%d", targetPort)
	}

	fmt.Printf("Opening CUPS webui in browser: %s\n", targetURL)
	if err := openBrowser(targetURL); err != nil {
		fmt.Fprintf(os.Stderr, "Error opening the browser: %v\n", err)
		os.Exit(1)
	}
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
		fmt.Println("   The server stops when you close its browser tab. 'cups-webui open' starts it again.")

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

// serverAnswers reports whether a CUPS webui server of this user answers at
// host:port. Its /api/ request resets the server's idle timer, so the server
// does not stop while the browser opens. The error is set when the listener
// belongs to another user, or its owner cannot be checked.
func serverAnswers(host string, port int) (bool, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	foreign := false
	client := &http.Client{
		Timeout: 500 * time.Millisecond,
		Transport: &http.Transport{
			DisableKeepAlives: true,
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
				if err != nil {
					return nil, err
				}
				uid, err := peercred.OwnerUID(conn.LocalAddr(), conn.RemoteAddr())
				if errors.Is(err, peercred.ErrUnsupported) {
					return conn, nil
				}
				if err != nil || uid != uint32(os.Getuid()) {
					foreign = true
					_ = conn.Close()
					return nil, errors.New("listener is not owned by this user")
				}
				return conn, nil
			},
		},
	}
	resp, err := client.Get("http://" + addr + "/api/v1/auth/status")
	if foreign {
		return false, fmt.Errorf("the process that listens at %s does not belong to your user account", addr)
	}
	if err != nil {
		return false, nil
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}

// clearSessionInfo removes the session file, unless another instance wrote it.
func clearSessionInfo() {
	if session, err := loadSessionInfo(); err == nil && session.PID != os.Getpid() {
		return
	}
	_ = os.Remove(getSessionFilePath())
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		// 'open' hands the URL to the browser and exits, so its exit status is the result.
		return exec.Command("open", url).Run()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
	return fmt.Errorf("no browser launcher for %s", runtime.GOOS)
}
