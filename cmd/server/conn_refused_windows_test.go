//go:build windows

// conn_refused_windows_test.go pins the one constant that decides whether a
// Windows deployment can recover from its own unclean exit.
package main

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"testing"
)

// TestIsConnRefused_TellsWinsocksRefusalFromEverythingElse verifies both halves
// of the predicate clearStaleSocket rests on.
//
// Recognizing the refusal is what lets a stale socket be removed at all here,
// and it went unrecognized for as long as the check compared against
// [syscall.ECONNREFUSED] alone: a server whose previous run exited uncleanly
// then refused to start until somebody deleted the socket by hand.
//
// Not recognizing anything else matters at least as much in the other
// direction. Only "nothing is listening" proves a socket is dead; a timeout, a
// permission denial or a cancelled probe leaves the question open, and removing
// on an open question is how a live server loses its socket to a racing
// restart.
func TestIsConnRefused_TellsWinsocksRefusalFromEverythingElse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "Winsock's refusal, wrapped the way a failed dial returns it",
			err:  &os.SyscallError{Syscall: "connect", Err: wsaeConnRefused},
			want: true,
		},
		{
			name: "the POSIX spelling, which no socket here produces but which costs nothing to accept",
			err:  syscall.ECONNREFUSED,
			want: true,
		},
		{
			name: "a cancelled probe answered nothing",
			err:  fmt.Errorf("dial unix: %w", context.Canceled),
			want: false,
		},
		{
			name: "a permission denial answered nothing either",
			err:  &os.SyscallError{Syscall: "connect", Err: syscall.EACCES},
			want: false,
		},
		{
			name: "no error at all is not a refusal",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isConnRefused(tt.err); got != tt.want {
				t.Errorf("isConnRefused(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
