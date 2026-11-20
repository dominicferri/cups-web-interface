package web

import (
	"io/fs"
	"regexp"
	"testing"
)

// The server sends script-src 'self', which blocks every inline script.
func TestIndexHasNoInlineScript(t *testing.T) {
	index, err := fs.ReadFile(GetFS(), "index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	for _, tag := range regexp.MustCompile(`(?i)<script\b[^>]*>`).FindAll(index, -1) {
		if !regexp.MustCompile(`(?i)\ssrc=`).Match(tag) {
			t.Errorf("inline script in index.html: %s", tag)
		}
	}
	if regexp.MustCompile(`(?i)\son[a-z]+=`).Match(index) {
		t.Error("inline event handler in index.html")
	}
	if _, err := fs.Stat(GetFS(), "theme-init.js"); err != nil {
		t.Errorf("theme-init.js is not embedded: %v", err)
	}
}
