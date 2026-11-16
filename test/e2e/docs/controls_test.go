// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// controls_test.go holds the runner to the verdicts it exists to give, on
// pages planted here and run in a local bash rather than in the reader, so
// they need neither GitLab nor Docker.
//
// The negative controls are the point: a runner that passes every page also
// passes the documentation, so each way a reader's step can break (a command
// that fails, one that hangs, a server that exits on its flags or stops
// later, a server that never answers) is planted once and must come out red,
// naming the page and the line. The positive ones pin what a reader's steps
// rely on: the substitutions, a server left running for the later blocks, and
// nothing left running after the page.
package docse2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/docblocks"
)

// localShell is the reader of the controls: bash on this machine, in a home
// directory of its own, so the runner's scripts run exactly as they do in the
// container, process groups and pid files included.
type localShell struct {
	home string
}

// newLocalShell returns a local reader in a fresh home directory, skipping on
// a machine whose bash cannot stand in for the reader's.
func newLocalShell(t *testing.T) *localShell {
	t.Helper()
	for _, tool := range []string{"bash", "setsid", "timeout"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("the controls run the runner's scripts in a local bash and need %s: %v", tool, err)
		}
	}
	return &localShell{home: t.TempDir()}
}

// write puts content at name under the home directory.
func (s *localShell) write(_ context.Context, name string, content []byte) error {
	path := filepath.Join(s.home, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o600)
}

// run runs script with bash in the home directory.
func (s *localShell) run(ctx context.Context, script string) (string, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	cmd.Dir = s.home
	cmd.Env = append(os.Environ(), "HOME="+s.home)
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// The values the controls' substitutions put in place of the page's.
const (
	controlVersion = "9.9.9"
	controlURL     = "http://gitlab-stand-in"
	controlToken   = "glpat-controlsecret"
)

// controlRunner returns a runner over a local reader with windows short
// enough for a control to fail in seconds.
func controlRunner(t *testing.T) (*runner, *localShell) {
	t.Helper()
	shell := newLocalShell(t)
	return &runner{
		shell:        shell,
		subs:         substitutions{version: controlVersion, gitlabURL: controlURL, token: controlToken},
		blockTimeout: 2 * time.Second,
		settle:       300 * time.Millisecond,
		readiness:    3 * time.Second,
		poll:         100 * time.Millisecond,
	}, shell
}

// markedBlocks parses a planted page and returns the blocks it marks.
func markedBlocks(t *testing.T, src string) []docblocks.Block {
	t.Helper()
	blocks, err := docblocks.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parsing the planted page: %v", err)
	}
	var out []docblocks.Block
	for _, block := range blocks {
		if block.Run != docblocks.RunNone {
			out = append(out, block)
		}
	}
	return out
}

// fence spells a marked bash block of a planted page.
func fence(mode docblocks.RunMode, body string) string {
	return fmt.Sprintf("```bash %s\n%s\n```\n\n", mode, body)
}

// TestRunPage_BrokenStep_IsRed is the negative controls: every planted page
// carries one step a reader would see fail, and the runner must fail the page
// at that step, naming the page, the line and what went wrong.
func TestRunPage_BrokenStep_IsRed(t *testing.T) {
	t.Parallel()

	bg := docblocks.RunBackground
	fg := docblocks.RunForeground
	tests := []struct {
		name string
		page string
		want error
		// says is what the report must contain: the line, and the output
		// or the reason a reader would be shown.
		says []string
	}{
		{
			name: "a command that fails",
			page: fence(fg, "echo fine") + fence(fg, "echo about to fail\nfalse\necho never"),
			want: errBlockFailed,
			says: []string{"guide.mdx:5", "about to fail", "exit status 1"},
		},
		{
			name: "a command that is not there",
			page: fence(fg, "no-such-command-anywhere --help"),
			want: errBlockFailed,
			says: []string{"guide.mdx:1", "no-such-command-anywhere"},
		},
		{
			name: "a command that hangs",
			page: fence(fg, "sleep 30"),
			want: errBlockTimedOut,
			says: []string{"guide.mdx:1", "did not end within 2s"},
		},
		{
			name: "a server that exits on its flags",
			page: fence(bg, "echo 'unknown flag: --htpp' >&2\nexit 2") + fence(fg, "true"),
			want: errBackgroundStopped,
			says: []string{"guide.mdx:1", "unknown flag: --htpp", "stopped"},
		},
		{
			name: "a server that stops before a later block",
			page: fence(bg, "sleep 0.5") + fence(fg, "sleep 1") + fence(fg, "true"),
			want: errBackgroundStopped,
			says: []string{"guide.mdx:1", "stopped", "guide.mdx:9"},
		},
		{
			name: "a server that stops before the page ends",
			page: fence(bg, "sleep 0.5") + fence(fg, "sleep 1"),
			want: errBackgroundStopped,
			says: []string{"guide.mdx:1", "stopped", "end of the page"},
		},
		{
			name: "a server that stops before the next one starts",
			page: fence(bg, "sleep 0.5") + fence(fg, "sleep 1") + fence(bg, "sleep 30"),
			want: errBackgroundStopped,
			says: []string{"guide.mdx:1", "stopped", "guide.mdx:9"},
		},
		{
			name: "a server that never answers its probe",
			page: fence(bg, "sleep 30") + fence(fg, "test -e ready"),
			want: errBlockFailed,
			says: []string{"guide.mdx:5", "readiness", "3s"},
		},
		{
			name: "a server that dies while its probe waits",
			page: fence(bg, "echo listening; sleep 0.6; echo crashed; exit 3") + fence(fg, "test -e ready"),
			want: errBackgroundStopped,
			says: []string{"guide.mdx:1", "crashed", "guide.mdx:5"},
		},
		{
			name: "a probe that hangs past the window is not retried forever",
			page: fence(bg, "sleep 30") + fence(fg, "sleep 30"),
			want: errBlockTimedOut,
			says: []string{"guide.mdx:5", "did not end within 2s"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r, shell := controlRunner(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			err := r.runPage(ctx, "guide.mdx", markedBlocks(t, tt.page))
			if !errors.Is(err, tt.want) {
				t.Fatalf("runPage = %v; want an error wrapping %q", err, tt.want)
			}
			for _, say := range tt.says {
				if !strings.Contains(err.Error(), say) {
					t.Errorf("the report does not say %q:\n%v", say, err)
				}
			}
			assertNothingRunning(t, shell)
		})
	}
}

// TestRunPage_ReaderSteps_Pass pins what a passing page relies on: a server
// left running for the blocks after it, a probe retried until the server is
// up, a second server replacing the first, and the substitutions.
func TestRunPage_ReaderSteps_Pass(t *testing.T) {
	t.Parallel()

	bg := docblocks.RunBackground
	fg := docblocks.RunForeground
	tests := []struct {
		name string
		page string
	}{
		{
			name: "commands that end",
			page: fence(fg, "echo one > one.txt") + fence(fg, "grep -q one one.txt"),
		},
		{
			name: "a server the later blocks reach, probed until it is up",
			page: fence(bg, "sleep 1\necho up > ready\nsleep 30") + fence(fg, "test -e ready") + fence(fg, "test -e ready"),
		},
		{
			name: "a second server replaces the first",
			page: fence(bg, "echo $$ > first.pid\nsleep 30") + fence(bg, "sleep 30") +
				fence(fg, `! kill -0 "$(cat first.pid)" 2>/dev/null`),
		},
		{
			name: "a pipeline is judged by its last command, as in a terminal",
			page: fence(fg, "yes | head -c 10"),
		},
		{
			name: "the version, the instance and the token are substituted",
			page: fence(fg, strings.Join([]string{
				`test "%%VERSION%%" = "` + controlVersion + `"`,
				`test "https://gitlab.com" = "` + controlURL + `"`,
				`test "--gitlab-url=https://gitlab.com,https://gitlab.example.com" = "--gitlab-url=` + controlURL + "," + controlURL + `"`,
				`test "https://gitlab.company.com" = "https://gitlab.company.com"`,
				`test "https://gitlab.internal.example.com" = "https://gitlab.internal.example.com"`,
				`test "glpat-your-token" = "` + controlToken + `"`,
				`test "glpat-xxxxxxxxxxxxxxxxxxxx" = "` + controlToken + `"`,
				`test "glpat-xxx" = "glpat-xxx"`,
			}, "\n")),
		},
		{
			name: "a page that marks nothing",
			page: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r, shell := controlRunner(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := r.runPage(ctx, "guide.mdx", markedBlocks(t, tt.page)); err != nil {
				t.Fatalf("runPage: %v", err)
			}
			assertNothingRunning(t, shell)
		})
	}
}

// TestRunPage_FailureReport_NeverPrintsTheToken pins that the token a block
// was handed is redacted from what a failure prints, since a report goes to
// a terminal and to the saved output of the run.
func TestRunPage_FailureReport_NeverPrintsTheToken(t *testing.T) {
	t.Parallel()

	r, shell := controlRunner(t)
	page := fence(docblocks.RunBackground, "echo serving with glpat-your-token\nexit 1") +
		fence(docblocks.RunForeground, "true")
	err := r.runPage(context.Background(), "guide.mdx", markedBlocks(t, page))
	if !errors.Is(err, errBackgroundStopped) {
		t.Fatalf("runPage = %v; want an error wrapping %q", err, errBackgroundStopped)
	}
	if strings.Contains(err.Error(), controlToken) {
		t.Errorf("the report prints the token:\n%v", err)
	}
	if !strings.Contains(err.Error(), "serving with [token]") {
		t.Errorf("the report does not show the output with the token redacted:\n%v", err)
	}

	r, shell2 := controlRunner(t)
	err = r.runPage(context.Background(), "guide.mdx", markedBlocks(t, fence(docblocks.RunForeground, "echo glpat-xxxxxxxx\nfalse")))
	if !errors.Is(err, errBlockFailed) || strings.Contains(err.Error(), controlToken) || !strings.Contains(err.Error(), "[token]") {
		t.Errorf("runPage = %v; want a failure that redacts the token", err)
	}
	assertNothingRunning(t, shell)
	assertNothingRunning(t, shell2)
}

// TestRunPage_ReaderUnusable_IsRed pins that a reader the runner cannot
// write to or run in fails the page rather than passing it with nothing run.
func TestRunPage_ReaderUnusable_IsRed(t *testing.T) {
	t.Parallel()

	page := fence(docblocks.RunBackground, "sleep 30") + fence(docblocks.RunForeground, "true")
	tests := []struct {
		name  string
		shell shell
		want  string
	}{
		{name: "nothing can be written", shell: brokenShell{writeErr: errors.New("disk full")}, want: "disk full"},
		{name: "nothing can be run", shell: brokenShell{runErr: errors.New("no such container")}, want: "no such container"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := &runner{shell: tt.shell, blockTimeout: time.Second, settle: time.Millisecond, readiness: time.Second, poll: time.Millisecond}
			err := r.runPage(context.Background(), "guide.mdx", markedBlocks(t, page))
			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), "guide.mdx:1") {
				t.Errorf("runPage = %v; want a failure at guide.mdx:1 naming %q", err, tt.want)
			}
		})
	}
}

// brokenShell is a reader that refuses what the runner asks of it.
type brokenShell struct {
	writeErr error
	runErr   error
}

func (s brokenShell) write(context.Context, string, []byte) error { return s.writeErr }

func (s brokenShell) run(context.Context, string) (string, error) { return "", s.runErr }

// assertNothingRunning fails the test when a block the runner started is
// still running after the page: every pid file it wrote names a process
// group that is gone.
func assertNothingRunning(t *testing.T, shell *localShell) {
	t.Helper()
	out, err := shell.run(context.Background(), `
for f in `+runnerDir+`/*.pid; do
  [ -e "$f" ] || continue
  if kill -0 -- -"$(cat "$f")" 2>/dev/null; then echo "still running: $f"; fi
done`)
	if err != nil || out != "" {
		t.Errorf("after the page: %v %s", err, out)
	}
}
