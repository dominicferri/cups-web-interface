package cups

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

// Invented printers on TEST-NET addresses, in the shape a print server export has.
const inventorySample = `"PrinterName","Model","Driver","PortName","IPAddress"
"Front Desk (Main)","Example Laser 400","Example PCL6","192.0.2.10","192.0.2.10"
"Library 2.01","SNMP unavailable","Example PS","IP_192.0.2.11","N/A"
"Copy Room","Example MFP 9","Example PS","192.0.2.12_1","N/A"
"Lab-3","Example Laser 400","Example PCL6","192.0.2.13  Lab3","N/A"
"Kiosk","","Example PCL6","TS001","N/A"
"Fax","","Example Fax","PORTPROMPT:","N/A"
"Null Queue","","Example PCL6","nul",""
"Odd","","","192.0.2.14","not an ip"
"Short Row","Example"
"front desk (main)","Example Laser 400","Example PCL6","192.0.2.15","192.0.2.15"
"By Name","Example","Example PS","printer7","printer7.example.com"
`

func TestParsePrinterInventoryCSV(t *testing.T) {
	entries, issues, err := ParsePrinterInventoryCSV([]byte(inventorySample))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range entries {
		got[e.Name] = e.Address
	}
	want := map[string]string{
		"Front Desk (Main)": "192.0.2.10",
		"Library 2.01":      "192.0.2.11", // IP_ port
		"Copy Room":         "192.0.2.12", // second port "_1"
		"Lab-3":             "192.0.2.13", // address then a note
		"By Name":           "printer7.example.com",
	}
	if len(got) != len(want) {
		t.Errorf("entries = %v", got)
	}
	for n, a := range want {
		if got[n] != a {
			t.Errorf("%s: %q, want %q", n, got[n], a)
		}
	}
	for _, e := range entries {
		if e.Name == "Library 2.01" && e.Model != "" {
			t.Errorf("\"SNMP unavailable\" kept as a model: %q", e.Model)
		}
	}
	reasons := map[int]string{}
	for _, i := range issues {
		reasons[i.Line] = i.Reason
	}
	for line, part := range map[int]string{
		6: `port "TS001" isn't a network port`, 7: "PORTPROMPT:", 8: "nul",
		9: "isn't an IP address", 10: "has 2 fields", 11: "same name as line 2",
	} {
		if !strings.Contains(reasons[line], part) {
			t.Errorf("line %d: reason %q, want it to mention %q", line, reasons[line], part)
		}
	}
	if len(issues) != 6 {
		t.Errorf("issues = %+v", issues)
	}
}

func utf16LE(s string) []byte {
	b := []byte{0xFF, 0xFE}
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u), byte(u>>8))
	}
	return b
}

func TestPrinterInventoryEncodings(t *testing.T) {
	typed := "#TYPE Selected.Microsoft.Management.Infrastructure.CimInstance\r\n" + strings.ReplaceAll(inventorySample, "\n", "\r\n")
	for name, data := range map[string][]byte{
		"utf-8 bom":    append([]byte{0xEF, 0xBB, 0xBF}, inventorySample...),
		"utf-16le bom": utf16LE(inventorySample),
		"#TYPE line":   []byte(typed),
	} {
		entries, issues, err := ParsePrinterInventoryCSV(data)
		if err != nil || len(entries) != 5 {
			t.Errorf("%s: %d entries, %v", name, len(entries), err)
			continue
		}
		if name == "#TYPE line" && issues[0].Line != 7 {
			t.Errorf("line numbers must count the #TYPE line: %+v", issues[0])
		}
	}
}

func TestPrinterInventoryErrors(t *testing.T) {
	header := `"PrinterName","Model","Driver","PortName","IPAddress"` + "\n"
	cases := map[string]struct {
		data string
		want string
	}{
		"empty":         {"  \n", "empty"},
		"header only":   {header, "no printers"},
		"wrong columns": {"Name,Address\nA,192.0.2.1\n", "no PrinterName"},
		"no address":    {"PrinterName,Model\nA,B\n", "IPAddress or PortName"},
		"bad quote":     {header + "\"A,\"B\",\"C\",\"D\",\"E\"\n\"X\"oops,,,,\n", "line"},
		"binary":        {"PrinterName,IPAddress\x00\x01\n", "NUL"},
		"not utf-8":     {"PrinterName,IPAddress\n\xff\xfe\xfd,192.0.2.1\n", "UTF-8"},
		"no usable":     {header + `"A","","","TS001","N/A"` + "\n", "none of the 1 printers"},
		"dup column":    {"PrinterName,IPAddress,ipaddress\nA,192.0.2.1,192.0.2.2\n", "twice"},
		"too big":       {header + strings.Repeat("x", maxInventoryBytes), "larger than"},
	}
	for name, c := range cases {
		_, _, err := ParsePrinterInventoryCSV([]byte(c.data))
		var fe *InventoryFormatError
		if !errors.As(err, &fe) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want an InventoryFormatError mentioning %q", name, err, c.want)
		}
	}
}

func TestPrinterInventoryLookup(t *testing.T) {
	entries, _, err := ParsePrinterInventoryCSV([]byte(inventorySample))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ queue, uri, want string }{
		// The printer name in the print server URI wins over the queue name.
		{"Renamed_Queue", "ipps://printserver.example.com:9164/printers/Front%20Desk%20%28Main%29?auth=true", "192.0.2.10"},
		{"X", "papercut:ipps://printserver.example.com:9164/printers/Library%202.01", "192.0.2.11"},
		// Queue names CUPS made from the share name.
		{"Front_Desk__Main_", "ipp://127.0.0.1:631/ipp/print", "192.0.2.10"},
		{"lab-3", "dnssd://x", "192.0.2.13"},
		{"Library_2.01", "", "192.0.2.11"},
		{"Nowhere", "ipps://printserver.example.com/printers/Nowhere", ""},
	}
	for _, c := range cases {
		e := lookupEntry(entries, c.queue, c.uri)
		got := ""
		if e != nil {
			got = e.Address
		}
		if got != c.want {
			t.Errorf("%s %s: %q, want %q", c.queue, c.uri, got, c.want)
		}
	}
}

func TestImportPrinterInventory(t *testing.T) {
	src := newFakeSource()
	src.uri = "ipps://printserver.example.com:9164/printers/Copy%20Room"
	m := NewAccessoryManager(src, nil)
	m.Features = NewMemoryFeatureStore()

	src.admin = false
	if _, err := m.ImportPrinterInventory("list.csv", []byte(inventorySample)); !errors.Is(err, ErrNotCUPSAdmin) {
		t.Fatalf("non-admin import: %v", err)
	}
	src.admin = true

	res, err := m.ImportPrinterInventory("/tmp/some/list.csv", []byte(inventorySample))
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 5 || res.Rows != 11 || len(res.Skipped) != 6 || res.SourceName != "list.csv" || res.Queues != 2 || res.Matched != 2 {
		t.Errorf("result: %+v", res)
	}

	// The hint is used only while the SNMP check is on.
	if rep, _ := m.Report("Q"); rep.SNMPHostHint != "" || rep.PrintServerEntry != nil {
		t.Errorf("hint shown with SNMP off: %+v", rep)
	}
	_ = m.Features.Save(AccessoryFeatures{SNMPEnabled: true})
	rep, _ := m.Report("Q")
	if rep.SNMPHostHint != "192.0.2.12" || rep.SNMPHostSource != SNMPHostFromPrintServerList || rep.PrintServerEntry == nil || rep.PrintServerEntry.Model != "Example MFP 9" {
		t.Errorf("report: hint %q source %q entry %+v", rep.SNMPHostHint, rep.SNMPHostSource, rep.PrintServerEntry)
	}
	// A direct device URI still wins.
	src.uri = "socket://198.51.100.7:9100"
	if rep, _ = m.Report("Q"); rep.SNMPHostHint != "198.51.100.7" || rep.SNMPHostSource != SNMPHostFromDeviceURI {
		t.Errorf("direct: %q %q", rep.SNMPHostHint, rep.SNMPHostSource)
	}

	// A bad file leaves the old list in place.
	if _, err := m.ImportPrinterInventory("bad.csv", []byte("Name\nA\n")); err == nil {
		t.Fatal("bad file accepted")
	}
	if st := m.PrinterInventory(); !st.Loaded || st.Count != 5 || st.SourceName != "list.csv" {
		t.Errorf("after a failed import: %+v", st)
	}

	src.admin = false
	if _, err := m.RemovePrinterInventory(); !errors.Is(err, ErrNotCUPSAdmin) {
		t.Errorf("non-admin remove: %v", err)
	}
	src.admin = true
	if st, err := m.RemovePrinterInventory(); err != nil || st.Loaded {
		t.Errorf("remove: %+v %v", st, err)
	}
}

func TestInventoryStoreFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "printer-inventory.json")
	src := newFakeSource()
	m := NewAccessoryManager(src, nil)
	m.Inventory = NewInventoryStore(path)
	if st := m.PrinterInventory(); st.Loaded || st.Error != "" {
		t.Fatalf("no file yet: %+v", st)
	}
	if _, err := m.ImportPrinterInventory("list.csv", []byte(inventorySample)); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0600 {
		t.Fatalf("mode: %v %v", fi, err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "PrinterName") || !strings.Contains(string(data), `"version": 1`) {
		t.Error("the store must hold the parsed list, not the CSV")
	}
	if e := NewInventoryStore(path).Lookup("Copy_Room", ""); e == nil || e.Address != "192.0.2.12" {
		t.Errorf("reload lookup: %+v", e)
	}

	if err := os.WriteFile(path, []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if st := m.PrinterInventory(); st.Loaded || !strings.Contains(st.Error, "import the CSV again") {
		t.Errorf("damaged: %+v", st)
	}
	if e := m.Inventory.Lookup("Copy_Room", ""); e != nil {
		t.Error("a damaged file must not give addresses")
	}
	if _, err := m.RemovePrinterInventory(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file still there: %v", err)
	}
}
