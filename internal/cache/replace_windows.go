//go:build windows

package cache

import (
	"errors"
	"os"
)

// Replace first, preserving the old cache on ordinary failures. Go's Windows
// rename cannot replace some read-only destinations, so only that failed
// replacement falls back to removing the destination before retrying.
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
