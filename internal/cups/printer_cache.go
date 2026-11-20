package cups

import (
	"errors"
	"sync"
	"time"
)

// printerListTTL is how long one printer list is reused. The page asks for
// the system status and the printer list in the same poll, and both need the
// full list, which costs about 250 ms of lpstat calls. The page polls every
// 5 seconds, so a change made outside this server shows at most 1 second later
// than before.
const printerListTTL = time.Second

// printerCache shares one printer list between callers. Callers that arrive
// while a fetch is running wait for that fetch. The zero value is ready to use.
type printerCache struct {
	mu        sync.Mutex
	gen       uint64 // incremented by invalidate
	printers  []Printer
	fetchedAt time.Time
	valid     bool
	inflight  *printerFetch
}

type printerFetch struct {
	gen      uint64
	done     chan struct{}
	printers []Printer
	err      error
}

// get returns the stored list when it is newer than ttl. If not, it runs fetch,
// or waits for a fetch that is already running. Errors are never stored. The
// returned slice belongs to the caller; the values its pointer fields refer to
// are shared and must not be changed.
func (c *printerCache) get(ttl time.Duration, fetch func() ([]Printer, error)) ([]Printer, error) {
	c.mu.Lock()
	if c.valid && time.Since(c.fetchedAt) < ttl {
		out := copyPrinters(c.printers)
		c.mu.Unlock()
		return out, nil
	}
	// A fetch that started before the last invalidate can hold the old state,
	// so only a fetch of the current generation is joined.
	if f := c.inflight; f != nil && f.gen == c.gen {
		c.mu.Unlock()
		<-f.done
		if f.err != nil {
			return nil, f.err
		}
		return copyPrinters(f.printers), nil
	}
	f := &printerFetch{gen: c.gen, done: make(chan struct{})}
	c.inflight = f
	c.mu.Unlock()

	finished := false
	defer func() {
		if !finished {
			// fetch panicked: callers that wait must not get an empty list.
			f.err = errors.New("printer list fetch did not complete")
		}
		c.mu.Lock()
		// Do not store a list that was read before a change: the next read
		// must see the change.
		if finished && f.err == nil && c.gen == f.gen {
			c.printers = f.printers
			c.fetchedAt = time.Now()
			c.valid = true
		}
		if c.inflight == f {
			c.inflight = nil
		}
		c.mu.Unlock()
		close(f.done)
	}()

	f.printers, f.err = fetch()
	finished = true
	if f.err != nil {
		return nil, f.err
	}
	return copyPrinters(f.printers), nil
}

// invalidate drops the stored list. A fetch that is running now is not stored.
func (c *printerCache) invalidate() {
	c.mu.Lock()
	c.gen++
	c.valid = false
	c.printers = nil
	c.mu.Unlock()
}

func copyPrinters(in []Printer) []Printer {
	if in == nil {
		return nil
	}
	out := make([]Printer, len(in))
	copy(out, in)
	return out
}

// commandKeepsPrinterList reports whether a command only reads CUPS state, so
// the stored printer list stays correct after it. A command that is not listed
// here is treated as a change.
func commandKeepsPrinterList(name string, args []string) bool {
	switch name {
	case "lpstat", "lpinfo", "cups-config", "tail", "ipptool", "cupstestppd":
		return true
	case "cupsctl":
		// Without arguments cupsctl prints the settings.
		return len(args) == 0
	case "lpoptions":
		// "lpoptions -p <name> -l" lists options; -o, -d, -r and -x change them.
		list := false
		for _, a := range args {
			switch a {
			case "-o", "-d", "-r", "-x":
				return false
			case "-l":
				list = true
			}
		}
		return list
	}
	return false
}
