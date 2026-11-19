package cups

import (
	"bytes"
	"context"
	"fmt"
	"github.com/PaperCutSoftware/cups-web-interface/internal/macos"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// cleanCUPSError strips repetitive CUPS deprecation notices so the real underlying error is prominent.
func cleanCUPSError(errStr string) string {
	lines := strings.Split(errStr, "\n")
	var meaningful []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}
		if strings.Contains(trimmed, "Printer drivers are deprecated and will stop working in a future version of CUPS") {
			continue
		}
		meaningful = append(meaningful, trimmed)
	}
	if len(meaningful) > 0 {
		return strings.Join(meaningful, " | ")
	}
	return errStr
}

// CLIAdapter communicates with CUPS using macOS/Linux standard CLI commands.
type CLIAdapter struct {
	timeout time.Duration

	// ListenPort is the port this server runs on. A LaunchAgent installed from
	// the web interface is registered on the same port. Zero means 6310.
	ListenPort int

	// Accessories handles manually set installable options. Created on first
	// use when nil; tests can set their own.
	Accessories *AccessoryManager
	accOnce     sync.Once
}

// NewCLIAdapter creates a new CLI-based CUPS adapter.
func NewCLIAdapter() *CLIAdapter {
	return &CLIAdapter{
		timeout: 10 * time.Second,
	}
}

// runCommand executes a command with timeout and returns stdout.
func (a *CLIAdapter) runCommand(ctx context.Context, name string, args ...string) (string, error) {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), a.timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		errStr := strings.TrimSpace(stderr.String())
		if errStr == "" {
			errStr = err.Error()
		} else {
			cleaned := cleanCUPSError(errStr)
			if cleaned != "" {
				errStr = cleaned
			}
		}
		return stdout.String(), fmt.Errorf("command %s %v failed: %s", name, args, errStr)
	}

	return stdout.String(), nil
}

// GetPrinters retrieves all printers and classes with their statuses, URIs, and details.
func (a *CLIAdapter) GetPrinters() ([]Printer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	// 1. Fetch status: -p (printers), -d (default), -v (devices), -a (accepting)
	statusOut, err := a.runCommand(ctx, "lpstat", "-p", "-d", "-v", "-a")
	if err != nil {
		return nil, fmt.Errorf("failed to execute lpstat: %w", err)
	}

	// 2. Fetch classes: -c
	classOut, _ := a.runCommand(ctx, "lpstat", "-c")

	printersMap := ParseCombinedStatus(statusOut, classOut)

	// 3. Fetch detailed info: -l -p
	detailsOut, _ := a.runCommand(ctx, "lpstat", "-l", "-p")
	if detailsOut != "" {
		ParsePrinterDetails(printersMap, detailsOut)
	}

	// 4. Fetch active jobs to compute QueuedJobsCount
	activeJobs, _ := a.GetJobs("active", "")
	for _, j := range activeJobs {
		if p, ok := printersMap[j.Printer]; ok {
			p.QueuedJobsCount++
		}
	}

	// Convert map to sorted slice
	result := make([]Printer, 0, len(printersMap))
	for _, p := range printersMap {
		if audit, err := InspectPrinterArch(p.Name); err == nil && audit != nil {
			p.ArchReadiness = &ArchReadiness{
				Status:          audit.Status,
				Label:           audit.Label,
				DriverModel:     audit.DriverModel,
				PPDPath:         audit.PPDPath,
				RosettaRequired: audit.RosettaRequired,
				BinaryCount:     len(audit.Binaries),
			}
		}
		if p.State == "stopped" || p.State == "error" || p.StateMessage != "" {
			p.Diagnostic = ClassifyError(p.StateMessage)
		}
		result = append(result, *p)
	}
	sort.Slice(result, func(i, j int) bool {
		// Always list printers alphabetically, case-insensitive
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	return result, nil
}

// GetPrinter retrieves a single printer by name.
func (a *CLIAdapter) GetPrinter(name string) (*Printer, error) {
	printers, err := a.GetPrinters()
	if err != nil {
		return nil, err
	}
	for _, p := range printers {
		if p.Name == name {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("printer %q not found", name)
}

// GetJobs returns active or completed jobs.
// which can be "active", "completed", or "all".
func (a *CLIAdapter) GetJobs(which string, printerFilter string) ([]Job, error) {
	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	var jobs []Job

	// Fetch active jobs
	if which == "active" || which == "all" || which == "" {
		activeArgs := []string{"-o"}
		if printerFilter != "" {
			activeArgs = []string{"-o", printerFilter}
		}
		activeOut, _ := a.runCommand(ctx, "lpstat", activeArgs...)
		activeJobs := ParseJobsOutput(activeOut, "pending")
		// The first active job for a printer is usually processing
		seenPrinters := make(map[string]bool)
		for i := range activeJobs {
			if !seenPrinters[activeJobs[i].Printer] {
				activeJobs[i].State = "processing"
				seenPrinters[activeJobs[i].Printer] = true
			}
		}
		jobs = append(jobs, activeJobs...)
	}

	// Fetch completed jobs
	if which == "completed" || which == "all" {
		completedArgs := []string{"-W", "completed", "-o"}
		if printerFilter != "" {
			completedArgs = []string{"-W", "completed", "-o", printerFilter}
		}
		completedOut, _ := a.runCommand(ctx, "lpstat", completedArgs...)
		completedJobs := ParseJobsOutput(completedOut, "completed")
		jobs = append(jobs, completedJobs...)
	}

	// Filter by printer if needed and not already filtered
	if printerFilter != "" {
		filtered := make([]Job, 0, len(jobs))
		for _, j := range jobs {
			if j.Printer == printerFilter {
				filtered = append(filtered, j)
			}
		}
		jobs = filtered
	}

	// Enrich job document titles using CUPS IPP if available
	if len(jobs) > 0 {
		titles := a.fetchIPPJobTitles(ctx)
		for i := range jobs {
			if t, ok := titles[jobs[i].ID]; ok && t != "" {
				jobs[i].Title = t
			} else if jobs[i].Title == fmt.Sprintf("%s-%s", jobs[i].Printer, jobs[i].ID) {
				jobs[i].Title = fmt.Sprintf("Print Job #%s", jobs[i].ID)
			}
		}
	}

	for i := range jobs {
		if jobs[i].StateReason != "" {
			jobs[i].Diagnostic = ClassifyError(jobs[i].StateReason)
		}
		if jobs[i].IsStuck && jobs[i].Diagnostic == nil {
			jobs[i].Diagnostic = &DiagnosticInfo{
				Code:        "job_stuck",
				Title:       "Stuck Print Job",
				Explanation: "This document has been spooling for an extended duration without progression or acknowledgment from the printer.",
				Remedies: []string{
					"Check if the physical printer display is requesting paper or showing an error code.",
					"Try restarting or cancelling the job from the Print Jobs table.",
					"Verify network connectivity to the target printer.",
				},
				Severity: "warning",
			}
		}
	}

	return jobs, nil
}

// fetchIPPJobTitles executes ipptool to fetch real document titles and job names.
func (a *CLIAdapter) fetchIPPJobTitles(ctx context.Context) map[string]string {
	ippReq := "{\n" +
		"    OPERATION Get-Jobs\n" +
		"    GROUP operation-attributes-tag\n" +
		"    ATTR charset attributes-charset utf-8\n" +
		"    ATTR naturalLanguage attributes-natural-language en\n" +
		"    ATTR uri printer-uri ipp://localhost:631/jobs\n" +
		"    ATTR keyword which-jobs all\n" +
		"    ATTR keyword requested-attributes job-id,job-name,document-name-supplied,com.apple.print.JobInfo.PMJobName\n" +
		"}\n"

	cmd := exec.CommandContext(ctx, "ipptool", "-t", "-v", "ipp://localhost:631/jobs", "/dev/stdin")
	cmd.Stdin = strings.NewReader(ippReq)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return ParseIPPJobTitlesOutput(string(out))
}

// CancelJob cancels a specific job by ID.
func (a *CLIAdapter) CancelJob(jobID string) error {
	if !IsValidJobID(jobID) {
		return fmt.Errorf("invalid job ID: %q", jobID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	_, err := a.runCommand(ctx, "cancel", jobID)
	if err != nil {
		return fmt.Errorf("failed to cancel job %s: %w", jobID, err)
	}
	return nil
}

// HoldJob holds a pending job indefinitely.
func (a *CLIAdapter) HoldJob(jobID string) error {
	if !IsValidJobID(jobID) {
		return fmt.Errorf("invalid job ID: %q", jobID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	_, err := a.runCommand(ctx, "lp", "-i", jobID, "-H", "hold")
	if err != nil {
		return fmt.Errorf("failed to hold job %s: %w", jobID, err)
	}
	return nil
}

// ReleaseJob releases a held job back to pending.
func (a *CLIAdapter) ReleaseJob(jobID string) error {
	if !IsValidJobID(jobID) {
		return fmt.Errorf("invalid job ID: %q", jobID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	_, err := a.runCommand(ctx, "lp", "-i", jobID, "-H", "resume")
	if err != nil {
		return fmt.Errorf("failed to release job %s: %w", jobID, err)
	}
	return nil
}

// RestartJob restarts a completed or cancelled job.
func (a *CLIAdapter) RestartJob(jobID string) error {
	if !IsValidJobID(jobID) {
		return fmt.Errorf("invalid job ID: %q", jobID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	_, err := a.runCommand(ctx, "lp", "-i", jobID, "-H", "restart")
	if err != nil {
		return fmt.Errorf("failed to restart job %s: %w", jobID, err)
	}
	return nil
}

// PurgeQueue cancels all pending and active jobs for a printer.
func (a *CLIAdapter) PurgeQueue(printerName string) error {
	if !IsValidPrinterName(printerName) {
		return fmt.Errorf("invalid printer name: %q", printerName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	_, err := a.runCommand(ctx, "cancel", "-a", printerName)
	if err != nil {
		return fmt.Errorf("failed to purge queue for %s: %w", printerName, err)
	}
	return nil
}

// DeletePrinter deletes a printer queue using lpadmin -x <name>.
func (a *CLIAdapter) DeletePrinter(name string) error {
	if !IsValidPrinterName(name) {
		return fmt.Errorf("invalid printer name: %q", name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	_, err := a.runCommand(ctx, "lpadmin", "-x", name)
	if err != nil {
		return fmt.Errorf("failed to delete printer %s: %w", name, err)
	}
	// A new queue with the same name must not inherit manual accessory settings.
	_ = a.accessories().ForgetQueue(name)
	return nil
}

// PausePrinter pauses the printer queue.
func (a *CLIAdapter) PausePrinter(name string) error {
	if !IsValidPrinterName(name) {
		return fmt.Errorf("invalid printer name: %q", name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	_, err := a.runCommand(ctx, "cupsdisable", name)
	if err != nil {
		return fmt.Errorf("failed to pause printer %s: %w", name, err)
	}
	return nil
}

// ResumePrinter resumes the printer queue.
func (a *CLIAdapter) ResumePrinter(name string) error {
	if !IsValidPrinterName(name) {
		return fmt.Errorf("invalid printer name: %q", name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	_, err := a.runCommand(ctx, "cupsenable", name)
	if err != nil {
		return fmt.Errorf("failed to resume printer %s: %w", name, err)
	}
	return nil
}

// SetDefaultPrinter sets the user and system default destination.
func (a *CLIAdapter) SetDefaultPrinter(name string) error {
	if !IsValidPrinterName(name) {
		return fmt.Errorf("invalid printer name: %q", name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	// 1. Set CUPS system default destination via lpadmin
	_, err := a.runCommand(ctx, "lpadmin", "-d", name)
	if err != nil {
		return fmt.Errorf("failed to set system default printer %s: %w", name, err)
	}

	// 2. Also set user-level default destination via lpoptions so local CLI commands agree
	_, _ = a.runCommand(ctx, "lpoptions", "-d", name)

	return nil
}

// GetSystemStatus retrieves current CUPS daemon status and version.
func (a *CLIAdapter) GetSystemStatus() (*SystemStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	status := &SystemStatus{
		ServerTime: time.Now(),
	}

	// Check if scheduler is running: `lpstat -r`
	schedOut, err := a.runCommand(ctx, "lpstat", "-r")
	if err == nil && strings.Contains(schedOut, "scheduler is running") {
		status.DaemonRunning = true
	}

	// Get CUPS version
	verOut, err := a.runCommand(ctx, "cups-config", "--version")
	if err == nil {
		status.CUPSVersion = strings.TrimSpace(verOut)
	} else {
		status.CUPSVersion = "2.x (Standard CUPS)"
	}

	// Check cupsctl
	ctlOut, err := a.runCommand(ctx, "cupsctl")
	if err == nil {
		ctlMap := ParseCupsctlOutput(ctlOut)
		if val, ok := ctlMap["WebInterface"]; ok {
			status.WebInterfaceEnabled = strings.EqualFold(val, "yes") || val == "1"
		}
	}

	// Get default printer
	defOut, err := a.runCommand(ctx, "lpstat", "-d")
	if err == nil {
		if strings.HasPrefix(defOut, "system default destination: ") {
			status.DefaultPrinter = strings.TrimSpace(strings.TrimPrefix(defOut, "system default destination: "))
		}
	}

	// Get counts
	printers, err := a.GetPrinters()
	if err == nil {
		status.PrintersCount = len(printers)
		for _, p := range printers {
			status.ActiveJobsCount += p.QueuedJobsCount
		}
	}

	return status, nil
}

// GetPrinterOptions queries the printer's configurable PPD options via `lpoptions -p <name> -l`.
func (a *CLIAdapter) GetPrinterOptions(name string) (*PrinterOptionsResponse, error) {
	if !IsValidPrinterName(name) {
		return nil, fmt.Errorf("invalid printer name: %q", name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	out, err := a.runCommand(ctx, "lpoptions", "-p", name, "-l")
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve options for printer %s: %w", name, err)
	}

	resp := ParseLpoptionsOutput(name, out)
	// Same source as the accessories tab: the PPD file, or cupsd if the file
	// can't be read by this user.
	if data, err := (&cliAccessorySource{a: a}).QueuePPD(name); err == nil && len(data) > 0 {
		ppd := ParsePPDSummary(data)
		// Group by the PPD's own InstallableOptions group; the keyword
		// guess in ParseLpoptionsOutput misses names such as OptTray2.
		if ppd.HasInstallableGroup {
			RegroupByInstallable(resp, ppd)
		}
		ApplyPPDChoiceLabels(resp, ppd)
	}
	return resp, nil
}

// ApplyPPDChoiceLabels replaces choice names with the PPD's readable labels.
// `lpoptions -l` only prints choice names (e.g. "PrinterS"); the PPD has the
// label the print dialog shows ("Use Printer Setting").
func ApplyPPDChoiceLabels(resp *PrinterOptionsResponse, ppd *PPDSummary) {
	apply := func(opts []PrinterOption) {
		for i := range opts {
			for j := range opts[i].Choices {
				c := &opts[i].Choices[j]
				if l := ppd.ChoiceLabel(opts[i].Key, c.Name); l != "" && l != c.Name {
					c.Label = l
				}
			}
		}
	}
	apply(resp.General)
	apply(resp.Accessories)
}

// RegroupByInstallable puts exactly the options in the PPD's InstallableOptions
// group under Accessories and everything else under General.
func RegroupByInstallable(resp *PrinterOptionsResponse, ppd *PPDSummary) {
	all := append(append([]PrinterOption{}, resp.General...), resp.Accessories...)
	resp.General = []PrinterOption{}
	resp.Accessories = []PrinterOption{}
	for _, opt := range all {
		if ppd.Option(opt.Key) != nil {
			opt.Group = "accessories"
			resp.Accessories = append(resp.Accessories, opt)
		} else {
			opt.Group = "general"
			resp.General = append(resp.General, opt)
		}
	}
}

// SetPrinterOptions writes updated options via `lpoptions -p <name> -o <k>=<v> ...`.
// SetPrinterOptions configures default PPD and queue options.
// It uses `lpadmin -p <name> -o <key>=<value>` to modify the system PPD (*Default<Option>)
// and `printers.conf` directly, ensuring cloning tools like PaperCut Print Deploy capture them.
func (a *CLIAdapter) SetPrinterOptions(name string, options map[string]string) error {
	if !IsValidPrinterName(name) {
		return fmt.Errorf("invalid printer name: %q", name)
	}
	if len(options) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	args := []string{"-p", name}
	for k, v := range options {
		if !IsValidOptionKey(k) {
			return fmt.Errorf("invalid option key: %q", k)
		}
		if !IsValidOptionValue(v) {
			return fmt.Errorf("invalid option value for key %q", k)
		}
		args = append(args, "-o", fmt.Sprintf("%s=%s", k, v))
	}

	// 1. Commit options to the system PPD (/etc/cups/ppd/<name>.ppd) via lpadmin
	_, err := a.runCommand(ctx, "lpadmin", args...)
	if err != nil {
		return fmt.Errorf("failed to set options for printer %s: %w", name, err)
	}

	// 2. Also keep user-level lpoptions synchronized
	_, _ = a.runCommand(ctx, "lpoptions", args...)

	return nil
}

// GetPrinterAudit inspects Mach-O slices and Rosetta 2 dependencies for a specific printer.
func (a *CLIAdapter) GetPrinterAudit(name string) (*ArchAuditResult, error) {
	return InspectPrinterArch(name)
}

// GetFleetAudit calculates global Rosetta 2 readiness statistics across all printers.
func (a *CLIAdapter) GetFleetAudit() (*FleetAuditSummary, error) {
	printers, err := a.GetPrinters()
	if err != nil {
		return nil, err
	}

	summary := &FleetAuditSummary{
		TotalQueues: len(printers),
	}

	for _, p := range printers {
		audit, err := a.GetPrinterAudit(p.Name)
		if err != nil || audit == nil {
			summary.BrokenQueues++
			continue
		}
		switch audit.Status {
		case "universal", "native_arm64":
			summary.UniversalQueues++
		case "rosetta_dependent":
			summary.RosettaQueues++
		case "driverless":
			summary.DriverlessQueues++
		case "missing_binary":
			summary.BrokenQueues++
		default:
			summary.UniversalQueues++
		}
	}

	return summary, nil
}

// ParseLpoptionsOutput parses standard `lpoptions -p <name> -l` output into options.
func ParseLpoptionsOutput(printerName, output string) *PrinterOptionsResponse {
	resp := &PrinterOptionsResponse{
		PrinterName: printerName,
		General:     []PrinterOption{},
		Accessories: []PrinterOption{},
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}

		header := line[:colonIdx]
		choicesStr := strings.TrimSpace(line[colonIdx+1:])

		var key, displayName string
		slashIdx := strings.Index(header, "/")
		if slashIdx != -1 {
			key = strings.TrimSpace(header[:slashIdx])
			displayName = strings.TrimSpace(header[slashIdx+1:])
		} else {
			key = strings.TrimSpace(header)
			displayName = key
		}

		rawChoices := strings.Fields(choicesStr)
		var choices []OptionChoice
		var defaultVal string

		for _, raw := range rawChoices {
			isDef := strings.HasPrefix(raw, "*")
			cName := strings.TrimPrefix(raw, "*")
			cLabel := formatChoiceLabel(key, cName)
			if isDef {
				defaultVal = cName
			}
			choices = append(choices, OptionChoice{
				Name:      cName,
				Label:     cLabel,
				IsDefault: isDef,
			})
		}

		// Categorize into General vs Accessories (Installable Options)
		group := "general"
		lowerKey := strings.ToLower(key)
		if strings.HasPrefix(lowerKey, "option") ||
			strings.HasPrefix(lowerKey, "installed") ||
			strings.Contains(lowerKey, "cassette") ||
			strings.Contains(lowerKey, "tray3") ||
			strings.Contains(lowerKey, "tray4") ||
			strings.Contains(lowerKey, "finisher") ||
			strings.Contains(lowerKey, "stapler") ||
			strings.Contains(lowerKey, "harddisk") {
			group = "accessories"
		}

		opt := PrinterOption{
			Key:          key,
			DisplayName:  displayName,
			DefaultValue: defaultVal,
			CurrentValue: defaultVal,
			Choices:      choices,
			Group:        group,
		}

		if group == "accessories" {
			resp.Accessories = append(resp.Accessories, opt)
		} else {
			resp.General = append(resp.General, opt)
		}
	}

	return resp
}

func formatChoiceLabel(key, choice string) string {
	switch strings.ToLower(choice) {
	case "duplexnotumble":
		return "2-Sided (Long-Edge)"
	case "duplextumble":
		return "2-Sided (Short-Edge)"
	case "none":
		if strings.EqualFold(key, "Duplex") {
			return "Off (1-Sided)"
		}
		return "None"
	case "rgb", "cmyk", "color":
		return "Color"
	case "gray", "devicegray", "monochrome", "grayscale":
		return "Grayscale / Mono"
	case "false":
		return "Not Installed / Off"
	case "true":
		return "Installed / On"
	default:
		return choice
	}
}

// AddPrinter creates a new print queue using lpadmin.
func (a *CLIAdapter) AddPrinter(params AddPrinterParams) error {
	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	if !IsValidPrinterName(params.Name) {
		return fmt.Errorf("invalid printer name %q: max 127 characters, must not start with '-', and no spaces, control characters or / \\ ' \" ? #", params.Name)
	}
	if !IsValidPrinterURI(params.URI) {
		return fmt.Errorf("invalid printer connection URI %q: use an ipp, ipps, http, https, socket, lpd, usb, smb, dnssd or mdns address (or file:/dev/null for a null queue) without control characters or malformed %%-escapes", params.URI)
	}
	if params.PPDPath != "" && !IsValidDriverOrPPD(params.PPDPath) {
		return fmt.Errorf("invalid or untrusted PPD file path: %q", params.PPDPath)
	}
	if params.Model != "" && !IsValidDriverOrPPD(params.Model) {
		return fmt.Errorf("invalid or untrusted driver model identifier: %q", params.Model)
	}

	args := []string{"-p", params.Name, "-v", params.URI, "-E"}
	if params.Description != "" {
		args = append(args, "-D", params.Description)
	}
	if params.Location != "" {
		args = append(args, "-L", params.Location)
	}
	if params.Shared {
		args = append(args, "-o", "printer-is-shared=true")
	} else {
		args = append(args, "-o", "printer-is-shared=false")
	}

	if params.PPDPath != "" {
		args = append(args, "-P", params.PPDPath)
	} else if params.Model != "" {
		if filepath.IsAbs(params.Model) {
			args = append(args, "-P", params.Model)
		} else {
			args = append(args, "-m", params.Model)
		}
	}

	if _, err := a.runCommand(ctx, "lpadmin", args...); err != nil {
		return fmt.Errorf("failed to create printer: %w", err)
	}

	// Ensure queue is accepting jobs and enabled
	_, _ = a.runCommand(ctx, "cupsenable", params.Name)
	_, _ = a.runCommand(ctx, "cupsaccept", params.Name)

	return nil
}

// ModifyPrinter updates an existing queue's URI, description, location, and sharing state.
func (a *CLIAdapter) ModifyPrinter(name string, params ModifyPrinterParams) error {
	if !IsValidPrinterName(name) {
		return fmt.Errorf("invalid printer name: %q", name)
	}
	if params.URI != "" && !IsValidPrinterURI(params.URI) {
		return fmt.Errorf("invalid printer connection URI: %q", params.URI)
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	if _, err := a.runCommand(ctx, "lpadmin", modifyPrinterArgs(name, params)...); err != nil {
		return fmt.Errorf("failed to update printer: %w", err)
	}

	return nil
}

// modifyPrinterArgs builds the lpadmin arguments for ModifyPrinter. A nil field is
// left out so CUPS keeps its current value; an empty description or location is
// passed through, which clears it.
func modifyPrinterArgs(name string, params ModifyPrinterParams) []string {
	args := []string{"-p", name}
	if params.URI != "" {
		args = append(args, "-v", params.URI)
	}
	if params.Description != nil {
		args = append(args, "-D", *params.Description)
	}
	if params.Location != nil {
		args = append(args, "-L", *params.Location)
	}
	if params.Shared != nil {
		args = append(args, "-o", fmt.Sprintf("printer-is-shared=%t", *params.Shared))
	}
	return args
}

// SetPrinterDriver swaps the driver/PPD on an existing queue.
func (a *CLIAdapter) SetPrinterDriver(name string, modelOrPPD string) error {
	if !IsValidPrinterName(name) {
		return fmt.Errorf("invalid printer name: %q", name)
	}
	if !IsValidDriverOrPPD(modelOrPPD) {
		return fmt.Errorf("invalid or untrusted driver model or PPD path: %q", modelOrPPD)
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	args := []string{"-p", name}
	if filepath.IsAbs(modelOrPPD) {
		args = append(args, "-P", modelOrPPD)
	} else {
		args = append(args, "-m", modelOrPPD)
	}

	if _, err := a.runCommand(ctx, "lpadmin", args...); err != nil {
		return fmt.Errorf("failed to update printer driver: %w", err)
	}

	// A new PPD starts from the driver defaults. Keep the admin's manual
	// accessory settings (they are only dropped by "Reset to detected values").
	if err := a.accessories().ReapplyOverrides(name); err != nil {
		return fmt.Errorf("driver updated, but manual accessory settings could not be re-applied: %w", err)
	}

	return nil
}

// PrintTestPage sends a standard CUPS test page job.
func (a *CLIAdapter) PrintTestPage(name string) (string, error) {
	if !IsValidPrinterName(name) {
		return "", fmt.Errorf("invalid printer name: %q", name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	// Locate testprint file
	candidates := []string{
		"/usr/share/cups/data/testprint",
		"/usr/local/share/cups/data/testprint",
		"/opt/homebrew/share/cups/data/testprint",
	}
	var testFile string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			testFile = c
			break
		}
	}

	var out string
	var err error
	if testFile != "" {
		out, err = a.runCommand(ctx, "lp", "-d", name, "-t", "CUPS Test Page", testFile)
	} else {
		// Fallback: send simple text stream
		cmd := exec.CommandContext(ctx, "lp", "-d", name, "-t", "CUPS Test Page")
		cmd.Stdin = strings.NewReader("CUPS webui Test Page\nPrinter: " + name + "\nDate: " + time.Now().Format(time.RFC1123) + "\n")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Run()
		out = stdout.String()
		if err != nil {
			err = fmt.Errorf("%s: %s", err, stderr.String())
		}
	}

	if err != nil {
		return "", fmt.Errorf("failed to submit test print job: %w", err)
	}

	// Parse job ID from output: "request id is OfficeLaserJet-145 (1 file(s))"
	jobID := name + "-test"
	if idx := strings.Index(out, "request id is "); idx != -1 {
		remainder := out[idx+len("request id is "):]
		parts := strings.Fields(remainder)
		if len(parts) > 0 {
			jobID = parts[0]
		}
	}

	return jobID, nil
}

// parseLPInfoLongOutput parses the verbose output of `lpinfo -l -m` where paths with spaces are preserved.
func parseLPInfoLongOutput(out string) []DriverInfo {
	var drivers []DriverInfo
	var current DriverInfo
	var inModel bool

	lines := strings.Split(out, "\n")
	for _, line := range lines {
		lineTrimmed := strings.TrimSpace(line)
		if lineTrimmed == "" {
			continue
		}

		if strings.HasPrefix(lineTrimmed, "Model:") {
			if inModel && current.Name != "" {
				drivers = append(drivers, current)
			}
			current = DriverInfo{
				ArchReadiness: ArchReadiness{
					Status:          "universal",
					Label:           "Universal (Apple Silicon Ready)",
					RosettaRequired: false,
					BinaryCount:     1,
				},
			}
			inModel = true

			rem := strings.TrimSpace(strings.TrimPrefix(lineTrimmed, "Model:"))
			if strings.HasPrefix(rem, "name =") {
				current.Name = strings.TrimSpace(strings.TrimPrefix(rem, "name ="))
			}
			continue
		}

		if !inModel {
			continue
		}

		parts := strings.SplitN(lineTrimmed, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			switch key {
			case "name":
				current.Name = val
			case "make-and-model":
				current.Model = val
				fields := strings.Fields(val)
				if len(fields) > 0 {
					current.Make = fields[0]
				}
			case "device-id":
				if current.Make == "" || current.Make == "Generic" {
					for _, item := range strings.Split(val, ";") {
						sub := strings.SplitN(item, ":", 2)
						if len(sub) == 2 && strings.TrimSpace(sub[0]) == "MFG" {
							current.Make = strings.TrimSpace(sub[1])
						}
					}
				}
			}
		}
	}

	if inModel && current.Name != "" {
		drivers = append(drivers, current)
	}

	for i := range drivers {
		if drivers[i].Make == "" {
			drivers[i].Make = "Generic"
		}
		if drivers[i].Model == "" {
			drivers[i].Model = drivers[i].Name
		}

		nameLower := strings.ToLower(drivers[i].Name)
		modelLower := strings.ToLower(drivers[i].Model)

		if strings.Contains(nameLower, "everywhere") || strings.Contains(modelLower, "everywhere") {
			drivers[i].ArchReadiness = ArchReadiness{
				Status:          "driverless",
				Label:           "Driverless (AirPrint / IPP)",
				RosettaRequired: false,
				BinaryCount:     0,
			}
		} else if strings.Contains(nameLower, "canon") || strings.Contains(modelLower, "canon") || strings.Contains(nameLower, "cncups") {
			drivers[i].ArchReadiness = ArchReadiness{
				Status:          "rosetta_dependent",
				Label:           "Intel Only (Rosetta 2)",
				RosettaRequired: true,
				BinaryCount:     1,
			}
		}
	}

	return drivers
}

// parseLPInfoShortOutput provides a fallback parser for `lpinfo -m` using extension boundaries.
func parseLPInfoShortOutput(out string) []DriverInfo {
	var drivers []DriverInfo
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var name, model string
		lower := strings.ToLower(line)
		var extIdx = -1
		var extLen = 0
		for _, ext := range []string{".ppd.gz", ".ppd", ".gz"} {
			if idx := strings.Index(lower, ext+" "); idx != -1 {
				extIdx = idx
				extLen = len(ext)
				break
			}
		}

		if extIdx != -1 {
			name = strings.TrimSpace(line[:extIdx+extLen])
			model = strings.TrimSpace(line[extIdx+extLen:])
		} else {
			parts := strings.SplitN(line, " ", 2)
			name = strings.TrimSpace(parts[0])
			if len(parts) > 1 {
				model = strings.TrimSpace(parts[1])
			}
		}

		if name == "" {
			continue
		}
		if model == "" {
			model = name
		}

		fields := strings.Fields(model)
		makeName := "Generic"
		if len(fields) > 0 {
			makeName = fields[0]
		}

		status := "universal"
		label := "Universal (Apple Silicon Ready)"
		rosetta := false
		binCount := 1

		nameLower := strings.ToLower(name)
		modelLower := strings.ToLower(model)
		if strings.Contains(nameLower, "everywhere") {
			status = "driverless"
			label = "Driverless (AirPrint / IPP)"
			rosetta = false
			binCount = 0
		} else if strings.Contains(nameLower, "canon") || strings.Contains(modelLower, "canon") || strings.Contains(nameLower, "cncups") {
			status = "rosetta_dependent"
			label = "Intel Only (Rosetta 2)"
			rosetta = true
		}

		drivers = append(drivers, DriverInfo{
			Name:  name,
			Make:  makeName,
			Model: model,
			ArchReadiness: ArchReadiness{
				Status:          status,
				Label:           label,
				RosettaRequired: rosetta,
				BinaryCount:     binCount,
			},
		})
	}
	return drivers
}

// GetDrivers lists available drivers/models from lpinfo -m or lpinfo -l -m.
func (a *CLIAdapter) GetDrivers(search string) ([]DriverInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	var allDrivers []DriverInfo

	// Attempt verbose format first (lpinfo -l -m) to reliably extract names with spaces
	out, err := a.runCommand(ctx, "lpinfo", "-l", "-m")
	if err == nil && strings.Contains(out, "Model:") {
		allDrivers = parseLPInfoLongOutput(out)
	} else {
		// Fallback to standard lpinfo -m with extension-aware parsing
		shortOut, shortErr := a.runCommand(ctx, "lpinfo", "-m")
		if shortErr != nil {
			return nil, fmt.Errorf("failed to retrieve drivers from lpinfo: %w", shortErr)
		}
		allDrivers = parseLPInfoShortOutput(shortOut)
	}

	searchLower := strings.ToLower(strings.TrimSpace(search))
	if searchLower == "" {
		if len(allDrivers) > 300 {
			return allDrivers[:300], nil
		}
		return allDrivers, nil
	}

	var filtered []DriverInfo
	for _, d := range allDrivers {
		if strings.Contains(strings.ToLower(d.Name), searchLower) ||
			strings.Contains(strings.ToLower(d.Model), searchLower) ||
			strings.Contains(strings.ToLower(d.Make), searchLower) {
			filtered = append(filtered, d)
			if len(filtered) >= 300 {
				break
			}
		}
	}

	return filtered, nil
}

// ValidatePPD pre-flight checks a custom PPD file with cupstestppd.
func (a *CLIAdapter) ValidatePPD(content []byte) (*PPDValidationResult, error) {
	if len(content) == 0 {
		return &PPDValidationResult{
			Valid:  false,
			Errors: []string{"PPD content is empty"},
		}, nil
	}

	tmpFile, err := os.CreateTemp("", "cups-webui-ppd-*.ppd")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(content); err != nil {
		return nil, fmt.Errorf("failed to write PPD content: %w", err)
	}
	_ = tmpFile.Close()

	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "cupstestppd", "-v", tmpFile.Name())
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	outStr := stdout.String() + "\n" + stderr.String()
	var warnings []string
	var errors []string
	model := "Custom PostScript Printer"

	lines := strings.Split(outStr, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "WARN:") {
			warnings = append(warnings, strings.TrimPrefix(l, "WARN: "))
		} else if strings.HasPrefix(l, "FAIL:") {
			errors = append(errors, strings.TrimPrefix(l, "FAIL: "))
		}
	}

	// Try extracting ModelName from raw content
	for _, line := range bytes.Split(content, []byte("\n")) {
		trimmed := strings.TrimSpace(string(line))
		if strings.HasPrefix(trimmed, "*ModelName:") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				model = strings.Trim(strings.TrimSpace(parts[1]), "\"")
			}
			break
		}
	}

	// Security inspection for prohibited or hazardous directives (CVE-2024-47176 mitigation)
	secWarnings, secErrors := InspectDangerousPPDDirectives(content)
	warnings = append(warnings, secWarnings...)
	errors = append(errors, secErrors...)

	valid := runErr == nil && len(errors) == 0

	return &PPDValidationResult{
		Valid:    valid,
		Model:    model,
		Warnings: warnings,
		Errors:   errors,
	}, nil
}

// GetServerSettings fetches global cupsctl configurations.
func (a *CLIAdapter) GetServerSettings() (*ServerSettings, error) {
	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	out, err := a.runCommand(ctx, "cupsctl")
	if err != nil {
		return nil, fmt.Errorf("failed to run cupsctl: %w", err)
	}

	settings := &ServerSettings{}
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		switch k {
		case "_share_printers":
			settings.SharePrinters = (v == "1")
		case "_remote_any", "_remote_printers":
			settings.RemotePrinters = (v == "1")
		case "_debug_logging":
			settings.DebugLogging = (v == "1")
		}
	}

	return settings, nil
}

// SetServerSettings modifies global cupsctl configurations.
func (a *CLIAdapter) SetServerSettings(settings ServerSettings) error {
	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	var args []string
	if settings.SharePrinters {
		args = append(args, "--share-printers")
	} else {
		args = append(args, "--no-share-printers")
	}

	if settings.RemotePrinters {
		args = append(args, "--remote-any")
	} else {
		args = append(args, "--no-remote-any")
	}

	if settings.DebugLogging {
		args = append(args, "--debug-logging")
	} else {
		args = append(args, "--no-debug-logging")
	}

	if _, err := a.runCommand(ctx, "cupsctl", args...); err != nil {
		return fmt.Errorf("failed to apply cupsctl settings: %w", err)
	}

	return nil
}

// readTailLog safely reads at most maxBytes from the end of a log file without buffering entire huge files in RAM.
func readTailLog(path string, maxBytes int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return "", err
	}

	size := stat.Size()
	offset := int64(0)
	readLen := size
	if size > maxBytes {
		offset = size - maxBytes
		readLen = maxBytes
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return "", err
		}
	}

	buf := make([]byte, readLen)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", err
	}
	return string(buf[:n]), nil
}

// GetLogs retrieves and parses entries from /var/log/cups/error_log.
func (a *CLIAdapter) GetLogs(lines int, level string) ([]LogEntry, error) {
	if lines <= 0 {
		lines = 100
	} else if lines > 1000 {
		lines = 1000
	}

	logPaths := []string{
		"/var/log/cups/error_log",
		"/private/var/log/cups/error_log",
	}

	var content string
	for _, p := range logPaths {
		if data, err := readTailLog(p, 2<<20); err == nil && data != "" { // Max 2MB buffer
			content = data
			break
		}
	}

	if content == "" {
		// Fallback: try reading using tail if direct file read fails
		ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
		defer cancel()
		if out, err := a.runCommand(ctx, "tail", "-n", fmt.Sprintf("%d", lines*2), "/var/log/cups/error_log"); err == nil {
			content = out
		}
	}

	if content == "" {
		return []LogEntry{}, nil
	}

	entries := ParseErrorLog(content, lines, level)
	return entries, nil
}

// GetLaunchAgentStatus returns the current status of the LaunchAgent.
func (a *CLIAdapter) GetLaunchAgentStatus() (*LaunchAgentStatus, error) {
	binPath, err := os.Executable()
	if err != nil {
		binPath = os.Args[0]
	}
	s, err := macos.GetStatus(binPath)
	if err != nil {
		return nil, err
	}
	return &LaunchAgentStatus{
		Supported:    s.Supported,
		Installed:    s.Installed,
		Running:      s.Running,
		PlistPath:    s.PlistPath,
		Executable:   s.Executable,
		StatusDetail: s.StatusDetail,
	}, nil
}

// ManageLaunchAgent handles install or uninstall operations for the LaunchAgent.
func (a *CLIAdapter) ManageLaunchAgent(action string) error {
	binPath, err := os.Executable()
	if err != nil {
		binPath = os.Args[0]
	}
	switch strings.ToLower(action) {
	case "install":
		port := a.ListenPort
		if port == 0 {
			port = 6310
		}
		return macos.Install(binPath, port, "127.0.0.1")
	case "uninstall":
		return macos.Uninstall()
	default:
		return fmt.Errorf("unknown LaunchAgent action: %q (expected 'install' or 'uninstall')", action)
	}
}
