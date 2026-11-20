package macos

import (
	"encoding/xml"
	"io"
	"slices"
	"strings"
	"testing"
)

func TestGeneratePlist(t *testing.T) {
	plist, err := GeneratePlist("/usr/local/bin/cups-webui", 6310, "127.0.0.1")
	if err != nil {
		t.Fatalf("GeneratePlist failed: %v", err)
	}

	if !strings.Contains(plist, "<string>com.papercut.cups-webui</string>") {
		t.Errorf("expected label com.papercut.cups-webui in plist")
	}
	if !strings.Contains(plist, "<string>/usr/local/bin/cups-webui</string>") {
		t.Errorf("expected binary path in plist")
	}
	if !strings.Contains(plist, "<string>6310</string>") {
		t.Errorf("expected port in plist")
	}
	// The server exits when the browser closes. launchd must restart it after
	// a crash only, and must not start it at login.
	compact := strings.Join(strings.Fields(plist), "")
	if !strings.Contains(compact, "<key>RunAtLoad</key><false/>") {
		t.Errorf("expected RunAtLoad false in plist")
	}
	if !strings.Contains(compact, "<key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>") {
		t.Errorf("expected KeepAlive to restart only after an unsuccessful exit")
	}
	if !strings.Contains(plist, "<key>WorkingDirectory</key>") {
		t.Errorf("expected WorkingDirectory key in plist")
	}
}

func TestGeneratePlistEscapesPaths(t *testing.T) {
	plist, err := GeneratePlist("/Users/R&D <lab>/cups-webui", 6310, "127.0.0.1")
	if err != nil {
		t.Fatalf("GeneratePlist failed: %v", err)
	}

	var values []string
	dec := xml.NewDecoder(strings.NewReader(plist))
	dec.Strict = true
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("plist is not well-formed XML: %v", err)
		}
		if text, ok := tok.(xml.CharData); ok {
			values = append(values, string(text))
		}
	}
	if !slices.Contains(values, "/Users/R&D <lab>/cups-webui") {
		t.Errorf("binary path did not survive the XML round trip")
	}
}
