// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// shutdownGracePeriod is the maximum time runShutdown waits after graceful
// termination before force-killing remaining peer processes.
const shutdownGracePeriod = 5 * time.Second

// listProcesses enumerates every process on the machine. A variable because
// the listing fails only when procfs itself is unreadable, which no input to
// this binary produces, and the branch that reports it is otherwise never run.
var listProcesses = process.Processes

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
	return stopPeers(shutdownPeersOf(peers), os.Stderr, shutdownGracePeriod)
}

// shutdownPeer is what stopping a peer asks of it, which *process.Process
// answers for a real one.
//
// It is an interface so the two phases can be driven on every platform
// (issue 1212). Windows ends a real peer with TerminateProcess, which no
// process can decline, so a peer that outlives the grace period, the poll
// that finds it alive and the kill that follows could not be reached there
// with a real process, and their conditions were never evaluated on it.
type shutdownPeer interface {
	Terminate() error
	IsRunning() (bool, error)
	Kill() error
}

// shutdownPeersOf is the listed processes as the peers stopPeers stops.
func shutdownPeersOf(procs []*process.Process) []shutdownPeer {
	peers := make([]shutdownPeer, 0, len(procs))
	for _, p := range procs {
		peers = append(peers, p)
	}
	return peers
}

// stopPeers asks every peer to terminate, polls until all have gone or grace
// has lapsed, then force-kills what is left, and reports on stderr. It
// returns the exit code, which is success either way: a peer that had to be
// killed is gone all the same.
func stopPeers(peers []shutdownPeer, stderr io.Writer, grace time.Duration) int {
	// Phase 1: graceful termination (SIGTERM on Unix, TerminateProcess on Windows).
	for _, p := range peers {
		_ = p.Terminate()
	}

	// Phase 2: poll until all exit or deadline.
	deadline := time.After(grace)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			forceKillPeers(peers, stderr)
			return 0
		case <-ticker.C:
			if countAlive(peers) == 0 {
				fmt.Fprintf(stderr, "shutdown: all instances terminated\n")
				return 0
			}
		}
	}
}

// forceKillPeers kills every peer still running once the grace period has
// lapsed, and reports the outcome on stderr.
//
// Zero kills is a possible outcome rather than a contradiction: a peer can
// exit between the last poll and the deadline, and then there is nothing left
// to kill and nothing to complain about.
func forceKillPeers(peers []shutdownPeer, stderr io.Writer) {
	var killed int
	for _, p := range peers {
		running, _ := p.IsRunning()
		if running {
			_ = p.Kill()
			killed++
		}
	}
	if killed > 0 {
		fmt.Fprintf(stderr, "shutdown: force-killed %d instance(s)\n", killed)
		return
	}
	fmt.Fprintf(stderr, "shutdown: all instances terminated\n")
}

// findPeers returns all running processes matching this binary's base name,
// excluding the current process. Binary names are normalised by stripping
// OS/arch suffixes and .exe extension so that "gitlab-mcp-server-linux-amd64"
// matches a process named "gitlab-mcp-server".
func findPeers() ([]*process.Process, error) {
	self := os.Getpid()
	baseName := canonicalBinaryName(filepath.Base(os.Args[0]))

	all, err := listProcesses()
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
func countAlive(procs []shutdownPeer) int {
	var n int
	for _, p := range procs {
		running, _ := p.IsRunning()
		if running {
			n++
		}
	}
	return n
}
