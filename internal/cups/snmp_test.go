package cups

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// snmpWalkSample has the value forms snmpwalk -On prints: quoted and
// escaped strings, a string that wraps onto a second line, Hex-STRING text
// that wraps, an enum printed by name, an empty string, and an error row.
const snmpWalkSample = `.1.3.6.1.2.1.1.1.0 = STRING: "Xerox AltaLink C8155; SS 1.0, \"Net\" 2.0"
.1.3.6.1.2.1.1.5.0 = STRING: "XRX-Office"
.1.3.6.1.2.1.1.4.0 = STRING: cupswebui known-success control \"quoted\" C:\\temp
.1.3.6.1.2.1.25.3.2.1.3.1 = STRING: "Xerox AltaLink C8155"
.1.3.6.1.2.1.25.3.2.1.3.2 = STRING: "Line one
line two"
.1.3.6.1.2.1.43.8.2.1.2.1.1 = INTEGER: sheetFeedAutoRemovableTray(3)
.1.3.6.1.2.1.43.8.2.1.13.1.1 = Hex-STRING: 54 72 61 79 20 31 00
.1.3.6.1.2.1.43.8.2.1.18.1.1 = Hex-STRING: 4C 65 74 74 65 72 20 64 72 61 77 65 72 20
4F 6E 65
.1.3.6.1.2.1.43.8.2.1.9.1.1 = INTEGER: -2
.1.3.6.1.2.1.43.8.2.1.12.1.1 = ""
.1.3.6.1.2.1.43.30.1.1.2.1.1 = No Such Object available on this agent at this OID
`

func TestParseSNMPWalk(t *testing.T) {
	v := ParseSNMPWalk(snmpWalkSample)
	want := map[string]string{
		"1.3.6.1.2.1.1.1.0":           `Xerox AltaLink C8155; SS 1.0, "Net" 2.0`,
		"1.3.6.1.2.1.25.3.2.1.3.2":    "Line one\nline two",
		"1.3.6.1.2.1.1.4.0":           `cupswebui known-success control "quoted" C:\temp`,
		"1.3.6.1.2.1.43.8.2.1.2.1.1":  "3",
		"1.3.6.1.2.1.43.8.2.1.13.1.1": "Tray 1",
		"1.3.6.1.2.1.43.8.2.1.18.1.1": "Letter drawer One",
		"1.3.6.1.2.1.43.8.2.1.9.1.1":  "-2",
		"1.3.6.1.2.1.43.8.2.1.12.1.1": "",
	}
	for oid, w := range want {
		if got, ok := v[oid]; !ok || got != w {
			t.Errorf("%s = %q (present %v), want %q", oid, got, ok, w)
		}
	}
	if _, ok := v["1.3.6.1.2.1.43.30.1.1.2.1.1"]; ok {
		t.Error("an error row must not become a value")
	}
	if len(ParseSNMPWalk("Timeout: No Response from 10.0.0.9\n")) != 0 {
		t.Error("a timeout message must parse to nothing")
	}

	inv := &SNMPInventory{}
	inventoryFromSNMP(inv, v)
	if len(inv.Inputs) != 1 || inv.Inputs[0].Name != "Tray 1" || inv.Inputs[0].Type != "removable tray" || inv.Inputs[0].Capacity != 0 {
		t.Errorf("inputs = %+v", inv.Inputs)
	}
	if len(inv.Devices) != 2 || inv.SystemName != "XRX-Office" {
		t.Errorf("devices %v name %q", inv.Devices, inv.SystemName)
	}
}

func TestSNMPInputValidation(t *testing.T) {
	for _, h := range []string{"10.1.2.3", "printer.example.edu", "PRN-01", "fe80::1", "printer.local."} {
		if !IsValidSNMPHost(h) {
			t.Errorf("host %q should be accepted", h)
		}
	}
	for _, h := range []string{"", "-c", "-Ofoo", "a;b", "10.1.2.3:161", "http://10.1.2.3", "udp:10.1.2.3", "a b", "x/y", strings.Repeat("a", 254), "bad-.example"} {
		if IsValidSNMPHost(h) {
			t.Errorf("host %q should be refused", h)
		}
	}
	for _, c := range []string{"public", "S3cr3t!#"} {
		if !IsValidSNMPCommunity(c) {
			t.Errorf("community %q should be accepted", c)
		}
	}
	for _, c := range []string{"", "two words", "line\nbreak", strings.Repeat("c", 65), "café"} {
		if IsValidSNMPCommunity(c) {
			t.Errorf("community %q should be refused", c)
		}
	}
}

func TestSNMPCheck(t *testing.T) {
	src := newFakeSource()
	src.snmpOut = mockSNMPWalk
	m := NewAccessoryManager(src, nil)

	// Off by default: refused before anything is sent.
	if _, err := m.SNMPCheck("Q", "10.1.2.3", "public"); !errors.Is(err, ErrSNMPDisabled) {
		t.Fatalf("disabled: %v", err)
	}
	if rep, _ := m.Report("Q"); rep.SNMPEnabled {
		t.Error("report says SNMP is on")
	}

	src.admin = false
	if _, err := m.SetSNMPFeatures(AccessoryFeatures{SNMPEnabled: true}); !errors.Is(err, ErrNotCUPSAdmin) {
		t.Fatalf("non-admin turned the feature on: %v", err)
	}
	src.admin = true
	if st, err := m.SetSNMPFeatures(AccessoryFeatures{SNMPEnabled: true}); err != nil || !st.SNMPEnabled || !st.SNMPAvailable {
		t.Fatalf("enable: %+v %v", st, err)
	}
	if rep, _ := m.Report("Q"); !rep.SNMPEnabled {
		t.Error("report doesn't say SNMP is on")
	}

	src.admin = false
	if _, err := m.SNMPCheck("Q", "10.1.2.3", "public"); !errors.Is(err, ErrNotCUPSAdmin) {
		t.Errorf("non-admin: %v", err)
	}
	src.admin = true
	var inputErr *AccessoryInputError
	if _, err := m.SNMPCheck("Q", "-c", "public"); !errors.As(err, &inputErr) {
		t.Errorf("bad host: %v", err)
	}
	if _, err := m.SNMPCheck("Q", "10.1.2.3", "has space"); !errors.As(err, &inputErr) {
		t.Errorf("bad community: %v", err)
	}
	if src.snmpCalls != 0 {
		t.Fatalf("refused requests reached snmpwalk %d times", src.snmpCalls)
	}

	inv, err := m.SNMPCheck("Q", "10.1.2.3", "public")
	if err != nil || !inv.Queried {
		t.Fatalf("%+v %v", inv, err)
	}
	if len(inv.Inputs) != 3 || len(inv.Outputs) != 1 || len(inv.Finishers) != 2 || len(inv.Duplex) != 1 {
		t.Errorf("inventory: %+v", inv)
	}
	got := map[string]string{}
	for _, s := range inv.Suggestions {
		got[s.Key] = s.Evidence
	}
	// Two drawers, a stapler and a punch. Duplex is already True in the PPD.
	if len(got) != 3 || !strings.Contains(got["OptTray2"], "Tray 2") || !strings.Contains(got["OptStapler"], "stapler") || !strings.HasPrefix(got["OptHolePunch"], "SNMP: ") {
		t.Errorf("suggestions = %v", got)
	}
	if len(src.applied) != 0 {
		t.Error("the SNMP check must never change the queue")
	}

	// A drawer that holds envelopes is still a drawer; an envelope feeder is not.
	withEnv := strings.Replace(mockSNMPWalk, `.1.3.6.1.2.1.43.8.2.1.13.1.2 = STRING: "Tray 2"`,
		".1.3.6.1.2.1.43.8.2.1.13.1.2 = STRING: \"Tray 2\"\n.1.3.6.1.2.1.43.8.2.1.12.1.2 = STRING: \"Envelope #10\"", 1)
	src.snmpOut = withEnv
	inv, _ = m.SNMPCheck("Q", "10.1.2.3", "public")
	if !hasSuggestion(inv, "OptTray2") {
		t.Errorf("drawer with envelope media lost its tray: %+v", inv.Suggestions)
	}
	src.snmpOut = strings.Replace(mockSNMPWalk, `STRING: "Tray 2"`, `STRING: "Envelope Feeder"`, 1)
	inv, _ = m.SNMPCheck("Q", "10.1.2.3", "public")
	if hasSuggestion(inv, "OptTray2") {
		t.Errorf("an envelope feeder counted as a drawer: %+v", inv.Suggestions)
	}

	// A finisher reported as not present suggests nothing.
	src.snmpOut = strings.Replace(mockSNMPWalk, ".1.3.6.1.2.1.43.30.1.1.3.2.1 = INTEGER: 3", ".1.3.6.1.2.1.43.30.1.1.3.2.1 = INTEGER: 5", 1)
	inv, _ = m.SNMPCheck("Q", "10.1.2.3", "public")
	for _, s := range inv.Suggestions {
		if s.Key == "OptStapler" {
			t.Error("a stapler that is not present was suggested")
		}
	}

	// Unknown driver: the hardware list still comes back.
	src.snmpOut = mockSNMPWalk
	src.queuePPD["Q"] = strings.ReplaceAll(samplePPD, "Generic Universal Mono", "Xerox AltaLink C8155")
	inv, _ = m.SNMPCheck("Q", "10.1.2.3", "public")
	if !inv.Queried || len(inv.Inputs) != 3 || len(inv.Suggestions) != 0 || !strings.Contains(inv.Note, "No suggestion rules") {
		t.Errorf("unknown driver: %+v", inv)
	}

	// No answer (the known-failure control).
	src.snmpErr = errors.New("no response from 10.1.2.3")
	inv, err = m.SNMPCheck("Q", "10.1.2.3", "public")
	if err != nil || inv.Queried || len(inv.Inputs) != 0 || !strings.Contains(inv.Note, "didn't answer") {
		t.Errorf("no answer: %+v %v", inv, err)
	}

	src.snmpErr = nil
	src.noSNMP = true
	calls := src.snmpCalls
	if inv, _ = m.SNMPCheck("Q", "10.1.2.3", "public"); inv.Queried || !strings.Contains(inv.Note, "isn't installed") || src.snmpCalls != calls {
		t.Errorf("no snmpwalk: %+v", inv)
	}
}

func hasSuggestion(inv *SNMPInventory, key string) bool {
	for _, s := range inv.Suggestions {
		if s.Key == key {
			return true
		}
	}
	return false
}

func TestSNMPHostHint(t *testing.T) {
	src := newFakeSource()
	m := NewAccessoryManager(src, nil)
	cases := map[string]string{
		"socket://192.168.1.5:9100":                                 "192.168.1.5",
		"ipp://printer.example.edu/ipp/print":                       "printer.example.edu",
		"ipp://127.0.0.1:8699/ipp/print":                            "",
		"ipps://printserver.example.com:9164/printers/Front%20Desk": "",
		"usb://Xerox/AltaLink?serial=123":                           "",
	}
	for uri, want := range cases {
		src.uri = uri
		rep, err := m.Report("Q")
		if err != nil || rep.SNMPHostHint != want {
			t.Errorf("%s: hint %q, want %q (%v)", uri, rep.SNMPHostHint, want, err)
		}
	}
}

func TestFeatureStoreFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "features.json")
	s := NewFeatureStore(path)
	if f, err := s.Load(); err != nil || f.SNMPEnabled {
		t.Fatalf("missing file: %+v %v", f, err)
	}
	if err := s.Save(AccessoryFeatures{SNMPEnabled: true}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0600 {
		t.Fatalf("mode: %v %v", fi.Mode(), err)
	}
	if f, err := NewFeatureStore(path).Load(); err != nil || !f.SNMPEnabled {
		t.Fatalf("reload: %+v %v", f, err)
	}

	// A damaged file keeps the feature off.
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	src := newFakeSource()
	m := NewAccessoryManager(src, nil)
	m.Features = NewFeatureStore(path)
	if _, err := m.SNMPCheck("Q", "10.1.2.3", "public"); !errors.Is(err, ErrSNMPDisabled) {
		t.Errorf("damaged file: %v", err)
	}
	if st := m.SNMPFeatures(); st.SNMPEnabled || st.Error == "" {
		t.Errorf("status: %+v", st)
	}
	if f, err := NewFeatureStore("").Load(); err == nil || f.SNMPEnabled {
		t.Error("an unknown location must keep features off with an error")
	}
}
