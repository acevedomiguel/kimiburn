//go:build !windows

package credentials

import (
	"os"
	"path/filepath"
	"testing"
)

// On Unix the rename replaces the destination atomically, so replacement works
// even when the destination carries no write bit.
func TestReplaceFileOverwritesDestination(t *testing.T) {
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
	raw, err := os.ReadFile(dst)
	if err != nil || string(raw) != "new-token" {
		t.Errorf("contents = %q err=%v, want the replacement", raw, err)
	}
}
