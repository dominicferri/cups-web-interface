package cups

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

// A printer list exported from a Windows print server, used to find each
// printer's own address when a queue points at the print server.
//
// Expected columns (header names are matched case-insensitively, in any
// order; other columns are ignored):
//
//	PrinterName  required  the share/printer name on the print server
//	IPAddress    either    the printer's IP address or host name
//	PortName     either    the TCP/IP port; used when IPAddress is empty or "N/A"
//	Model        optional
//	Driver       optional

const (
	maxInventoryBytes = 2 << 20
	maxInventoryRows  = 5000
	maxInventoryField = 256
)

// InventoryEntry is one printer from the list.
type InventoryEntry struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Model   string `json:"model,omitempty"`
	Driver  string `json:"driver,omitempty"`
	Port    string `json:"port,omitempty"`
}

// InventoryRowIssue is a row that was skipped, with its line in the file.
type InventoryRowIssue struct {
	Line   int    `json:"line"`
	Name   string `json:"name,omitempty"`
	Reason string `json:"reason"`
}

// InventoryStatus describes the stored list.
type InventoryStatus struct {
	Loaded     bool      `json:"loaded"`
	SourceName string    `json:"source_name,omitempty"`
	ImportedAt time.Time `json:"imported_at,omitzero"`
	Count      int       `json:"count"`
	Error      string    `json:"error,omitempty"`
}

// InventoryImportResult is what an import did.
type InventoryImportResult struct {
	InventoryStatus
	Rows    int                 `json:"rows"`
	Skipped []InventoryRowIssue `json:"skipped"`
	// Queues is the number of CUPS queues on this Mac; Matched is how many
	// of them found a printer in the list.
	Queues  int `json:"queues"`
	Matched int `json:"matched"`
}

// InventoryFormatError means the file can't be used at all.
type InventoryFormatError struct{ Msg string }

func (e *InventoryFormatError) Error() string { return e.Msg }

func formatErr(format string, a ...any) error {
	return &InventoryFormatError{Msg: fmt.Sprintf(format, a...)}
}

// decodeInventoryText returns the file as UTF-8. PowerShell writes UTF-8
// (with or without a byte order mark) or UTF-16 with a byte order mark.
func decodeInventoryText(data []byte) (string, error) {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		data = data[3:]
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}), bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		le := data[0] == 0xFF
		data = data[2:]
		if len(data)%2 != 0 {
			return "", formatErr("the file looks like UTF-16 but has an odd number of bytes; export it again")
		}
		u := make([]uint16, len(data)/2)
		for i := range u {
			if le {
				u[i] = uint16(data[2*i]) | uint16(data[2*i+1])<<8
			} else {
				u[i] = uint16(data[2*i])<<8 | uint16(data[2*i+1])
			}
		}
		return string(utf16.Decode(u)), nil
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return "", formatErr("the file isn't text (it has NUL bytes). Export it with Export-Csv")
	}
	if !utf8.Valid(data) {
		return "", formatErr("the file isn't UTF-8 or UTF-16 text. Export it with Export-Csv -Encoding UTF8")
	}
	return string(data), nil
}

var portIPSuffix = regexp.MustCompile(`_[0-9]+$`)

// addressFromPort gets an IP address from a Windows TCP/IP port name such as
// "10.0.0.5", "IP_10.0.0.5" or "10.0.0.5_1" (a second port to the same
// printer). Local ports (TS001, PORTPROMPT:, nul, USB001) give "".
func addressFromPort(port string) string {
	p := strings.TrimSpace(port)
	if f := strings.Fields(p); len(f) > 0 {
		p = f[0]
	}
	p = strings.TrimPrefix(strings.TrimPrefix(p, "IP_"), "ip_")
	if net.ParseIP(p) != nil {
		return p
	}
	if q := portIPSuffix.ReplaceAllString(p, ""); net.ParseIP(q) != nil {
		return q
	}
	return ""
}

func cleanField(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	if len(s) > maxInventoryField {
		s = s[:maxInventoryField]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return s
}

// ParsePrinterInventoryCSV reads a print server export. It returns the usable
// printers and the rows it skipped, or an error if the file can't be used.
func ParsePrinterInventoryCSV(data []byte) ([]InventoryEntry, []InventoryRowIssue, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil, formatErr("the file is empty")
	}
	if len(data) > maxInventoryBytes {
		return nil, nil, formatErr("the file is larger than %d MB", maxInventoryBytes>>20)
	}
	text, err := decodeInventoryText(data)
	if err != nil {
		return nil, nil, err
	}
	// Windows PowerShell 5 Export-Csv starts with "#TYPE ..." unless
	// -NoTypeInformation is given.
	firstLine := 1
	if strings.HasPrefix(text, "#TYPE") {
		_, text, _ = strings.Cut(text, "\n")
		firstLine = 2
	}

	r := csv.NewReader(strings.NewReader(text))
	r.FieldsPerRecord = -1 // checked per row below, so one bad row doesn't stop the import
	header, err := r.Read()
	if err != nil {
		return nil, nil, formatErr("can't read the header row: %v", err)
	}
	col := map[string]int{}
	for i, h := range header {
		key := strings.ToLower(strings.TrimSpace(h))
		if _, dup := col[key]; dup && key != "" {
			return nil, nil, formatErr("the header has the column %q twice", strings.TrimSpace(h))
		}
		col[key] = i
	}
	_, hasName := col["printername"]
	_, hasIP := col["ipaddress"]
	_, hasPort := col["portname"]
	var missing []string
	if !hasName {
		missing = append(missing, "PrinterName")
	}
	if !hasIP && !hasPort {
		missing = append(missing, "IPAddress or PortName")
	}
	if len(missing) > 0 {
		return nil, nil, formatErr("the header has no %s column. Found: %s. Expected a print server export with PrinterName, Model, Driver, PortName, IPAddress",
			strings.Join(missing, " column and no "), strings.Join(header, ", "))
	}
	get := func(rec []string, name string) string {
		if i, ok := col[name]; ok && i < len(rec) {
			return rec[i]
		}
		return ""
	}

	var entries []InventoryEntry
	var issues []InventoryRowIssue
	seen := map[string]int{}
	rows := 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				return nil, nil, formatErr("line %d isn't valid CSV: %v. Fix the file or export it again", pe.Line+firstLine-1, pe.Err)
			}
			return nil, nil, formatErr("can't read the file: %v", err)
		}
		line, _ := r.FieldPos(0)
		line += firstLine - 1
		if len(rec) == 1 && strings.TrimSpace(rec[0]) == "" {
			continue // blank line
		}
		rows++
		if rows > maxInventoryRows {
			return nil, nil, formatErr("the file has more than %d printers", maxInventoryRows)
		}
		if len(rec) != len(header) {
			issues = append(issues, InventoryRowIssue{Line: line, Reason: fmt.Sprintf("has %d fields, the header has %d", len(rec), len(header))})
			continue
		}
		e := InventoryEntry{
			Name:   cleanField(get(rec, "printername")),
			Model:  cleanField(get(rec, "model")),
			Driver: cleanField(get(rec, "driver")),
			Port:   cleanField(get(rec, "portname")),
		}
		if e.Name == "" {
			issues = append(issues, InventoryRowIssue{Line: line, Reason: "no PrinterName"})
			continue
		}
		if e.Model == "SNMP unavailable" {
			e.Model = ""
		}
		ip := cleanField(get(rec, "ipaddress"))
		switch {
		case ip != "" && !strings.EqualFold(ip, "N/A") && IsValidSNMPHost(ip):
			e.Address = ip
		case ip != "" && !strings.EqualFold(ip, "N/A"):
			issues = append(issues, InventoryRowIssue{Line: line, Name: e.Name, Reason: fmt.Sprintf("IPAddress %q isn't an IP address or host name", ip)})
			continue
		default:
			e.Address = addressFromPort(e.Port)
		}
		if e.Address == "" {
			reason := "no IP address"
			if e.Port != "" {
				reason = fmt.Sprintf("no IP address, and port %q isn't a network port", e.Port)
			}
			issues = append(issues, InventoryRowIssue{Line: line, Name: e.Name, Reason: reason})
			continue
		}
		key := strings.ToLower(e.Name)
		if first, dup := seen[key]; dup {
			issues = append(issues, InventoryRowIssue{Line: line, Name: e.Name, Reason: fmt.Sprintf("same name as line %d; the first one is kept", first)})
			continue
		}
		seen[key] = line
		entries = append(entries, e)
	}
	if rows == 0 {
		return nil, nil, formatErr("the file has a header but no printers")
	}
	if len(entries) == 0 {
		return nil, issues, formatErr("none of the %d printers in the file has a usable address", rows)
	}
	return entries, issues, nil
}

// cupsQueueName is the queue name CUPS and macOS make from a shared printer
// name: characters other than letters, digits, ".", "-" and "_" become "_".
func cupsQueueName(name string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x80 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_') {
			return r
		}
		return '_'
	}, name)
}

// printServerName returns the printer name in a print server device URI such
// as ipps://server:9164/printers/Front%20Desk?auth=true, or "".
func printServerName(deviceURI string) string {
	u, err := url.Parse(strings.TrimPrefix(deviceURI, paperCutBackendPrefix))
	if err != nil {
		return ""
	}
	i := strings.LastIndex(u.Path, "/printers/")
	if i < 0 {
		return ""
	}
	return strings.Trim(u.Path[i+len("/printers/"):], "/")
}

// inventoryFile is what the store keeps on disk.
type inventoryFile struct {
	Version    int              `json:"version"`
	SourceName string           `json:"source_name"`
	ImportedAt time.Time        `json:"imported_at"`
	Entries    []InventoryEntry `json:"entries"`
}

// InventoryStore keeps the parsed list in ~/.cups-webui/printer-inventory.json
// (never the CSV itself), or in memory for mock mode and tests.
type InventoryStore struct {
	path   string
	memory bool

	mu  sync.Mutex
	mem *inventoryFile
}

func NewInventoryStore(path string) *InventoryStore { return &InventoryStore{path: path} }

func NewMemoryInventoryStore() *InventoryStore { return &InventoryStore{memory: true} }

// DefaultInventoryStorePath is ~/.cups-webui/printer-inventory.json.
func DefaultInventoryStorePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", errors.New("home folder not found")
	}
	return filepath.Join(home, ".cups-webui", "printer-inventory.json"), nil
}

func (s *InventoryStore) load() (*inventoryFile, error) {
	if s.memory {
		return s.mem, nil
	}
	if s.path == "" {
		return nil, errors.New("the printer list location is unknown")
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", s.path, err)
	}
	var f inventoryFile
	if err := json.Unmarshal(data, &f); err != nil || f.Version != 1 {
		return nil, fmt.Errorf("%s is damaged or from another version; import the CSV again", s.path)
	}
	return &f, nil
}

// Status reports whether a list is stored.
func (s *InventoryStore) Status() InventoryStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load()
	if err != nil {
		return InventoryStatus{Error: err.Error()}
	}
	if f == nil {
		return InventoryStatus{}
	}
	return InventoryStatus{Loaded: true, SourceName: f.SourceName, ImportedAt: f.ImportedAt, Count: len(f.Entries)}
}

// Lookup finds the printer for a queue: first by the printer name in a print
// server device URI, then by the queue name CUPS would make from it.
func (s *InventoryStore) Lookup(queue, deviceURI string) *InventoryEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load()
	if err != nil || f == nil {
		return nil
	}
	return lookupEntry(f.Entries, queue, deviceURI)
}

func lookupEntry(entries []InventoryEntry, queue, deviceURI string) *InventoryEntry {
	if name := printServerName(deviceURI); name != "" {
		for i := range entries {
			if strings.EqualFold(entries[i].Name, name) {
				return &entries[i]
			}
		}
	}
	for i := range entries {
		if strings.EqualFold(cupsQueueName(entries[i].Name), queue) {
			return &entries[i]
		}
	}
	return nil
}

func (s *InventoryStore) save(f *inventoryFile) error {
	if s.memory {
		s.mem = f
		return nil
	}
	if s.path == "" {
		return errors.New("the printer list location is unknown")
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path, append(data, '\n'))
}

// Remove forgets the stored list.
func (s *InventoryStore) Remove() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.memory {
		s.mem = nil
		return nil
	}
	if s.path == "" {
		return nil
	}
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// PrinterInventory returns the stored list's status.
func (m *AccessoryManager) PrinterInventory() *InventoryStatus {
	st := m.Inventory.Status()
	return &st
}

// ImportPrinterInventory replaces the stored list with a parsed CSV. Only CUPS
// administrators can import. A file that can't be used leaves the old list.
func (m *AccessoryManager) ImportPrinterInventory(sourceName string, data []byte) (*InventoryImportResult, error) {
	if !m.Source.IsAdmin() {
		return nil, ErrNotCUPSAdmin
	}
	entries, issues, err := ParsePrinterInventoryCSV(data)
	if err != nil {
		return nil, err
	}
	f := &inventoryFile{Version: 1, SourceName: cleanField(filepath.Base(sourceName)), ImportedAt: m.Now().UTC(), Entries: entries}
	m.Inventory.mu.Lock()
	err = m.Inventory.save(f)
	m.Inventory.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("cannot save the printer list: %w", err)
	}
	res := &InventoryImportResult{
		InventoryStatus: InventoryStatus{Loaded: true, SourceName: f.SourceName, ImportedAt: f.ImportedAt, Count: len(entries)},
		Rows:            len(entries) + len(issues),
		Skipped:         issues,
	}
	if res.Skipped == nil {
		res.Skipped = []InventoryRowIssue{}
	}
	if uris, err := m.Source.DeviceURIs(); err == nil {
		res.Queues = len(uris)
		for q, uri := range uris {
			if lookupEntry(entries, q, uri) != nil {
				res.Matched++
			}
		}
	}
	return res, nil
}

// RemovePrinterInventory forgets the stored list. Only CUPS administrators can remove it.
func (m *AccessoryManager) RemovePrinterInventory() (*InventoryStatus, error) {
	if !m.Source.IsAdmin() {
		return nil, ErrNotCUPSAdmin
	}
	if err := m.Inventory.Remove(); err != nil {
		return nil, fmt.Errorf("cannot remove the printer list: %w", err)
	}
	return m.PrinterInventory(), nil
}
