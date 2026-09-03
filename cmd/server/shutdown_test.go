// shutdown_test.go verifies --shutdown: how this binary recognizes other
// copies of itself, and what it does to them.
//
// The peers here are real processes, because every part of this that can be
// wrong is in the interaction with the operating system: which name a process
// reports, whether the signal arrives, and whether the wait observes the exit.
// They are made recognizable by pointing os.Args[0] at a private name for the
// duration of a test, so nothing on the machine outside the test's own children
// can match — which matters for a function whose job is to terminate whatever
// it matches.
package main

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// TestCanonicalBinaryName_PlatformVariantsCompareEqual covers the name
// comparison every peer decision rests on.
//
// The release ships one binary per platform under a decorated name, and an
// operator may rename it or not, so "gitlab-mcp-server-linux-amd64" and
// "gitlab-mcp-server.exe" have to be recognized as the same program. Getting
// this wrong is silent in both directions: too narrow and --shutdown reports no
// instances while one is running, too wide and it terminates something else.
func TestCanonicalBinaryName_PlatformVariantsCompareEqual(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "gitlab-mcp-server", want: "gitlab-mcp-server"},
		{name: "gitlab-mcp-server.exe", want: "gitlab-mcp-server"},
		{name: "gitlab-mcp-server-linux-amd64", want: "gitlab-mcp-server"},
		{name: "gitlab-mcp-server-linux-arm64", want: "gitlab-mcp-server"},
		{name: "gitlab-mcp-server-darwin-amd64", want: "gitlab-mcp-server"},
		{name: "gitlab-mcp-server-darwin-arm64", want: "gitlab-mcp-server"},
		{name: "gitlab-mcp-server-windows-amd64.exe", want: "gitlab-mcp-server"},
		{name: "gitlab-mcp-server-windows-arm64", want: "gitlab-mcp-server"},
		// Not one of ours: nothing is stripped, so it cannot collide with the
		// name above and be terminated by a --shutdown meant for this server.
		{name: "gitlab-runner", want: "gitlab-runner"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := canonicalBinaryName(tt.name); got != tt.want {
				t.Errorf("canonicalBinaryName(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

// TestCountAlive_CountsOnlyProcessesStillRunning covers the poll's predicate.
//
// It decides when the graceful phase is over, so a count that never reaches
// zero costs every --shutdown the full grace period, and one that reaches zero
// early reports success while a process is still holding the listener.
func TestCountAlive_CountsOnlyProcessesStillRunning(t *testing.T) {
	t.Parallel()

	self, err := process.NewProcess(pid32(t, os.Getpid()))
	if err != nil {
		t.Fatalf("looking up this process: %v", err)
	}
	// A pid that has exited and been reaped: whether the lookup resolves at
	// all is the operating system's business, and either answer means the same
	// thing to countAlive.
	gone := &process.Process{Pid: pid32(t, exitedPID(t))}

	tests := []struct {
		name  string
		procs []*process.Process
		want  int
	}{
		{name: "nothing to wait for", procs: nil, want: 0},
		{name: "this process is alive", procs: []*process.Process{self}, want: 1},
		{name: "an exited process is not counted", procs: []*process.Process{gone}, want: 0},
		{name: "one of each", procs: []*process.Process{self, gone}, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := countAlive(tt.procs); got != tt.want {
				t.Errorf("countAlive = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestRunShutdown_NoPeers_SucceedsWithoutTouchingAnything covers the ordinary
// case: somebody runs --shutdown when nothing is running.
//
// The exit status is the whole interface of this mode — an updater runs it
// before replacing the binary on disk and acts on what it returns — so "found
// nothing" has to be success rather than an error, and it must not be reached
// by matching some unrelated process on the machine.
func TestRunShutdown_NoPeers_SucceedsWithoutTouchingAnything(t *testing.T) {
	withArgv0(t, filepath.Join(t.TempDir(), peerName(t)))

	if got := runShutdown(); got != 0 {
		t.Errorf("runShutdown() = %d with no instance running, want 0", got)
	}
}

// TestRunShutdown_RunningPeers_AreStoppedBeforeItReturns covers both phases of
// the termination the updater depends on.
//
// Returning early is the failure that matters in either case, because the
// caller replaces the binary next and a surviving process would be writing to a
// file that no longer exists. A peer that honors SIGTERM must be waited for
// rather than assumed gone, and one that ignores it must be killed rather than
// left behind, which is why the second case pays the grace period in full.
//
// Only one of those two phases exists on Windows, so only part of this is
// assertable there. gopsutil's Terminate is TerminateProcess, which is a kill:
// a process cannot decline it, cannot run cleanup before it, and cannot be told
// apart by how long it took to go. The wedged case therefore has an impossible
// premise there and skips, and the graceful case keeps everything except the
// timing claim, which on Windows would be measuring how fast a kill is rather
// than whether a signal was honored. What survives on every platform is the
// contract the updater actually depends on: every peer is gone, and the exit
// status says so.
//
// This used to be hidden. The peers were copies of /bin/sleep and /bin/sh, and
// Windows has neither, so the whole test skipped there by accident. Building
// the peer from testdata removed that accident and left the distinction above
// to be made deliberately.
func TestRunShutdown_RunningPeers_AreStoppedBeforeItReturns(t *testing.T) {
	tests := []struct {
		name string
		// ignoresTerm makes the peer ignore SIGTERM, so the call has to fall
		// through to the kill.
		ignoresTerm bool
		// graceful says the peer stops on SIGTERM, so the call must return
		// well inside the grace period rather than fall through to the kill.
		graceful bool
	}{
		{name: "a peer that stops on SIGTERM", graceful: true},
		// The process an operator is really trying to clear when a server is
		// wedged.
		{name: "a peer that ignores SIGTERM", ignoresTerm: true},
	}

	// terminateIsASignal says whether Terminate asks the process to stop, which
	// it can decline, rather than ending it outright.
	terminateIsASignal := runtime.GOOS != "windows"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.ignoresTerm && !terminateIsASignal {
				t.Skip("Terminate is TerminateProcess here, which no process can ignore, so there is no fall-through to a kill to observe")
			}

			binary := buildPeer(t, filepath.Join(t.TempDir(), peerName(t)))
			withArgv0(t, binary)

			var env []string
			if tt.ignoresTerm {
				env = []string{peerIgnoreTermEnv + "=1"}
			}
			peers := []*peerProcess{startPeer(t, binary, env), startPeer(t, binary, env)}
			waitForPeers(t, len(peers))

			started := time.Now()
			code := runShutdown()
			elapsed := time.Since(started)

			if code != 0 {
				t.Errorf("runShutdown() = %d, want 0 once every instance is gone", code)
			}
			if tt.graceful && terminateIsASignal && elapsed >= shutdownGracePeriod {
				t.Errorf("runShutdown took %s: a peer that stops on SIGTERM must not cost the whole grace period", elapsed)
			}
			for i, peer := range peers {
				if !peer.exited(t) {
					t.Errorf("peer %d was still running when runShutdown returned", i)
				}
			}
		})
	}
}

// peerName returns a process name nothing else on the machine will carry.
//
// Kept under fifteen characters because that is where the kernel truncates the
// name a process reports, and a truncated name would not match what findPeers
// derives from os.Args[0]. The Windows suffix does not threaten that: it is
// added only where nothing truncates, and [canonicalBinaryName] strips it from
// both sides of the comparison anyway.
//
// The suffix is not cosmetic there. Windows will not execute a file without it,
// so a peer built under the bare name failed to start at all: `exec: "...
// \mcppeer-2504": executable file not found in %PATH%`.
func peerName(t *testing.T) string {
	t.Helper()

	name := "mcppeer-" + strconv.Itoa(os.Getpid()%100000)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// withArgv0 points os.Args[0] at path for the duration of the test, which is
// where findPeers reads the name it hunts for.
func withArgv0(t *testing.T, path string) {
	t.Helper()
	original := os.Args[0]
	os.Args[0] = path
	t.Cleanup(func() { os.Args[0] = original })
}

// peerIgnoreTermEnv is the variable testdata/peer reads to decide whether to
// ignore SIGTERM. Declared here rather than imported, because a program under
// testdata is deliberately not part of any package this one can import.
const peerIgnoreTermEnv = "PEER_IGNORE_SIGTERM"

// buildPeer compiles testdata/peer to path, so a peer process carries the name
// findPeers hunts for, and returns path.
//
// It builds rather than copying a system binary, which is what this used to do
// and what does not survive contact with a second platform: /bin/sleep and
// /bin/sh copied under a new name worked on Linux, and on macOS the shell copy
// started and never became visible under that name, so the test saw no peers at
// all. Whatever a platform does with a copy of its own shell, a program built
// from this repository does the same thing everywhere, and the two cases stop
// depending on which shell /bin/sh happens to be.
//
// It also stops orphaning a process. The shell forked "sleep 300", and
// runShutdown kills the processes it matched by name rather than their children,
// so every run of this test left a five-minute sleep behind.
func buildPeer(t *testing.T, path string) string {
	t.Helper()

	build := exec.CommandContext(t.Context(), "go", "build", "-o", path, "./testdata/peer")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the peer binary: %v\n%s", err, out)
	}
	return path
}

// peerProcess is one running peer, reaped as soon as it exits.
//
// The reaping is not incidental. An unreaped child stays visible as a zombie,
// which the poll under test reads as still running — true of a test's own child
// and never of the unrelated processes --shutdown really finds, so without it
// the graceful case could not be told from the wedged one.
type peerProcess struct {
	cmd    *exec.Cmd
	waited chan struct{}
	once   sync.Once
}

// exited reports whether the peer has stopped, waiting briefly for the reaper.
func (p *peerProcess) exited(t *testing.T) bool {
	t.Helper()
	select {
	case <-p.waited:
		return true
	case <-time.After(2 * time.Second):
		return false
	}
}

// startPeer runs one peer under the private binary name, with env added to the
// process environment.
func startPeer(t *testing.T, binary string, env []string) *peerProcess {
	t.Helper()
	// The binary is the one this test just built, in its own temporary
	// directory.
	cmd := exec.CommandContext(t.Context(), binary)
	cmd.Env = append(os.Environ(), env...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting a peer: %v", err)
	}
	peer := &peerProcess{cmd: cmd, waited: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		peer.once.Do(func() { close(peer.waited) })
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-peer.waited
	})
	return peer
}

// waitForPeers blocks until the operating system reports the expected number of
// peers, so the shutdown under test is not racing their startup.
func waitForPeers(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		peers, err := findPeers()
		if err != nil {
			t.Fatalf("findPeers: %v", err)
		}
		if len(peers) >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d peers were visible before the deadline", len(peers), want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// pid32 narrows a process id to the width gopsutil uses, failing the test
// rather than wrapping if a platform ever hands out a wider one.
func pid32(t *testing.T, pid int) int32 {
	t.Helper()
	if pid <= 0 || pid > math.MaxInt32 {
		t.Fatalf("pid %d does not fit the process id width", pid)
	}
	return int32(pid) //#nosec G115 -- bounded against MaxInt32 immediately above
}

// exitedPID starts a process, waits for it, and returns its pid, which is then
// a pid that is no longer running.
func exitedPID(t *testing.T) int {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "/bin/sleep", "0")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start a child process: %v", err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Wait()
	return pid
}
