package cups

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// samplePPD is trimmed from "Generic Universal Mono" (FUJIFILM / Lexmark),
// including a multi-line quoted value whose lines look like PPD keywords.
const samplePPD = `*PPD-Adobe: "4.3"
*PCFileName: "GNGUPDM.PPD"
*ModelName: "Generic Universal Mono"
*NickName: "Generic Universal Mono"
*APAutoSetupTool: "/Library/Printers/Generic/AutoConfigurationToolG2"
*OpenGroup: Finishing/Finishing
*OpenUI *StapleJob/Staple Job: PickOne
*DefaultStapleJob: PrinterS
*StapleJob PrinterS/Printer Setting: ""
*StapleJob TopLeft/Top Left: "<< /Staple 14 >> setpagedevice"
*CloseUI: *StapleJob
*CloseGroup: Finishing
*UIConstraints: *OptStapler False *StapleJob TopLeft
*OpenGroup: InstallableOptions/Installable Options
*OpenUI *OptDuplex/Duplexer: Boolean
*OrderDependency: 0 AnySetup *OptDuplex
*DefaultOptDuplex: True
*OptDuplex False/Not Installed: ""
*OptDuplex True/Installed: ""
*CloseUI: *OptDuplex
*OpenUI *OptTray2/Tray 2: Boolean
*DefaultOptTray2: False
*OptTray2 False/Not Installed: ""
*OptTray2 True/Installed: "
*OptTray2 Bogus/Not a real choice: ""
*OptTray2 Bogus2/Also not a choice: ""
"
*End
*CloseUI: *OptTray2
*OpenUI *OptStapler/Staple: Boolean
*DefaultOptStapler: False
*OptStapler False/Not Installed: ""
*OptStapler True/Installed: ""
*CloseUI: *OptStapler
*OpenUI *OptHolePunch/Hole Punch: Boolean
*DefaultOptHolePunch: False
*OptHolePunch False/Not Installed: ""
*OptHolePunch True/Installed: ""
*CloseUI: *OptHolePunch
*CloseGroup: InstallableOptions
*OpenGroup: Paper/Paper
*OpenUI *OptNotInstallable/Not installable: Boolean
*DefaultOptNotInstallable: False
*CloseUI: *OptNotInstallable
*CloseGroup: Paper
`

func setDefault(ppd, key, value string) string {
	re := regexp.MustCompile(`(?m)^\*Default` + key + `: .*$`)
	return re.ReplaceAllString(ppd, "*Default"+key+": "+value)
}

func TestParsePPDSummary(t *testing.T) {
	s := ParsePPDSummary([]byte(samplePPD))
	if s.NickName != "Generic Universal Mono" || s.PCFileName != "GNGUPDM.PPD" {
		t.Fatalf("metadata: %+v", s)
	}
	if !s.HasInstallableGroup || s.Driverless {
		t.Fatalf("group/driverless wrong: %+v", s)
	}
	var keys []string
	for _, o := range s.Installable {
		keys = append(keys, o.Key)
	}
	if got := strings.Join(keys, ","); got != "OptDuplex,OptTray2,OptStapler,OptHolePunch" {
		t.Fatalf("installable keys = %s", got)
	}
	tray := s.Option("OptTray2")
	if tray.Label != "Tray 2" || tray.Default != "False" || len(tray.Choices) != 2 {
		t.Fatalf("OptTray2 = %+v (the quoted value must not add choices)", tray)
	}
	if tray.Choices[1].Name != "True" || tray.Choices[1].Label != "Installed" || !tray.Choices[0].IsDefault {
		t.Fatalf("OptTray2 choices = %+v", tray.Choices)
	}
	if s.Option("OptStapler").Default != "False" || s.Option("OptDuplex").Default != "True" {
		t.Fatal("defaults not read")
	}
	if s.Option("StapleJob") != nil || s.Option("OptNotInstallable") != nil {
		t.Fatal("options outside InstallableOptions must be ignored")
	}
}

func TestDecodePPDText(t *testing.T) {
	cases := map[string]string{
		"2<2F>4 Holes":   "2/4 Holes",
		"<4142 43>":      "ABC",
		"plain":          "plain",
		"a<zz>b":         "a<zz>b",
		"<2F":            "<2F",
		"Tray <33> only": "Tray 3 only",
	}
	for in, want := range cases {
		if got := decodePPDText(in); got != want {
			t.Errorf("decodePPDText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParsePPDSummaryLineEndings(t *testing.T) {
	for name, sep := range map[string]string{"CRLF": "\r\n", "CR": "\r"} {
		s := ParsePPDSummary([]byte(strings.ReplaceAll(samplePPD, "\n", sep)))
		if len(s.Installable) != 4 || s.Option("OptStapler").Default != "False" {
			t.Errorf("%s: got %d options", name, len(s.Installable))
		}
	}
}

func TestParsePPDSummaryDriverless(t *testing.T) {
	cases := map[string]string{
		"airprint": "*PPD-Adobe: \"4.3\"\n*APAirPrint: True\n*NickName: \"HP ENVY-AirPrint\"\n",
		"ipp2ppd":  "*PPD-Adobe: \"4.3\"\n*% PPD created by ipp2ppd (v4)\n*NickName: \"X\"\n",
		"ippeve":   "*PPD-Adobe: \"4.3\"\n*PCFileName: \"ippeve.ppd\"\n*NickName: \"PaperCut - IPP Everywhere\"\n",
	}
	for name, ppd := range cases {
		if !ParsePPDSummary([]byte(ppd)).Driverless {
			t.Errorf("%s: not detected as driverless", name)
		}
	}
	if ParsePPDSummary([]byte(samplePPD)).Driverless {
		t.Error("vendor PPD detected as driverless")
	}
}

func TestClassifyDeviceURI(t *testing.T) {
	cases := []struct {
		uri    string
		direct bool
	}{
		{"socket://192.168.1.20:9100", true},
		{"lpd://printer.example.com/queue", true},
		{"ipp://192.168.1.65/ipp/print", true},
		{"ipps://printer.local:631/ipp/print", true},
		{"ipp://127.0.0.1:8699/ipp/print", false},
		{"http://127.0.0.1:9167/ipp/localhost_9164/abc/printers/demo", false},
		{"ipp://localhost:631/printers/Office", false},
		{"ipp://[::1]:631/ipp/print", false},
		{"ipp://printserver.example.com/printers/Floor2", false},
		{"dnssd://HP%20ENVY%20Photo%207800%20series%20%5BC58A29%5D._ipps._tcp.local./?uuid=688c", false},
		{"smb://server/printer", false},
		{"file:/dev/null", false},
	}
	for _, c := range cases {
		direct, note := ClassifyDeviceURI(c.uri)
		if direct != c.direct || note == "" {
			t.Errorf("%s: direct=%v note=%q, want direct=%v", c.uri, direct, note, c.direct)
		}
	}
	if _, note := ClassifyDeviceURI("dnssd://HP%20ENVY._ipps._tcp.local./?uuid=1"); !strings.Contains(note, "Bonjour") {
		t.Errorf("dnssd note = %q", note)
	}
}

const ippToolSample = `"get-printer-attributes.test":
    Get-Printer-Attributes:
        attributes-charset (charset) = utf-8
    RECEIVED: 4321 bytes in response
        finishings-supported (1setOf enum) = none,staple-top-left,punch-dual-left
        media-source-supported (1setOf keyword) = auto,tray-1,tray-2,tray-3,by-pass-tray
        sides-supported (1setOf keyword) = one-sided,two-sided-long-edge
        printer-make-and-model (textWithoutLanguage) = Demo, Model 2
`

func TestParseIPPToolAttributes(t *testing.T) {
	a := ParseIPPToolAttributes(ippToolSample)
	if got := strings.Join(a["finishings-supported"], "|"); got != "none|staple-top-left|punch-dual-left" {
		t.Errorf("finishings = %s", got)
	}
	if len(a["media-source-supported"]) != 5 || len(a["sides-supported"]) != 2 {
		t.Errorf("attrs = %v", a)
	}
}

func TestCapabilitiesNumericEnums(t *testing.T) {
	for _, n := range []string{"4", "20", "23", "28", "35"} {
		if c := capabilitiesFromIPP(map[string][]string{"finishings-supported": {n}}); len(c.staple) != 1 {
			t.Errorf("enum %s should mean staple", n)
		}
	}
	for _, n := range []string{"3", "24", "27", "36"} {
		if c := capabilitiesFromIPP(map[string][]string{"finishings-supported": {n}}); len(c.staple) != 0 {
			t.Errorf("enum %s should not mean staple", n)
		}
	}
	if c := capabilitiesFromIPP(map[string][]string{"finishings-supported": {"5", "74", "93"}}); len(c.punch) != 2 || len(c.fold) != 1 {
		t.Errorf("punch/fold enums: %+v", c)
	}
}

// fakeSource is an in-memory AccessorySource that behaves like lpadmin -o.
type fakeSource struct {
	queuePPD   map[string]string
	driverPPD  string
	uri        string
	admin      bool
	attrs      string
	attrsErr   error
	applied    []map[string]string
	applyErr   error
	ignoreKeys map[string]bool // keys CUPS "accepts" but doesn't store
	uuids      map[string]string
	queues     []string // nil: every queue in queuePPD
	afterApply func()
}

func newFakeSource() *fakeSource {
	return &fakeSource{
		queuePPD:  map[string]string{"Q": samplePPD, "Other": samplePPD},
		driverPPD: samplePPD,
		uri:       "ipp://127.0.0.1:8699/ipp/print",
		admin:     true,
	}
}

func (f *fakeSource) QueuePPD(name string) ([]byte, error) {
	p, ok := f.queuePPD[name]
	if !ok {
		return nil, fmt.Errorf("no PPD")
	}
	return []byte(p), nil
}
func (f *fakeSource) DriverPPD(nick string) (string, []byte, error) {
	if f.driverPPD == "" {
		return "", nil, fmt.Errorf("unknown driver")
	}
	return "fake.ppd", []byte(f.driverPPD), nil
}
func (f *fakeSource) DeviceURI(name string) (string, error) {
	if _, ok := f.queuePPD[name]; !ok && name != "NoPPD" {
		return "", fmt.Errorf("printer %q not found", name)
	}
	return f.uri, nil
}
func (f *fakeSource) ApplyOptions(name string, opts map[string]string) error {
	if f.applyErr != nil {
		return f.applyErr
	}
	f.applied = append(f.applied, opts)
	if f.afterApply != nil {
		defer f.afterApply()
	}
	for k, v := range opts {
		if !f.ignoreKeys[k] {
			f.queuePPD[name] = setDefault(f.queuePPD[name], k, v)
		}
	}
	return nil
}
func (f *fakeSource) PrinterAttributes(uri string) (string, error) { return f.attrs, f.attrsErr }
func (f *fakeSource) IsAdmin() bool                                { return f.admin }
func (f *fakeSource) UserName() string                             { return "alice" }
func (f *fakeSource) QueueUUID(name string) (string, error) {
	if u, ok := f.uuids[name]; ok {
		return u, nil
	}
	return "urn:uuid:" + strings.ToLower(name), nil
}
func (f *fakeSource) QueueNames() ([]string, error) {
	if f.queues != nil {
		return f.queues, nil
	}
	var names []string
	for n := range f.queuePPD {
		names = append(names, n)
	}
	return names, nil
}

func optionByKey(t *testing.T, rep *AccessoryReport, key string) AccessoryOption {
	t.Helper()
	for _, o := range rep.Options {
		if o.Key == key {
			return o
		}
	}
	t.Fatalf("option %s missing", key)
	return AccessoryOption{}
}

func TestAccessoryReportSources(t *testing.T) {
	src := newFakeSource()
	// The driver's auto-setup found a second tray when the queue was added.
	src.queuePPD["Q"] = setDefault(samplePPD, "OptTray2", "True")
	m := NewAccessoryManager(src, nil)

	rep, err := m.Report("Q")
	if err != nil || !rep.Available || !rep.IsAdmin || !rep.DriverDefaultsKnown {
		t.Fatalf("report = %+v, %v", rep, err)
	}
	if rep.DirectConnection {
		t.Error("loopback IPP must not be a direct connection")
	}
	if o := optionByKey(t, rep, "OptTray2"); o.Source != AccessorySourceDetected || o.Value != "True" || o.DriverDefault != "False" {
		t.Errorf("OptTray2 = %+v", o)
	}
	if o := optionByKey(t, rep, "OptStapler"); o.Source != AccessorySourceDriverDefault {
		t.Errorf("OptStapler = %+v", o)
	}

	src.driverPPD = ""
	rep, _ = m.Report("Q")
	if rep.DriverDefaultsKnown || optionByKey(t, rep, "OptTray2").Source != AccessorySourceUnknown {
		t.Error("without a driver PPD the source must be unknown")
	}
}

func TestAccessoryReportUnavailable(t *testing.T) {
	src := newFakeSource()
	src.queuePPD["Airprint"] = "*PPD-Adobe: \"4.3\"\n*APAirPrint: True\n*OpenGroup: InstallableOptions\n*OpenUI *Duplexer/Duplexer: Boolean\n*CloseUI: *Duplexer\n*CloseGroup: InstallableOptions\n"
	src.queuePPD["Plain"] = "*PPD-Adobe: \"4.3\"\n*NickName: \"Plain\"\n"
	m := NewAccessoryManager(src, nil)
	for name, want := range map[string]string{
		"Airprint": AccessoryUnavailableDriverless,
		"Plain":    AccessoryUnavailableNoInstallable,
		"NoPPD":    AccessoryUnavailableNoPPD,
	} {
		rep, err := m.Report(name)
		if err != nil || rep.Available || rep.UnavailableReason != want {
			t.Errorf("%s: %+v %v, want %s", name, rep, err, want)
		}
	}
	if _, err := m.Report("Missing"); err == nil {
		t.Error("missing queue should be an error")
	}
	if _, err := m.Report("-bad"); err == nil {
		t.Error("invalid name should be an error")
	}
}

func TestAccessorySetRecordsOverrideAndVerifies(t *testing.T) {
	src := newFakeSource()
	store := NewOverrideStore(filepath.Join(t.TempDir(), "o.json"))
	m := NewAccessoryManager(src, store)
	m.Now = func() time.Time { return time.Date(2026, 11, 19, 1, 2, 3, 0, time.UTC) }

	rep, err := m.Set("Q", map[string]string{"OptStapler": "True", "OptDuplex": "True"}, true)
	if err != nil {
		t.Fatal(err)
	}
	// OptDuplex was already True: only the real change goes to CUPS.
	if len(src.applied) != 1 || len(src.applied[0]) != 1 || src.applied[0]["OptStapler"] != "True" {
		t.Fatalf("applied = %v", src.applied)
	}
	o := optionByKey(t, rep, "OptStapler")
	if o.Source != AccessorySourceManual || o.Value != "True" || o.Override == nil {
		t.Fatalf("OptStapler = %+v", o)
	}
	if o.Override.SetBy != "alice" || o.Override.PreviousValue != "False" || !o.Override.SetAt.Equal(m.Now()) {
		t.Errorf("override = %+v", o.Override)
	}
	// Other queues are not touched.
	if strings.Contains(src.queuePPD["Other"], "*DefaultOptStapler: True") {
		t.Error("another queue changed")
	}

	// The store persists across managers (a restart).
	m2 := NewAccessoryManager(src, NewOverrideStore(store.path))
	rep2, _ := m2.Report("Q")
	if optionByKey(t, rep2, "OptStapler").Source != AccessorySourceManual {
		t.Error("override not persisted")
	}
	if info, err := os.Stat(store.path); err != nil || info.Mode().Perm() != 0600 {
		t.Errorf("store file mode: %v %v", info, err)
	}

	// Setting it back to the value it had before removes the override.
	rep, err = m.Set("Q", map[string]string{"OptStapler": "False"}, true)
	if err != nil || rep.HasOverrides || optionByKey(t, rep, "OptStapler").Source != AccessorySourceDriverDefault {
		t.Fatalf("after revert: %+v %v", rep, err)
	}
}

func TestAccessorySetKeepsFirstPreviousValue(t *testing.T) {
	src := newFakeSource()
	src.queuePPD["Q"] = strings.Replace(samplePPD, "*OptHolePunch True/Installed: \"\"", "*OptHolePunch True/Installed: \"\"\n*OptHolePunch Pro/Pro unit: \"\"", 1)
	m := NewAccessoryManager(src, nil)
	if _, err := m.Set("Q", map[string]string{"OptHolePunch": "True"}, true); err != nil {
		t.Fatal(err)
	}
	rep, err := m.Set("Q", map[string]string{"OptHolePunch": "Pro"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if ov := optionByKey(t, rep, "OptHolePunch").Override; ov == nil || ov.PreviousValue != "False" || ov.Value != "Pro" {
		t.Errorf("override = %+v", ov)
	}
}

func TestAccessorySetRejects(t *testing.T) {
	src := newFakeSource()
	m := NewAccessoryManager(src, nil)

	if _, err := m.Set("Q", map[string]string{"OptStapler": "True"}, false); !isInputErr(err) {
		t.Errorf("unacknowledged: %v", err)
	}
	for _, vals := range []map[string]string{
		{"OptStapler": "Maybe"},
		{"StapleJob": "TopLeft"},      // not an installable option
		{"OptNotInstallable": "True"}, // outside the group
		{"OptStapler": "True -E"},     // not a PPD choice
		{"-o": "x"},
		{},
	} {
		if _, err := m.Set("Q", vals, true); !isInputErr(err) {
			t.Errorf("%v: err = %v", vals, err)
		}
	}
	src.queuePPD["Plain"] = "*PPD-Adobe: \"4.3\"\n"
	if _, err := m.Set("Plain", map[string]string{"X": "True"}, true); !errors.Is(err, ErrAccessoriesUnavailable) {
		t.Errorf("no installable options: %v", err)
	}
	src.admin = false
	if _, err := m.Set("Q", map[string]string{"OptStapler": "True"}, true); !errors.Is(err, ErrNotCUPSAdmin) {
		t.Errorf("non-admin: %v", err)
	}
	if _, err := m.Reset("Q"); !errors.Is(err, ErrNotCUPSAdmin) {
		t.Errorf("non-admin reset: %v", err)
	}
	if len(src.applied) != 0 {
		t.Errorf("nothing should have been applied: %v", src.applied)
	}
}

func isInputErr(err error) bool {
	var e *AccessoryInputError
	return errors.As(err, &e)
}

func TestAccessorySetDetectsCUPSMismatch(t *testing.T) {
	src := newFakeSource()
	src.ignoreKeys = map[string]bool{"OptStapler": true}
	m := NewAccessoryManager(src, nil)
	rep, err := m.Set("Q", map[string]string{"OptStapler": "True", "OptTray2": "True"}, true)
	var verr *AccessoryVerifyError
	if !errors.As(err, &verr) || !strings.Contains(err.Error(), "CUPS stored False") {
		t.Fatalf("err = %v", err)
	}
	// Only the value CUPS actually stored is recorded as manual.
	if optionByKey(t, rep, "OptStapler").Override != nil || optionByKey(t, rep, "OptTray2").Source != AccessorySourceManual {
		t.Errorf("report = %+v", rep.Options)
	}

	src.ignoreKeys = nil
	src.applyErr = errors.New("lpadmin: Forbidden")
	if _, err := m.Set("Q", map[string]string{"OptFold": "True", "OptStapler": "True"}, true); err == nil {
		t.Error("lpadmin failure must be returned")
	}
}

func TestAccessoryResetRestoresDetectedValues(t *testing.T) {
	src := newFakeSource()
	src.queuePPD["Q"] = setDefault(samplePPD, "OptTray2", "True") // detected
	m := NewAccessoryManager(src, nil)
	if _, err := m.Set("Q", map[string]string{"OptTray2": "False", "OptStapler": "True"}, true); err != nil {
		t.Fatal(err)
	}
	rep, err := m.Reset("Q")
	if err != nil {
		t.Fatal(err)
	}
	if rep.HasOverrides {
		t.Error("overrides not cleared")
	}
	if o := optionByKey(t, rep, "OptTray2"); o.Value != "True" || o.Source != AccessorySourceDetected {
		t.Errorf("OptTray2 = %+v", o)
	}
	if o := optionByKey(t, rep, "OptStapler"); o.Value != "False" {
		t.Errorf("OptStapler = %+v", o)
	}
	// Reset with nothing recorded changes nothing.
	n := len(src.applied)
	if _, err := m.Reset("Q"); err != nil || len(src.applied) != n {
		t.Errorf("empty reset applied something: %v", err)
	}
}

func TestAccessoryReapplyAfterPPDReset(t *testing.T) {
	src := newFakeSource()
	m := NewAccessoryManager(src, nil)
	if _, err := m.Set("Q", map[string]string{"OptStapler": "True"}, true); err != nil {
		t.Fatal(err)
	}
	// Something outside the app (a new PPD, auto-setup) puts the default back.
	src.queuePPD["Q"] = samplePPD
	rep, _ := m.Report("Q")
	if o := optionByKey(t, rep, "OptStapler"); !o.OverrideReplaced || o.Source == AccessorySourceManual {
		t.Errorf("replaced override not flagged: %+v", o)
	}
	if err := m.ReapplyOverrides("Q"); err != nil {
		t.Fatal(err)
	}
	rep, _ = m.Report("Q")
	if o := optionByKey(t, rep, "OptStapler"); o.Value != "True" || o.Source != AccessorySourceManual {
		t.Errorf("after reapply: %+v", o)
	}
	// A new driver without the option: nothing applied, override kept.
	src.queuePPD["Q"] = "*PPD-Adobe: \"4.3\"\n*OpenGroup: InstallableOptions\n*OpenUI *Other/Other: Boolean\n*DefaultOther: False\n*Other False/No: \"\"\n*CloseUI: *Other\n*CloseGroup: InstallableOptions\n"
	n := len(src.applied)
	if err := m.ReapplyOverrides("Q"); err != nil || len(src.applied) != n {
		t.Errorf("reapply on new driver: %v", err)
	}
	if ov, _ := m.Store.ForQueue("Q"); len(ov) != 1 {
		t.Error("override should be kept")
	}
	if err := m.ForgetQueue("Q"); err != nil {
		t.Fatal(err)
	}
	if ov, _ := m.Store.ForQueue("Q"); len(ov) != 0 {
		t.Error("ForgetQueue left overrides")
	}
}

func TestOverrideStoreCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "o.json")
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewOverrideStore(path)
	var corrupt *HistoryCorruptError
	if _, err := s.ForQueue("Q"); !errors.As(err, &corrupt) {
		t.Errorf("corrupt file should be reported: %v", err)
	}
	if err := s.Update("Q", []AccessoryOverride{{Option: "A", Value: "True"}}, nil); err == nil {
		t.Error("update must not overwrite a corrupt file")
	}
	if data, _ := os.ReadFile(path); string(data) != "{not json" {
		t.Error("corrupt file was overwritten")
	}

	// The tab still works: values come from CUPS, changes are refused before CUPS is touched.
	src := newFakeSource()
	m := NewAccessoryManager(src, s)
	rep, err := m.Report("Q")
	if err != nil || !rep.Available || !strings.Contains(rep.HistoryError, "damaged") {
		t.Fatalf("report with corrupt history: %+v %v", rep, err)
	}
	var herr *AccessoryHistoryError
	if _, err := m.Set("Q", map[string]string{"OptStapler": "True"}, true); !errors.As(err, &herr) {
		t.Errorf("Set: %v", err)
	}
	if _, err := m.Reset("Q"); !errors.As(err, &herr) {
		t.Errorf("Reset: %v", err)
	}
	if len(src.applied) != 0 {
		t.Error("CUPS must not be changed when the history can't be recorded")
	}
}

func TestOverrideStoreTwoProcessesShareFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "o.json")
	a, b := NewOverrideStore(path), NewOverrideStore(path)
	// Interleave writes from two stores (two server processes).
	for i := 0; i < 20; i++ {
		s := a
		if i%2 == 1 {
			s = b
		}
		if err := s.Update("Q", []AccessoryOverride{{Option: fmt.Sprintf("Opt%d", i), Value: "True"}}, nil); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []*OverrideStore{a, b} {
		got, err := s.ForQueue("Q")
		if err != nil || len(got) != 20 {
			t.Fatalf("expected 20 notes from both stores, got %d (%v)", len(got), err)
		}
	}
	// Concurrent writers.
	done := make(chan error, 40)
	for i := 0; i < 40; i++ {
		go func(i int) {
			s := a
			if i%2 == 1 {
				s = b
			}
			done <- s.Update("R", []AccessoryOverride{{Option: fmt.Sprintf("Opt%d", i), Value: "True"}}, nil)
		}(i)
	}
	for i := 0; i < 40; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := a.ForQueue("R"); len(got) != 40 {
		t.Errorf("concurrent writers lost updates: %d of 40", len(got))
	}
}

func TestOverrideStoreFileHandling(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "o.json")
	s := NewOverrideStore(path)
	// Reading before anything exists is fine and creates nothing.
	if got, err := s.ForQueue("Q"); err != nil || len(got) != 0 {
		t.Fatalf("empty read: %v %v", got, err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Error("a read must not create the config folder")
	}
	if err := s.Update("Q", []AccessoryOverride{{Option: "A", Value: "True"}}, nil); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Errorf("file mode: %v %v", info, err)
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil || info.Mode().Perm() != 0700 {
		t.Errorf("folder mode: %v %v", info, err)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temporary file left behind: %s", e.Name())
		}
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"version": 1`) {
		t.Errorf("version not written: %s", data)
	}
	// Queue names match case-insensitively, like CUPS.
	if got, _ := s.ForQueue("q"); len(got) != 1 {
		t.Error("lookup should ignore case")
	}
	if err := s.DeleteQueue("q"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ForQueue("Q"); len(got) != 0 {
		t.Error("DeleteQueue should ignore case")
	}
}

func TestOverrideStoreNewerVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "o.json")
	newer := `{"version": 2, "overrides": [{"queue": "Q", "option": "A", "value": "True", "future_field": "x"}]}`
	if err := os.WriteFile(path, []byte(newer), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewOverrideStore(path)
	if got, err := s.ForQueue("Q"); err != nil || got["A"].Value != "True" {
		t.Errorf("newer file should still be readable: %v %v", got, err)
	}
	if err := s.Writable(); !errors.Is(err, ErrHistoryTooNew) {
		t.Errorf("Writable: %v", err)
	}
	if err := s.Update("Q", []AccessoryOverride{{Option: "B", Value: "True"}}, nil); !errors.Is(err, ErrHistoryTooNew) {
		t.Errorf("Update: %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != newer {
		t.Error("a newer file must not be rewritten")
	}
}

func TestOverrideStoreUnavailable(t *testing.T) {
	s := NewOverrideStore("")
	if err := s.Writable(); !errors.Is(err, ErrHistoryUnavailable) {
		t.Errorf("Writable: %v", err)
	}
	src := newFakeSource()
	m := NewAccessoryManager(src, s)
	var herr *AccessoryHistoryError
	if _, err := m.Set("Q", map[string]string{"OptStapler": "True"}, true); !errors.As(err, &herr) || !errors.Is(err, ErrHistoryUnavailable) {
		t.Errorf("Set: %v", err)
	}
	if len(src.applied) != 0 {
		t.Error("CUPS must not be changed without a place to record it")
	}
}

func TestAccessoryNotesIgnoreRecreatedQueue(t *testing.T) {
	src := newFakeSource()
	m := NewAccessoryManager(src, nil)
	if _, err := m.Set("Q", map[string]string{"OptStapler": "True"}, true); err != nil {
		t.Fatal(err)
	}
	if ov, _ := m.Store.ForQueue("Q"); ov["OptStapler"].QueueUUID != "urn:uuid:q" {
		t.Errorf("uuid not recorded: %+v", ov)
	}
	// The queue is deleted outside the app and re-created with the same name.
	src.uuids = map[string]string{"Q": "urn:uuid:new"}
	src.queuePPD["Q"] = setDefault(samplePPD, "OptStapler", "True")
	rep, _ := m.Report("Q")
	if o := optionByKey(t, rep, "OptStapler"); o.Source == AccessorySourceManual || o.Override != nil || rep.IgnoredNotes != 1 || rep.HasOverrides {
		t.Errorf("old notes should be ignored: %+v ignored=%d", o, rep.IgnoredNotes)
	}
	// Reapply must not push the old queue's settings onto the new one.
	src.queuePPD["Q"] = samplePPD
	n := len(src.applied)
	if err := m.ReapplyOverrides("Q"); err != nil || len(src.applied) != n {
		t.Errorf("reapply used stale notes: %v", err)
	}
	// The next change drops the stale notes.
	if _, err := m.Set("Q", map[string]string{"OptHolePunch": "True"}, true); err != nil {
		t.Fatal(err)
	}
	ov, _ := m.Store.ForQueue("Q")
	if _, ok := ov["OptStapler"]; ok || ov["OptHolePunch"].QueueUUID != "urn:uuid:new" {
		t.Errorf("after change: %+v", ov)
	}
}

func TestAccessoryPrunesDeletedQueues(t *testing.T) {
	src := newFakeSource()
	m := NewAccessoryManager(src, nil)
	if _, err := m.Set("Other", map[string]string{"OptStapler": "True"}, true); err != nil {
		t.Fatal(err)
	}
	src.queues = []string{"Q"} // "Other" was deleted outside the app
	if _, err := m.Set("Q", map[string]string{"OptStapler": "True"}, true); err != nil {
		t.Fatal(err)
	}
	if ov, _ := m.Store.ForQueue("Other"); len(ov) != 0 {
		t.Error("notes for a deleted queue were kept")
	}
	// If the queue list can't be read, nothing is pruned.
	src.queues = []string{}
	if _, err := m.Set("Q", map[string]string{"OptHolePunch": "True"}, true); err != nil {
		t.Fatal(err)
	}
	if ov, _ := m.Store.ForQueue("Q"); len(ov) != 2 {
		t.Error("an empty queue list must not prune everything")
	}
}

func TestAccessorySetWarnsWhenHistoryWriteFails(t *testing.T) {
	dir := t.TempDir()
	src := newFakeSource()
	m := NewAccessoryManager(src, NewOverrideStore(filepath.Join(dir, "o.json")))
	// The folder becomes read-only after CUPS was changed.
	src.afterApply = func() { _ = os.Chmod(dir, 0500) }
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
	rep, err := m.Set("Q", map[string]string{"OptStapler": "True"}, true)
	if err != nil {
		t.Fatalf("CUPS was changed, so this is not a failure: %v", err)
	}
	if !strings.Contains(rep.Warning, "Saved to CUPS, but the change history couldn't be recorded") {
		t.Errorf("warning = %q", rep.Warning)
	}
	if o := optionByKey(t, rep, "OptStapler"); o.Value != "True" {
		t.Errorf("report should show what CUPS stored: %+v", o)
	}
}

func TestAccessorySuggestions(t *testing.T) {
	src := newFakeSource()
	src.attrs = ippToolSample
	m := NewAccessoryManager(src, nil)
	res, err := m.Suggest("Q")
	if err != nil || !res.Queried {
		t.Fatalf("%+v %v", res, err)
	}
	got := map[string]string{}
	for _, s := range res.Suggestions {
		got[s.Key] = s.Evidence
	}
	// Stapler, hole punch and tray 2. Duplex is already True. No OptTray3 in this PPD.
	if len(got) != 3 || !strings.Contains(got["OptStapler"], "staple-top-left") || got["OptHolePunch"] == "" || got["OptTray2"] == "" {
		t.Errorf("suggestions = %v", got)
	}
	if !strings.Contains(res.Note, "doesn't point straight at the printer") {
		t.Errorf("loopback note missing: %q", res.Note)
	}
	if len(src.applied) != 0 {
		t.Error("suggestions must never be applied")
	}

	// Unknown driver: no query, no suggestions.
	src.queuePPD["Q"] = strings.ReplaceAll(samplePPD, "Generic Universal Mono", "Some Other Driver")
	res, _ = m.Suggest("Q")
	if res.Queried || len(res.Suggestions) != 0 || !strings.Contains(res.Note, "No suggestion rules") {
		t.Errorf("unknown driver: %+v", res)
	}

	// Non-IPP device address.
	src.queuePPD["Q"] = samplePPD
	src.uri = "socket://192.168.1.5:9100"
	if res, _ = m.Suggest("Q"); res.Queried {
		t.Error("socket queue must not be queried")
	}

	// Printer does not answer.
	src.uri = "ipp://192.168.1.5/ipp/print"
	src.attrsErr = errors.New("timeout")
	if res, _ = m.Suggest("Q"); res.Queried || !strings.Contains(res.Note, "didn't answer") {
		t.Errorf("no answer: %+v", res)
	}
}

func TestPPDChoiceLabels(t *testing.T) {
	ppd := ParsePPDSummary([]byte(samplePPD))
	if got := ppd.ChoiceLabel("StapleJob", "PrinterS"); got != "Printer Setting" {
		t.Errorf("StapleJob PrinterS = %q", got)
	}
	if got := ppd.ChoiceLabel("OptStapler", "True"); got != "Installed" {
		t.Errorf("OptStapler True = %q", got)
	}
	if got := ppd.ChoiceLabel("OptTray2", "Bogus"); got != "" {
		t.Errorf("a line inside a quoted value must not become a choice: %q", got)
	}
	resp := ParseLpoptionsOutput("Q", "StapleJob/Staple Job: *PrinterS TopLeft Unknown\nOptStapler/Staple: *False True\n")
	ApplyPPDChoiceLabels(resp, ppd)
	labels := map[string]string{}
	for _, o := range append(resp.General, resp.Accessories...) {
		for _, c := range o.Choices {
			labels[o.Key+"="+c.Name] = c.Label
		}
	}
	want := map[string]string{
		"StapleJob=PrinterS": "Printer Setting",
		"StapleJob=TopLeft":  "Top Left",
		"StapleJob=Unknown":  "Unknown", // not in the PPD: keep the name
		"OptStapler=True":    "Installed",
		"OptStapler=False":   "Not Installed",
	}
	for k, v := range want {
		if labels[k] != v {
			t.Errorf("%s = %q, want %q", k, labels[k], v)
		}
	}
	// lpoptions reports a PPD choice named "Custom" as "_Custom".
	custom := ParsePPDSummary([]byte("*PPD-Adobe: \"4.3\"\n*OpenUI *XRBookletSize/Booklet Size: PickOne\n*XRBookletSize Custom/Custom Size: \"\"\n*CloseUI: *XRBookletSize\n"))
	if got := custom.ChoiceLabel("XRBookletSize", "_Custom"); got != "Custom Size" {
		t.Errorf("_Custom label = %q", got)
	}

	// Legacy ISO-8859-1 labels become valid UTF-8.
	latin := ParsePPDSummary([]byte("*PPD-Adobe: \"4.3\"\n*OpenUI *Q/Qualit\xe9: PickOne\n*Q Haute/Haute qualit\xe9: \"\"\n*CloseUI: *Q\n"))
	if got := latin.ChoiceLabel("Q", "Haute"); got != "Haute qualité" {
		t.Errorf("latin-1 label = %q", got)
	}
}

func TestRegroupByInstallable(t *testing.T) {
	resp := ParseLpoptionsOutput("Q", "OptTray2/Tray 2: *False True\nOptStapler/Staple: *False True\nStapleJob/Staple Job: *PrinterS TopLeft\nPageSize/Media Size: *A4 Letter\n")
	RegroupByInstallable(resp, ParsePPDSummary([]byte(samplePPD)))
	var gen, acc []string
	for _, o := range resp.General {
		gen = append(gen, o.Key)
	}
	for _, o := range resp.Accessories {
		acc = append(acc, o.Key)
	}
	if strings.Join(acc, ",") != "OptTray2,OptStapler" || strings.Join(gen, ",") != "StapleJob,PageSize" {
		t.Errorf("general=%v accessories=%v", gen, acc)
	}
}

func TestPPDDriverNames(t *testing.T) {
	s := ParsePPDSummary([]byte("*PPD-Adobe: \"4.3\"\n*Manufacturer: \"Kyocera\"\n*ModelName: \"Kyocera CS 7003i KPDL\"\n*NickName: \"CS 7003i KPDL\"\n"))
	if got := strings.Join(s.DriverNames(), "|"); got != "CS 7003i KPDL|Kyocera CS 7003i KPDL" {
		t.Errorf("Kyocera names = %s", got)
	}
	s = ParsePPDSummary([]byte(samplePPD))
	if got := strings.Join(s.DriverNames(), "|"); got != "Generic Universal Mono" {
		t.Errorf("Generic names = %s", got)
	}
}

func TestDriverNameFromLPInfo(t *testing.T) {
	out := `Model:  name = Library/Printers/PPDs/Contents/Resources/Generic Universal Mono.gz
        natural_language = en
        make-and-model = Generic Universal Mono
        device-id =
Model:  name = Library/Printers/PPDs/Contents/Resources/Kyocera CS 255c.ppd
        natural_language = en
        make-and-model = Kyocera CS 255c (KPDL)
Model:  name = everywhere
        make-and-model = IPP Everywhere
`
	cases := map[string]string{
		"Generic Universal Mono": "Library/Printers/PPDs/Contents/Resources/Generic Universal Mono.gz",
		"Kyocera CS 255c (KPDL)": "Library/Printers/PPDs/Contents/Resources/Kyocera CS 255c.ppd",
		"Generic Universal":      "",
		"IPP Everywhere":         "",
		"Nope":                   "",
	}
	for mm, want := range cases {
		if got := driverNameFromLPInfo(out, mm); got != want {
			t.Errorf("%q: got %q, want %q", mm, got, want)
		}
	}
}

func TestMockAdapterDeleteForgetsNotes(t *testing.T) {
	m := NewMockAdapter()
	add := func() {
		t.Helper()
		if err := m.AddPrinter(AddPrinterParams{Name: "Recreated", URI: "ipp://192.168.1.9/ipp/print", Model: "drv:///sample.drv/generic.ppd"}); err != nil {
			t.Fatal(err)
		}
	}
	add()
	rep, err := m.GetAccessories("Recreated")
	if err != nil || !rep.Available || len(rep.Options) == 0 {
		t.Fatalf("new mock queue: %+v %v", rep, err)
	}
	o := rep.Options[0]
	other := o.Choices[0].Name
	if other == o.Value {
		other = o.Choices[1].Name
	}
	if _, err := m.SetAccessories("Recreated", map[string]string{o.Key: other}, true); err != nil {
		t.Fatal(err)
	}
	if err := m.DeletePrinter("Recreated"); err != nil {
		t.Fatal(err)
	}
	add()
	if rep, _ := m.GetAccessories("Recreated"); rep.HasOverrides {
		t.Error("re-created queue inherited the deleted queue's notes")
	}
}

func TestMockAdapterAccessories(t *testing.T) {
	m := NewMockAdapter()
	rep, err := m.GetAccessories("Finance_Secure_Copier")
	if err != nil || !rep.Available || len(rep.Options) == 0 {
		t.Fatalf("%+v %v", rep, err)
	}
	if rep, _ := m.GetAccessories("Warehouse_Zebra_Labels"); rep.Available || rep.UnavailableReason != AccessoryUnavailableDriverless {
		t.Errorf("driverless mock queue: %+v", rep)
	}
	key := rep.Options[0].Key
	var other string
	for _, c := range rep.Options[0].Choices {
		if c.Name != rep.Options[0].Value {
			other = c.Name
			break
		}
	}
	rep, err = m.SetAccessories("Finance_Secure_Copier", map[string]string{key: other}, true)
	if err != nil || optionByKey(t, rep, key).Source != AccessorySourceManual {
		t.Fatalf("set: %+v %v", rep, err)
	}
	opts, _ := m.GetPrinterOptions("Finance_Secure_Copier")
	for _, o := range opts.Accessories {
		if o.Key == key && o.CurrentValue != other {
			t.Errorf("mock options not updated: %+v", o)
		}
	}
}
