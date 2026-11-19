package cups

import (
	"bufio"
	"strconv"
	"strings"
	"time"
)

// ParseCombinedStatus parses the combined output of `lpstat -p -d -v -a` and `lpstat -c`.
func ParseCombinedStatus(statusOutput, classOutput string) map[string]*Printer {
	printers := make(map[string]*Printer)

	var defaultPrinter string
	classes := make(map[string]bool)

	// Parse classes first if available
	if classOutput != "" {
		scanner := bufio.NewScanner(strings.NewReader(classOutput))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			// Format: "members of class <class>: <members...>"
			if strings.HasPrefix(line, "members of class ") {
				parts := strings.SplitN(line[len("members of class "):], ":", 2)
				if len(parts) > 0 {
					className := strings.TrimSpace(parts[0])
					classes[className] = true
				}
			}
		}
	}

	scanner := bufio.NewScanner(strings.NewReader(statusOutput))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// 1. Default destination
		if strings.HasPrefix(line, "system default destination: ") {
			defaultPrinter = strings.TrimSpace(strings.TrimPrefix(line, "system default destination: "))
			continue
		} else if line == "no system default destination" {
			defaultPrinter = ""
			continue
		}

		// 2. Printer state:
		// "printer <name> is idle. enabled since ..."
		// "printer <name> is printing. enabled since ..."
		// "printer <name> disabled since <date> - <reason>"
		// "printer <name> is stopped. ..."
		if strings.HasPrefix(line, "printer ") {
			rest := strings.TrimPrefix(line, "printer ")
			fields := strings.Fields(rest)
			if len(fields) >= 2 {
				name := fields[0]
				p, exists := printers[name]
				if !exists {
					p = &Printer{Name: name, State: "unknown"}
					printers[name] = p
				}

				if fields[1] == "is" && len(fields) >= 3 {
					rawState := strings.TrimSuffix(fields[2], ".")
					switch rawState {
					case "idle":
						p.State = "idle"
					case "printing":
						p.State = "printing"
					case "stopped":
						p.State = "stopped"
					default:
						p.State = rawState
					}
					// State message: extract real reason, omitting unhelpful "enabled since <date>"
					p.StateMessage = cleanStateMessage(rest)
				} else if fields[1] == "disabled" {
					p.State = "stopped"
					p.StateMessage = cleanStateMessage(rest)
				} else {
					p.StateMessage = cleanStateMessage(rest)
				}
			}
			continue
		}

		// 3. Device URI: "device for <name>: <uri>"
		if strings.HasPrefix(line, "device for ") {
			rest := strings.TrimPrefix(line, "device for ")
			parts := strings.SplitN(rest, ":", 2)
			if len(parts) == 2 {
				name := strings.TrimSpace(parts[0])
				uri := strings.TrimSpace(parts[1])
				// lpstat prints file devices without their "file:" scheme
				// (e.g. "/dev/null"); restore it so the URI round-trips.
				if strings.HasPrefix(uri, "/") {
					uri = "file:" + uri
				}
				p, exists := printers[name]
				if !exists {
					p = &Printer{Name: name, State: "unknown"}
					printers[name] = p
				}
				p.URI = uri
			}
			continue
		}

		// 4. Accepting requests: "<name> accepting requests since ..." or "<name> not accepting requests since ..."
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			name := fields[0]
			if strings.Contains(line, "accepting requests") {
				p, exists := printers[name]
				if !exists {
					p = &Printer{Name: name, State: "unknown"}
					printers[name] = p
				}
				p.AcceptingJobs = !strings.Contains(line, "not accepting requests")
			}
		}
	}

	// Apply default flags & class markings
	for name, p := range printers {
		if name == defaultPrinter && defaultPrinter != "" {
			p.IsDefault = true
		}
		if classes[name] {
			p.IsClass = true
		}
	}

	return printers
}

// ParsePrinterDetails parses `lpstat -l -p` and updates printer metadata (Description, Location, Alerts).
func ParsePrinterDetails(printers map[string]*Printer, detailsOutput string) {
	scanner := bufio.NewScanner(strings.NewReader(detailsOutput))
	var currentPrinter *Printer

	for scanner.Scan() {
		rawLine := scanner.Text()
		trimmedLine := strings.TrimSpace(rawLine)
		if trimmedLine == "" {
			continue
		}

		if strings.HasPrefix(trimmedLine, "printer ") {
			fields := strings.Fields(strings.TrimPrefix(trimmedLine, "printer "))
			if len(fields) > 0 {
				name := fields[0]
				currentPrinter = printers[name]
			}
			continue
		}

		if currentPrinter == nil {
			continue
		}

		if strings.HasPrefix(trimmedLine, "Description:") {
			currentPrinter.Info = strings.TrimSpace(strings.TrimPrefix(trimmedLine, "Description:"))
		} else if strings.HasPrefix(trimmedLine, "Location:") {
			currentPrinter.Location = strings.TrimSpace(strings.TrimPrefix(trimmedLine, "Location:"))
		} else if strings.HasPrefix(trimmedLine, "Alerts:") {
			alertStr := strings.TrimSpace(strings.TrimPrefix(trimmedLine, "Alerts:"))
			if alertStr != "" && alertStr != "none" {
				currentPrinter.Alerts = strings.Split(alertStr, ",")
				for i := range currentPrinter.Alerts {
					currentPrinter.Alerts[i] = strings.TrimSpace(currentPrinter.Alerts[i])
				}
			}
		}
	}
}

var dateFormats = []string{
	"Mon _2 Jan 15:04:05 2006",
	"Mon 02 Jan 15:04:05 2006",
	"Mon Jan _2 15:04:05 2006",
	"Mon Jan 02 15:04:05 2006",
	time.ANSIC,
	time.RFC1123,
	time.RFC822,
	"2006-01-02 15:04:05",
}

// ParseJobsOutput parses output from `lpstat -o` or `lpstat -W completed -o`.
// Example line:
// Direct_Physical_Printer-49 PaperCut 108544 Fri 21 Aug 16:31:39 2026
func ParseJobsOutput(output string, defaultState string) []Job {
	var jobs []Job
	scanner := bufio.NewScanner(strings.NewReader(output))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "\t") {
			// Skip indent lines from `lpstat -l` if present
			continue
		}

		fields := strings.Fields(line)
		// We expect at least: <job-id> <user> <size> <date...> (minimum 4 fields)
		if len(fields) < 4 {
			continue
		}

		jobTarget := fields[0]
		user := fields[1]
		sizeStr := fields[2]

		sizeBytes, _ := strconv.ParseInt(sizeStr, 10, 64)

		// Parse printer name and numeric job ID from jobTarget, e.g. "Direct_Physical_Printer-49"
		lastDash := strings.LastIndex(jobTarget, "-")
		printerName := jobTarget
		jobID := jobTarget
		if lastDash != -1 {
			printerName = jobTarget[:lastDash]
			jobID = jobTarget[lastDash+1:]
		}

		// Remaining fields form the raw date string
		rawDate := strings.Join(fields[3:], " ")
		var submittedAt time.Time
		for _, layout := range dateFormats {
			if t, err := time.Parse(layout, rawDate); err == nil {
				submittedAt = t
				break
			}
		}

		state := defaultState
		if state == "" {
			state = "pending"
		}

		// Stuck job heuristic:
		// If job has been processing or pending for more than 10 minutes, mark as potentially stuck
		isStuck := false
		if !submittedAt.IsZero() && (state == "processing" || state == "pending") {
			if time.Since(submittedAt) > 10*time.Minute {
				isStuck = true
			}
		}

		jobs = append(jobs, Job{
			ID:          jobID,
			Printer:     printerName,
			Title:       jobTarget,
			User:        user,
			State:       state,
			SizeBytes:   sizeBytes,
			SubmittedAt: submittedAt,
			RawDate:     rawDate,
			IsStuck:     isStuck,
		})
	}

	return jobs
}

// ParseCupsctlOutput parses key-value pairs from `cupsctl` output.
func ParseCupsctlOutput(output string) map[string]string {
	res := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			res[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return res
}

// cleanStateMessage removes "enabled since <date>" / "disabled since <date>" timestamps,
// preserving any actual status explanation (e.g. "Filter failed" or "offline").
func cleanStateMessage(rest string) string {
	if idx := strings.Index(rest, "enabled since"); idx != -1 {
		after := rest[idx+len("enabled since"):]
		if dashIdx := strings.Index(after, " - "); dashIdx != -1 {
			return strings.TrimSpace(after[dashIdx+3:])
		}
		return ""
	}
	if idx := strings.Index(rest, "disabled since"); idx != -1 {
		after := rest[idx+len("disabled since"):]
		if dashIdx := strings.Index(after, " - "); dashIdx != -1 {
			return strings.TrimSpace(after[dashIdx+3:])
		}
		return ""
	}
	return strings.TrimSpace(rest)
}

// ParseIPPJobTitlesOutput parses `ipptool` Get-Jobs output and extracts mapping of numeric job ID to document title.
func ParseIPPJobTitlesOutput(output string) map[string]string {
	titles := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(output))

	var currentID string
	var docNameSupplied string
	var pmJobName string
	var jobName string

	saveCurrent := func() {
		if currentID != "" {
			// Best candidate priority: document-name-supplied > com.apple.print.JobInfo.PMJobName > job-name
			var chosen string
			if docNameSupplied != "" {
				chosen = docNameSupplied
			} else if pmJobName != "" {
				chosen = pmJobName
			} else if jobName != "" {
				chosen = jobName
			}
			if chosen != "" {
				titles[currentID] = chosen
			}
		}
		currentID = ""
		docNameSupplied = ""
		pmJobName = ""
		jobName = ""
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "-- separator --") {
			saveCurrent()
			continue
		}

		eqIdx := strings.Index(line, "=")
		if eqIdx == -1 {
			continue
		}

		keyPart := strings.TrimSpace(line[:eqIdx])
		val := strings.TrimSpace(line[eqIdx+1:])

		if strings.HasPrefix(keyPart, "job-id ") || keyPart == "job-id" {
			currentID = val
			continue
		}

		if strings.HasPrefix(keyPart, "document-name-supplied ") || keyPart == "document-name-supplied" {
			docNameSupplied = val
			continue
		}

		if strings.HasPrefix(keyPart, "com.apple.print.JobInfo.PMJobName ") || keyPart == "com.apple.print.JobInfo.PMJobName" {
			pmJobName = val
			continue
		}

		if strings.HasPrefix(keyPart, "job-name ") || keyPart == "job-name" {
			jobName = val
			continue
		}
	}
	saveCurrent()

	return titles
}

