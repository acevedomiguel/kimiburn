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
	// Windows reports 0666 for every file, so a group/world check there would
	// reject the fallback unconditionally. On Windows the file is protected by
	// the ACL inherited from the per-user profile directory it lives in, not by
	// any mode bit this package can set.
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

// writeSecretFile writes a 0600 secret inside a 0700 directory. The content is
// staged in a temporary file in the same directory and renamed over the target,
// so a crash never leaves a half-written token and an existing looser file is
// replaced rather than reused. replaceFile carries the platform-specific
// replacement behavior. POSIX mode bits are applied only where the OS enforces
// them; on Windows the user-profile ACL governs access.
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

	tmp, err := os.CreateTemp(dir, ".token-*.tmp")
	if err != nil {
		return fmt.Errorf("credentials: create temporary file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}

	if PermissionsAreEnforced {
		if err := tmp.Chmod(0o600); err != nil {
			cleanup()
			return fmt.Errorf("credentials: restrict %s: %w", tmpName, err)
		}
	}
	if _, err := tmp.WriteString(secret); err != nil {
		cleanup()
		return fmt.Errorf("credentials: write %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("credentials: close %s: %w", tmpName, err)
	}
	if err := replaceFile(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("credentials: replace %s: %w", path, err)
	}
	return nil
}
