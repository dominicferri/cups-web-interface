package macos

import (
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
	if !strings.Contains(plist, "<key>RunAtLoad</key>") {
		t.Errorf("expected RunAtLoad key in plist")
	}
	if !strings.Contains(plist, "<key>WorkingDirectory</key>") {
		t.Errorf("expected WorkingDirectory key in plist")
	}
}
