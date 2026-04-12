// Shutdown logic for terminating all running instances of this binary.
// Invoked via the --shutdown CLI flag by pe-agnostic-store before replacing
// the binary on disk so that MCP clients restart with the new version.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

const shutdownGracePeriod = 5 * time.Second

// runShutdown finds all running instances of this binary (excluding self),
// sends SIGTERM (Unix) / TerminateProcess (Windows), waits up to 5 seconds
// for graceful exit, then force-kills any remaining processes. Returns exit code.
func runShutdown() int {
	peers, err := findPeers()
	if err != nil {
		fmt.Fprintf(os.Stderr, "shutdown: error listing processes: %v\n", err)
		return 1
	}
	if len(peers) == 0 {
		return 0
	}

	fmt.Fprintf(os.Stderr, "shutdown: found %d running instance(s)\n", len(peers))

	// Phase 1: graceful termination (SIGTERM on Unix, TerminateProcess on Windows).
	for _, p := range peers {
		_ = p.Terminate()
	}

	// Phase 2: poll until all exit or deadline.
	deadline := time.After(shutdownGracePeriod)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			goto forceKill
		case <-ticker.C:
			if countAlive(peers) == 0 {
				fmt.Fprintf(os.Stderr, "shutdown: all instances terminated\n")
				return 0
			}
		}
	}

forceKill:
	var killed int
	for _, p := range peers {
		running, _ := p.IsRunning()
		if running {
			_ = p.Kill()
			killed++
		}
	}
	if killed > 0 {
		fmt.Fprintf(os.Stderr, "shutdown: force-killed %d instance(s)\n", killed)
	} else {
		fmt.Fprintf(os.Stderr, "shutdown: all instances terminated\n")
	}
	return 0
}

// findPeers returns all running processes matching this binary's base name,
// excluding the current process. Binary names are normalised by stripping
// OS/arch suffixes and .exe extension so that "gitlab-mcp-server-linux-amd64"
// matches a process named "gitlab-mcp-server".
func findPeers() ([]*process.Process, error) {
	self := os.Getpid()
	baseName := canonicalBinaryName(filepath.Base(os.Args[0]))

	all, err := process.Processes()
	if err != nil {
		return nil, err
	}

	var peers []*process.Process
	for _, p := range all {
		if int(p.Pid) == self {
			continue
		}
		name, nameErr := p.Name()
		if nameErr != nil {
			continue
		}
		if canonicalBinaryName(name) == baseName {
			peers = append(peers, p)
		}
	}
	return peers, nil
}

// canonicalBinaryName strips OS/arch suffixes and .exe extension from a
// binary name so that all platform variants compare equal.
//
//	"gitlab-mcp-server-linux-amd64" → "gitlab-mcp-server"
//	"gitlab-mcp-server.exe"         → "gitlab-mcp-server"
//	"gitlab-mcp-server"             → "gitlab-mcp-server"
func canonicalBinaryName(name string) string {
	name = strings.TrimSuffix(name, ".exe")
	for _, suffix := range []string{
		"-linux-amd64", "-linux-arm64",
		"-darwin-amd64", "-darwin-arm64",
		"-windows-amd64", "-windows-arm64",
	} {
		name = strings.TrimSuffix(name, suffix)
	}
	return name
}

// countAlive returns how many processes in the list are still running.
func countAlive(procs []*process.Process) int {
	var n int
	for _, p := range procs {
		running, _ := p.IsRunning()
		if running {
			n++
		}
	}
	return n
}
