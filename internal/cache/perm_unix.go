//go:build !windows

package cache

func permissionsAreEnforced() bool { return true }
