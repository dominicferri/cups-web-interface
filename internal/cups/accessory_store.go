package cups

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// overrideFileVersion is the format this code reads and writes. A file with a
// higher version was written by a newer app; it is read, but never rewritten,
// so fields this version doesn't know about are not lost.
const overrideFileVersion = 1

// AccessoryOverride records an installable option an admin set in this app.
// The PPD only holds the value, so who set it, when, and what it was before
// are kept here. These are notes for the admin, not an audit log: the file
// belongs to the account running the service and can be edited by it.
type AccessoryOverride struct {
	Queue string `json:"queue"`
	// QueueUUID is CUPS's printer-uuid for the queue when the value was set.
	// It changes when a queue is deleted and re-created, so a new queue with
	// an old name doesn't inherit these notes. Empty if it couldn't be read.
	QueueUUID string `json:"queue_uuid,omitempty"`
	Option    string `json:"option"`
	Value     string `json:"value"`
	// PreviousValue is the value before the first manual change (the detected
	// or driver default value). "Reset to detected values" restores it.
	PreviousValue string    `json:"previous_value"`
	SetBy         string    `json:"set_by"`
	SetAt         time.Time `json:"set_at"`
}

type overrideFile struct {
	Version   int                 `json:"version"`
	Overrides []AccessoryOverride `json:"overrides"`
}

var (
	// ErrHistoryUnavailable means there is nowhere to keep the notes (no home folder).
	ErrHistoryUnavailable = errors.New("the app's config folder is unavailable, so manual accessory changes can't be recorded")
	// ErrHistoryTooNew means the file was written by a newer version of this app.
	ErrHistoryTooNew = errors.New("the accessory history file was written by a newer version of this app; update the app before changing accessories")
)

// HistoryCorruptError means the history file exists but can't be parsed. It
// is never overwritten automatically, so nothing is lost.
type HistoryCorruptError struct {
	Path string
	Err  error
}

func (e *HistoryCorruptError) Error() string {
	return fmt.Sprintf("the accessory history file %s is damaged (%v); fix or remove it before changing accessories", e.Path, e.Err)
}

func (e *HistoryCorruptError) Unwrap() error { return e.Err }

// OverrideStore keeps manual accessory notes in a small JSON file.
//
// Every operation re-reads the file under an OS file lock, so several
// processes (the LaunchAgent and a manually started copy) can share it
// without losing each other's changes. Writes go to a temporary file that is
// flushed and renamed into place, so a crash never leaves a half-written file.
type OverrideStore struct {
	mu      sync.Mutex
	path    string // "" with memory=false: unavailable
	memory  bool
	initErr error
	mem     []AccessoryOverride // memory-only stores
}

// NewOverrideStore returns a store backed by the JSON file at path.
func NewOverrideStore(path string) *OverrideStore {
	if path == "" {
		return &OverrideStore{initErr: ErrHistoryUnavailable}
	}
	return &OverrideStore{path: path}
}

// NewMemoryOverrideStore returns a store that keeps notes in memory only (mock mode, tests).
func NewMemoryOverrideStore() *OverrideStore {
	return &OverrideStore{memory: true}
}

// DefaultOverrideStorePath is ~/.cups-webui/accessory-overrides.json, next to the session file.
func DefaultOverrideStorePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", ErrHistoryUnavailable
	}
	return filepath.Join(home, ".cups-webui", "accessory-overrides.json"), nil
}

// sameQueue compares queue names the way CUPS does (case-insensitively).
func sameQueue(a, b string) bool { return strings.EqualFold(a, b) }

// withFile runs fn on the current contents. When write is true and fn reports
// a change, the result is saved before the lock is released.
func (s *OverrideStore) withFile(write bool, fn func(f *overrideFile) (changed bool, err error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.initErr != nil {
		return s.initErr
	}
	if s.memory {
		f := &overrideFile{Version: overrideFileVersion, Overrides: append([]AccessoryOverride(nil), s.mem...)}
		changed, err := fn(f)
		if err == nil && write && changed {
			s.mem = f.Overrides
		}
		return err
	}

	if write {
		if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
			return fmt.Errorf("failed to create config folder: %w", err)
		}
	}
	unlock, err := lockFile(s.path+".lock", write)
	if err != nil {
		if !write && errors.Is(err, os.ErrNotExist) {
			// The config folder doesn't exist, so nothing has been recorded yet.
			_, ferr := fn(&overrideFile{Version: overrideFileVersion})
			return ferr
		}
		return fmt.Errorf("failed to lock accessory history: %w", err)
	}
	defer unlock()

	f, err := s.read()
	if err != nil {
		return err
	}
	changed, err := fn(f)
	if err != nil || !write || !changed {
		return err
	}
	if f.Version > overrideFileVersion {
		return ErrHistoryTooNew
	}
	f.Version = overrideFileVersion
	return s.writeAtomic(f)
}

func (s *OverrideStore) read() (*overrideFile, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return &overrideFile{Version: overrideFileVersion}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read accessory history: %w", err)
	}
	var f overrideFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, &HistoryCorruptError{Path: s.path, Err: err}
	}
	if f.Version < 1 {
		return nil, &HistoryCorruptError{Path: s.path, Err: fmt.Errorf("missing or invalid version")}
	}
	return &f, nil
}

func (s *OverrideStore) writeAtomic(f *overrideFile) error {
	sort.Slice(f.Overrides, func(i, j int) bool {
		a, b := f.Overrides[i], f.Overrides[j]
		if !sameQueue(a.Queue, b.Queue) {
			return strings.ToLower(a.Queue) < strings.ToLower(b.Queue)
		}
		return a.Option < b.Option
	})
	if f.Overrides == nil {
		f.Overrides = []AccessoryOverride{}
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".accessory-overrides-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to save accessory history: %w", err)
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to save accessory history: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to save accessory history: %w", err)
	}
	// Flush to disk before the rename makes it visible.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to save accessory history: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to save accessory history: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("failed to save accessory history: %w", err)
	}
	ok = true
	// Make the rename itself durable. Not all systems support syncing a folder.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
	return nil
}

// Writable reports whether a change could be recorded: the store is
// available, the file isn't damaged, and it wasn't written by a newer app.
func (s *OverrideStore) Writable() error {
	return s.withFile(false, func(f *overrideFile) (bool, error) {
		if f.Version > overrideFileVersion {
			return false, ErrHistoryTooNew
		}
		return false, nil
	})
}

// ForQueue returns the notes recorded for a queue, keyed by option.
func (s *OverrideStore) ForQueue(queue string) (map[string]AccessoryOverride, error) {
	out := map[string]AccessoryOverride{}
	err := s.withFile(false, func(f *overrideFile) (bool, error) {
		for _, o := range f.Overrides {
			if sameQueue(o.Queue, queue) {
				out[o.Option] = o
			}
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Update applies changes for one queue in a single save: set records or
// replaces notes, remove deletes the listed options.
func (s *OverrideStore) Update(queue string, set []AccessoryOverride, remove []string) error {
	if len(set) == 0 && len(remove) == 0 {
		return nil
	}
	drop := map[string]bool{}
	for _, k := range remove {
		drop[k] = true
	}
	for _, o := range set {
		drop[o.Option] = true
	}
	return s.withFile(true, func(f *overrideFile) (bool, error) {
		kept := make([]AccessoryOverride, 0, len(f.Overrides)+len(set))
		for _, o := range f.Overrides {
			if sameQueue(o.Queue, queue) && drop[o.Option] {
				continue
			}
			kept = append(kept, o)
		}
		for _, o := range set {
			o.Queue = queue
			kept = append(kept, o)
		}
		f.Overrides = kept
		return true, nil
	})
}

// DeleteQueue removes every note for a queue.
func (s *OverrideStore) DeleteQueue(queue string) error {
	return s.Prune(func(q string) bool { return !sameQueue(q, queue) })
}

// Prune keeps only notes for queues where keep returns true. It is used to
// drop notes for queues that were deleted outside this app.
func (s *OverrideStore) Prune(keep func(queue string) bool) error {
	return s.withFile(true, func(f *overrideFile) (bool, error) {
		kept := make([]AccessoryOverride, 0, len(f.Overrides))
		for _, o := range f.Overrides {
			if keep(o.Queue) {
				kept = append(kept, o)
			}
		}
		changed := len(kept) != len(f.Overrides)
		f.Overrides = kept
		return changed, nil
	})
}
