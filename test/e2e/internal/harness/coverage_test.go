//go:build e2e

// coverage_test.go covers where a run's coverage data goes and what makes a
// child contribute any.
//
// The last test here is the one the seam exists for: an instrumented process
// that is asked to stop writes its counters, and the same process killed
// writes none, which is what every child of this suite was until closeSessions
// stopped canceling before it closed.

package harness

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestCoverageDir_Unset_MeasuresNothing checks the property that keeps an
// ordinary run free: with the variable absent, no directory is resolved, no
// directory is created and a child carries no GOCOVERDIR.
//
// A child given an empty one would not be silent about it: the runtime reports
// at exit that it could not write, on the stderr a failing call quotes.
func TestCoverageDir_Unset_MeasuresNothing(t *testing.T) {
	dir, err := coverageDir(testSettings(map[string]string{}))
	if err != nil {
		t.Fatalf("coverageDir() error = %v, want nil", err)
	}
	if dir != "" {
		t.Fatalf("coverageDir() = %q, want the empty string", dir)
	}

	vars, err := coverageVariables(testSettings(map[string]string{}))
	if err != nil {
		t.Fatalf("coverageVariables() error = %v, want nil", err)
	}
	if len(vars) != 0 {
		t.Fatalf("coverageVariables() = %v, want nothing", vars)
	}
}

// TestCoverageDir_RelativePath_IsRefused checks that a relative directory is
// an error naming the variable rather than a path resolved against whatever
// the run happened to start in.
//
// Resolving it would put the counters inside the session directory of each
// child, which closeSessions deletes wholesale at the end of the run, and the
// measurement would report nothing with nothing to say why.
func TestCoverageDir_RelativePath_IsRefused(t *testing.T) {
	_, err := coverageDir(testSettings(map[string]string{coverDirEnv: filepath.Join("dist", "e2e-cover")}))
	if err == nil {
		t.Fatal("coverageDir() accepted a relative path")
	}
	if !strings.Contains(err.Error(), coverDirEnv) {
		t.Fatalf("the error should name %s: %v", coverDirEnv, err)
	}
}

// TestCoverageDir_AbsolutePath_IsCreated checks that the directory exists
// before any child is handed it.
//
// The toolchain refuses a directory that is not there rather than creating
// one, and it refuses it on the child's stderr, where an operator looking at
// an empty coverage directory would never think to look.
func TestCoverageDir_AbsolutePath_IsCreated(t *testing.T) {
	want := filepath.Join(t.TempDir(), "cover", "ce")

	got, err := coverageDir(testSettings(map[string]string{coverDirEnv: want}))
	if err != nil {
		t.Fatalf("coverageDir() error = %v, want nil", err)
	}
	if got != want {
		t.Fatalf("coverageDir() = %q, want %q", got, want)
	}
	info, err := os.Stat(want)
	if err != nil {
		t.Fatalf("the coverage directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", want)
	}
}

// TestCoverageVariables_Set_ReachTheChild checks that GOCOVERDIR survives both
// environments a child can be given, and that the sorted order the launcher
// guarantees survives the new key.
//
// The HTTP half matters on its own: that environment is built by dropping the
// credential, and an implementation that dropped anything else with it would
// leave exactly the transport whose child is stopped by a signal contributing
// nothing.
func TestCoverageVariables_Set_ReachTheChild(t *testing.T) {
	dir := t.TempDir()
	vars, err := coverageVariables(testSettings(map[string]string{coverDirEnv: dir}))
	if err != nil {
		t.Fatalf("coverageVariables() error = %v, want nil", err)
	}

	env := newChildEnv(testSettings(map[string]string{
		envGitLabURL:   "http://gitlab.test",
		envGitLabToken: "glpat-x",
	}), t.TempDir(), vars)

	for name, environ := range map[string][]string{
		"stdio": env.environ(),
		"http":  env.environWithoutCredential(),
	} {
		t.Run(name, func(t *testing.T) {
			if got := environMap(t, environ)[goCoverDirVar]; got != dir {
				t.Fatalf("%s = %q, want %q", goCoverDirVar, got, dir)
			}
			if !slices.IsSorted(environ) {
				t.Fatalf("the child environment is not sorted: %v", environ)
			}
		})
	}
}

// TestCountCoverageCounters_MetaData_IsNotCounted checks that the report
// counts what a child wrote when it exited and not what it wrote when it
// started.
//
// The meta-data file is written once per binary at startup, so counting it
// would report a flush for a run in which every child was killed, which is
// precisely the state this measurement exists to make visible.
func TestCountCoverageCounters_MetaData_IsNotCounted(t *testing.T) {
	dir := t.TempDir()
	plant := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("planting %s: %v", name, err)
		}
	}
	// One directory of a run that started two children: the meta-data file
	// they share, a counter file each, and two things that are neither.
	plant("covmeta.4e5c1f")
	plant(counterFilePrefix + "4e5c1f.1234.16789")
	plant(counterFilePrefix + "4e5c1f.1235.16790")
	plant("notes.txt")
	if err := os.Mkdir(filepath.Join(dir, counterFilePrefix+"directory"), 0o750); err != nil {
		t.Fatalf("planting a directory: %v", err)
	}

	got, err := countCoverageCounters(dir)
	if err != nil {
		t.Fatalf("countCoverageCounters() error = %v, want nil", err)
	}
	if got != 2 {
		t.Fatalf("countCoverageCounters() = %d, want 2", got)
	}
}

// TestCoverageCounters_OnlyAChildAskedToStop_WritesThem is the whole argument
// for the termination seam, run against the toolchain rather than asserted
// about it.
//
// An instrumented program writes its counters from an exit hook, and an exit
// hook runs on a normal return from main and on nothing else. So a child that
// is asked to stop contributes what it executed and a child that is killed
// contributes nothing, however much it ran. Every child of this suite was in
// the second state until closeSessions stopped canceling the session context
// before it closed the sessions.
//
// The probe stands in for the server because what is under test is the pair of
// endings, not the binary: it installs the same signal handler cmd/server
// installs, and it is a few hundred milliseconds to build rather than the
// minutes an instrumented build of the real thing would cost on every push.
func TestCoverageCounters_OnlyAChildAskedToStop_WritesThem(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("there is no SIGTERM here, so the harness keeps exec's kill and this is not the ending under test")
	}
	probe := buildCoverageProbe(t)

	cases := []struct {
		name         string
		ending       func(*exec.Cmd)
		wantCounters bool
	}{
		{
			name:         "asked to stop",
			ending:       terminateOnCancel,
			wantCounters: true,
		},
		{
			name:         "killed",
			ending:       func(*exec.Cmd) {},
			wantCounters: false,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			vars, err := coverageVariables(testSettings(map[string]string{coverDirEnv: dir}))
			if err != nil {
				t.Fatalf("coverageVariables() error = %v, want nil", err)
			}
			env := newChildEnv(testSettings(map[string]string{
				envGitLabURL:   "http://gitlab.test",
				envGitLabToken: "glpat-x",
			}), t.TempDir(), vars)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cmd := exec.CommandContext(ctx, probe) //#nosec G204 -- the path is what buildCoverageProbe just built under t.TempDir
			testCase.ending(cmd)
			cmd.Env = env.environ()

			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatalf("opening the probe's stdout: %v", err)
			}
			if err = cmd.Start(); err != nil {
				t.Fatalf("starting the probe: %v", err)
			}
			// Waiting for the probe to say it is ready rather than sleeping:
			// a signal that arrives before the handler is installed kills the
			// process by the default disposition, which would make the two
			// endings the same one and the test green for the wrong reason.
			if _, err = bufio.NewReader(stdout).ReadString('\n'); err != nil {
				t.Fatalf("waiting for the probe to install its handler: %v", err)
			}

			cancel()
			// The error is the ending itself (a canceled context, or the
			// kill), so it is the exit that is asserted and not this.
			_ = cmd.Wait()

			counters, err := countCoverageCounters(dir)
			if err != nil {
				t.Fatalf("countCoverageCounters() error = %v, want nil", err)
			}
			if got := counters > 0; got != testCase.wantCounters {
				t.Fatalf("%d counter files in %s, want some = %t", counters, dir, testCase.wantCounters)
			}
		})
	}
}

// coverageProbeSource is a program shaped like the server in the one respect
// this test is about: it handles the termination signal and returns from main
// rather than dying where it stands.
const coverageProbeSource = `package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Println("ready")
	<-ctx.Done()
}
`

// buildCoverageProbe compiles the probe with coverage on and returns its path.
func buildCoverageProbe(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("writing the probe's %s: %v", name, err)
		}
	}
	// A module of its own, so the build is the probe and nothing else, and an
	// older language version than this repository's, which any toolchain that
	// can build this test can also build.
	write("go.mod", "module coverageprobe\n\ngo 1.24\n")
	write("main.go", coverageProbeSource)

	out := filepath.Join(dir, "probe")
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-cover", "-o", out, ".") //#nosec G204 -- every argument is a constant of this file, plus a path under t.TempDir
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the coverage probe: %v\n%s", err, output)
	}
	return out
}
