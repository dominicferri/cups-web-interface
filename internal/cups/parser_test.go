package cups

import (
	"testing"
)

func TestParseCombinedStatus(t *testing.T) {
	statusSample := `printer _192_168_1_65 is idle.  enabled since Fri 21 Aug 15:04:57 2026
printer Direct_Physical_Printer is idle.  enabled since Fri 21 Aug 16:31:39 2026
printer Error_Printer is stopped.  enabled since Fri 21 Aug 12:00:00 2026
printer Disabled_Printer disabled since Thu 20 Aug 10:00:00 2026 - offline
system default destination: Direct_Physical_Printer
device for _192_168_1_65: socket://192.168.1.65/
device for Direct_Physical_Printer: ipp://192.168.1.65/ipp/print
device for Error_Printer: usb://Vendor/Model
_192_168_1_65 accepting requests since Fri 21 Aug 15:04:57 2026
Direct_Physical_Printer accepting requests since Fri 21 Aug 16:31:39 2026
Disabled_Printer not accepting requests since Thu 20 Aug 10:00:00 2026
`
	classSample := `members of class Office_Pool: Direct_Physical_Printer, _192_168_1_65`

	printers := ParseCombinedStatus(statusSample, classSample)

	if len(printers) < 4 {
		t.Fatalf("expected at least 4 printers, got %d", len(printers))
	}

	dp, ok := printers["Direct_Physical_Printer"]
	if !ok {
		t.Fatal("Direct_Physical_Printer not found")
	}
	if !dp.IsDefault {
		t.Errorf("expected Direct_Physical_Printer to be default")
	}
	if dp.State != "idle" {
		t.Errorf("expected state idle, got %s", dp.State)
	}
	if dp.URI != "ipp://192.168.1.65/ipp/print" {
		t.Errorf("expected URI ipp://192.168.1.65/ipp/print, got %s", dp.URI)
	}
	if !dp.AcceptingJobs {
		t.Errorf("expected Direct_Physical_Printer to accept jobs")
	}

	dis, ok := printers["Disabled_Printer"]
	if !ok {
		t.Fatal("Disabled_Printer not found")
	}
	if dis.State != "stopped" {
		t.Errorf("expected disabled printer state to be stopped, got %s", dis.State)
	}
	if dis.AcceptingJobs {
		t.Errorf("expected Disabled_Printer to NOT accept jobs")
	}
	if dis.StateMessage != "offline" {
		t.Errorf("expected Disabled_Printer state message 'offline', got %q", dis.StateMessage)
	}
	if dp.StateMessage != "" {
		t.Errorf("expected Direct_Physical_Printer state message to be empty, got %q", dp.StateMessage)
	}
}

func TestParsePrinterDetails(t *testing.T) {
	printers := map[string]*Printer{
		"Direct_Physical_Printer": {Name: "Direct_Physical_Printer"},
	}

	detailsSample := `printer Direct_Physical_Printer is idle.  enabled since Fri 21 Aug 16:31:39 2026
	Form mounted:
	Content types: any
	Printer types: unknown
	Description: Direct HP Test
	Alerts: media-empty-report, low-toner
	Location: Building 4
	Connection: direct
	Interface: /private/etc/cups/ppd/Direct_Physical_Printer.ppd
`
	ParsePrinterDetails(printers, detailsSample)

	dp := printers["Direct_Physical_Printer"]
	if dp.Info != "Direct HP Test" {
		t.Errorf("expected info 'Direct HP Test', got '%s'", dp.Info)
	}
	if dp.Location != "Building 4" {
		t.Errorf("expected location 'Building 4', got '%s'", dp.Location)
	}
	if len(dp.Alerts) != 2 || dp.Alerts[0] != "media-empty-report" || dp.Alerts[1] != "low-toner" {
		t.Errorf("unexpected alerts: %+v", dp.Alerts)
	}
}

func TestParseJobsOutput(t *testing.T) {
	jobSample := `Direct_Physical_Printer-49 PaperCut        108544   Fri 21 Aug 16:31:39 2026
Direct_Physical_Printer-48 PaperCut         52224   Fri 21 Aug 15:18:32 2026
`
	jobs := ParseJobsOutput(jobSample, "completed")
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}

	j1 := jobs[0]
	if j1.ID != "49" {
		t.Errorf("expected job ID 49, got %s", j1.ID)
	}
	if j1.Printer != "Direct_Physical_Printer" {
		t.Errorf("expected printer Direct_Physical_Printer, got %s", j1.Printer)
	}
	if j1.User != "PaperCut" {
		t.Errorf("expected user PaperCut, got %s", j1.User)
	}
	if j1.SizeBytes != 108544 {
		t.Errorf("expected size 108544, got %d", j1.SizeBytes)
	}
	if j1.State != "completed" {
		t.Errorf("expected state completed, got %s", j1.State)
	}
}

func TestParseCupsctlOutput(t *testing.T) {
	sample := `_debug_logging=0
_remote_admin=0
_share_printers=1
WebInterface=No
`
	res := ParseCupsctlOutput(sample)
	if res["WebInterface"] != "No" {
		t.Errorf("expected WebInterface No, got %s", res["WebInterface"])
	}
	if res["_share_printers"] != "1" {
		t.Errorf("expected _share_printers 1, got %s", res["_share_printers"])
	}
}

func TestParseLPInfoLongOutput(t *testing.T) {
	sample := `Model:  name = Library/Printers/PPDs/Contents/Resources/Kyocera CS 205c.ppd
        natural_language = en
        make-and-model = Kyocera CS 205c (KPDL)
        device-id = MFG:Kyocera;MODEL:Kyocera CS 205c;
Model:  name = everywhere
        natural_language = en
        make-and-model = IPP Everywhere
        device-id = CMD:PwgRaster
`
	drivers := parseLPInfoLongOutput(sample)
	if len(drivers) != 2 {
		t.Fatalf("expected 2 drivers, got %d", len(drivers))
	}

	d0 := drivers[0]
	if d0.Name != "Library/Printers/PPDs/Contents/Resources/Kyocera CS 205c.ppd" {
		t.Errorf("expected full PPD path with spaces, got %q", d0.Name)
	}
	if d0.Make != "Kyocera" {
		t.Errorf("expected Make Kyocera, got %q", d0.Make)
	}
	if d0.Model != "Kyocera CS 205c (KPDL)" {
		t.Errorf("expected Model 'Kyocera CS 205c (KPDL)', got %q", d0.Model)
	}

	d1 := drivers[1]
	if d1.ArchReadiness.Status != "driverless" {
		t.Errorf("expected driverless status, got %s", d1.ArchReadiness.Status)
	}
}

func TestParseLPInfoShortOutput(t *testing.T) {
	sample := `Library/Printers/PPDs/Contents/Resources/Kyocera CS 205c.ppd Kyocera CS 205c (KPDL)
drv:///sample.drv/laserjet.ppd HP LaserJet Series PCL 4/5
everywhere IPP Everywhere
`
	drivers := parseLPInfoShortOutput(sample)
	if len(drivers) != 3 {
		t.Fatalf("expected 3 drivers, got %d", len(drivers))
	}

	d0 := drivers[0]
	if d0.Name != "Library/Printers/PPDs/Contents/Resources/Kyocera CS 205c.ppd" {
		t.Errorf("expected full path with spaces, got %q", d0.Name)
	}
	if d0.Model != "Kyocera CS 205c (KPDL)" {
		t.Errorf("expected model 'Kyocera CS 205c (KPDL)', got %q", d0.Model)
	}
}

func TestParseIPPJobTitlesOutput(t *testing.T) {
	sample := `
        job-id (integer) = 50
        com.apple.print.JobInfo.PMJobName (nameWithoutLanguage) = PAPERCUT_POCKET_PRINTER - CUPS 2.3.4
        document-name-supplied (nameWithoutLanguage) = PAPERCUT_POCKET_PRINTER - CUPS 2.3.4
        -- separator --
        job-id (integer) = 51
        com.apple.print.JobInfo.PMJobName (nameWithoutLanguage) = New tab
        document-name-supplied (nameWithoutLanguage) = New tab
        -- separator --
        job-id (integer) = 52
        job-name (nameWithoutLanguage) = Annual_Report_2026.pdf
`
	titles := ParseIPPJobTitlesOutput(sample)
	if len(titles) != 3 {
		t.Fatalf("expected 3 titles, got %d", len(titles))
	}
	if titles["50"] != "PAPERCUT_POCKET_PRINTER - CUPS 2.3.4" {
		t.Errorf("expected title for 50, got %q", titles["50"])
	}
	if titles["51"] != "New tab" {
		t.Errorf("expected title for 51 'New tab', got %q", titles["51"])
	}
	if titles["52"] != "Annual_Report_2026.pdf" {
		t.Errorf("expected title for 52 'Annual_Report_2026.pdf', got %q", titles["52"])
	}
}


// lpstat prints file devices without the "file:" scheme; the parser restores it
// so a null queue's URI can be shown and saved again unchanged.
func TestParseCombinedStatusNullDevice(t *testing.T) {
	status := `printer Null_Queue is idle.  enabled since Fri 13 Nov 15:27:06 2026
printer Null_Queue_Slashes is idle.  enabled since Fri 13 Nov 15:27:08 2026
printer Net_Queue is idle.  enabled since Fri 13 Nov 15:27:08 2026
device for Null_Queue: /dev/null
device for Null_Queue_Slashes: ///dev/null
device for Net_Queue: socket://192.168.1.50:9100
`
	printers := ParseCombinedStatus(status, "")
	for name, want := range map[string]string{
		"Null_Queue":         "file:/dev/null",
		"Null_Queue_Slashes": "file:///dev/null",
		"Net_Queue":          "socket://192.168.1.50:9100",
	} {
		p := printers[name]
		if p == nil {
			t.Fatalf("printer %s not parsed", name)
		}
		if p.URI != want {
			t.Errorf("%s: expected URI %q, got %q", name, want, p.URI)
		}
		if name != "Net_Queue" && !IsValidPrinterURI(p.URI) {
			t.Errorf("%s: parsed URI %q must pass validation so the queue can be edited", name, p.URI)
		}
	}
}
