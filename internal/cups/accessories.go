package cups

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Where an accessory value came from.
const (
	AccessorySourceManual        = "manual"         // set in this app (recorded in the override store)
	AccessorySourceDetected      = "detected"       // differs from the driver default, not set in this app
	AccessorySourceDriverDefault = "driver_default" // same as the driver's own default
	AccessorySourceUnknown       = "unknown"        // driver default could not be read
)

// Reasons the manual accessory section is not offered for a queue.
const (
	AccessoryUnavailableNoPPD         = "no_ppd"
	AccessoryUnavailableDriverless    = "driverless"
	AccessoryUnavailableNoInstallable = "no_installable_options"
)

var (
	// ErrNotCUPSAdmin is returned when the user running the server is not a CUPS administrator.
	ErrNotCUPSAdmin = errors.New("only CUPS administrators can change printer accessories")
	// ErrAccessoriesUnavailable is returned for queues without installable options.
	ErrAccessoriesUnavailable = errors.New("this queue has no installable options to set")
)

// AccessoryInputError is a problem with the request (unknown option, invalid choice).
type AccessoryInputError struct{ Msg string }

func (e *AccessoryInputError) Error() string { return e.Msg }

// AccessoryVerifyError means CUPS accepted the change but stored a different value.
type AccessoryVerifyError struct{ Msg string }

func (e *AccessoryVerifyError) Error() string { return e.Msg }

// AccessoryOption is one installable option with its current value and where it came from.
type AccessoryOption struct {
	Key           string             `json:"key"`
	Label         string             `json:"label"`
	Choices       []OptionChoice     `json:"choices"`
	Value         string             `json:"value"`          // what the queue's PPD holds now
	DriverDefault string             `json:"driver_default"` // the driver's own default ("" if unknown)
	Source        string             `json:"source"`
	Override      *AccessoryOverride `json:"override,omitempty"`
	// OverrideReplaced is true when a manual value was recorded but the queue
	// now holds something else (changed outside this app).
	OverrideReplaced bool `json:"override_replaced"`
}

// AccessoryReport describes a queue's installable options for the UI.
type AccessoryReport struct {
	PrinterName         string            `json:"printer_name"`
	Available           bool              `json:"available"`
	UnavailableReason   string            `json:"unavailable_reason,omitempty"`
	IsAdmin             bool              `json:"is_admin"`
	DriverModel         string            `json:"driver_model"`
	DriverPPD           string            `json:"driver_ppd,omitempty"`
	DriverDefaultsKnown bool              `json:"driver_defaults_known"`
	DeviceURI           string            `json:"device_uri"`
	DirectConnection    bool              `json:"direct_connection"`
	ConnectionNote      string            `json:"connection_note"`
	HasOverrides        bool              `json:"has_overrides"`
	Options             []AccessoryOption `json:"options"`
	// HistoryError is set when the app's notes file can't be used. Values
	// still come from CUPS; changes are refused until it is fixed.
	HistoryError string `json:"history_error,omitempty"`
	// IgnoredNotes counts notes recorded for an earlier queue with this name
	// (a different printer-uuid). They are not shown and are dropped on the next change.
	IgnoredNotes int `json:"ignored_notes,omitempty"`
	// Warning is set when CUPS was changed but the notes couldn't be saved.
	Warning string `json:"warning,omitempty"`
}

// AccessorySuggestion is a value the printer's IPP attributes suggest. It is never applied automatically.
type AccessorySuggestion struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Value    string `json:"value"`
	Evidence string `json:"evidence"`
}

// AccessorySuggestions is the result of asking the printer over IPP.
type AccessorySuggestions struct {
	PrinterName string                `json:"printer_name"`
	Queried     bool                  `json:"queried"`
	Note        string                `json:"note,omitempty"`
	Suggestions []AccessorySuggestion `json:"suggestions"`
}

// AccessorySource is what the manager needs from CUPS. The CLI and mock
// adapters each provide one.
type AccessorySource interface {
	// QueuePPD returns the queue's PPD as CUPS stores it.
	QueuePPD(name string) ([]byte, error)
	// DriverPPD returns the driver's original PPD for a NickName, and the
	// driver name it came from. It returns nil data if the driver is unknown.
	DriverPPD(nickName string) (driverName string, data []byte, err error)
	DeviceURI(name string) (string, error)
	// ApplyOptions sets PPD defaults the way `lpadmin -p <name> -o k=v` does.
	ApplyOptions(name string, options map[string]string) error
	// PrinterAttributes returns `ipptool -tv` Get-Printer-Attributes output.
	PrinterAttributes(uri string) (string, error)
	IsAdmin() bool
	UserName() string
	// QueueUUID returns CUPS's printer-uuid for the queue ("" if unknown).
	QueueUUID(name string) (string, error)
	// QueueNames lists the queues that exist, to drop notes for deleted ones.
	QueueNames() ([]string, error)
}

// AccessoryHistoryError wraps a problem with the notes file that blocks changes.
type AccessoryHistoryError struct{ Err error }

func (e *AccessoryHistoryError) Error() string { return e.Err.Error() }
func (e *AccessoryHistoryError) Unwrap() error { return e.Err }

// AccessoryManager implements the manual accessory feature on top of an AccessorySource.
type AccessoryManager struct {
	Source AccessorySource
	Store  *OverrideStore
	Now    func() time.Time

	mu sync.Mutex // serialises changes so read-apply-verify-record is atomic
}

// NewAccessoryManager creates a manager.
func NewAccessoryManager(src AccessorySource, store *OverrideStore) *AccessoryManager {
	if store == nil {
		store = NewMemoryOverrideStore()
	}
	return &AccessoryManager{Source: src, Store: store, Now: time.Now}
}

// Report returns the queue's installable options with their sources.
func (m *AccessoryManager) Report(name string) (*AccessoryReport, error) {
	if !IsValidPrinterName(name) {
		return nil, &AccessoryInputError{Msg: fmt.Sprintf("invalid printer name: %q", name)}
	}
	rep := &AccessoryReport{
		PrinterName: name,
		IsAdmin:     m.Source.IsAdmin(),
		Options:     []AccessoryOption{},
	}
	if uri, err := m.Source.DeviceURI(name); err == nil {
		rep.DeviceURI = uri
		rep.DirectConnection, rep.ConnectionNote = ClassifyDeviceURI(uri)
	} else {
		return nil, err
	}

	data, err := m.Source.QueuePPD(name)
	if err != nil || len(data) == 0 {
		rep.UnavailableReason = AccessoryUnavailableNoPPD
		return rep, nil
	}
	ppd := ParsePPDSummary(data)
	rep.DriverModel = ppd.NickName
	if rep.DriverModel == "" {
		rep.DriverModel = ppd.ModelName
	}
	switch {
	case ppd.Driverless:
		rep.UnavailableReason = AccessoryUnavailableDriverless
		return rep, nil
	case !ppd.HasInstallableGroup || len(ppd.Installable) == 0:
		rep.UnavailableReason = AccessoryUnavailableNoInstallable
		return rep, nil
	}
	rep.Available = true

	var driver *PPDSummary
	for _, name := range ppd.DriverNames() {
		if driverName, ddata, derr := m.Source.DriverPPD(name); derr == nil && len(ddata) > 0 {
			driver = ParsePPDSummary(ddata)
			rep.DriverPPD = driverName
			rep.DriverDefaultsKnown = true
			break
		}
	}

	overrides, stale, _, herr := m.queueNotes(name)
	if herr != nil {
		rep.HistoryError = herr.Error()
	}
	rep.IgnoredNotes = len(stale)
	rep.HasOverrides = len(overrides) > 0

	for _, opt := range ppd.Installable {
		ao := AccessoryOption{
			Key:     opt.Key,
			Label:   opt.Label,
			Choices: opt.Choices,
			Value:   opt.Default,
			Source:  AccessorySourceUnknown,
		}
		if driver != nil {
			if dopt := driver.Option(opt.Key); dopt != nil {
				ao.DriverDefault = dopt.Default
			}
		}
		if ov, ok := overrides[opt.Key]; ok {
			ov := ov
			ao.Override = &ov
			ao.OverrideReplaced = ov.Value != opt.Default
		}
		switch {
		case ao.Override != nil && !ao.OverrideReplaced:
			ao.Source = AccessorySourceManual
		case ao.DriverDefault == "":
			ao.Source = AccessorySourceUnknown
		case ao.DriverDefault == ao.Value:
			ao.Source = AccessorySourceDriverDefault
		default:
			ao.Source = AccessorySourceDetected
		}
		rep.Options = append(rep.Options, ao)
	}
	return rep, nil
}

func (rep *AccessoryReport) option(key string) *AccessoryOption {
	for i := range rep.Options {
		if rep.Options[i].Key == key {
			return &rep.Options[i]
		}
	}
	return nil
}

func hasChoice(opt *AccessoryOption, value string) bool {
	for _, c := range opt.Choices {
		if c.Name == value {
			return true
		}
	}
	return false
}

// queueNotes returns the notes for this queue, the options of notes that
// belong to an earlier queue with the same name (different printer-uuid), and
// the queue's current uuid.
func (m *AccessoryManager) queueNotes(name string) (notes map[string]AccessoryOverride, stale []string, uuid string, err error) {
	notes = map[string]AccessoryOverride{}
	all, err := m.Store.ForQueue(name)
	if err != nil {
		return notes, nil, "", err
	}
	uuid, _ = m.Source.QueueUUID(name)
	for k, ov := range all {
		if ov.QueueUUID != "" && uuid != "" && ov.QueueUUID != uuid {
			stale = append(stale, k)
			continue
		}
		notes[k] = ov
	}
	sort.Strings(stale)
	return notes, stale, uuid, nil
}

// checkHistoryWritable refuses a change before CUPS is touched if the notes
// can't be recorded afterwards.
func (m *AccessoryManager) checkHistoryWritable() error {
	if err := m.Store.Writable(); err != nil {
		return &AccessoryHistoryError{Err: err}
	}
	return nil
}

// pruneDeletedQueues drops notes for queues that no longer exist (deleted
// outside this app). Best effort: if the queue list can't be read, nothing is dropped.
func (m *AccessoryManager) pruneDeletedQueues() {
	names, err := m.Source.QueueNames()
	if err != nil || len(names) == 0 {
		return
	}
	_ = m.Store.Prune(func(q string) bool {
		for _, n := range names {
			if sameQueue(n, q) {
				return true
			}
		}
		return false
	})
}

// Set applies manual values through CUPS, reads the PPD back, and records a
// note for each value CUPS stored. acknowledged must be true: the caller has
// shown the "can't be checked against the printer" warning.
func (m *AccessoryManager) Set(name string, values map[string]string, acknowledged bool) (*AccessoryReport, error) {
	if !m.Source.IsAdmin() {
		return nil, ErrNotCUPSAdmin
	}
	if !acknowledged {
		return nil, &AccessoryInputError{Msg: "confirm the warning before saving: these settings can't be checked against the printer"}
	}
	if len(values) == 0 {
		return nil, &AccessoryInputError{Msg: "no accessory values given"}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.checkHistoryWritable(); err != nil {
		return nil, err
	}
	before, err := m.Report(name)
	if err != nil {
		return nil, err
	}
	if !before.Available {
		return nil, ErrAccessoriesUnavailable
	}

	changes := map[string]string{}
	for k, v := range values {
		opt := before.option(k)
		if opt == nil {
			return nil, &AccessoryInputError{Msg: fmt.Sprintf("%q is not an installable option of this queue", k)}
		}
		if !IsValidOptionKey(k) || !IsValidOptionValue(v) || !hasChoice(opt, v) {
			return nil, &AccessoryInputError{Msg: fmt.Sprintf("%q is not a valid choice for %s", v, opt.Label)}
		}
		if opt.Value != v {
			changes[k] = v
		}
	}
	if len(changes) == 0 {
		return before, nil
	}

	if err := m.Source.ApplyOptions(name, changes); err != nil {
		return nil, err
	}

	after, err := m.Report(name)
	if err != nil {
		return nil, err
	}
	_, stale, uuid, _ := m.queueNotes(name)

	var set []AccessoryOverride
	remove := append([]string(nil), stale...)
	var mismatches []string
	user := m.Source.UserName()
	now := m.Now().UTC()
	for _, k := range sortedKeys(changes) {
		want := changes[k]
		prev := before.option(k)
		got := after.option(k)
		if got == nil || got.Value != want {
			stored := "(missing)"
			if got != nil {
				stored = got.Value
			}
			mismatches = append(mismatches, fmt.Sprintf("%s: asked for %s, CUPS stored %s", prev.Label, want, stored))
			continue
		}
		previous := prev.Value
		if prev.Override != nil {
			previous = prev.Override.PreviousValue
		}
		if want == previous {
			// Back to the value it had before any manual change.
			remove = append(remove, k)
			continue
		}
		set = append(set, AccessoryOverride{
			QueueUUID:     uuid,
			Option:        k,
			Value:         want,
			PreviousValue: previous,
			SetBy:         user,
			SetAt:         now,
		})
	}
	storeErr := m.Store.Update(name, set, remove)
	if storeErr == nil {
		m.pruneDeletedQueues()
	}

	final, err := m.Report(name)
	if err != nil {
		return nil, err
	}
	if storeErr != nil {
		final.Warning = "Saved to CUPS, but the change history couldn't be recorded: " + storeErr.Error()
	}
	if len(mismatches) > 0 {
		return final, &AccessoryVerifyError{Msg: "CUPS did not store every value: " + strings.Join(mismatches, "; ")}
	}
	return final, nil
}

// Reset restores the values each option had before it was set manually, and
// forgets the notes.
func (m *AccessoryManager) Reset(name string) (*AccessoryReport, error) {
	if !m.Source.IsAdmin() {
		return nil, ErrNotCUPSAdmin
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.checkHistoryWritable(); err != nil {
		return nil, err
	}
	before, err := m.Report(name)
	if err != nil {
		return nil, err
	}
	overrides, stale, _, err := m.queueNotes(name)
	if err != nil {
		return nil, &AccessoryHistoryError{Err: err}
	}
	if len(overrides) == 0 && len(stale) == 0 {
		return before, nil
	}

	changes := map[string]string{}
	for k, ov := range overrides {
		opt := before.option(k)
		if opt == nil || ov.PreviousValue == "" || !hasChoice(opt, ov.PreviousValue) {
			continue
		}
		if opt.Value != ov.PreviousValue && IsValidOptionKey(k) && IsValidOptionValue(ov.PreviousValue) {
			changes[k] = ov.PreviousValue
		}
	}
	if len(changes) > 0 {
		if err := m.Source.ApplyOptions(name, changes); err != nil {
			return nil, err
		}
	}

	after, err := m.Report(name)
	if err != nil {
		return nil, err
	}
	var mismatches []string
	for _, k := range sortedKeys(changes) {
		if got := after.option(k); got == nil || got.Value != changes[k] {
			mismatches = append(mismatches, k)
		}
	}
	if len(mismatches) > 0 {
		// Keep the notes so the admin can see what is still set and retry.
		return after, &AccessoryVerifyError{Msg: "CUPS did not restore: " + strings.Join(mismatches, ", ")}
	}

	keys := append([]string(nil), stale...)
	for k := range overrides {
		keys = append(keys, k)
	}
	storeErr := m.Store.Update(name, nil, keys)
	final, err := m.Report(name)
	if err != nil {
		return nil, err
	}
	if storeErr != nil {
		final.Warning = "Restored in CUPS, but the change history couldn't be updated: " + storeErr.Error()
	}
	return final, nil
}

// ReapplyOverrides sets the recorded manual values again, for example after
// the queue's driver was replaced and its PPD defaults were reset. Options the
// new PPD doesn't have are skipped (and kept, in case the driver comes back).
func (m *AccessoryManager) ReapplyOverrides(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	overrides, _, _, err := m.queueNotes(name)
	if err != nil || len(overrides) == 0 {
		return err
	}
	rep, err := m.Report(name)
	if err != nil || !rep.Available {
		return err
	}
	changes := map[string]string{}
	for k, ov := range overrides {
		opt := rep.option(k)
		if opt != nil && opt.Value != ov.Value && hasChoice(opt, ov.Value) && IsValidOptionValue(ov.Value) {
			changes[k] = ov.Value
		}
	}
	if len(changes) == 0 {
		return nil
	}
	return m.Source.ApplyOptions(name, changes)
}

// ForgetQueue drops the notes of a deleted queue.
func (m *AccessoryManager) ForgetQueue(name string) error {
	return m.Store.DeleteQueue(name)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ClassifyDeviceURI reports whether a device URI addresses a printer directly,
// with a short description. Loopback addresses (Mobility Print and other local
// proxies), print-server paths and SMB shares are not direct, so the UI shows
// a stronger warning for them.
func ClassifyDeviceURI(uri string) (direct bool, note string) {
	scheme, _, found := strings.Cut(uri, "://")
	if !found || scheme == "" {
		return false, "Unrecognised device address"
	}
	scheme = strings.ToLower(scheme)
	// dnssd:// hosts contain %-escapes that url.Parse rejects; the scheme is enough for them.
	u, err := url.Parse(uri)
	if err != nil {
		if scheme != "dnssd" && scheme != "mdns" {
			return false, "Unrecognised device address"
		}
		u = &url.URL{}
	}
	host := strings.ToLower(u.Hostname())
	isLoopback := host == "localhost" || strings.HasSuffix(host, ".localhost")
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		isLoopback = true
	}

	switch scheme {
	case "socket", "lpd", "usb":
		if isLoopback {
			return false, "Local service on this Mac, not the printer itself"
		}
		return true, "Direct connection to the printer"
	case "ipp", "ipps", "http", "https":
		if isLoopback {
			return false, "IPP service on this Mac (for example PaperCut Mobility Print), not the printer itself"
		}
		path := strings.ToLower(u.Path)
		if strings.Contains(path, "/printers/") || strings.Contains(path, "/classes/") || strings.Contains(path, "/queues/") {
			return false, "IPP print server or print queue, not the printer itself"
		}
		return true, "Network IPP printer"
	case "dnssd", "mdns":
		return false, "Bonjour address: may be a printer or a queue shared by another computer"
	case "smb":
		return false, "Windows print server share"
	default:
		return false, "Not a direct printer address"
	}
}

// IsIPPDeviceURI reports whether Get-Printer-Attributes can be sent to uri.
func IsIPPDeviceURI(uri string) bool {
	u, err := url.Parse(uri)
	if err != nil || u.Host == "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "ipp", "ipps", "http", "https":
		return true
	}
	return false
}

// ParseIPPToolAttributes parses `ipptool -tv` output lines such as
// "finishings-supported (1setOf enum) = staple-top-left,punch-dual-left".
func ParseIPPToolAttributes(out string) map[string][]string {
	attrs := map[string][]string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		paren := strings.Index(line, " (")
		eq := strings.Index(line, ") = ")
		if paren <= 0 || eq < paren {
			continue
		}
		name := line[:paren]
		if strings.ContainsAny(name, " :") {
			continue
		}
		var vals []string
		for _, v := range strings.Split(line[eq+4:], ",") {
			if v = strings.TrimSpace(v); v != "" {
				vals = append(vals, v)
			}
		}
		attrs[name] = append(attrs[name], vals...)
	}
	return attrs
}

// printerCapabilities is what the IPP attributes say the device has.
type printerCapabilities struct {
	staple, punch, fold, duplex, bypass, envelope []string // evidence values
	trays                                         map[int]string
}

func inRange(n, lo, hi int) bool { return n >= lo && n <= hi }

func capabilitiesFromIPP(attrs map[string][]string) printerCapabilities {
	caps := printerCapabilities{trays: map[int]string{}}
	for _, v := range attrs["finishings-supported"] {
		lv := strings.ToLower(v)
		n, numErr := strconv.Atoi(lv)
		isNum := numErr == nil
		switch {
		case lv == "staple" || strings.HasPrefix(lv, "staple-") ||
			(isNum && (n == 4 || inRange(n, 20, 23) || inRange(n, 28, 35))):
			caps.staple = append(caps.staple, v)
		case lv == "punch" || strings.HasPrefix(lv, "punch-") ||
			(isNum && (n == 5 || inRange(n, 70, 85))):
			caps.punch = append(caps.punch, v)
		case lv == "fold" || strings.HasPrefix(lv, "fold-") ||
			(isNum && (n == 10 || inRange(n, 90, 101))):
			caps.fold = append(caps.fold, v)
		}
	}
	for _, v := range attrs["sides-supported"] {
		if strings.HasPrefix(strings.ToLower(v), "two-sided") {
			caps.duplex = append(caps.duplex, v)
		}
	}
	for _, v := range attrs["media-source-supported"] {
		lv := strings.ToLower(v)
		switch {
		case strings.HasPrefix(lv, "tray-"):
			if n, err := strconv.Atoi(strings.TrimPrefix(lv, "tray-")); err == nil {
				caps.trays[n] = v
			}
		case lv == "by-pass-tray" || lv == "manual" || lv == "multipurpose":
			caps.bypass = append(caps.bypass, v)
		case strings.HasPrefix(lv, "envelope"):
			caps.envelope = append(caps.envelope, v)
		}
	}
	return caps
}

// accessoryRule maps one capability to a PPD option for a driver.
type accessoryRule struct {
	option string
	value  string
	match  func(c printerCapabilities) (evidence string, ok bool)
}

func evidenceFrom(attr string, vals []string) (string, bool) {
	if len(vals) == 0 {
		return "", false
	}
	return attr + " includes " + strings.Join(vals, ", "), true
}

func trayRule(n int) accessoryRule {
	return accessoryRule{option: fmt.Sprintf("OptTray%d", n), value: "True", match: func(c printerCapabilities) (string, bool) {
		if v, ok := c.trays[n]; ok {
			return "media-source-supported includes " + v, true
		}
		return "", false
	}}
}

// genericUniversalRules covers the Lexmark-built "Generic Universal" PPDs
// (FUJIFILM ApeosPort driver 2.0.0). Verified against Generic Universal Mono.
var genericUniversalRules = []accessoryRule{
	{option: "OptStapler", value: "True", match: func(c printerCapabilities) (string, bool) {
		return evidenceFrom("finishings-supported", c.staple)
	}},
	{option: "OptHolePunch", value: "True", match: func(c printerCapabilities) (string, bool) {
		return evidenceFrom("finishings-supported", c.punch)
	}},
	{option: "OptFold", value: "True", match: func(c printerCapabilities) (string, bool) {
		return evidenceFrom("finishings-supported", c.fold)
	}},
	{option: "OptDuplex", value: "True", match: func(c printerCapabilities) (string, bool) {
		return evidenceFrom("sides-supported", c.duplex)
	}},
	trayRule(2), trayRule(3), trayRule(4), trayRule(5),
	{option: "OptMultipurposeFeeder", value: "True", match: func(c printerCapabilities) (string, bool) {
		return evidenceFrom("media-source-supported", c.bypass)
	}},
	{option: "OptEnvFeeder", value: "True", match: func(c printerCapabilities) (string, bool) {
		return evidenceFrom("media-source-supported", c.envelope)
	}},
}

// accessorySuggestionRules maps a driver NickName to its rules. Drivers not
// listed get no suggestions: option names differ between manufacturers.
var accessorySuggestionRules = map[string][]accessoryRule{
	"Generic Universal Mono": genericUniversalRules,
}

// SuggestFromIPP matches IPP attributes against a driver's rules. It only
// suggests values that differ from the queue's current value.
func SuggestFromIPP(rep *AccessoryReport, attrs map[string][]string) []AccessorySuggestion {
	rules := accessorySuggestionRules[rep.DriverModel]
	caps := capabilitiesFromIPP(attrs)
	out := []AccessorySuggestion{}
	for _, r := range rules {
		opt := rep.option(r.option)
		if opt == nil || opt.Value == r.value || !hasChoice(opt, r.value) {
			continue
		}
		if ev, ok := r.match(caps); ok {
			out = append(out, AccessorySuggestion{Key: opt.Key, Label: opt.Label, Value: r.value, Evidence: ev})
		}
	}
	return out
}

// Suggest asks the printer for its attributes and suggests settings. It never
// changes the queue.
func (m *AccessoryManager) Suggest(name string) (*AccessorySuggestions, error) {
	rep, err := m.Report(name)
	if err != nil {
		return nil, err
	}
	res := &AccessorySuggestions{PrinterName: name, Suggestions: []AccessorySuggestion{}}
	if !rep.Available {
		res.Note = "This queue has no installable options."
		return res, nil
	}
	if _, ok := accessorySuggestionRules[rep.DriverModel]; !ok {
		res.Note = fmt.Sprintf("No suggestion rules for the %q driver. Set the accessories by hand.", rep.DriverModel)
		return res, nil
	}
	if !IsIPPDeviceURI(rep.DeviceURI) {
		res.Note = "Suggestions need an IPP device address; this queue doesn't use one."
		return res, nil
	}
	out, err := m.Source.PrinterAttributes(rep.DeviceURI)
	if err != nil {
		res.Note = "The device didn't answer the IPP query: " + err.Error()
		return res, nil
	}
	res.Queried = true
	res.Suggestions = SuggestFromIPP(rep, ParseIPPToolAttributes(out))
	if len(res.Suggestions) == 0 {
		res.Note = "The device's IPP attributes don't suggest any changes."
	}
	if !rep.DirectConnection {
		res.Note = strings.TrimSpace(res.Note + " This queue doesn't point straight at the printer, so the answer may describe the server rather than the physical printer.")
	}
	return res, nil
}
