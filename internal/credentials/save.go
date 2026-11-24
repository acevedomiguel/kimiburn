package credentials

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// readSecretFile enforces 0600 (no group or world access) before reading.
func readSecretFile(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("credentials: inspect %s: %w", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("credentials: %s is a directory", path)
	}
	// Windows reports 0666 for every file, so the group/world check would
	// reject the fallback unconditionally. There the file's protection comes
	// from the 0700 directory that writeSecretFile sets.
	if PermissionsAreEnforced {
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			return "", ErrInsecurePermissions
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("credentials: read %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// writeSecretFile writes a 0600 file inside a 0700 directory.
func writeSecretFile(path, secret string) error {
	dir := filepath.Dir(path)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("credentials: create directory: %w", err)
		}
		if PermissionsAreEnforced {
			// MkdirAll leaves an existing, looser directory alone.
			if err := os.Chmod(dir, 0o700); err != nil {
				return fmt.Errorf("credentials: restrict %s: %w", dir, err)
			}
		}
	}
	// os.WriteFile only applies the mode when it creates the file, so replace
	// an existing file to guarantee 0600.
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("credentials: replace %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(secret), 0o600); err != nil {
		return fmt.Errorf("credentials: write %s: %w", path, err)
	}
	if PermissionsAreEnforced {
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("credentials: restrict %s: %w", path, err)
		}
	}
	return nil
}
