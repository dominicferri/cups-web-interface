//go:build unix

package cups

import (
	"os"
	"syscall"
)

// lockFile takes an advisory lock on path (exclusive for writers, shared for
// readers) and returns a function that releases it. The lock file is created
// if needed; os.ErrNotExist means its folder doesn't exist yet.
func lockFile(path string, exclusive bool) (func(), error) {
	flags := os.O_RDONLY | os.O_CREATE
	if exclusive {
		flags = os.O_RDWR | os.O_CREATE
	}
	f, err := os.OpenFile(path, flags, 0600)
	if err != nil {
		return nil, err
	}
	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}
	if err := syscall.Flock(int(f.Fd()), how); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
