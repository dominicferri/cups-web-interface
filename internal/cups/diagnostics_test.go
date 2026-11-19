package cups

import (
	"testing"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedCode string
		expectedSev  string
	}{
		{
			name:         "filter failed",
			input:        "Stopped - Filter failed",
			expectedCode: "filter_failed",
			expectedSev:  "error",
		},
		{
			name:         "cgpdftoraster failed",
			input:        "/usr/libexec/cups/filter/cgpdftoraster failed with exit code 1",
			expectedCode: "filter_failed",
			expectedSev:  "error",
		},
		{
			name:         "host is down",
			input:        "Unable to locate printer: Host is down",
			expectedCode: "printer_unreachable",
			expectedSev:  "error",
		},
		{
			name:         "printer offline",
			input:        "printer is offline",
			expectedCode: "printer_offline",
			expectedSev:  "warning",
		},
		{
			name:         "paper out",
			input:        "Media tray empty",
			expectedCode: "out_of_paper",
			expectedSev:  "warning",
		},
		{
			name:         "ppd missing",
			input:        "Unable to open \"/Library/Printers/PPDs/Kyocera\" - No such file or directory",
			expectedCode: "ppd_error",
			expectedSev:  "error",
		},
		{
			name:         "broken pipe",
			input:        "Write failure: Broken pipe",
			expectedCode: "connection_dropped",
			expectedSev:  "error",
		},
		{
			name:         "sandbox denial",
			input:        "Sandbox: cupsd(123) deny(1) file-read-data",
			expectedCode: "sandbox_denial",
			expectedSev:  "error",
		},
		{
			name:         "rosetta bad cpu",
			input:        "PID 1234 (/Library/Printers/Kyocera/filter) crashed: Bad CPU type in executable",
			expectedCode: "rosetta_bad_cpu",
			expectedSev:  "error",
		},
		{
			name:         "dns lookup failed",
			input:        "Unable to locate printer: getaddrinfo failed - Name or service not known",
			expectedCode: "dns_lookup_failure",
			expectedSev:  "error",
		},
		{
			name:         "ssl tls failure",
			input:        "Unable to establish TLS connection: certificate verify failed",
			expectedCode: "ssl_tls_error",
			expectedSev:  "error",
		},
		{
			name:         "auth required",
			input:        "HTTP 401 Unauthorized: client-error-not-authorized",
			expectedCode: "auth_required",
			expectedSev:  "error",
		},
		{
			name:         "unsupported format",
			input:        "client-error-attributes-or-values-not-supported: document-format-not-supported",
			expectedCode: "unsupported_format",
			expectedSev:  "error",
		},
		{
			name:         "quota or disk full",
			input:        "No space left on device: spool directory disk full",
			expectedCode: "quota_disk_full",
			expectedSev:  "error",
		},
		{
			name:         "paper jam",
			input:        "The printer reported: Paper jam in Tray 2",
			expectedCode: "media_jam",
			expectedSev:  "warning",
		},
		{
			name:         "toner low",
			input:        "printer-state-reasons=marker-supply-low: Black toner low",
			expectedCode: "toner_supply_low",
			expectedSev:  "warning",
		},
		{
			name:         "door open",
			input:        "printer-state-reasons=cover-open: Top cover open",
			expectedCode: "door_open",
			expectedSev:  "warning",
		},
		{
			name:         "cupsd.conf syntax error",
			input:        "Unknown Policy Limit directive <Limit on line 73 of /private/etc/cups/cupsd.conf.",
			expectedCode: "cupsd_conf_syntax_error",
			expectedSev:  "error",
		},
		{
			name:         "unrelated text",
			input:        "Printer is ready and accepting jobs",
			expectedCode: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diag := ClassifyError(tc.input)
			if tc.expectedCode == "" {
				if diag != nil {
					t.Fatalf("expected nil diagnostic, got %+v", diag)
				}
				return
			}
			if diag == nil {
				t.Fatalf("expected diagnostic with code %q, got nil", tc.expectedCode)
			}
			if diag.Code != tc.expectedCode {
				t.Errorf("expected code %q, got %q", tc.expectedCode, diag.Code)
			}
			if diag.Severity != tc.expectedSev {
				t.Errorf("expected severity %q, got %q", tc.expectedSev, diag.Severity)
			}
			if len(diag.Remedies) == 0 {
				t.Errorf("expected remedies, got empty list")
			}
		})
	}
}

func TestParseErrorLog(t *testing.T) {
	sampleLog := `
I [10/Sep/2026:20:00:00 +1000] Listening to 127.0.0.1:631
E [10/Sep/2026:23:39:44 +1000] [cups-driverd] Unable to open \"/Library/Printers/PPDs/Kyocera\" - No such file or directory
W [10/Sep/2026:23:40:00 +1000] [Job 105] Printer is offline.
E [10/Sep/2026:23:45:10 +1000] [Job 105] Stopped - Filter failed
E [10/Sep/2026:23:50:00 +1000] Returning IPP server-error for CUPS-Add-Modify-Printer (ipp://localhost:631/printers/Marketing)
`

	entries := ParseErrorLog(sampleLog, 100, "all")
	if len(entries) != 5 {
		t.Fatalf("expected 5 log entries, got %d", len(entries))
	}

	// Entry 1: info
	if entries[0].Level != "info" {
		t.Errorf("expected entry 0 level 'info', got %q", entries[0].Level)
	}

	// Entry 2: error with PPD error diagnostic
	if entries[1].Level != "error" {
		t.Errorf("expected entry 1 level 'error', got %q", entries[1].Level)
	}
	if entries[1].Diagnostic == nil || entries[1].Diagnostic.Code != "ppd_error" {
		t.Errorf("expected ppd_error diagnostic for entry 1, got %+v", entries[1].Diagnostic)
	}

	// Entry 3: warning with Job 105
	if entries[2].JobID != "105" {
		t.Errorf("expected job_id '105', got %q", entries[2].JobID)
	}
	if entries[2].Diagnostic == nil || entries[2].Diagnostic.Code != "printer_offline" {
		t.Errorf("expected printer_offline diagnostic for entry 2, got %+v", entries[2].Diagnostic)
	}

	// Entry 4: error with filter_failed
	if entries[3].Diagnostic == nil || entries[3].Diagnostic.Code != "filter_failed" {
		t.Errorf("expected filter_failed diagnostic for entry 3, got %+v", entries[3].Diagnostic)
	}

	// Entry 5: printer extraction
	if entries[4].Printer != "Marketing" {
		t.Errorf("expected printer 'Marketing', got %q", entries[4].Printer)
	}

	// Test level filtering
	errorOnly := ParseErrorLog(sampleLog, 100, "error")
	if len(errorOnly) != 3 {
		t.Errorf("expected 3 error entries, got %d", len(errorOnly))
	}

	warnOnly := ParseErrorLog(sampleLog, 100, "warn")
	if len(warnOnly) != 1 {
		t.Errorf("expected 1 warn entry, got %d", len(warnOnly))
	} else if warnOnly[0].Level != "warn" {
		t.Errorf("expected warn level, got %s", warnOnly[0].Level)
	}

	infoOnly := ParseErrorLog(sampleLog, 100, "info")
	if len(infoOnly) != 1 {
		t.Errorf("expected 1 info entry, got %d", len(infoOnly))
	} else if infoOnly[0].Level != "info" {
		t.Errorf("expected info level, got %s", infoOnly[0].Level)
	}

	problems := ParseErrorLog(sampleLog, 100, "problems")
	if len(problems) != 4 { // 3 errors + 1 warning
		t.Errorf("expected 4 problem entries (errors + warnings), got %d", len(problems))
	}
}
