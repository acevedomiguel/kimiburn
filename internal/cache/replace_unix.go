//go:build !windows

package cache

import "os"

func replaceFile(src, dst string) error { return os.Rename(src, dst) }
