package cups

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseJobsOutputUsesLocalTime(t *testing.T) {
	orig := time.Local
	time.Local = time.FixedZone("west", -5*3600)
	defer func() { time.Local = orig }()

	recent := time.Now().Add(-time.Minute).Format("Mon Jan _2 15:04:05 2006")
	jobs := ParseJobsOutput("Q-7 alice 1024 "+recent+"\n", "pending")
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if age := time.Since(jobs[0].SubmittedAt); age < 0 || age > 5*time.Minute {
		t.Errorf("job submitted a minute ago parsed as %v old", age)
	}
	if jobs[0].IsStuck {
		t.Errorf("job submitted a minute ago must not be stuck")
	}
}

const ippJobsSample = `        job-id (integer) = 10
        job-name (nameWithoutLanguage) = First
        job-state (enum) = pending-held
        -- separator --
        job-id (integer) = 11
        job-name (nameWithoutLanguage) = Second
        job-state (enum) = processing
        -- separator --
        job-id (integer) = 12
        job-state (enum) = pending
        -- separator --
        job-id (integer) = 13
        job-state (enum) = completed
`

func TestParseIPPJobStatesOutput(t *testing.T) {
	got := ParseIPPJobStatesOutput(ippJobsSample)
	want := map[string]string{"10": "held", "11": "processing", "12": "pending"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for id, state := range want {
		if got[id] != state {
			t.Errorf("job %s: got %q, want %q", id, got[id], state)
		}
	}
	if titles := ParseIPPJobTitlesOutput(ippJobsSample); titles["10"] != "First" || titles["11"] != "Second" {
		t.Errorf("titles must still parse from the same output, got %v", titles)
	}
}

func TestApplyActiveJobStates(t *testing.T) {
	old := time.Now().Add(-time.Hour)
	newJobs := func() []Job {
		return []Job{
			{ID: "10", Printer: "Q", State: "pending", SubmittedAt: old},
			{ID: "12", Printer: "Q", State: "pending", SubmittedAt: old},
		}
	}

	jobs := newJobs()
	applyActiveJobStates(jobs, ParseIPPJobStatesOutput(ippJobsSample))
	if jobs[0].State != "held" || jobs[0].IsStuck {
		t.Errorf("held job: got state %q stuck %v, want held and not stuck", jobs[0].State, jobs[0].IsStuck)
	}
	if jobs[1].State != "pending" || !jobs[1].IsStuck {
		t.Errorf("old pending job: got state %q stuck %v, want pending and stuck", jobs[1].State, jobs[1].IsStuck)
	}

	jobs = newJobs()
	applyActiveJobStates(jobs, nil)
	if jobs[0].State != "processing" || jobs[1].State != "pending" {
		t.Errorf("without IPP states the first job per printer is processing, got %q and %q", jobs[0].State, jobs[1].State)
	}
}

func TestParseIPPPrinterSharingOutput(t *testing.T) {
	sample := `        printer-is-shared (boolean) = true
        printer-name (nameWithoutLanguage) = Front_Desk
        -- separator --
        printer-is-shared (boolean) = false
        printer-name (nameWithoutLanguage) = Back_Office
`
	got := ParseIPPPrinterSharingOutput(sample)
	if !got["Front_Desk"] || got["Back_Office"] || len(got) != 2 {
		t.Errorf("got %v, want Front_Desk shared and Back_Office not shared", got)
	}
}

func TestCupstestppdMessage(t *testing.T) {
	tests := []struct {
		line, level, want string
		ok                bool
	}{
		// Real cupstestppd -v output on macOS.
		{`**FAIL**  Unable to open PPD file - Illegal whitespace character on line 2846.`, "FAIL", "Unable to open PPD file - Illegal whitespace character on line 2846.", true},
		{`WARN    Size "Tabloid" should be the Adobe standard name "Ledger".`, "WARN", `Size "Tabloid" should be the Adobe standard name "Ledger".`, true},
		{`WARN: Obsolete PPD version`, "WARN", "Obsolete PPD version", true},
		{`FAIL: Missing required attribute`, "FAIL", "Missing required attribute", true},
		{`/etc/cups/ppd/Q.ppd: FAIL`, "FAIL", "", false},
		{`WARNING unrelated`, "WARN", "", false},
	}
	for _, tt := range tests {
		got, ok := cupstestppdMessage(tt.line, tt.level)
		if got != tt.want || ok != tt.ok {
			t.Errorf("cupstestppdMessage(%q, %q) = %q, %v; want %q, %v", tt.line, tt.level, got, ok, tt.want, tt.ok)
		}
	}
}

func TestInspectDangerousPPDDirectivesLongAndCRLines(t *testing.T) {
	long := []byte("*PPD-Adobe: \"4.3\"\n*Comment: " + strings.Repeat("x", 100*1024) + "\n*FoomaticRIPCommandLine: \"touch /tmp/pwned\"\n")
	if _, errs := InspectDangerousPPDDirectives(long); len(errs) == 0 {
		t.Errorf("directive after a 100 KB line was not flagged")
	}

	crOnly := []byte("*PPD-Adobe: \"4.3\"\r*FoomaticRIPCommandLine: \"touch /tmp/pwned\"\r")
	if _, errs := InspectDangerousPPDDirectives(crOnly); len(errs) == 0 {
		t.Errorf("directive in a CR-only file was not flagged")
	}
}

func TestRedactURICredentials(t *testing.T) {
	in := `[-p Q -v smb://svcuser:hunter2@srv/queue] failed: lpadmin: bad URI "smb://svcuser:hunter2@srv/queue"`
	got := redactURICredentials(in)
	if strings.Contains(got, "hunter2") || strings.Contains(got, "svcuser") {
		t.Errorf("credentials left in %q", got)
	}
	if !strings.Contains(got, "smb://<redacted>@srv/queue") {
		t.Errorf("host and queue must stay readable, got %q", got)
	}
	// A password may itself contain "@"; everything up to the last one is the credential.
	if got := redactURICredentials("smb://user:p@ss@srv/q"); got != "smb://<redacted>@srv/q" {
		t.Errorf("password containing @ leaked: %q", got)
	}
	if plain := "[-p Q -v ipp://192.168.1.5/ipp/print]"; redactURICredentials(plain) != plain {
		t.Errorf("URI without credentials was changed: %q", redactURICredentials(plain))
	}
}

func TestPPDUploadRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ppd-uploads")
	content := []byte("*PPD-Adobe: \"4.3\"\n")

	id, err := savePPDUpload(dir, content)
	if err != nil {
		t.Fatalf("savePPDUpload: %v", err)
	}
	if !IsValidPPDUploadID(id) {
		t.Fatalf("issued ID %q is not a valid upload ID", id)
	}

	a := &CLIAdapter{PPDUploadDir: dir}
	path, err := a.uploadedPPDPath(id)
	if err != nil {
		t.Fatalf("uploadedPPDPath: %v", err)
	}
	if stored, _ := os.ReadFile(path); !bytes.Equal(stored, content) {
		t.Errorf("stored PPD differs from the upload")
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0600 {
		t.Errorf("stored PPD mode = %v, want 0600", info.Mode().Perm())
	}

	// The uploaded file wins over a catalog model and reaches lpadmin as -P.
	args := addPrinterArgs(AddPrinterParams{Name: "Q", URI: "ipp://h/ipp/print", Model: "everywhere", PPDUploadID: id}, path)
	if i := slices.Index(args, "-P"); i == -1 || args[i+1] != path || slices.Contains(args, "-m") {
		t.Errorf("addPrinterArgs = %v, want -P %s and no -m", args, path)
	}

	for _, bad := range []string{"", "../../etc/passwd", strings.Repeat("a", 32), strings.Repeat("g", 32)} {
		if _, err := a.uploadedPPDPath(bad); err == nil {
			t.Errorf("uploadedPPDPath(%q) succeeded, want an error", bad)
		}
	}
	if err := a.SetPrinterDriverFromUpload("Q", strings.Repeat("a", 32)); err == nil {
		t.Errorf("SetPrinterDriverFromUpload with an unknown ID succeeded")
	}
	if err := a.AddPrinter(AddPrinterParams{Name: "Q", URI: "ipp://h/ipp/print", PPDUploadID: strings.Repeat("a", 32)}); err == nil {
		t.Errorf("AddPrinter with an unknown upload ID succeeded")
	}
}

func TestSavePPDUploadDropsExpired(t *testing.T) {
	dir := t.TempDir()
	oldID, err := savePPDUpload(dir, []byte("old"))
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * ppdUploadMaxAge)
	if err := os.Chtimes(filepath.Join(dir, oldID+".ppd"), past, past); err != nil {
		t.Fatal(err)
	}
	newID, err := savePPDUpload(dir, []byte("new"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ppdUploadPath(dir, oldID); err == nil {
		t.Errorf("expired upload is still available")
	}
	if _, err := ppdUploadPath(dir, newID); err != nil {
		t.Errorf("fresh upload missing: %v", err)
	}
}

func TestMockAdapterPPDUpload(t *testing.T) {
	m := NewMockAdapter()
	res, err := m.ValidatePPD([]byte("*PPD-Adobe: \"4.3\"\n*ModelName: \"Acme 9000\"\n"))
	if err != nil || !res.Valid || !IsValidPPDUploadID(res.UploadID) {
		t.Fatalf("ValidatePPD = %+v, %v; want a valid result with an upload ID", res, err)
	}
	if err := m.AddPrinter(AddPrinterParams{Name: "Up", URI: "ipp://h/ipp/print", PPDUploadID: res.UploadID}); err != nil {
		t.Errorf("AddPrinter with a valid upload ID: %v", err)
	}
	if err := m.SetPrinterDriverFromUpload("Up", res.UploadID); err != nil {
		t.Errorf("SetPrinterDriverFromUpload: %v", err)
	}
	if err := m.SetPrinterDriverFromUpload("Up", strings.Repeat("a", 32)); err == nil {
		t.Errorf("unknown upload ID accepted")
	}
}
