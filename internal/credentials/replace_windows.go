//go:build windows

package credentials

import (
	"errors"
	"os"
)

// replaceFile moves src onto dst. Windows maps os.Rename to MoveFileEx with
// MOVEFILE_REPLACE_EXISTING, which still fails when dst carries the read-only
// attribute, and Go documents Rename as non-atomic on non-Unix platforms. The
// rename is therefore tried first, so the ordinary case keeps the existing
// token intact, and only a failed rename falls back to clearing dst.
func replaceFile(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	if _, statErr := os.Stat(dst); statErr != nil {
		return err
	}
	if removeErr := os.Remove(dst); removeErr != nil {
		return errors.Join(err, removeErr)
	}
	return os.Rename(src, dst)
}
