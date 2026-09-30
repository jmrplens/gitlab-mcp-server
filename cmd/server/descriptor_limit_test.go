//go:build linux || darwin

package main

import (
	"errors"
	"syscall"
	"testing"
)

// TestDescriptorLimitFrom_ReadsTheSoftLimit covers the read that sizes the
// held-request ceiling: the soft RLIMIT_NOFILE, which is the one an open is
// refused at, and not the hard one beside it.
func TestDescriptorLimitFrom_ReadsTheSoftLimit(t *testing.T) {
	t.Parallel()
	var asked int
	got, ok := descriptorLimitFrom(func(resource int, limit *syscall.Rlimit) error {
		asked = resource
		limit.Cur, limit.Max = 1024, 4096
		return nil
	})
	if !ok || got != 1024 {
		t.Errorf("descriptorLimitFrom = %d, %v; want the soft limit 1024", got, ok)
	}
	if asked != syscall.RLIMIT_NOFILE {
		t.Errorf("asked for resource %d, want RLIMIT_NOFILE", asked)
	}
}

// TestDescriptorLimitFrom_SaysNothingWhenTheReadFails covers a platform that
// will not say: the caller is told there is no limit to size from, and falls
// back to the register's figure.
func TestDescriptorLimitFrom_SaysNothingWhenTheReadFails(t *testing.T) {
	t.Parallel()
	got, ok := descriptorLimitFrom(func(int, *syscall.Rlimit) error { return errors.New("refused") })
	if ok || got != 0 {
		t.Errorf("descriptorLimitFrom = %d, %v after a failed read; want nothing", got, ok)
	}
}

// TestDescriptorLimit_ReadsThisProcess pins the platform's read to the
// process's own soft limit, the one the runtime raised to the hard limit
// before the tests began.
func TestDescriptorLimit_ReadsThisProcess(t *testing.T) {
	t.Parallel()
	var want syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &want); err != nil {
		t.Fatalf("Getrlimit: %v", err)
	}
	if got, ok := descriptorLimit(); !ok || got != want.Cur {
		t.Errorf("descriptorLimit = %d, %v; want this process's soft limit %d", got, ok, want.Cur)
	}
}
