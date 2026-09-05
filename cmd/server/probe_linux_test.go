package main

import (
	"os"
	"os/exec"
	"testing"
)

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
