package cups

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ErrSNMPDisabled is returned when the SNMP hardware check is turned off.
var ErrSNMPDisabled = errors.New("the SNMP hardware check is turned off; an administrator can turn it on in Server Settings")

// SNMP roots walked by the hardware check (RFC 3805 Printer MIB v2, RFC 3806
// Printer Finishing MIB, RFC 2790 Host Resources MIB).
const (
	oidSysDescr         = "1.3.6.1.2.1.1.1"
	oidSysName          = "1.3.6.1.2.1.1.5"
	oidHrDeviceDescr    = "1.3.6.1.2.1.25.3.2.1.3"
	oidPrtInputEntry    = "1.3.6.1.2.1.43.8.2.1"
	oidPrtOutputEntry   = "1.3.6.1.2.1.43.9.2.1"
	oidPrtMediaPathType = "1.3.6.1.2.1.43.13.4.1.9"
	oidFinDeviceEntry   = "1.3.6.1.2.1.43.30.1.1"
)

// SNMPRoots is the list of subtrees the hardware check walks, in order. The
// first is a single object, so a host that doesn't answer fails fast.
var SNMPRoots = []string{
	oidSysDescr, oidSysName, oidHrDeviceDescr,
	oidPrtInputEntry, oidPrtOutputEntry, oidPrtMediaPathType, oidFinDeviceEntry,
}

// SNMPInput is one paper source from prtInputTable.
type SNMPInput struct {
	Index       string `json:"index"`
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Media       string `json:"media,omitempty"`
	Capacity    int    `json:"capacity,omitempty"` // sheets; 0 when the printer doesn't say
}

// SNMPOutput is one output bin from prtOutputTable.
type SNMPOutput struct {
	Index       string `json:"index"`
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Capacity    int    `json:"capacity,omitempty"`
}

// SNMPFinisher is one finishing device from finDeviceTable.
type SNMPFinisher struct {
	Index       string `json:"index"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Present     string `json:"present,omitempty"`
}

// SNMPInventory is what the printer reports about its installed hardware.
type SNMPInventory struct {
	PrinterName       string                `json:"printer_name"`
	Host              string                `json:"host"`
	Queried           bool                  `json:"queried"`
	Note              string                `json:"note,omitempty"`
	SystemDescription string                `json:"system_description,omitempty"`
	SystemName        string                `json:"system_name,omitempty"`
	Devices           []string              `json:"devices"`
	Inputs            []SNMPInput           `json:"inputs"`
	Outputs           []SNMPOutput          `json:"outputs"`
	Finishers         []SNMPFinisher        `json:"finishers"`
	Duplex            []string              `json:"duplex"`
	Suggestions       []AccessorySuggestion `json:"suggestions"`
}

var snmpHostLabel = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

// IsValidSNMPHost accepts an IP address or a DNS host name, nothing else:
// no port, scheme, path, spaces or leading "-" that snmpwalk could read as an option.
func IsValidSNMPHost(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return true
	}
	for _, label := range strings.Split(strings.TrimSuffix(host, "."), ".") {
		if !snmpHostLabel.MatchString(label) {
			return false
		}
	}
	return true
}

// IsValidSNMPCommunity accepts 1-64 printable ASCII characters without
// spaces, so the value fits on one line of snmp.conf.
func IsValidSNMPCommunity(c string) bool {
	if c == "" || len(c) > 64 {
		return false
	}
	for i := 0; i < len(c); i++ {
		if c[i] <= 0x20 || c[i] >= 0x7f {
			return false
		}
	}
	return true
}

var (
	snmpLine   = regexp.MustCompile(`^\.?([0-9]+(?:\.[0-9]+)+) = (.*)$`)
	snmpEnumed = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*\((-?[0-9]+)\)$`)
)

// ParseSNMPWalk parses `snmpwalk -On` output into OID (without the leading
// dot) to value. Strings are unquoted, Hex-STRING values that hold text are
// decoded, and enums are reduced to their number. Lines that don't start a
// new OID continue the previous value (snmpwalk wraps long values).
func ParseSNMPWalk(out string) map[string]string {
	raw := map[string]string{}
	var order []string
	last := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := snmpLine.FindStringSubmatch(line); m != nil {
			last = m[1]
			if _, seen := raw[last]; !seen {
				order = append(order, last)
			}
			raw[last] = m[2]
			continue
		}
		if last != "" && line != "" {
			raw[last] += "\n" + line
		}
	}
	vals := map[string]string{}
	for _, oid := range order {
		if v, ok := decodeSNMPValue(raw[oid]); ok {
			vals[oid] = v
		}
	}
	return vals
}

func decodeSNMPValue(v string) (string, bool) {
	if strings.HasPrefix(v, "No Such ") || strings.HasPrefix(v, "No more variables") {
		return "", false
	}
	if strings.HasPrefix(v, `"`) {
		return unquoteSNMP(v), true
	}
	typ, val, found := strings.Cut(v, ": ")
	if !found {
		return strings.TrimSpace(v), true
	}
	val = strings.TrimSpace(val)
	switch typ {
	case "STRING":
		// macOS snmpwalk prints STRING values without quotes; other builds quote them.
		if strings.HasPrefix(val, `"`) {
			return unquoteSNMP(val), true
		}
		return unescapeSNMP(val), true
	case "Hex-STRING":
		b, err := hex.DecodeString(strings.Join(strings.Fields(val), ""))
		if err != nil {
			return val, true
		}
		text := strings.TrimRight(string(b), "\x00")
		if utf8.ValidString(text) && isPrintableText(text) {
			return text, true
		}
		return val, true
	case "INTEGER":
		if m := snmpEnumed.FindStringSubmatch(val); m != nil {
			return m[1], true
		}
		return val, true
	}
	return val, true
}

func unquoteSNMP(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, `"`)
	v = strings.TrimSuffix(v, `"`)
	return unescapeSNMP(v)
}

// unescapeSNMP undoes the \" and \\ escapes snmpwalk adds inside strings.
func unescapeSNMP(v string) string {
	return strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(v)
}

func isPrintableText(s string) bool {
	for _, r := range s {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return false
		}
	}
	return true
}

// snmpRows groups the values under a table entry OID by row index, keyed by
// column number: "1.3.6...8.2.1.13.1.2" is column 13 of row "1.2".
func snmpRows(vals map[string]string, entry string) map[string]map[int]string {
	rows := map[string]map[int]string{}
	prefix := entry + "."
	for oid, v := range vals {
		rest, ok := strings.CutPrefix(oid, prefix)
		if !ok {
			continue
		}
		colStr, index, ok := strings.Cut(rest, ".")
		col, err := strconv.Atoi(colStr)
		if !ok || err != nil {
			continue
		}
		if rows[index] == nil {
			rows[index] = map[int]string{}
		}
		rows[index][col] = v
	}
	return rows
}

// sortedSNMPIndexes orders row indexes such as "1.2" and "1.10" numerically.
func sortedSNMPIndexes[T any](rows map[string]T) []string {
	idx := make([]string, 0, len(rows))
	for k := range rows {
		idx = append(idx, k)
	}
	sort.Slice(idx, func(i, j int) bool {
		a, b := strings.Split(idx[i], "."), strings.Split(idx[j], ".")
		for k := 0; k < len(a) && k < len(b); k++ {
			x, _ := strconv.Atoi(a[k])
			y, _ := strconv.Atoi(b[k])
			if x != y {
				return x < y
			}
		}
		return len(a) < len(b)
	})
	return idx
}

func snmpCapacity(v string) int {
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0 // -1 other, -2 unknown
	}
	return n
}

func enumLabel(labels map[string]string, v string) string {
	if l, ok := labels[v]; ok {
		return l
	}
	if v == "" {
		return "unknown"
	}
	return "type " + v
}

// PrtInputTypeTC (RFC 3805).
var snmpInputTypes = map[string]string{
	"1": "other", "2": "unknown", "3": "removable tray", "4": "fixed tray",
	"5": "manual feed", "6": "roll", "7": "fan-fold",
}

// PrtOutputTypeTC (RFC 3805).
var snmpOutputTypes = map[string]string{
	"1": "other", "2": "unknown", "3": "removable bin", "4": "fixed bin",
	"5": "roll", "6": "mailbox", "7": "fan-fold",
}

// FinDeviceTypeTC (RFC 3806).
var snmpFinisherTypes = map[string]string{
	"1": "other", "2": "unknown", "3": "stapler", "4": "folder", "5": "binder",
	"6": "trimmer", "7": "die cutter", "8": "hole punch", "9": "perforator",
	"10": "slitter", "11": "separation cutter", "12": "imprinter", "13": "wrapper",
	"14": "bander", "15": "envelope maker", "16": "stacker", "17": "sheet rotator",
	"18": "inserter",
}

// PresentOnOff (RFC 3805).
var snmpPresentOnOff = map[string]string{"1": "other", "3": "on", "4": "off", "5": "not present"}

// PrtMediaPathTypeTC (RFC 3805).
var snmpMediaPathTypes = map[string]string{
	"3": "long-edge duplex", "4": "short-edge duplex", "5": "simplex",
}

// inventoryFromSNMP turns walk values into the hardware list.
func inventoryFromSNMP(inv *SNMPInventory, vals map[string]string) {
	inv.SystemDescription = vals[oidSysDescr+".0"]
	inv.SystemName = vals[oidSysName+".0"]

	devs := map[string]string{}
	for oid, v := range vals {
		if idx, ok := strings.CutPrefix(oid, oidHrDeviceDescr+"."); ok && strings.TrimSpace(v) != "" {
			devs[idx] = v
		}
	}
	for _, idx := range sortedSNMPIndexes(devs) {
		inv.Devices = append(inv.Devices, devs[idx])
	}

	inputs := snmpRows(vals, oidPrtInputEntry)
	for _, idx := range sortedSNMPIndexes(inputs) {
		r := inputs[idx]
		inv.Inputs = append(inv.Inputs, SNMPInput{
			Index: idx, Type: enumLabel(snmpInputTypes, r[2]),
			Name: r[13], Description: r[18], Media: r[12], Capacity: snmpCapacity(r[9]),
		})
	}
	outputs := snmpRows(vals, oidPrtOutputEntry)
	for _, idx := range sortedSNMPIndexes(outputs) {
		r := outputs[idx]
		inv.Outputs = append(inv.Outputs, SNMPOutput{
			Index: idx, Type: enumLabel(snmpOutputTypes, r[2]),
			Name: r[7], Description: r[12], Capacity: snmpCapacity(r[4]),
		})
	}
	fins := snmpRows(vals, oidFinDeviceEntry)
	for _, idx := range sortedSNMPIndexes(fins) {
		r := fins[idx]
		f := SNMPFinisher{Index: idx, Type: enumLabel(snmpFinisherTypes, r[2]), Description: r[10]}
		if p, ok := r[3]; ok {
			f.Present = enumLabel(snmpPresentOnOff, p)
		}
		inv.Finishers = append(inv.Finishers, f)
	}
	paths := map[string]string{}
	for oid, v := range vals {
		if idx, ok := strings.CutPrefix(oid, oidPrtMediaPathType+"."); ok {
			paths[idx] = v
		}
	}
	for _, idx := range sortedSNMPIndexes(paths) {
		if v := paths[idx]; v == "3" || v == "4" {
			inv.Duplex = append(inv.Duplex, "media path "+idx+": "+snmpMediaPathTypes[v])
		}
	}
}

func inputLabel(in SNMPInput) string {
	name := in.Name
	if name == "" {
		name = in.Description
	}
	if name == "" {
		name = "input " + in.Index
	}
	s := fmt.Sprintf("%q (%s", name, in.Type)
	if in.Capacity > 0 {
		s += fmt.Sprintf(", %d sheets", in.Capacity)
	}
	return s + ")"
}

// capabilitiesFromSNMP maps the inventory onto the same capabilities the IPP
// rules use. Automatic trays are numbered in index order (tray 1 is the
// first), so a printer with two drawers suggests OptTray2.
func capabilitiesFromSNMP(inv *SNMPInventory) printerCapabilities {
	caps := printerCapabilities{trays: map[int]string{}, viaSNMP: true}
	tray := 0
	for _, in := range inv.Inputs {
		// Name and description only: a drawer that holds envelopes right now
		// (prtInputMediaName) is still a drawer.
		text := strings.ToLower(in.Name + " " + in.Description)
		switch {
		case strings.Contains(text, "envelope"):
			caps.envelope = append(caps.envelope, "input "+inputLabel(in))
		case in.Type == snmpInputTypes["5"]:
			caps.bypass = append(caps.bypass, "input "+inputLabel(in))
		case in.Type == snmpInputTypes["3"] || in.Type == snmpInputTypes["4"]:
			tray++
			caps.trays[tray] = "input " + inputLabel(in)
		}
	}
	for _, f := range inv.Finishers {
		if f.Present == "not present" || f.Present == "off" {
			continue
		}
		ev := "finisher " + f.Type
		if f.Description != "" {
			ev += fmt.Sprintf(" (%s)", f.Description)
		}
		switch f.Type {
		case snmpFinisherTypes["3"]:
			caps.staple = append(caps.staple, ev)
		case snmpFinisherTypes["8"]:
			caps.punch = append(caps.punch, ev)
		case snmpFinisherTypes["4"]:
			caps.fold = append(caps.fold, ev)
		}
	}
	caps.duplex = append(caps.duplex, inv.Duplex...)
	return caps
}

// SNMPFeatures returns the optional-feature settings for the settings screen.
func (m *AccessoryManager) SNMPFeatures() *AccessoryFeatureStatus {
	st := &AccessoryFeatureStatus{SNMPAvailable: m.Source.SNMPAvailable(), IsAdmin: m.Source.IsAdmin()}
	f, err := m.Features.Load()
	st.AccessoryFeatures = f
	if err != nil {
		st.Error = err.Error()
	}
	return st
}

// SetSNMPFeatures saves the optional-feature settings. Only CUPS administrators can change them.
func (m *AccessoryManager) SetSNMPFeatures(f AccessoryFeatures) (*AccessoryFeatureStatus, error) {
	if !m.Source.IsAdmin() {
		return nil, ErrNotCUPSAdmin
	}
	if err := m.Features.Save(f); err != nil {
		return nil, fmt.Errorf("cannot save the feature settings: %w", err)
	}
	return m.SNMPFeatures(), nil
}

// SNMPCheck walks the printer at host over SNMP and reports its installed
// hardware, with suggestions where the driver has rules. It never changes the queue.
func (m *AccessoryManager) SNMPCheck(name, host, community string) (*SNMPInventory, error) {
	if !IsValidPrinterName(name) {
		return nil, &AccessoryInputError{Msg: fmt.Sprintf("invalid printer name: %q", name)}
	}
	if f, err := m.Features.Load(); err != nil || !f.SNMPEnabled {
		return nil, ErrSNMPDisabled
	}
	if !m.Source.IsAdmin() {
		return nil, ErrNotCUPSAdmin
	}
	if !IsValidSNMPHost(host) {
		return nil, &AccessoryInputError{Msg: "enter the printer's IP address or host name, without a port or http://"}
	}
	if !IsValidSNMPCommunity(community) {
		return nil, &AccessoryInputError{Msg: "the community name must be 1 to 64 characters with no spaces"}
	}
	rep, err := m.Report(name)
	if err != nil {
		return nil, err
	}
	inv := &SNMPInventory{
		PrinterName: name, Host: host,
		Devices: []string{}, Inputs: []SNMPInput{}, Outputs: []SNMPOutput{},
		Finishers: []SNMPFinisher{}, Duplex: []string{}, Suggestions: []AccessorySuggestion{},
	}
	if !m.Source.SNMPAvailable() {
		inv.Note = "snmpwalk isn't installed on this Mac, so the printer can't be checked over SNMP."
		return inv, nil
	}
	out, err := m.Source.SNMPWalk(host, community, SNMPRoots)
	if err != nil {
		inv.Note = "The printer didn't answer over SNMP: " + err.Error() + ". Check the address, the community name, and that this network can reach the printer."
		return inv, nil
	}
	inv.Queried = true
	inventoryFromSNMP(inv, ParseSNMPWalk(out))

	var notes []string
	if len(inv.Inputs) == 0 && len(inv.Outputs) == 0 && len(inv.Finishers) == 0 {
		notes = append(notes, "The device answered but reported no Printer MIB tables. It may be a print server, not the printer.")
	}
	switch {
	case !rep.Available:
		notes = append(notes, "This queue has no installable options, so there is nothing to fill in.")
	case accessorySuggestionRules[rep.DriverModel] == nil:
		notes = append(notes, fmt.Sprintf("No suggestion rules for the %q driver: use the list above to set the accessories by hand.", rep.DriverModel))
	default:
		inv.Suggestions = suggestFromCaps(rep, capabilitiesFromSNMP(inv))
		if len(inv.Suggestions) == 0 {
			notes = append(notes, "The hardware the printer reports doesn't suggest any changes.")
		}
	}
	inv.Note = strings.Join(notes, " ")
	return inv, nil
}
