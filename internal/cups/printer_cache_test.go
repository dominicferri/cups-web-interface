package cups

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPrinterCacheReusesListWithinTTL(t *testing.T) {
	var c printerCache
	calls := 0
	fetch := func() ([]Printer, error) {
		calls++
		return []Printer{{Name: "A"}}, nil
	}
	first, err := c.get(time.Minute, fetch)
	if err != nil || len(first) != 1 {
		t.Fatalf("first get: %v, %v", first, err)
	}
	first[0].Name = "changed by caller"
	second, _ := c.get(time.Minute, fetch)
	if calls != 1 {
		t.Errorf("expected 1 fetch within the TTL, got %d", calls)
	}
	if second[0].Name != "A" {
		t.Errorf("a caller's change reached the stored list: %q", second[0].Name)
	}
	if _, _ = c.get(0, fetch); calls != 2 {
		t.Errorf("expected a new fetch after the TTL, got %d fetches", calls)
	}
}

func TestPrinterCacheSharesRunningFetch(t *testing.T) {
	var c printerCache
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	fetch := func() ([]Printer, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return []Printer{{Name: "A"}}, nil
	}
	var wg sync.WaitGroup
	results := make([][]Printer, 4)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], _ = c.get(time.Minute, fetch)
		}()
		if i == 0 {
			<-started
		}
	}
	// The other callers have no signal to wait on; give them time to reach get.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if n := calls.Load(); n != 1 {
		t.Errorf("expected 1 shared fetch, got %d", n)
	}
	for i, r := range results {
		if len(r) != 1 || r[0].Name != "A" {
			t.Errorf("caller %d got %v", i, r)
		}
	}
}

func TestPrinterCacheInvalidate(t *testing.T) {
	var c printerCache
	state := "before"
	fetch := func() ([]Printer, error) { return []Printer{{Name: state}}, nil }
	_, _ = c.get(time.Minute, fetch)
	state = "after"
	c.invalidate()
	got, _ := c.get(time.Minute, fetch)
	if got[0].Name != "after" {
		t.Errorf("expected a new fetch after invalidate, got %q", got[0].Name)
	}
}

// A list that was read before a change must not be stored after the change.
func TestPrinterCacheDropsFetchThatOverlapsChange(t *testing.T) {
	var c printerCache
	state := "before"
	stale, _ := c.get(time.Minute, func() ([]Printer, error) {
		read := state
		state = "after"
		c.invalidate()
		return []Printer{{Name: read}}, nil
	})
	if stale[0].Name != "before" {
		t.Fatalf("test setup: got %q", stale[0].Name)
	}
	got, _ := c.get(time.Minute, func() ([]Printer, error) { return []Printer{{Name: state}}, nil })
	if got[0].Name != "after" {
		t.Errorf("stale list was stored: got %q", got[0].Name)
	}
}

func TestPrinterCacheDoesNotStoreErrors(t *testing.T) {
	var c printerCache
	if _, err := c.get(time.Minute, func() ([]Printer, error) { return nil, errors.New("lpstat failed") }); err == nil {
		t.Fatal("expected the fetch error")
	}
	got, err := c.get(time.Minute, func() ([]Printer, error) { return []Printer{{Name: "A"}}, nil })
	if err != nil || len(got) != 1 {
		t.Errorf("expected a new fetch after an error, got %v, %v", got, err)
	}
}

func TestCommandKeepsPrinterList(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"lpstat", []string{"-p", "-d", "-v", "-a"}, true},
		{"cupsctl", nil, true},
		{"cupsctl", []string{"--share-printers"}, false},
		{"lpoptions", []string{"-p", "Q", "-l"}, true},
		{"lpoptions", []string{"-p", "Q", "-o", "PageSize=A4"}, false},
		{"lpoptions", []string{"-d", "Q"}, false},
		{"lpadmin", []string{"-x", "Q"}, false},
		{"cupsdisable", []string{"Q"}, false},
		{"cancel", []string{"12"}, false},
		{"lp", []string{"-d", "Q"}, false},
		{"unknown-command", nil, false},
	}
	for _, tc := range cases {
		if got := commandKeepsPrinterList(tc.name, tc.args); got != tc.want {
			t.Errorf("%s %v: got %v, want %v", tc.name, tc.args, got, tc.want)
		}
	}
}

// The adapter must drop the stored list after a command that can change CUPS.
func TestRunCommandDropsStoredPrinterList(t *testing.T) {
	a := &CLIAdapter{}
	calls := 0
	fetch := func() ([]Printer, error) {
		calls++
		return []Printer{{Name: "A"}}, nil
	}
	_, _ = a.printers.get(time.Minute, fetch)

	// "lpstat -r" only reads, so the list is kept.
	_, _ = a.runCommand(context.Background(), "lpstat", "-r")
	if _, _ = a.printers.get(time.Minute, fetch); calls != 1 {
		t.Fatalf("a read command dropped the list: %d fetches", calls)
	}

	// "true" is not a known read command, so it counts as a change. It changes nothing.
	if _, err := a.runCommand(context.Background(), "true"); err != nil {
		t.Fatalf("run true: %v", err)
	}
	if _, _ = a.printers.get(time.Minute, fetch); calls != 2 {
		t.Errorf("expected a new fetch after a changing command, got %d fetches", calls)
	}
}
