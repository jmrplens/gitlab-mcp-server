//go:build !linux && !darwin

// descriptor_limit_other.go answers for the platforms with no RLIMIT_NOFILE this
// server reads: Windows has no per-process descriptor limit of that kind.

package main

// descriptorLimit says the platform has no limit to read, so the held-request
// ceiling is sized against the register's fallback (register row HLD-011).
// Issue 951 kept that fallback rather than no ceiling, so a Windows process
// is not left unbounded by default although 192 protects no descriptor there.
func descriptorLimit() (uint64, bool) {
	return 0, false
}
