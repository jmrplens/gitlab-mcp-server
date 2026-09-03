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
// left behind — which is why the second case pays the grace period in full.
func TestRunShutdown_RunningPeers_AreStoppedBeforeItReturns(t *testing.T) {
	tests := []struct {
		name string
		// source is the executable copied under the peer name, and args is how
		// it is run.
		source string
		args   []string
		// graceful says the peer stops on SIGTERM, so the call must return
		// well inside the grace period rather than fall through to the kill.
		graceful bool
	}{
		{name: "a peer that stops on SIGTERM", source: "/bin/sleep", args: []string{"300"}, graceful: true},
		// A shell running a script that ignores SIGTERM: the process an
		// operator is really trying to clear when a server is wedged.
		//
		// The trailing "exit 0" is load-bearing. Without it, sleep is the last
		// command of the -c script, and a shell is then free to exec it in
		// place rather than fork: an ignored signal disposition survives exec,
		// so the optimization preserves the semantics and is perfectly correct.
		// What it does not preserve is the process name, and this test finds
		// its peers by name. On Linux /bin/sh is dash and forks; on macOS it is
		// bash, which took the shortcut, so both peers reported themselves as
		// "sleep" and the test saw none of them. Keeping a command after the
		// sleep leaves the shell resident on every platform.
		{name: "a peer that ignores SIGTERM", source: "/bin/sh", args: []string{"-c", "trap '' TERM; sleep 300; exit 0"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binary := filepath.Join(t.TempDir(), peerName(t))
			copyExecutable(t, tt.source, binary)
			withArgv0(t, binary)

			peers := []*peerProcess{startPeer(t, binary, tt.args...), startPeer(t, binary, tt.args...)}
			waitForPeers(t, len(peers))

			started := time.Now()
			code := runShutdown()
			elapsed := time.Since(started)

			if code != 0 {
				t.Errorf("runShutdown() = %d, want 0 once every instance is gone", code)
			}
			if tt.graceful && elapsed >= shutdownGracePeriod {
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
// derives from os.Args[0].
func peerName(t *testing.T) string {
	t.Helper()
	return "mcppeer-" + strconv.Itoa(os.Getpid()%100000)
}

// withArgv0 points os.Args[0] at path for the duration of the test, which is
// where findPeers reads the name it hunts for.
func withArgv0(t *testing.T, path string) {
	t.Helper()
	original := os.Args[0]
	os.Args[0] = path
	t.Cleanup(func() { os.Args[0] = original })
}

// copyExecutable copies src to dst and makes it runnable, so the child process
// reports dst's base name as its own.
func copyExecutable(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Skipf("no executable to copy for a peer process: %v", err)
	}
	if writeErr := os.WriteFile(dst, data, 0o700); writeErr != nil { //nolint:gosec // the copy has to be executable
		t.Fatalf("writing the peer binary: %v", writeErr)
	}
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

// startPeer runs one peer under the private binary name.
func startPeer(t *testing.T, binary string, args ...string) *peerProcess {
	t.Helper()
	// The binary is the copy this test just made, in its own temporary
	// directory.
	cmd := exec.CommandContext(t.Context(), binary, args...)
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
