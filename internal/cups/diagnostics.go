package cups

import (
	"bufio"
	"regexp"
	"strings"
	"time"
)

// DiagnosticRule defines a pattern to match against error text and provide troubleshooting guidance.
type DiagnosticRule struct {
	Code        string
	Pattern     *regexp.Regexp
	Title       string
	Explanation string
	Remedies    []string
	Severity    string
}

var diagnosticRules = []DiagnosticRule{
	// --- CRITICAL ERRORS ---
	{
		Code:    "rosetta_bad_cpu",
		Pattern: regexp.MustCompile(`(?i)(bad\s+cpu\s+type\s+in\s+executable|crashed\s+on\s+apple\s+silicon|rosetta\s+error|bad\s+mach-o\s+slice|incompatible\s+architecture)`),
		Title:   "Apple Silicon Architecture Incompatibility",
		Explanation: "A printer driver filter or backend binary was compiled exclusively for Intel (x86_64) processors and cannot execute because Rosetta 2 translation is missing or failed.",
		Remedies: []string{
			"Install Rosetta 2 on this Mac by opening Terminal and running: softwareupdate --install-rosetta",
			"Switch the printer queue driver to 'Generic PostScript Printer' or 'Driverless IPP Everywhere'.",
			"Check the manufacturer's website for an Apple Silicon (ARM64 native) driver update.",
		},
		Severity: "error",
	},
	{
		Code:    "filter_failed",
		Pattern: regexp.MustCompile(`(?i)(filter\s+failed|cgpdftoraster\s+failed|foomatic-rip\s+failed|rasterto\w+\s+failed|pstops\s+failed)`),
		Title:   "Document Conversion Crash",
		Explanation: "The printer driver failed while converting the document into the printer's native page-description language.",
		Remedies: []string{
			"Verify that the print document is not password-protected, encrypted, or corrupted.",
			"Check if the printer driver is Intel-only (x86_64) and crashing on Apple Silicon without Rosetta.",
			"Switch the driver to 'Generic PostScript Printer' or 'Driverless IPP Everywhere' in the PPD settings.",
			"Inspect /var/log/cups/error_log for sandbox denial or missing filter dependencies.",
		},
		Severity: "error",
	},
	{
		Code:    "dns_lookup_failure",
		Pattern: regexp.MustCompile(`(?i)(getaddrinfo\s+failed|name\s+or\s+service\s+not\s+known|nodename\s+nor\s+servname\s+provided|failure\s+in\s+name\s+resolution)`),
		Title:   "DNS / Hostname Resolution Failure",
		Explanation: "macOS was unable to resolve the printer's network hostname or Bonjour mDNS address (.local) to an IP address.",
		Remedies: []string{
			"Verify that Bonjour / mDNS is functioning on your local network subnet.",
			"Replace the printer's .local hostname with a static IPv4 address in the printer's Device URI.",
			"Ensure your Mac's DNS server configuration is reachable and functioning.",
		},
		Severity: "error",
	},
	{
		Code:    "printer_unreachable",
		Pattern: regexp.MustCompile(`(?i)(unable\s+to\s+locate\s+printer|host\s+is\s+down|no\s+route\s+to\s+host|network\s+host\s+.*is\s+busy|printer\s+not\s+responding|connection\s+timed\s+out)`),
		Title:   "Printer Unreachable on Network",
		Explanation: "The network address or hostname configured for this printer cannot be reached over the local network.",
		Remedies: []string{
			"Check if the physical printer is powered on and connected to the Wi-Fi or Ethernet network.",
			"Verify that the printer's IP address has not changed due to a DHCP lease renewal.",
			"Check local firewall rules to ensure port 9100 (AppSocket/JetDirect) or 631 (IPP) is open.",
			"Ping the printer's IP address from Terminal to verify network routing.",
		},
		Severity: "error",
	},
	{
		Code:    "connection_dropped",
		Pattern: regexp.MustCompile(`(?i)(broken\s+pipe|connection\s+refused|connection\s+reset\s+by\s+peer|socket\s+error|unable\s+to\s+write\s+print\s+data)`),
		Title:   "Connection Dropped by Device",
		Explanation: "The printer abruptly closed, refused, or reset the network socket connection during print data transmission.",
		Remedies: []string{
			"Power-cycle the printer to clear hung network spooler sessions or memory buffer overflows.",
			"Ensure port 9100 (AppSocket) or port 631 (IPP) is not blocked by a firewall or network switch.",
			"If printing high-resolution graphics, lower the resolution in PPD options (e.g. 600 DPI) to prevent printer memory exhaustion.",
		},
		Severity: "error",
	},
	{
		Code:    "ssl_tls_error",
		Pattern: regexp.MustCompile(`(?i)(ssl\s+negotiation\s+failed|certificate\s+verify\s+failed|self-signed\s+certificate|unable\s+to\s+establish\s+tls|peer\s+certificate\s+cannot\s+be\s+authenticated|tls\s+handshake\s+failed|tlsv1\s+alert)`),
		Title:   "TLS / SSL Certificate Negotiation Failure",
		Explanation: "CUPS failed to establish an encrypted connection to the IPPS printer due to an expired, untrusted, or self-signed SSL/TLS certificate.",
		Remedies: []string{
			"Import the printer's CA certificate into the macOS System Keychain and mark it as trusted.",
			"Switch the device URI scheme from 'ipps://' to 'ipp://' if encryption is not required on your trusted LAN.",
			"Verify the date and time on the physical printer to ensure its onboard certificate has not expired.",
		},
		Severity: "error",
	},
	{
		Code:    "auth_required",
		Pattern: regexp.MustCompile(`(?i)(authentication\s+required|401\s+unauthorized|403\s+forbidden|client-error-not-authorized|client-error-forbidden|bad\s+credentials|password\s+incorrect)`),
		Title:   "Authentication or Accounting Code Required",
		Explanation: "The print server or multifunction printer rejected the job because valid user credentials, an accounting PIN, or department code were missing or rejected.",
		Remedies: []string{
			"Configure your user ID or billing department code in the printer's PPD options.",
			"Verify your print server username and password.",
			"Check if the print queue requires Kerberos or Active Directory credentials.",
		},
		Severity: "error",
	},
	{
		Code:    "sandbox_denial",
		Pattern: regexp.MustCompile(`(?i)(sandbox:\s+.*deny|operation\s+not\s+permitted|permission\s+denied)`),
		Title:   "macOS Sandbox Security Denial",
		Explanation: "macOS App Sandbox or filesystem permissions prevented the CUPS backend or filter binary from executing.",
		Remedies: []string{
			"Verify that the driver filter binaries are code-signed and notarized for macOS.",
			"Check permissions on /Library/Printers to ensure read and execute rights for group _lp.",
			"Review macOS Console (Console.app) for Sandbox deny reports.",
		},
		Severity: "error",
	},
	{
		Code:    "ppd_error",
		Pattern: regexp.MustCompile(`(?i)(unable\s+to\s+open.*ppd|empty\s+ppd\s+file|unable\s+to\s+copy\s+ppd|no\s+such\s+file\s+or\s+directory.*ppd|ppd\s+file\s+.*not\s+found|failed\s+to\s+copy\s+ppd)`),
		Title:   "PPD Driver File Error",
		Explanation: "CUPS was unable to locate, open, or copy the specified PPD driver file.",
		Remedies: []string{
			"Verify that the vendor driver package is fully installed in /Library/Printers or /usr/share/cups/model.",
			"Re-install the manufacturer driver package or upload a custom validated PPD.",
			"Assign the 'Generic PostScript Printer' or 'Driverless IPP Everywhere' driver to restore queue functionality.",
		},
		Severity: "error",
	},
	{
		Code:    "unsupported_format",
		Pattern: regexp.MustCompile(`(?i)(client-error-attributes-or-values-not-supported|unsupported\s+format|document-format-not-supported|mime\s+type\s+not\s+supported)`),
		Title:   "Unsupported Document Format",
		Explanation: "The printer or driver filter does not support the MIME type or page attributes of the submitted document.",
		Remedies: []string{
			"Ensure standard CUPS conversion filters (cgpdftops, cgpdftoraster) are installed and intact.",
			"Check if the printer PPD requires PostScript Level 2 or Level 3 emulation.",
			"Try printing the document as an image or flattened PDF.",
		},
		Severity: "error",
	},
	{
		Code:    "quota_disk_full",
		Pattern: regexp.MustCompile(`(?i)(quota\s+exceeded|file\s+too\s+large|job-quota-reached|disk\s+full|no\s+space\s+left\s+on\s+device)`),
		Title:   "Disk Full or Spool Quota Exceeded",
		Explanation: "The Mac's storage drive is full, or the CUPS spool directory (/var/spool/cups) has reached its maximum quota.",
		Remedies: []string{
			"Free up storage space on the Mac startup volume.",
			"Cancel old stalled print jobs in CUPS webui to purge temporary spool files.",
			"Check /var/spool/cups permissions and available inodes.",
		},
		Severity: "error",
	},
	{
		Code:    "invalid_queue",
		Pattern: regexp.MustCompile(`(?i)(destination\s+printer\s+does\s+not\s+exist|unknown\s+destination|invalid\s+printer\s+name)`),
		Title:   "Invalid Queue Destination",
		Explanation: "The target print queue was renamed, deleted, or unregistered from the CUPS spooler.",
		Remedies: []string{
			"Cancel or delete the remaining print job from the spool queue.",
			"Re-assign the document to an active, registered printer queue.",
		},
		Severity: "error",
	},
	{
		Code:    "cupsd_conf_syntax_error",
		Pattern: regexp.MustCompile(`(?i)(unknown\s+(?:policy\s+limit|location\s+limit|directive)\s+.*on\s+line\s+\d+\s+of\s+.*cupsd\.conf|syntax\s+error\s+on\s+line\s+\d+\s+of\s+.*cupsd\.conf)`),
		Title:   "CUPS Configuration File Syntax Error",
		Explanation: "A directive or tag in /private/etc/cups/cupsd.conf is misspelled, unclosed, or nested illegally, preventing CUPS from parsing its security policy.",
		Remedies: []string{
			"Inspect the indicated line in /private/etc/cups/cupsd.conf for unclosed XML-style tags like </Limit> or </Policy>.",
			"Ensure directives like 'Order deny,allow' and '</Limit>' are placed on separate lines.",
			"Restore a clean configuration backup from /private/etc/cups/cupsd.conf.default if necessary.",
		},
		Severity: "error",
	},

	// --- HARDWARE & OPERATIONAL WARNINGS ---
	{
		Code:    "media_jam",
		Pattern: regexp.MustCompile(`(?i)(paper\s+jam|media\s+jam|jam\s+in\s+tray|media-jam|paper-jam)`),
		Title:   "Paper Jam Detected",
		Explanation: "A physical sheet of paper is jammed in the feed rollers, fuser, or duplex unit.",
		Remedies: []string{
			"Open the indicated printer access doors (Tray 1/2, rear cover, or duplexer) and gently pull out the jammed sheet.",
			"Check for small torn paper fragments or foreign objects along the paper path.",
			"Close all printer covers firmly, then tap 'Resume Printer' in CUPS webui.",
		},
		Severity: "warning",
	},
	{
		Code:    "out_of_paper",
		Pattern: regexp.MustCompile(`(?i)(media\s+tray\s+empty|paper\s+out|out\s+of\s+paper|tray\s+\d+\s+empty|load\s+paper|out-of-paper|media-empty)`),
		Title:   "Out of Paper / Media Tray Empty",
		Explanation: "The requested paper cassette, bypass tray, or multipurpose feeder is out of paper.",
		Remedies: []string{
			"Load paper of the requested size (Letter, A4, etc.) into the designated printer cassette.",
			"Adjust the paper edge guides inside the cassette snugly against the stack.",
			"Tap 'Resume Printer' or acknowledge the paper prompt on the physical printer panel.",
		},
		Severity: "warning",
	},
	{
		Code:    "toner_supply_low",
		Pattern: regexp.MustCompile(`(?i)(marker-supply-low|marker-supply-empty|toner\s+low|toner\s+empty|ink\s+low|ink\s+empty|cartridge\s+missing|toner\s+exhausted|waste\s+toner\s+full)`),
		Title:   "Consumable Supply Low or Empty",
		Explanation: "Toner, ink, drum unit, or waste toner bottle is near exhaustion or completely empty.",
		Remedies: []string{
			"Check the physical printer display or supply gauges for which color (C, M, Y, K) is depleted.",
			"Have a replacement toner or ink cartridge ready to swap in.",
			"If printing continues, output quality may be degraded or faded.",
		},
		Severity: "warning",
	},
	{
		Code:    "door_open",
		Pattern: regexp.MustCompile(`(?i)(cover\s+open|door\s+open|interlock\s+open|top\s+cover\s+open|cover-open|door-open)`),
		Title:   "Printer Cover or Door Open",
		Explanation: "An access door, toner door, or paper cassette interlock on the printer is open or unlatched.",
		Remedies: []string{
			"Inspect the printer and firmly close all access covers and side doors.",
			"Ensure paper cassettes are pushed all the way in until they click.",
			"Once latched, the printer will re-initialize and resume printing automatically.",
		},
		Severity: "warning",
	},
	{
		Code:    "printer_offline",
		Pattern: regexp.MustCompile(`(?i)(printer\s+is\s+offline|printer\s+offline|device\s+is\s+offline|device\s+offline)`),
		Title:   "Printer Offline / Low-Power Sleep",
		Explanation: "The printer is currently in low-power sleep mode, turned off, or disconnected from the interface.",
		Remedies: []string{
			"Wake the printer by pressing the Power or Wake button on its physical control panel.",
			"Check USB or network cable connections if directly attached.",
			"Tap 'Resume Printer' in CUPS webui once the printer display shows ready.",
		},
		Severity: "warning",
	},
}

// ClassifyError inspects error text and returns structured diagnostic guidance if a known issue is matched.
func ClassifyError(text string) *DiagnosticInfo {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	for _, rule := range diagnosticRules {
		if rule.Pattern.MatchString(text) {
			remedies := make([]string, len(rule.Remedies))
			copy(remedies, rule.Remedies)
			return &DiagnosticInfo{
				Code:        rule.Code,
				Title:       rule.Title,
				Explanation: rule.Explanation,
				Remedies:    remedies,
				Severity:    rule.Severity,
			}
		}
	}

	return nil
}

var (
	logLineRegex   = regexp.MustCompile(`^([EWID])\s+\[([^\]]+)\]\s+(.*)$`)
	printerInURL   = regexp.MustCompile(`printers/([A-Za-z0-9_.-]+)`)
	jobInMsg       = regexp.MustCompile(`\[Job\s+(\d+)\]`)
	cupsDateFormat = "02/Jan/2006:15:04:05 -0700"
)

// ParseErrorLog scans CUPS log content and returns structured log entries annotated with diagnostics.
func ParseErrorLog(content string, maxLines int, levelFilter string) []LogEntry {
	if maxLines <= 0 {
		maxLines = 100
	}
	levelFilter = strings.ToLower(strings.TrimSpace(levelFilter))

	var allEntries []LogEntry
	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		matches := logLineRegex.FindStringSubmatch(line)
		if len(matches) != 4 {
			// Unformatted line: treat as generic info/error line
			diag := ClassifyError(line)
			entry := LogEntry{
				RawDate:    "",
				Level:      "info",
				Message:    line,
				Diagnostic: diag,
			}
			if diag != nil {
				entry.Level = diag.Severity
				if entry.Level == "warning" {
					entry.Level = "warn"
				}
			}
			if matchesLevel(entry.Level, levelFilter) {
				allEntries = append(allEntries, entry)
			}
			continue
		}

		levelCode := matches[1]
		rawDate := matches[2]
		msg := matches[3]

		var level string
		switch levelCode {
		case "E", "X":
			level = "error"
		case "W":
			level = "warn"
		case "I":
			level = "info"
		case "D":
			level = "debug"
		default:
			level = "info"
		}

		if !matchesLevel(level, levelFilter) {
			continue
		}

		ts, err := time.Parse(cupsDateFormat, rawDate)
		if err != nil {
			ts = time.Now()
		}

		// Extract printer name if mentioned
		var printer string
		if pMatch := printerInURL.FindStringSubmatch(msg); len(pMatch) >= 2 {
			printer = pMatch[1]
		}

		// Extract job ID if mentioned
		var jobID string
		if jMatch := jobInMsg.FindStringSubmatch(msg); len(jMatch) >= 2 {
			jobID = jMatch[1]
		}

		diag := ClassifyError(msg)

		allEntries = append(allEntries, LogEntry{
			Timestamp:  ts,
			RawDate:    rawDate,
			Level:      level,
			Message:    msg,
			Printer:    printer,
			JobID:      jobID,
			Diagnostic: diag,
		})
	}

	// Return the latest entries up to maxLines
	if len(allEntries) > maxLines {
		return allEntries[len(allEntries)-maxLines:]
	}
	return allEntries
}

func matchesLevel(entryLevel, filter string) bool {
	if filter == "" || filter == "all" {
		return true
	}
	switch strings.ToLower(filter) {
	case "error", "errors":
		return entryLevel == "error"
	case "warn", "warning", "warnings":
		return entryLevel == "warn" || entryLevel == "warning"
	case "problems", "issues", "errors_and_warnings":
		return entryLevel == "error" || entryLevel == "warn" || entryLevel == "warning"
	case "info":
		return entryLevel == "info"
	case "debug":
		return entryLevel == "debug"
	default:
		return entryLevel == filter
	}
}

