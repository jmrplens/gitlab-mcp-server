// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package recipe

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"testing"
)

// TestExec_AnAddressSpaceCeiling_HoldsWhatItStartsAndNotItself runs a real
// program under a ceiling and reads the limit it finds: the soft limit is the
// ceiling, the hard one what this process had, and this process is under
// neither once the program has started.
func TestExec_AnAddressSpaceCeiling_HoldsWhatItStartsAndNotItself(t *testing.T) {
	softBefore, hardBefore, err := AddressSpace()
	if err != nil {
		t.Fatal(err)
	}
	const ceiling = 6 << 30
	if hardBefore < ceiling {
		t.Skipf("this process is already held under %d bytes of address space", hardBefore)
	}
	for _, tc := range []struct {
		name          string
		asked, within uint64
	}{{"a ceiling", ceiling, ceiling}, {"none", 0, softBefore}} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			call := helper(t, "address-space")
			call.AddressSpace, call.Stdout = tc.asked, &stdout
			if status, runErr := Exec(t.Context(), call); runErr != nil || status != 0 {
				t.Fatalf("Exec() = %d, %v", status, runErr)
			}
			if want := fmt.Sprintln(tc.within, hardBefore, nil); stdout.String() != want {
				t.Errorf("the program found the limits %q, want %q", stdout.String(), want)
			}
			if soft, hard, getErr := AddressSpace(); getErr != nil || soft != softBefore || hard != hardBefore {
				t.Errorf("this process is held to %d and %d (%v), want %d and %d", soft, hard, getErr, softBefore, hardBefore)
			}
		})
	}
}

// TestStartLimited_ARefusedLimit_IsAnErrorAndNothingRuns verifies that a
// limit that cannot be read or set keeps the program from starting, that one
// that cannot be put back ends the program it was set for, and that a program
// that cannot start is that error with the limit put back.
func TestStartLimited_ARefusedLimit_IsAnErrorAndNothingRuns(t *testing.T) {
	planted := errors.New("planted")
	restore := func() { getrlimit, setrlimit = syscall.Getrlimit, syscall.Setrlimit }
	t.Cleanup(restore)
	refuseSet := func(from int) func(int, *syscall.Rlimit) error {
		calls := 0
		return func(int, *syscall.Rlimit) error {
			calls++
			if calls >= from {
				return planted
			}
			return nil
		}
	}
	cases := []struct {
		name    string
		prepare func()
		absent  bool
		want    error
	}{
		{name: "the limit cannot be read", prepare: func() { getrlimit = func(int, *syscall.Rlimit) error { return planted } }, want: planted},
		{name: "the limit cannot be set", prepare: func() { setrlimit = refuseSet(1) }, want: planted},
		{name: "the limit cannot be put back", prepare: func() { setrlimit = refuseSet(2) }, want: planted},
		{name: "neither the program nor the limit", prepare: func() { setrlimit = refuseSet(2) }, absent: true, want: errors.ErrUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restore()
			tc.prepare()
			call := helper(t, "address-space")
			cmd := exec.CommandContext(t.Context(), call.Program, call.Args...) //#nosec G204 -- this test binary
			cmd.Env = call.Env
			if tc.absent {
				cmd.Err = errors.ErrUnsupported
			}
			if err := startLimited(cmd, 6<<30); !errors.Is(err, tc.want) {
				t.Errorf("startLimited() = %v, want %v", err, tc.want)
			}
			if cmd.Process != nil && cmd.ProcessState == nil {
				t.Error("the program was left running")
			}
		})
	}
	restore()
	getrlimit = func(int, *syscall.Rlimit) error { return planted }
	if _, _, err := AddressSpace(); !errors.Is(err, planted) {
		t.Errorf("AddressSpace() error = %v, want the planted one", err)
	}
}
