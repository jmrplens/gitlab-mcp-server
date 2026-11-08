// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestStdinIsNullUnder_ReadsTheDescriptorLink covers the procfs read with a
// fake root: a link to the null device, a link elsewhere, and no process. It
// moved here with the function, which only Linux builds (issue 1212).
func TestStdinIsNullUnder_ReadsTheDescriptorLink(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mk := func(pid, target string) {
		dir := filepath.Join(root, pid, "fd")
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(dir, "0")); err != nil {
			t.Fatal(err)
		}
	}
	mk("10", os.DevNull)
	mk("11", "pipe:[12345]")

	cases := []struct {
		name    string
		pid     int32
		want    bool
		wantErr bool
	}{
		{name: "the null device", pid: 10, want: true},
		{name: "a pipe", pid: 11, want: false},
		{name: "no such process", pid: 12, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := stdinIsNullUnder(root, tc.pid)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("stdinIsNullUnder(%d) = %v, want an error", tc.pid, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("stdinIsNullUnder(%d): %v", tc.pid, err)
			}
			if got != tc.want {
				t.Errorf("stdinIsNullUnder(%d) = %v, want %v", tc.pid, got, tc.want)
			}
		})
	}
}

// TestPeerStdinIsNull_ReadsTheProcessFileDescriptor covers the procfs read
// behind a discovered --transport auto instance, against real children
// rather than a fabricated tree: the null device is what a container started
// without -i gives, and a pipe is what every MCP client gives.
//
// A child with no Stdin set is the null-device case, because that is what
// os/exec connects file descriptor 0 to when nothing is supplied.
func TestPeerStdinIsNull_ReadsTheProcessFileDescriptor(t *testing.T) {
	t.Parallel()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})

	cases := []struct {
		name  string
		stdin *os.File
		want  bool
	}{
		{name: "a child reading the null device", stdin: nil, want: true},
		{name: "a child reading a pipe", stdin: reader, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			child := exec.CommandContext(t.Context(), "/bin/sleep", "30")
			if tc.stdin != nil {
				child.Stdin = tc.stdin
			}
			if startErr := child.Start(); startErr != nil {
				t.Skipf("cannot start a child process: %v", startErr)
			}
			t.Cleanup(func() {
				_ = child.Process.Kill()
				_ = child.Wait()
			})

			got, readErr := peerStdinIsNull(pid32(t, child.Process.Pid))
			if readErr != nil {
				t.Fatalf("peerStdinIsNull: %v", readErr)
			}
			if got != tc.want {
				t.Errorf("peerStdinIsNull = %v, want %v", got, tc.want)
			}
		})
	}
}
