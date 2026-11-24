package cups

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// AccessoryFeatures holds the optional parts of the accessories tab. Every
// feature is off until an administrator turns it on.
type AccessoryFeatures struct {
	// SNMPEnabled allows the SNMP hardware check. It makes this Mac send SNMP
	// requests to an address an administrator types, so it is off by default.
	SNMPEnabled bool `json:"snmp_enabled"`
}

// AccessoryFeatureStatus is what the settings screen shows.
type AccessoryFeatureStatus struct {
	AccessoryFeatures
	// SNMPAvailable is false when snmpwalk can't be found on this Mac.
	SNMPAvailable bool   `json:"snmp_available"`
	IsAdmin       bool   `json:"is_admin"`
	Error         string `json:"error,omitempty"`
}

// FeatureStore keeps AccessoryFeatures in ~/.cups-webui/features.json, or in
// memory for mock mode and tests.
type FeatureStore struct {
	path   string
	memory bool

	mu  sync.Mutex
	mem AccessoryFeatures
}

// NewFeatureStore returns a store backed by the JSON file at path. An empty
// path gives a store that always reports the defaults and refuses changes.
func NewFeatureStore(path string) *FeatureStore {
	return &FeatureStore{path: path}
}

// NewMemoryFeatureStore returns a store that keeps the settings in memory only.
func NewMemoryFeatureStore() *FeatureStore {
	return &FeatureStore{memory: true}
}

// DefaultFeatureStorePath is ~/.cups-webui/features.json.
func DefaultFeatureStorePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", errors.New("home folder not found")
	}
	return filepath.Join(home, ".cups-webui", "features.json"), nil
}

// Load returns the saved settings. A missing file gives the defaults (all
// off). A damaged file gives the defaults and an error.
func (s *FeatureStore) Load() (AccessoryFeatures, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.memory {
		return s.mem, nil
	}
	if s.path == "" {
		return AccessoryFeatures{}, errors.New("the settings file location is unknown")
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return AccessoryFeatures{}, nil
	}
	if err != nil {
		return AccessoryFeatures{}, fmt.Errorf("cannot read %s: %w", s.path, err)
	}
	var f AccessoryFeatures
	if err := json.Unmarshal(data, &f); err != nil {
		return AccessoryFeatures{}, fmt.Errorf("%s is damaged: %w", s.path, err)
	}
	return f, nil
}

// Save writes the settings atomically, readable only by this account.
func (s *FeatureStore) Save(f AccessoryFeatures) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.memory {
		s.mem = f
		return nil
	}
	if s.path == "" {
		return errors.New("the settings file location is unknown")
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path, append(data, '\n'))
}

// writeFileAtomic writes data to path through a temporary file in the same
// folder, readable only by this account, so a crash never leaves half a file.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	ok = true
	return nil
}
