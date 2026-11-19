//go:build !unix

package cups

import (
	"os"
	"path/filepath"
)

// lockFile is a no-op where flock isn't available; the in-process mutex still
// serialises access within one server.
func lockFile(path string, exclusive bool) (func(), error) {
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		return nil, err
	}
	return func() {}, nil
}
