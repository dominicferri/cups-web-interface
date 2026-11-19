package cups

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"time"
)

// cliAccessorySource reads PPDs and applies options with the CUPS command-line tools.
type cliAccessorySource struct {
	a *CLIAdapter
}

func (s *cliAccessorySource) QueuePPD(name string) ([]byte, error) {
	if !IsValidPrinterName(name) {
		return nil, fmt.Errorf("invalid printer name: %q", name)
	}
	// /etc/cups/ppd/<queue>.ppd is what cupsd uses and what lpadmin -o updates.
	if path := FindPPDFile(name); path != "" {
		if data, err := readLimited(path); err == nil {
			return data, nil
		}
	}
	// Fall back to asking cupsd (the file can be unreadable on some systems).
	ctx, cancel := context.WithTimeout(context.Background(), s.a.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:631/printers/"+url.PathEscape(name)+".ppd", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PPD from cupsd: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("queue %s has no PPD (cupsd answered %s)", name, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxPPDSize))
}

func readLimited(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, maxPPDSize))
}

// cupsDriverdPaths are where CUPS installs cups-driverd, which serves the
// original driver PPDs (including drv:/// drivers) for lpadmin -m.
var cupsDriverdPaths = []string{
	"/usr/libexec/cups/daemon/cups-driverd",
	"/usr/lib/cups/daemon/cups-driverd",
}

func (s *cliAccessorySource) DriverPPD(nickName string) (string, []byte, error) {
	if nickName == "" || strings.ContainsAny(nickName, "\r\n\x00") {
		return "", nil, fmt.Errorf("invalid driver name")
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.a.timeout)
	defer cancel()

	// List every driver and match make-and-model exactly. lpinfo's own
	// --make-and-model filter is a fuzzy match that misses names such as
	// "Kyocera CS 255c (KPDL)". The full list takes about 0.1 s.
	out, err := s.a.runCommand(ctx, "lpinfo", "-l", "-m")
	if err != nil {
		return "", nil, err
	}
	driverName := driverNameFromLPInfo(out, nickName)
	if driverName == "" {
		return "", nil, fmt.Errorf("driver %q not found", nickName)
	}

	for _, bin := range cupsDriverdPaths {
		if _, err := os.Stat(bin); err != nil {
			continue
		}
		cmd := exec.CommandContext(ctx, bin, "cat", driverName)
		cmd.Stdin = nil
		data, err := cmd.Output()
		if err != nil {
			return "", nil, fmt.Errorf("failed to read driver PPD %s: %w", driverName, err)
		}
		if len(data) > maxPPDSize {
			data = data[:maxPPDSize]
		}
		return driverName, data, nil
	}
	return "", nil, fmt.Errorf("cups-driverd not found")
}

// driverNameFromLPInfo returns the name of the driver whose make-and-model is
// exactly makeAndModel, from `lpinfo -l -m` output.
func driverNameFromLPInfo(out, makeAndModel string) string {
	for _, d := range parseLPInfoLongOutput(out) {
		if d.Model == makeAndModel && d.Name != "everywhere" {
			return d.Name
		}
	}
	return ""
}

func (s *cliAccessorySource) DeviceURI(name string) (string, error) {
	if !IsValidPrinterName(name) {
		return "", fmt.Errorf("invalid printer name: %q", name)
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.a.timeout)
	defer cancel()
	out, err := s.a.runCommand(ctx, "lpstat", "-v", name)
	if err != nil {
		return "", fmt.Errorf("printer %q not found: %w", name, err)
	}
	for _, line := range strings.Split(out, "\n") {
		prefix := "device for " + name + ": "
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix)), nil
		}
	}
	return "", fmt.Errorf("printer %q not found", name)
}

// ApplyOptions runs `lpadmin -p <name> -o k=v ...`, which updates the
// *Default lines in the queue's own PPD (/etc/cups/ppd/<name>.ppd). The
// shared driver PPD is never touched. User-level lpoptions are not written:
// installable options describe the hardware, not one user's preferences.
func (s *cliAccessorySource) ApplyOptions(name string, options map[string]string) error {
	if !IsValidPrinterName(name) {
		return fmt.Errorf("invalid printer name: %q", name)
	}
	args := []string{"-p", name}
	for _, k := range sortedKeys(options) {
		v := options[k]
		if !IsValidOptionKey(k) || !IsValidOptionValue(v) {
			return fmt.Errorf("invalid option %q", k)
		}
		args = append(args, "-o", k+"="+v)
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.a.timeout)
	defer cancel()
	if _, err := s.a.runCommand(ctx, "lpadmin", args...); err != nil {
		return fmt.Errorf("failed to set accessories for %s: %w", name, err)
	}
	return nil
}

func (s *cliAccessorySource) PrinterAttributes(uri string) (string, error) {
	if !IsIPPDeviceURI(uri) || !IsValidPrinterURI(uri) {
		return "", fmt.Errorf("not an IPP address")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ipptool", "-tv", "-T", "10", uri, "get-printer-attributes.test")
	out, err := cmd.CombinedOutput()
	text := string(out)
	// ipptool exits non-zero when a test "fails" (for example a badly encoded
	// attribute) even though the printer answered. Use the answer if there is one.
	if !strings.Contains(text, "RECEIVED:") {
		if err == nil {
			err = fmt.Errorf("no response")
		}
		return "", fmt.Errorf("Get-Printer-Attributes failed: %v", err)
	}
	return text, nil
}

// QueueUUID asks cupsd for the queue's printer-uuid. It stays the same while
// the queue exists and changes if the queue is deleted and re-created.
func (s *cliAccessorySource) QueueUUID(name string) (string, error) {
	if !IsValidPrinterName(name) {
		return "", fmt.Errorf("invalid printer name: %q", name)
	}
	uri := "ipp://localhost/printers/" + url.PathEscape(name)
	req := "{\n" +
		"    OPERATION Get-Printer-Attributes\n" +
		"    GROUP operation-attributes-tag\n" +
		"    ATTR charset attributes-charset utf-8\n" +
		"    ATTR naturalLanguage attributes-natural-language en\n" +
		"    ATTR uri printer-uri $uri\n" +
		"    ATTR keyword requested-attributes printer-uuid\n" +
		"}\n"
	ctx, cancel := context.WithTimeout(context.Background(), s.a.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ipptool", "-tv", uri, "/dev/stdin")
	cmd.Stdin = strings.NewReader(req)
	out, _ := cmd.Output()
	for _, v := range ParseIPPToolAttributes(string(out))["printer-uuid"] {
		if strings.HasPrefix(v, "urn:uuid:") {
			return v, nil
		}
	}
	return "", fmt.Errorf("printer-uuid not found for %s", name)
}

// QueueNames lists the queues CUPS has (lpstat -e).
func (s *cliAccessorySource) QueueNames() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.a.timeout)
	defer cancel()
	out, err := s.a.runCommand(ctx, "lpstat", "-e")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

func (s *cliAccessorySource) IsAdmin() bool {
	return currentUserIsCUPSAdmin()
}

func (s *cliAccessorySource) UserName() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return "unknown"
}

// cupsSystemGroups reads SystemGroup from cups-files.conf. Members of these
// groups are CUPS administrators.
func cupsSystemGroups() []string {
	for _, path := range []string{"/etc/cups/cups-files.conf", "/private/etc/cups/cups-files.conf"} {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		var groups []string
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) >= 2 && strings.EqualFold(fields[0], "SystemGroup") {
				groups = append(groups, fields[1:]...)
			}
		}
		f.Close()
		if len(groups) > 0 {
			return groups
		}
	}
	// CUPS defaults: "admin" on macOS, "lpadmin sys root" on Linux.
	return []string{"admin", "lpadmin", "sys", "root"}
}

// currentUserIsCUPSAdmin reports whether the user running this server is root
// or in one of CUPS's SystemGroups.
func currentUserIsCUPSAdmin() bool {
	u, err := user.Current()
	if err != nil {
		return false
	}
	if u.Uid == "0" {
		return true
	}
	system := map[string]bool{}
	for _, g := range cupsSystemGroups() {
		system[g] = true
	}
	gids, err := u.GroupIds()
	if err != nil {
		return false
	}
	for _, gid := range gids {
		if g, err := user.LookupGroupId(gid); err == nil && system[g.Name] {
			return true
		}
	}
	return false
}

// accessories returns the adapter's accessory manager, creating it on first use.
func (a *CLIAdapter) accessories() *AccessoryManager {
	a.accOnce.Do(func() {
		if a.Accessories == nil {
			path, _ := DefaultOverrideStorePath() // "" makes the store refuse changes with a clear error
			a.Accessories = NewAccessoryManager(&cliAccessorySource{a: a}, NewOverrideStore(path))
		}
	})
	return a.Accessories
}

// GetAccessories returns the queue's installable options and where each value came from.
func (a *CLIAdapter) GetAccessories(name string) (*AccessoryReport, error) {
	return a.accessories().Report(name)
}

// SetAccessories sets installable options manually and records the overrides.
func (a *CLIAdapter) SetAccessories(name string, values map[string]string, acknowledged bool) (*AccessoryReport, error) {
	return a.accessories().Set(name, values, acknowledged)
}

// ResetAccessories restores the values from before the manual changes.
func (a *CLIAdapter) ResetAccessories(name string) (*AccessoryReport, error) {
	return a.accessories().Reset(name)
}

// SuggestAccessories queries the printer over IPP and suggests settings.
func (a *CLIAdapter) SuggestAccessories(name string) (*AccessorySuggestions, error) {
	return a.accessories().Suggest(name)
}
