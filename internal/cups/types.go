package cups

import "time"

// Printer represents a configured CUPS printer or class.
type Printer struct {
	Name            string   `json:"name"`
	URI             string   `json:"uri"`
	Info            string   `json:"info"`
	Location        string   `json:"location"`
	State           string   `json:"state"` // "idle", "printing", "stopped", "error", "unknown"
	StateMessage    string   `json:"state_message"`
	IsDefault       bool     `json:"is_default"`
	IsShared        bool     `json:"is_shared"`
	IsClass         bool            `json:"is_class"`
	AcceptingJobs   bool            `json:"accepting_jobs"`
	QueuedJobsCount int             `json:"queued_jobs_count"`
	Alerts          []string        `json:"alerts,omitempty"`
	ArchReadiness   *ArchReadiness  `json:"arch_readiness,omitempty"`
	Diagnostic      *DiagnosticInfo `json:"diagnostic,omitempty"`
}

// DiagnosticInfo provides plain-English translation and actionable remedies for CUPS errors.
type DiagnosticInfo struct {
	Code        string   `json:"code"`
	Title       string   `json:"title"`
	Explanation string   `json:"explanation"`
	Remedies    []string `json:"remedies"`
	Severity    string   `json:"severity"` // "error", "warning", "info"
}

// LogEntry represents a structured, parsed line from CUPS error_log or access_log.
type LogEntry struct {
	Timestamp  time.Time       `json:"timestamp"`
	RawDate    string          `json:"raw_date"`
	Level      string          `json:"level"` // "error", "warn", "info", "debug"
	Message    string          `json:"message"`
	Printer    string          `json:"printer,omitempty"`
	JobID      string          `json:"job_id,omitempty"`
	Diagnostic *DiagnosticInfo `json:"diagnostic,omitempty"`
}

// LaunchAgentStatus describes the state of the macOS LaunchAgent.
type LaunchAgentStatus struct {
	Supported    bool   `json:"supported"` // true on darwin
	Installed    bool   `json:"installed"`
	Running      bool   `json:"running"`
	PlistPath    string `json:"plist_path"`
	Executable   string `json:"executable"`
	StatusDetail string `json:"status_detail"`
}

// ArchReadiness models the binary architecture support for macOS queues.
type ArchReadiness struct {
	Status          string `json:"status"` // "universal", "native_arm64", "rosetta_dependent", "driverless", "missing_binary"
	Label           string `json:"label"`
	DriverModel     string `json:"driver_model,omitempty"`
	PPDPath         string `json:"ppd_path,omitempty"`
	RosettaRequired bool   `json:"rosetta_required"`
	BinaryCount     int    `json:"binary_count"`
}

// Job represents a print job in CUPS.
type Job struct {
	ID          string          `json:"id"`
	Printer     string          `json:"printer"`
	Title       string          `json:"title"`
	User        string          `json:"user"`
	State       string          `json:"state"` // "pending", "processing", "held", "completed", "cancelled", "unknown"
	StateReason string          `json:"state_reason,omitempty"`
	SizeBytes   int64           `json:"size_bytes"`
	SubmittedAt time.Time       `json:"submitted_at"`
	RawDate     string          `json:"raw_date"`
	IsStuck     bool            `json:"is_stuck"`
	Diagnostic  *DiagnosticInfo `json:"diagnostic,omitempty"`
}

// SystemStatus represents general CUPS daemon health and configuration.
type SystemStatus struct {
	DaemonRunning       bool      `json:"daemon_running"`
	CUPSVersion         string    `json:"cups_version"`
	WebInterfaceEnabled bool      `json:"web_interface_enabled"`
	DefaultPrinter      string    `json:"default_printer"`
	ServerTime          time.Time `json:"server_time"`
	PrintersCount       int       `json:"printers_count"`
	ActiveJobsCount     int       `json:"active_jobs_count"`
	ServerAddress       string    `json:"server_address"`
}

// OptionChoice represents a selectable choice for a printer option.
type OptionChoice struct {
	Name      string `json:"name"`      // e.g. "A4" or "DuplexNoTumble"
	Label     string `json:"label"`     // e.g. "A4" or "Long-Edge Binding"
	IsDefault bool   `json:"is_default"` // true if marked with * in lpoptions
}

// PrinterOption represents a configurable PPD option.
type PrinterOption struct {
	Key          string         `json:"key"`          // e.g. "PageSize", "Duplex"
	DisplayName  string         `json:"display_name"` // e.g. "Media Size", "2-Sided Printing"
	DefaultValue string         `json:"default_value"`
	CurrentValue string         `json:"current_value"`
	Choices      []OptionChoice `json:"choices"`
	Group        string         `json:"group"` // "general", "accessories"
}

// PrinterOptionsResponse wraps options grouped for UI presentation.
type PrinterOptionsResponse struct {
	PrinterName string          `json:"printer_name"`
	General     []PrinterOption `json:"general"`
	Accessories []PrinterOption `json:"accessories"` // InstallableOptions
}

// BinaryInfo represents an inspected filter, backend, or rasterizer executable.
type BinaryInfo struct {
	Path            string   `json:"path"`
	Name            string   `json:"name"`
	Type            string   `json:"type"` // "filter", "backend", "pde"
	Exists          bool     `json:"exists"`
	SizeBytes       int64    `json:"size_bytes"`
	Slices          []string `json:"slices"` // e.g. ["arm64", "x86_64"] or ["x86_64"]
	IsUniversal     bool     `json:"is_universal"`
	IsNativeApple   bool     `json:"is_native_apple"`
	RosettaRequired bool     `json:"rosetta_required"`
	Recommendation  string   `json:"recommendation"`
}

// ArchAuditResult represents the detailed architecture audit for a printer.
type ArchAuditResult struct {
	PrinterName     string       `json:"printer_name"`
	PPDPath         string       `json:"ppd_path"`
	DriverModel     string       `json:"driver_model"`
	Status          string       `json:"status"` // "universal", "native_arm64", "rosetta_dependent", "driverless", "missing_binary"
	Label           string       `json:"label"`
	RosettaRequired bool         `json:"rosetta_required"`
	Binaries        []BinaryInfo `json:"binaries"`
	MigrationAdvice string       `json:"migration_advice"`
}

// FleetAuditSummary provides an overall summary of Rosetta 2 migration readiness across all queues.
type FleetAuditSummary struct {
	TotalQueues      int `json:"total_queues"`
	UniversalQueues  int `json:"universal_queues"`
	RosettaQueues    int `json:"rosetta_queues"`
	DriverlessQueues int `json:"driverless_queues"`
	BrokenQueues     int `json:"broken_queues"`
}

// AddPrinterParams holds fields for creating a new print queue.
type AddPrinterParams struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	URI         string `json:"uri"`
	Location    string `json:"location"`
	Shared      bool   `json:"shared"`
	Make        string `json:"make"`
	Model       string `json:"model"`
	PPDPath     string `json:"ppd_path"`
}

// ModifyPrinterParams holds fields for updating an existing print queue.
//
// Description, Location and Shared are pointers so that a field left out of the
// request (nil: leave unchanged) can be told apart from a field deliberately set
// to empty or false (clear it / stop sharing). An empty URI means "unchanged",
// because a queue cannot exist without a device URI.
type ModifyPrinterParams struct {
	Description *string `json:"description"`
	URI         string  `json:"uri"`
	Location    *string `json:"location"`
	Shared      *bool   `json:"shared"`
}

// DriverInfo describes an available printer driver/model.
type DriverInfo struct {
	Name          string        `json:"name"`  // e.g. "drv:///sample.drv/laserjet.ppd" or "everywhere"
	Make          string        `json:"make"`  // e.g. "HP" or "Generic"
	Model         string        `json:"model"` // Human-readable e.g. "HP Color LaserJet Pro M479fdw"
	ArchReadiness ArchReadiness `json:"arch_readiness"`
}

// PPDValidationResult holds syntax and directive check results from cupstestppd.
type PPDValidationResult struct {
	Valid    bool     `json:"valid"`
	Model    string   `json:"model"`
	Warnings []string `json:"warnings"`
	Errors   []string `json:"errors"`
}

// ServerSettings holds global CUPS configuration toggles managed via cupsctl.
type ServerSettings struct {
	SharePrinters  bool `json:"share_printers"`
	RemotePrinters bool `json:"remote_printers"`
	DebugLogging   bool `json:"debug_logging"`
}

// Adapter defines the interface for interacting with CUPS.
type Adapter interface {
	GetPrinters() ([]Printer, error)
	GetPrinter(name string) (*Printer, error)
	GetJobs(which string, printerFilter string) ([]Job, error)
	CancelJob(jobID string) error
	HoldJob(jobID string) error
	ReleaseJob(jobID string) error
	RestartJob(jobID string) error
	GetSystemStatus() (*SystemStatus, error)
	PausePrinter(name string) error
	ResumePrinter(name string) error
	SetDefaultPrinter(name string) error
	PurgeQueue(printerName string) error
	DeletePrinter(name string) error
	AddPrinter(params AddPrinterParams) error
	ModifyPrinter(name string, params ModifyPrinterParams) error
	SetPrinterDriver(name string, modelOrPPD string) error
	PrintTestPage(name string) (string, error)
	GetDrivers(search string) ([]DriverInfo, error)
	ValidatePPD(content []byte) (*PPDValidationResult, error)
	GetServerSettings() (*ServerSettings, error)
	SetServerSettings(settings ServerSettings) error
	GetPrinterOptions(name string) (*PrinterOptionsResponse, error)
	SetPrinterOptions(name string, options map[string]string) error
	GetPrinterAudit(name string) (*ArchAuditResult, error)
	GetFleetAudit() (*FleetAuditSummary, error)
	GetLogs(lines int, level string) ([]LogEntry, error)
	GetLaunchAgentStatus() (*LaunchAgentStatus, error)
	ManageLaunchAgent(action string) error

	// Manually set installable options (accessories).
	GetAccessories(name string) (*AccessoryReport, error)
	SetAccessories(name string, values map[string]string, acknowledged bool) (*AccessoryReport, error)
	ResetAccessories(name string) (*AccessoryReport, error)
	SuggestAccessories(name string) (*AccessorySuggestions, error)
}
