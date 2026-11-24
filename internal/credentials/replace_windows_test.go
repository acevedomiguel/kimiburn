//go:build windows

package credentials

import (
	"os"
	"path/filepath"
	"testing"
)

// A read-only destination makes MoveFileEx with MOVEFILE_REPLACE_EXISTING fail,
// so the Windows path must fall back to clearing the destination instead of
// leaving a stale token in place.
func TestReplaceFileOverwritesReadOnlyDestination(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "token")
	if err := os.WriteFile(dst, []byte("old-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	// On Windows Chmod toggles the read-only attribute.
	if err := os.Chmod(dst, 0o400); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, ".token-stage.tmp")
	if err := os.WriteFile(src, []byte("new-token"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := replaceFile(src, dst); err != nil {
		t.Fatalf("replaceFile: %v", err)
	}
	raw, err := os.ReadFile(dst)
	if err != nil || string(raw) != "new-token" {
		t.Errorf("contents = %q err=%v, want the replacement", raw, err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("the staged file must be moved, not copied: %v", err)
	}
}

// The ordinary Windows replacement must also succeed.
func TestReplaceFileOverwritesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "token")
	if err := os.WriteFile(dst, []byte("old-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, ".token-stage.tmp")
	if err := os.WriteFile(src, []byte("new-token"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := replaceFile(src, dst); err != nil {
		t.Fatalf("replaceFile: %v", err)
	}
	if raw, err := os.ReadFile(dst); err != nil || string(raw) != "new-token" {
		t.Errorf("contents = %q err=%v", raw, err)
	}
}
