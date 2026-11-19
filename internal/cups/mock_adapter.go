package cups

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// MockAdapter provides an in-memory simulation of CUPS printers and queues.
type MockAdapter struct {
	mu           sync.RWMutex
	printers             map[string]*Printer
	jobs                 []Job
	options              map[string]*PrinterOptionsResponse
	defaultPrint         string
	webInterface         bool
	settings             ServerSettings
	mockDrivers          []DriverInfo
	launchAgentInstalled bool
	logs                 []LogEntry

	// Accessories handles manually set installable options (in memory).
	Accessories *AccessoryManager
	accOnce     sync.Once
}

// NewMockAdapter initializes a mock adapter populated with realistic test data.
func NewMockAdapter() *MockAdapter {
	now := time.Now()
	m := &MockAdapter{
		printers:             make(map[string]*Printer),
		options:              make(map[string]*PrinterOptionsResponse),
		defaultPrint:         "Office_LaserJet_Pro",
		webInterface:         false,
		launchAgentInstalled: false,
		settings: ServerSettings{
			SharePrinters:  false,
			RemotePrinters: false,
			DebugLogging:   false,
		},
		logs: []LogEntry{
			{
				Timestamp:  now.Add(-45 * time.Minute),
				RawDate:    now.Add(-45 * time.Minute).Format(cupsDateFormat),
				Level:      "info",
				Message:    "cupsd started successfully; listening on 127.0.0.1:631",
				Printer:    "",
				JobID:      "",
				Diagnostic: nil,
			},
			{
				Timestamp:  now.Add(-30 * time.Minute),
				RawDate:    now.Add(-30 * time.Minute).Format(cupsDateFormat),
				Level:      "warn",
				Message:    "[Job 103] Waiting for printer: Printer is offline",
				Printer:    "Design_Studio_Plotter",
				JobID:      "103",
				Diagnostic: ClassifyError("Printer is offline"),
			},
			{
				Timestamp:  now.Add(-22 * time.Minute),
				RawDate:    now.Add(-22 * time.Minute).Format(cupsDateFormat),
				Level:      "warn",
				Message:    "[Job 101] Media tray empty: Load letter paper into Tray 2",
				Printer:    "Office_LaserJet_Pro",
				JobID:      "101",
				Diagnostic: ClassifyError("Media tray empty"),
			},
			{
				Timestamp:  now.Add(-18 * time.Minute),
				RawDate:    now.Add(-18 * time.Minute).Format(cupsDateFormat),
				Level:      "warn",
				Message:    "[Job 101] The printer reported: Paper jam in Tray 2",
				Printer:    "Office_LaserJet_Pro",
				JobID:      "101",
				Diagnostic: ClassifyError("Paper jam in Tray 2"),
			},
			{
				Timestamp:  now.Add(-15 * time.Minute),
				RawDate:    now.Add(-15 * time.Minute).Format(cupsDateFormat),
				Level:      "error",
				Message:    "[cups-driverd] Unable to open \"/Library/Printers/Canon/PPDs/iradv.ppd\" - No such file or directory",
				Printer:    "Finance_Secure_Copier",
				JobID:      "",
				Diagnostic: ClassifyError("Unable to open \"/Library/Printers/Canon/PPDs/iradv.ppd\" - No such file or directory"),
			},
			{
				Timestamp:  now.Add(-12 * time.Minute),
				RawDate:    now.Add(-12 * time.Minute).Format(cupsDateFormat),
				Level:      "error",
				Message:    "[Job 102] /Library/Printers/Canon/filter/rastertoiradv crashed: Bad CPU type in executable",
				Printer:    "Finance_Secure_Copier",
				JobID:      "102",
				Diagnostic: ClassifyError("Bad CPU type in executable"),
			},
			{
				Timestamp:  now.Add(-10 * time.Minute),
				RawDate:    now.Add(-10 * time.Minute).Format(cupsDateFormat),
				Level:      "error",
				Message:    "[Job 102] /usr/libexec/cups/filter/pstops failed with exit code 1: Filter failed",
				Printer:    "Finance_Secure_Copier",
				JobID:      "102",
				Diagnostic: ClassifyError("Filter failed"),
			},
			{
				Timestamp:  now.Add(-5 * time.Minute),
				RawDate:    now.Add(-5 * time.Minute).Format(cupsDateFormat),
				Level:      "error",
				Message:    "Unable to locate printer: Host is down (192.168.1.88)",
				Printer:    "Warehouse_Zebra_Labels",
				JobID:      "104",
				Diagnostic: ClassifyError("Unable to locate printer: Host is down"),
			},
		},
		mockDrivers: []DriverInfo{
			{
				Name:  "drv:///sample.drv/generic.ppd",
				Make:  "Generic",
				Model: "Generic PostScript Printer",
				ArchReadiness: ArchReadiness{
					Status:          "universal",
					Label:           "Universal (Apple Silicon Ready)",
					RosettaRequired: false,
					BinaryCount:     1,
				},
			},
			{
				Name:  "drv:///sample.drv/laserjet.ppd",
				Make:  "HP",
				Model: "HP Color LaserJet Pro MFP M479fdw",
				ArchReadiness: ArchReadiness{
					Status:          "universal",
					Label:           "Universal (Apple Silicon Ready)",
					RosettaRequired: false,
					BinaryCount:     2,
				},
			},
			{
				Name:  "hp-laserjet_enterprise_m608-ps.ppd",
				Make:  "HP",
				Model: "HP LaserJet Enterprise M608 Postscript",
				ArchReadiness: ArchReadiness{
					Status:          "universal",
					Label:           "Universal (Apple Silicon Ready)",
					RosettaRequired: false,
					BinaryCount:     2,
				},
			},
			{
				Name:  "CNCUPSIRAC5840ZK.ppd",
				Make:  "Canon",
				Model: "Canon imageRUNNER ADVANCE DX C5840i UFR II",
				ArchReadiness: ArchReadiness{
					Status:          "rosetta_dependent",
					Label:           "Intel Only (Rosetta 2)",
					RosettaRequired: true,
					BinaryCount:     1,
				},
			},
			{
				Name:  "Library/Printers/PPDs/Contents/Resources/Kyocera CS 205c.ppd",
				Make:  "Kyocera",
				Model: "Kyocera CS 205c (KPDL)",
				ArchReadiness: ArchReadiness{
					Status:          "universal",
					Label:           "Universal (Apple Silicon Ready)",
					RosettaRequired: false,
					BinaryCount:     1,
				},
			},
			{
				Name:  "Library/Printers/PPDs/Contents/Resources/Kyocera TASKalfa 4003i.PPD",
				Make:  "Kyocera",
				Model: "Kyocera TASKalfa 4003i KPDL",
				ArchReadiness: ArchReadiness{
					Status:          "universal",
					Label:           "Universal (Apple Silicon Ready)",
					RosettaRequired: false,
					BinaryCount:     1,
				},
			},
			{
				Name:  "xrxaltac8170.ppd",
				Make:  "Xerox",
				Model: "Xerox AltaLink C8170 PS",
				ArchReadiness: ArchReadiness{
					Status:          "universal",
					Label:           "Universal (Apple Silicon Ready)",
					RosettaRequired: false,
					BinaryCount:     2,
				},
			},
			{
				Name:  "brother-HLL6400DW-cups-en.ppd",
				Make:  "Brother",
				Model: "Brother HL-L6400DW series CUPS",
				ArchReadiness: ArchReadiness{
					Status:          "universal",
					Label:           "Universal (Apple Silicon Ready)",
					RosettaRequired: false,
					BinaryCount:     1,
				},
			},
			{
				Name:  "zebra-zpl.ppd",
				Make:  "Zebra",
				Model: "Zebra ZPL Label Printer",
				ArchReadiness: ArchReadiness{
					Status:          "driverless",
					Label:           "Driverless (AirPrint / IPP)",
					RosettaRequired: false,
					BinaryCount:     0,
				},
			},
			{
				Name:  "everywhere",
				Make:  "Generic",
				Model: "Driverless IPP Everywhere / AirPrint",
				ArchReadiness: ArchReadiness{
					Status:          "driverless",
					Label:           "Driverless (AirPrint / IPP)",
					RosettaRequired: false,
					BinaryCount:     0,
				},
			},
		},
	}

	p1 := &Printer{
		Name:            "Office_LaserJet_Pro",
		URI:             "ipp://192.168.1.50/ipp/print",
		Info:            "HP Color LaserJet Pro MFP M479fdw",
		Location:        "2nd Floor Main Office",
		State:           "idle",
		StateMessage:    "Ready to print",
		IsDefault:       true,
		IsShared:        true,
		IsClass:         false,
		AcceptingJobs:   true,
		QueuedJobsCount: 1,
		Alerts:          []string{"toner-low-warning"},
		ArchReadiness: &ArchReadiness{
			Status:          "universal",
			Label:           "Universal (Apple Silicon Ready)",
			RosettaRequired: false,
			BinaryCount:     2,
		},
	}

	p2 := &Printer{
		Name:            "Warehouse_Zebra_Labels",
		URI:             "socket://192.168.1.88:9100",
		Info:            "Zebra ZT410 Industrial Label Printer",
		Location:        "Shipping Bay 2",
		State:           "printing",
		StateMessage:    "Printing job 103",
		IsDefault:       false,
		IsShared:        false,
		IsClass:         false,
		AcceptingJobs:   true,
		QueuedJobsCount: 2,
		Alerts:          nil,
		ArchReadiness: &ArchReadiness{
			Status:          "driverless",
			Label:           "Driverless (AirPrint / IPP)",
			RosettaRequired: false,
			BinaryCount:     0,
		},
	}

	p3 := &Printer{
		Name:            "Finance_Secure_Copier",
		URI:             "ipp://192.168.1.65/ipp/print",
		Info:            "Canon imageRUNNER ADVANCE DX",
		Location:        "Finance Suite 300",
		State:           "stopped",
		StateMessage:    "Stopped - Filter failed (Driver crash)",
		IsDefault:       false,
		IsShared:        false,
		IsClass:         false,
		AcceptingJobs:   false,
		QueuedJobsCount: 1,
		Alerts:          []string{"media-empty-report", "filter-failed"},
		ArchReadiness: &ArchReadiness{
			Status:          "rosetta_dependent",
			Label:           "Intel Only (Rosetta 2)",
			RosettaRequired: true,
			BinaryCount:     1,
		},
	}

	m.printers[p1.Name] = p1
	m.printers[p2.Name] = p2
	m.printers[p3.Name] = p3

	m.jobs = []Job{
		{
			ID:          "101",
			Printer:     "Finance_Secure_Copier",
			Title:       "FY26_Confidential_Audit_Report.pdf",
			User:        "jsmith",
			State:       "processing",
			StateReason: "job-printing",
			SizeBytes:   4582912,
			SubmittedAt: time.Now().Add(-22 * time.Minute),
			RawDate:     time.Now().Add(-22 * time.Minute).Format("Mon Jan 02 15:04:05 2006"),
			IsStuck:     true, // Stuck job demonstration
		},
		{
			ID:          "102",
			Printer:     "Office_LaserJet_Pro",
			Title:       "Customer_Onboarding_Packet.docx",
			User:        "alice",
			State:       "pending",
			SizeBytes:   512000,
			SubmittedAt: time.Now().Add(-3 * time.Minute),
			RawDate:     time.Now().Add(-3 * time.Minute).Format("Mon Jan 02 15:04:05 2006"),
			IsStuck:     false,
		},
		{
			ID:          "103",
			Printer:     "Warehouse_Zebra_Labels",
			Title:       "Shipping_Labels_Batch_8821.zpl",
			User:        "bob_shipping",
			State:       "processing",
			SizeBytes:   65536,
			SubmittedAt: time.Now().Add(-1 * time.Minute),
			RawDate:     time.Now().Add(-1 * time.Minute).Format("Mon Jan 02 15:04:05 2006"),
			IsStuck:     false,
		},
		{
			ID:          "99",
			Printer:     "Office_LaserJet_Pro",
			Title:       "Team_Standup_Agenda.pdf",
			User:        "jsmith",
			State:       "completed",
			SizeBytes:   128400,
			SubmittedAt: time.Now().Add(-45 * time.Minute),
			RawDate:     time.Now().Add(-45 * time.Minute).Format("Mon Jan 02 15:04:05 2006"),
			IsStuck:     false,
		},
	}

	// Mock Options & Accessories for Office_LaserJet_Pro
	m.options["Office_LaserJet_Pro"] = &PrinterOptionsResponse{
		PrinterName: "Office_LaserJet_Pro",
		General: []PrinterOption{
			{
				Key:          "PageSize",
				DisplayName:  "Media Size",
				DefaultValue: "Letter",
				CurrentValue: "Letter",
				Group:        "general",
				Choices: []OptionChoice{
					{Name: "Letter", Label: "Letter (8.5 x 11 in)", IsDefault: true},
					{Name: "A4", Label: "A4 (210 x 297 mm)", IsDefault: false},
					{Name: "Legal", Label: "Legal (8.5 x 14 in)", IsDefault: false},
					{Name: "Executive", Label: "Executive (7.25 x 10.5 in)", IsDefault: false},
				},
			},
			{
				Key:          "Duplex",
				DisplayName:  "2-Sided Printing",
				DefaultValue: "DuplexNoTumble",
				CurrentValue: "DuplexNoTumble",
				Group:        "general",
				Choices: []OptionChoice{
					{Name: "None", Label: "Off (1-Sided)", IsDefault: false},
					{Name: "DuplexNoTumble", Label: "2-Sided (Long-Edge)", IsDefault: true},
					{Name: "DuplexTumble", Label: "2-Sided (Short-Edge)", IsDefault: false},
				},
			},
			{
				Key:          "ColorModel",
				DisplayName:  "Color Mode",
				DefaultValue: "Color",
				CurrentValue: "Color",
				Group:        "general",
				Choices: []OptionChoice{
					{Name: "Color", Label: "Color", IsDefault: true},
					{Name: "Gray", Label: "Grayscale / Mono", IsDefault: false},
				},
			},
			{
				Key:          "Resolution",
				DisplayName:  "Print Resolution",
				DefaultValue: "600dpi",
				CurrentValue: "600dpi",
				Group:        "general",
				Choices: []OptionChoice{
					{Name: "300dpi", Label: "300 DPI (Draft)", IsDefault: false},
					{Name: "600dpi", Label: "600 DPI (Standard)", IsDefault: true},
					{Name: "1200dpi", Label: "1200 DPI (High Quality)", IsDefault: false},
				},
			},
			{
				Key:          "InputSlot",
				DisplayName:  "Paper Source",
				DefaultValue: "Auto",
				CurrentValue: "Auto",
				Group:        "general",
				Choices: []OptionChoice{
					{Name: "Auto", Label: "Auto Select", IsDefault: true},
					{Name: "Tray1", Label: "Tray 1 (Multipurpose)", IsDefault: false},
					{Name: "Tray2", Label: "Tray 2 (Main Cassette)", IsDefault: false},
				},
			},
		},
		Accessories: []PrinterOption{
			{
				Key:          "Option1",
				DisplayName:  "Optional 550-Sheet Paper Feeder (Tray 3)",
				DefaultValue: "True",
				CurrentValue: "True",
				Group:        "accessories",
				Choices: []OptionChoice{
					{Name: "False", Label: "Not Installed", IsDefault: false},
					{Name: "True", Label: "Installed", IsDefault: true},
				},
			},
			{
				Key:          "Duplexer",
				DisplayName:  "Hardware Duplex Unit",
				DefaultValue: "True",
				CurrentValue: "True",
				Group:        "accessories",
				Choices: []OptionChoice{
					{Name: "False", Label: "Not Installed", IsDefault: false},
					{Name: "True", Label: "Installed", IsDefault: true},
				},
			},
		},
	}

	// Mock Options & Accessories for Finance_Secure_Copier
	m.options["Finance_Secure_Copier"] = &PrinterOptionsResponse{
		PrinterName: "Finance_Secure_Copier",
		General: []PrinterOption{
			{
				Key:          "PageSize",
				DisplayName:  "Media Size",
				DefaultValue: "A4",
				CurrentValue: "A4",
				Group:        "general",
				Choices: []OptionChoice{
					{Name: "A4", Label: "A4 (210 x 297 mm)", IsDefault: true},
					{Name: "A3", Label: "A3 (297 x 420 mm)", IsDefault: false},
					{Name: "Letter", Label: "Letter (8.5 x 11 in)", IsDefault: false},
					{Name: "11x17", Label: "Tabloid / Ledger (11 x 17 in)", IsDefault: false},
				},
			},
			{
				Key:          "Duplex",
				DisplayName:  "2-Sided Printing",
				DefaultValue: "DuplexNoTumble",
				CurrentValue: "DuplexNoTumble",
				Group:        "general",
				Choices: []OptionChoice{
					{Name: "None", Label: "Off (1-Sided)", IsDefault: false},
					{Name: "DuplexNoTumble", Label: "2-Sided (Long-Edge)", IsDefault: true},
					{Name: "DuplexTumble", Label: "2-Sided (Short-Edge)", IsDefault: false},
				},
			},
			{
				Key:          "ColorModel",
				DisplayName:  "Color Mode",
				DefaultValue: "Gray",
				CurrentValue: "Gray",
				Group:        "general",
				Choices: []OptionChoice{
					{Name: "Gray", Label: "Grayscale / Mono", IsDefault: true},
					{Name: "Color", Label: "Color", IsDefault: false},
				},
			},
			{
				Key:          "Resolution",
				DisplayName:  "Print Resolution",
				DefaultValue: "600dpi",
				CurrentValue: "600dpi",
				Group:        "general",
				Choices: []OptionChoice{
					{Name: "600dpi", Label: "600 DPI", IsDefault: true},
					{Name: "1200dpi", Label: "1200 DPI", IsDefault: false},
				},
			},
		},
		Accessories: []PrinterOption{
			{
				Key:          "OptionCassette",
				DisplayName:  "High-Capacity Paper Deck Unit-E1 (3,500 sheets)",
				DefaultValue: "True",
				CurrentValue: "True",
				Group:        "accessories",
				Choices: []OptionChoice{
					{Name: "False", Label: "Not Installed", IsDefault: false},
					{Name: "True", Label: "Installed", IsDefault: true},
				},
			},
			{
				Key:          "Finisher",
				DisplayName:  "Output Finisher",
				DefaultValue: "Stapler",
				CurrentValue: "Stapler",
				Group:        "accessories",
				Choices: []OptionChoice{
					{Name: "None", Label: "Inner 2-Way Tray", IsDefault: false},
					{Name: "Stapler", Label: "Staple Finisher-AC1", IsDefault: true},
					{Name: "BookletMaker", Label: "Booklet Finisher-AC1", IsDefault: false},
				},
			},
			{
				Key:          "PuncherUnit",
				DisplayName:  "2/3 Hole Puncher Unit-A1",
				DefaultValue: "True",
				CurrentValue: "True",
				Group:        "accessories",
				Choices: []OptionChoice{
					{Name: "False", Label: "Not Installed", IsDefault: false},
					{Name: "True", Label: "Installed", IsDefault: true},
				},
			},
			{
				Key:          "HardDisk",
				DisplayName:  "Encrypted Internal Hard Disk",
				DefaultValue: "True",
				CurrentValue: "True",
				Group:        "accessories",
				Choices: []OptionChoice{
					{Name: "False", Label: "Not Installed", IsDefault: false},
					{Name: "True", Label: "Installed", IsDefault: true},
				},
			},
		},
	}

	// Mock Options & Accessories for Warehouse_Zebra_Labels
	m.options["Warehouse_Zebra_Labels"] = &PrinterOptionsResponse{
		PrinterName: "Warehouse_Zebra_Labels",
		General: []PrinterOption{
			{
				Key:          "PageSize",
				DisplayName:  "Label Size",
				DefaultValue: "4x6",
				CurrentValue: "4x6",
				Group:        "general",
				Choices: []OptionChoice{
					{Name: "4x6", Label: "4 x 6 inches (Standard Shipping)", IsDefault: true},
					{Name: "4x8", Label: "4 x 8 inches (Pallet / Freight)", IsDefault: false},
					{Name: "2x1", Label: "2 x 1 inches (Barcode / Asset)", IsDefault: false},
					{Name: "3x5", Label: "3 x 5 inches", IsDefault: false},
				},
			},
			{
				Key:          "PrintSpeed",
				DisplayName:  "Print Speed",
				DefaultValue: "6ips",
				CurrentValue: "6ips",
				Group:        "general",
				Choices: []OptionChoice{
					{Name: "2ips", Label: "2.0 in/sec", IsDefault: false},
					{Name: "4ips", Label: "4.0 in/sec", IsDefault: false},
					{Name: "6ips", Label: "6.0 in/sec", IsDefault: true},
					{Name: "8ips", Label: "8.0 in/sec (High Speed)", IsDefault: false},
				},
			},
			{
				Key:          "Darkness",
				DisplayName:  "Thermal Printhead Darkness",
				DefaultValue: "15",
				CurrentValue: "15",
				Group:        "general",
				Choices: []OptionChoice{
					{Name: "10", Label: "10 (Light)", IsDefault: false},
					{Name: "15", Label: "15 (Optimal Direct Thermal)", IsDefault: true},
					{Name: "20", Label: "20 (Thermal Transfer)", IsDefault: false},
					{Name: "25", Label: "25 (Heavy Poly)", IsDefault: false},
				},
			},
		},
		Accessories: []PrinterOption{
			{
				Key:          "Peeler",
				DisplayName:  "Label Peeler with Liner Take-Up",
				DefaultValue: "True",
				CurrentValue: "True",
				Group:        "accessories",
				Choices: []OptionChoice{
					{Name: "False", Label: "Disabled / Not Installed", IsDefault: false},
					{Name: "True", Label: "Installed & Active", IsDefault: true},
				},
			},
			{
				Key:          "Cutter",
				DisplayName:  "Heavy Duty Rotary Cutter",
				DefaultValue: "False",
				CurrentValue: "False",
				Group:        "accessories",
				Choices: []OptionChoice{
					{Name: "False", Label: "Not Installed", IsDefault: true},
					{Name: "True", Label: "Installed", IsDefault: false},
				},
			},
		},
	}

	return m
}

func (m *MockAdapter) GetPrinters() ([]Printer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]Printer, 0, len(m.printers))
	for _, p := range m.printers {
		cp := *p
		if cp.State == "stopped" || cp.State == "error" || cp.StateMessage != "" {
			cp.Diagnostic = ClassifyError(cp.StateMessage)
		}
		result = append(result, cp)
	}

	sort.Slice(result, func(i, j int) bool {
		// Always list printers alphabetically, case-insensitive
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	return result, nil
}

func (m *MockAdapter) GetPrinter(name string) (*Printer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, ok := m.printers[name]
	if !ok {
		return nil, fmt.Errorf("printer %q not found", name)
	}
	copy := *p
	if copy.State == "stopped" || copy.State == "error" || copy.StateMessage != "" {
		copy.Diagnostic = ClassifyError(copy.StateMessage)
	}
	return &copy, nil
}

func (m *MockAdapter) GetJobs(which string, printerFilter string) ([]Job, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []Job
	for _, j := range m.jobs {
		if printerFilter != "" && j.Printer != printerFilter {
			continue
		}
		if which == "active" && (j.State != "pending" && j.State != "processing" && j.State != "held") {
			continue
		}
		if which == "completed" && (j.State != "completed" && j.State != "cancelled") {
			continue
		}
		jobCopy := j
		if jobCopy.StateReason != "" {
			jobCopy.Diagnostic = ClassifyError(jobCopy.StateReason)
		}
		if jobCopy.IsStuck && jobCopy.Diagnostic == nil {
			jobCopy.Diagnostic = &DiagnosticInfo{
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
		result = append(result, jobCopy)
	}
	return result, nil
}

func (m *MockAdapter) CancelJob(jobID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	found := false
	for i := range m.jobs {
		if m.jobs[i].ID == jobID {
			m.jobs[i].State = "cancelled"
			m.jobs[i].IsStuck = false
			// Decrement queued count
			if p, ok := m.printers[m.jobs[i].Printer]; ok && p.QueuedJobsCount > 0 {
				p.QueuedJobsCount--
			}
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("job %s not found", jobID)
	}
	return nil
}

// HoldJob holds a job indefinitely.
func (m *MockAdapter) HoldJob(jobID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.jobs {
		if m.jobs[i].ID == jobID {
			if m.jobs[i].State == "completed" || m.jobs[i].State == "cancelled" {
				return fmt.Errorf("cannot hold job %s in state %s", jobID, m.jobs[i].State)
			}
			m.jobs[i].State = "held"
			m.jobs[i].StateReason = "job-held-by-user"
			return nil
		}
	}
	return fmt.Errorf("job %s not found", jobID)
}

// ReleaseJob releases a held job.
func (m *MockAdapter) ReleaseJob(jobID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.jobs {
		if m.jobs[i].ID == jobID {
			if m.jobs[i].State != "held" {
				return fmt.Errorf("job %s is not held (state: %s)", jobID, m.jobs[i].State)
			}
			m.jobs[i].State = "pending"
			m.jobs[i].StateReason = ""
			return nil
		}
	}
	return fmt.Errorf("job %s not found", jobID)
}

// RestartJob restarts a finished or cancelled job.
func (m *MockAdapter) RestartJob(jobID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.jobs {
		if m.jobs[i].ID == jobID {
			m.jobs[i].State = "pending"
			m.jobs[i].StateReason = ""
			m.jobs[i].SubmittedAt = time.Now()
			m.jobs[i].RawDate = time.Now().Format("Mon Jan 02 15:04:05 2006")
			m.jobs[i].IsStuck = false
			if p, ok := m.printers[m.jobs[i].Printer]; ok {
				p.QueuedJobsCount++
			}
			return nil
		}
	}
	return fmt.Errorf("job %s not found", jobID)
}

// PurgeQueue cancels all pending and active jobs for a printer.
func (m *MockAdapter) PurgeQueue(printerName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.printers[printerName]
	if !ok {
		return fmt.Errorf("printer %q not found", printerName)
	}

	for i := range m.jobs {
		if m.jobs[i].Printer == printerName {
			if m.jobs[i].State == "pending" || m.jobs[i].State == "processing" || m.jobs[i].State == "held" {
				m.jobs[i].State = "cancelled"
				m.jobs[i].IsStuck = false
			}
		}
	}
	p.QueuedJobsCount = 0
	return nil
}

// DeletePrinter deletes a printer queue from the mock store.
func (m *MockAdapter) DeletePrinter(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.printers[name]; !ok {
		return fmt.Errorf("printer %q not found", name)
	}

	delete(m.printers, name)
	delete(m.options, name)

	// Cancel any active jobs for this printer
	for i := range m.jobs {
		if m.jobs[i].Printer == name && (m.jobs[i].State == "pending" || m.jobs[i].State == "processing" || m.jobs[i].State == "held") {
			m.jobs[i].State = "cancelled"
			m.jobs[i].IsStuck = false
		}
	}

	// Like the live adapter: a new queue with the same name must not inherit
	// manual accessory notes (mock queue UUIDs are derived from the name).
	_ = m.accessories().ForgetQueue(name)

	return nil
}

func (m *MockAdapter) PausePrinter(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.printers[name]
	if !ok {
		return fmt.Errorf("printer %q not found", name)
	}
	p.State = "stopped"
	p.StateMessage = "Paused by administrator"
	return nil
}

func (m *MockAdapter) ResumePrinter(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.printers[name]
	if !ok {
		return fmt.Errorf("printer %q not found", name)
	}
	p.State = "idle"
	p.StateMessage = "Ready to print"
	return nil
}

func (m *MockAdapter) SetDefaultPrinter(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.printers[name]; !ok {
		return fmt.Errorf("printer %q not found", name)
	}

	for _, p := range m.printers {
		p.IsDefault = (p.Name == name)
	}
	m.defaultPrint = name
	return nil
}

func (m *MockAdapter) GetSystemStatus() (*SystemStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	activeCount := 0
	for _, j := range m.jobs {
		if j.State == "pending" || j.State == "processing" {
			activeCount++
		}
	}

	return &SystemStatus{
		DaemonRunning:       true,
		CUPSVersion:         "2.4.2 (Mock Mode)",
		WebInterfaceEnabled: m.webInterface,
		DefaultPrinter:      m.defaultPrint,
		ServerTime:          time.Now(),
		PrintersCount:       len(m.printers),
		ActiveJobsCount:     activeCount,
		ServerAddress:       "127.0.0.1:6310",
	}, nil
}

func (m *MockAdapter) GetPrinterOptions(name string) (*PrinterOptionsResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	opts, ok := m.options[name]
	if !ok {
		return &PrinterOptionsResponse{
			PrinterName: name,
			General:     []PrinterOption{},
			Accessories: []PrinterOption{},
		}, nil
	}

	// Return a copy so callers don't mutate concurrently
	copyResp := &PrinterOptionsResponse{
		PrinterName: opts.PrinterName,
		General:     make([]PrinterOption, len(opts.General)),
		Accessories: make([]PrinterOption, len(opts.Accessories)),
	}
	copy(copyResp.General, opts.General)
	copy(copyResp.Accessories, opts.Accessories)

	return copyResp, nil
}

func (m *MockAdapter) SetPrinterOptions(name string, options map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	opts, ok := m.options[name]
	if !ok {
		return fmt.Errorf("printer %q not found", name)
	}

	for k, v := range options {
		for i := range opts.General {
			if opts.General[i].Key == k {
				opts.General[i].CurrentValue = v
			}
		}
		for i := range opts.Accessories {
			if opts.Accessories[i].Key == k {
				opts.Accessories[i].CurrentValue = v
			}
		}
	}

	return nil
}

func (m *MockAdapter) GetPrinterAudit(name string) (*ArchAuditResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	switch name {
	case "Office_LaserJet_Pro":
		return &ArchAuditResult{
			PrinterName:     "Office_LaserJet_Pro",
			PPDPath:         "/private/etc/cups/ppd/Office_LaserJet_Pro.ppd",
			DriverModel:     "HP Color LaserJet Pro MFP M479fdw PostScript",
			Status:          "universal",
			Label:           "Universal (Apple Silicon Ready)",
			RosettaRequired: false,
			Binaries: []BinaryInfo{
				{
					Path:            "/usr/libexec/cups/filter/pstops",
					Name:            "pstops",
					Type:            "filter",
					Exists:          true,
					SizeBytes:       143280,
					Slices:          []string{"arm64e", "x86_64"},
					IsUniversal:     true,
					IsNativeApple:   true,
					RosettaRequired: false,
					Recommendation:  "Universal Mach-O binary containing native Apple Silicon (arm64e) and Intel slices.",
				},
				{
					Path:            "/Library/Printers/hp/filter/rastertohp",
					Name:            "rastertohp",
					Type:            "filter",
					Exists:          true,
					SizeBytes:       189440,
					Slices:          []string{"arm64", "x86_64"},
					IsUniversal:     true,
					IsNativeApple:   true,
					RosettaRequired: false,
					Recommendation:  "Universal Mach-O binary containing native Apple Silicon (arm64) and Intel slices.",
				},
			},
			MigrationAdvice: "All filter and backend executables include native Apple Silicon (arm64) slices. This queue will continue operating seamlessly in a world without Rosetta 2.",
		}, nil

	case "Finance_Secure_Copier":
		return &ArchAuditResult{
			PrinterName:     "Finance_Secure_Copier",
			PPDPath:         "/private/etc/cups/ppd/Finance_Secure_Copier.ppd",
			DriverModel:     "Canon imageRUNNER ADVANCE DX C5840i UFR II",
			Status:          "rosetta_dependent",
			Label:           "Intel Only (Rosetta 2)",
			RosettaRequired: true,
			Binaries: []BinaryInfo{
				{
					Path:            "/Library/Printers/Canon/CUPS_MF_Printer/Bins/captdfilter",
					Name:            "captdfilter",
					Type:            "filter",
					Exists:          true,
					SizeBytes:       425984,
					Slices:          []string{"x86_64"},
					IsUniversal:     false,
					IsNativeApple:   false,
					RosettaRequired: true,
					Recommendation:  "Intel x86_64 binary running via Rosetta 2 dynamic translation. Update driver to universal version before Rosetta 2 is discontinued.",
				},
				{
					Path:            "/Library/Printers/Canon/CUPS_MF_Printer/Bins/canon_ufr2_raster",
					Name:            "canon_ufr2_raster",
					Type:            "filter",
					Exists:          true,
					SizeBytes:       512000,
					Slices:          []string{"x86_64"},
					IsUniversal:     false,
					IsNativeApple:   false,
					RosettaRequired: true,
					Recommendation:  "Intel x86_64 binary running via Rosetta 2 dynamic translation. Update driver to universal version before Rosetta 2 is discontinued.",
				},
			},
			MigrationAdvice: "This printer queue relies on Intel x86_64 driver binaries running through Rosetta 2 translation. When Apple removes Rosetta 2 in a future macOS release, this queue will fail. Recommendation: Obtain updated Apple Silicon drivers from Canon or reconfigure this queue with Driverless IPP Everywhere.",
		}, nil

	case "Warehouse_Zebra_Labels":
		return &ArchAuditResult{
			PrinterName:     "Warehouse_Zebra_Labels",
			PPDPath:         "",
			DriverModel:     "Driverless IPP Everywhere / System Rasterizer",
			Status:          "driverless",
			Label:           "Driverless (AirPrint / IPP)",
			RosettaRequired: false,
			Binaries:        []BinaryInfo{},
			MigrationAdvice: "Queue uses driverless IPP Everywhere / AirPrint with native system filters. Zero vendor binaries are executed, making this queue 100% future-proof.",
		}, nil

	default:
		return &ArchAuditResult{
			PrinterName:     name,
			Status:          "driverless",
			Label:           "Driverless (AirPrint / IPP)",
			RosettaRequired: false,
			Binaries:        []BinaryInfo{},
			MigrationAdvice: "Standard driverless queue.",
		}, nil
	}
}

func (m *MockAdapter) GetFleetAudit() (*FleetAuditSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	total := len(m.printers)
	universal := 0
	rosetta := 0
	driverless := 0

	for _, p := range m.printers {
		if p.ArchReadiness != nil {
			switch p.ArchReadiness.Status {
			case "universal":
				universal++
			case "rosetta_dependent":
				rosetta++
			case "driverless":
				driverless++
			}
		}
	}

	return &FleetAuditSummary{
		TotalQueues:      total,
		UniversalQueues:  universal,
		RosettaQueues:    rosetta,
		DriverlessQueues: driverless,
		BrokenQueues:     0,
	}, nil
}

// AddPrinter creates a new printer in the mock store.
func (m *MockAdapter) AddPrinter(params AddPrinterParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.printers[params.Name]; exists {
		return fmt.Errorf("printer %q already exists", params.Name)
	}

	info := params.Description
	if info == "" {
		info = params.Name
	}

	status := "universal"
	label := "Universal (Apple Silicon Ready)"
	rosetta := false
	binCount := 2

	if strings.Contains(strings.ToLower(params.Model), "everywhere") || strings.Contains(strings.ToLower(params.URI), "null") {
		status = "driverless"
		label = "Driverless (AirPrint / IPP)"
		rosetta = false
		binCount = 0
	} else if strings.Contains(strings.ToLower(params.Model), "canon") || strings.Contains(strings.ToLower(params.Make), "canon") {
		status = "rosetta_dependent"
		label = "Intel Only (Rosetta 2)"
		rosetta = true
		binCount = 1
	}

	p := &Printer{
		Name:            params.Name,
		URI:             params.URI,
		Info:            info,
		Location:        params.Location,
		State:           "idle",
		StateMessage:    "Ready to print",
		IsDefault:       false,
		IsShared:        params.Shared,
		IsClass:         false,
		AcceptingJobs:   true,
		QueuedJobsCount: 0,
		ArchReadiness: &ArchReadiness{
			Status:          status,
			Label:           label,
			RosettaRequired: rosetta,
			BinaryCount:     binCount,
		},
	}

	m.printers[params.Name] = p

	// Seed standard options so configuration drawer works immediately
	m.options[params.Name] = &PrinterOptionsResponse{
		General: []PrinterOption{
			{
				Key:          "PageSize",
				DisplayName:  "Media Size",
				DefaultValue: "Letter",
				CurrentValue: "Letter",
				Choices: []OptionChoice{
					{Name: "Letter", Label: "US Letter (8.5 x 11 in)", IsDefault: true},
					{Name: "A4", Label: "A4 (210 x 297 mm)", IsDefault: false},
					{Name: "Legal", Label: "US Legal (8.5 x 14 in)", IsDefault: false},
				},
				Group: "general",
			},
			{
				Key:          "Duplex",
				DisplayName:  "2-Sided Printing",
				DefaultValue: "DuplexNoTumble",
				CurrentValue: "DuplexNoTumble",
				Choices: []OptionChoice{
					{Name: "None", Label: "Off (1-Sided)", IsDefault: false},
					{Name: "DuplexNoTumble", Label: "2-Sided (Long-Edge)", IsDefault: true},
					{Name: "DuplexTumble", Label: "2-Sided (Short-Edge)", IsDefault: false},
				},
				Group: "general",
			},
			{
				Key:          "ColorModel",
				DisplayName:  "Color Mode",
				DefaultValue: "Color",
				CurrentValue: "Color",
				Choices: []OptionChoice{
					{Name: "Color", Label: "Color", IsDefault: true},
					{Name: "Gray", Label: "Grayscale / Mono", IsDefault: false},
				},
				Group: "general",
			},
		},
		Accessories: []PrinterOption{
			{
				Key:          "OptionTray3",
				DisplayName:  "Optional 550-Sheet Paper Feeder (Tray 3)",
				DefaultValue: "False",
				CurrentValue: "False",
				Choices: []OptionChoice{
					{Name: "False", Label: "Not Installed / Off", IsDefault: true},
					{Name: "True", Label: "Installed / On", IsDefault: false},
				},
				Group: "accessories",
			},
		},
	}

	return nil
}

// ModifyPrinter updates an existing printer's details.
func (m *MockAdapter) ModifyPrinter(name string, params ModifyPrinterParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, exists := m.printers[name]
	if !exists {
		return fmt.Errorf("printer %q not found", name)
	}

	if params.URI != "" {
		p.URI = params.URI
	}
	if params.Description != nil {
		p.Info = *params.Description
	}
	if params.Location != nil {
		p.Location = *params.Location
	}
	if params.Shared != nil {
		p.IsShared = *params.Shared
	}

	return nil
}

// SetPrinterDriver updates a printer's active driver/PPD.
func (m *MockAdapter) SetPrinterDriver(name string, modelOrPPD string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, exists := m.printers[name]
	if !exists {
		return fmt.Errorf("printer %q not found", name)
	}

	p.Info = modelOrPPD
	lower := strings.ToLower(modelOrPPD)
	if strings.Contains(lower, "canon") || strings.Contains(lower, "cncups") {
		p.ArchReadiness = &ArchReadiness{
			Status:          "rosetta_dependent",
			Label:           "Intel Only (Rosetta 2)",
			RosettaRequired: true,
			BinaryCount:     1,
		}
	} else if strings.Contains(lower, "everywhere") {
		p.ArchReadiness = &ArchReadiness{
			Status:          "driverless",
			Label:           "Driverless (AirPrint / IPP)",
			RosettaRequired: false,
			BinaryCount:     0,
		}
	} else {
		p.ArchReadiness = &ArchReadiness{
			Status:          "universal",
			Label:           "Universal (Apple Silicon Ready)",
			RosettaRequired: false,
			BinaryCount:     2,
		}
	}

	return nil
}

// PrintTestPage generates a mock test page print job.
func (m *MockAdapter) PrintTestPage(name string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, exists := m.printers[name]
	if !exists {
		return "", fmt.Errorf("printer %q not found", name)
	}

	jobID := fmt.Sprintf("%d", 300+len(m.jobs)+1)
	testJob := Job{
		ID:          name + "-" + jobID,
		Printer:     name,
		Title:       "CUPS Diagnostic Test Page",
		User:        "admin",
		SizeBytes:   24576,
		State:       "processing",
		StateReason: "job-printing",
		SubmittedAt: time.Now(),
		RawDate:     time.Now().Format("Mon Jan 2 15:04:05 2006"),
		IsStuck:     false,
	}

	m.jobs = append(m.jobs, testJob)
	p.QueuedJobsCount++

	return testJob.ID, nil
}

// GetDrivers returns the list of simulated installed drivers matching search.
func (m *MockAdapter) GetDrivers(search string) ([]DriverInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if search == "" {
		return m.mockDrivers, nil
	}

	q := strings.ToLower(search)
	var matches []DriverInfo
	for _, d := range m.mockDrivers {
		if strings.Contains(strings.ToLower(d.Make), q) ||
			strings.Contains(strings.ToLower(d.Model), q) ||
			strings.Contains(strings.ToLower(d.Name), q) {
			matches = append(matches, d)
		}
	}
	return matches, nil
}

// ValidatePPD validates custom PPD file contents.
func (m *MockAdapter) ValidatePPD(content []byte) (*PPDValidationResult, error) {
	if len(content) == 0 {
		return &PPDValidationResult{
			Valid:  false,
			Errors: []string{"Uploaded PPD file is empty"},
		}, nil
	}

	if !bytes.Contains(content, []byte("*PPD-Adobe")) {
		return &PPDValidationResult{
			Valid:  false,
			Errors: []string{"File does not conform to Adobe PPD Specification: missing *PPD-Adobe header line."},
		}, nil
	}

	// Extract ModelName line if present
	model := "Custom PostScript Printer"
	lines := bytes.Split(content, []byte("\n"))
	for _, line := range lines {
		trimmed := strings.TrimSpace(string(line))
		if strings.HasPrefix(trimmed, "*ModelName:") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				model = strings.Trim(strings.TrimSpace(parts[1]), "\"")
			}
			break
		}
	}

	return &PPDValidationResult{
		Valid:    true,
		Model:    model,
		Warnings: []string{},
		Errors:   []string{},
	}, nil
}

// GetServerSettings returns global CUPS server settings.
func (m *MockAdapter) GetServerSettings() (*ServerSettings, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cpy := m.settings
	return &cpy, nil
}

// SetServerSettings modifies global CUPS server settings.
func (m *MockAdapter) SetServerSettings(settings ServerSettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings = settings
	return nil
}

// GetLogs returns simulated log entries filtered by level.
func (m *MockAdapter) GetLogs(lines int, level string) ([]LogEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var filtered []LogEntry
	for _, entry := range m.logs {
		if matchesLevel(entry.Level, level) {
			filtered = append(filtered, entry)
		}
	}

	if lines > 0 && len(filtered) > lines {
		filtered = filtered[len(filtered)-lines:]
	}

	return filtered, nil
}

// GetLaunchAgentStatus returns simulated macOS LaunchAgent status.
func (m *MockAdapter) GetLaunchAgentStatus() (*LaunchAgentStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	detail := "Not installed"
	if m.launchAgentInstalled {
		detail = "Installed and running under launchd (mock)"
	}

	return &LaunchAgentStatus{
		Supported:    true,
		Installed:    m.launchAgentInstalled,
		Running:      m.launchAgentInstalled,
		PlistPath:    "/Users/mockuser/Library/LaunchAgents/com.papercut.cups-webui.plist",
		Executable:   "/usr/local/bin/cups-webui",
		StatusDetail: detail,
	}, nil
}

// ManageLaunchAgent handles simulated install/uninstall of LaunchAgent.
func (m *MockAdapter) ManageLaunchAgent(action string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch strings.ToLower(action) {
	case "install":
		m.launchAgentInstalled = true
		return nil
	case "uninstall":
		m.launchAgentInstalled = false
		return nil
	default:
		return fmt.Errorf("unknown action: %q", action)
	}
}
