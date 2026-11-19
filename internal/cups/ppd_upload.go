package cups

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// A validated PPD upload is kept on disk for this long, so the admin can
// finish the Add Printer or driver swap form that uploaded it.
const ppdUploadMaxAge = time.Hour

var ppdUploadIDRegex = regexp.MustCompile(`^[0-9a-f]{32}$`)

// IsValidPPDUploadID reports whether id has the form issued by ValidatePPD.
func IsValidPPDUploadID(id string) bool {
	return ppdUploadIDRegex.MatchString(id)
}

// defaultPPDUploadDir is the private folder for validated PPD uploads.
func defaultPPDUploadDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cups-webui", "ppd-uploads"), nil
}

// savePPDUpload stores validated PPD content in dir and returns its upload ID.
// Uploads older than ppdUploadMaxAge are removed first.
func savePPDUpload(dir string, content []byte) (string, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > ppdUploadMaxAge {
				_ = os.Remove(filepath.Join(dir, e.Name()))
			}
		}
	}

	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := hex.EncodeToString(b)
	if err := os.WriteFile(filepath.Join(dir, id+".ppd"), content, 0600); err != nil {
		return "", err
	}
	return id, nil
}

// ppdUploadPath returns the stored file for an upload ID issued by savePPDUpload.
func ppdUploadPath(dir, id string) (string, error) {
	if !IsValidPPDUploadID(id) {
		return "", fmt.Errorf("invalid PPD upload ID")
	}
	path := filepath.Join(dir, id+".ppd")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("uploaded PPD not found or expired; upload the file again")
	}
	return path, nil
}
