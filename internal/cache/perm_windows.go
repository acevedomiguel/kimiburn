//go:build windows

package cache

// Windows uses the inherited user-profile ACL; POSIX mode bits are not
// enforced by this package.
func permissionsAreEnforced() bool { return false }
