//go:build !linux && !darwin

// descriptor_limit_other.go answers for the platforms with no RLIMIT_NOFILE this
// server reads: Windows has no per-process descriptor limit of that kind.

package main

// descriptorLimit says the platform has no limit to read, so the held-request
// ceiling is sized against the register's fallback (register row HLD-011).
func descriptorLimit() (uint64, bool) {
	return 0, false
}
