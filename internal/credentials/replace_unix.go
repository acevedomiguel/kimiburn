//go:build !windows

package credentials

import "os"

// replaceFile moves src onto dst. On Unix the rename replaces dst atomically,
// even when dst is read-only, so no fallback is needed.
func replaceFile(src, dst string) error { return os.Rename(src, dst) }
