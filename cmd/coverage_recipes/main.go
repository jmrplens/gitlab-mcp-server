// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/conditions"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// toolName is how every message about this command's own arguments names its
// source. A recipe's messages name the tool it drives, gobco or gremlins, or
// the gate, as they did when each was a script.
const toolName = "coverage_recipes"

// usage is printed when the subcommand is missing or unknown.
const usage = `usage: coverage_recipes <conditions|install-gobco|mutants|spec-conditions> [arguments]
  conditions <package> [tags]                    condition or branch coverage of one package (gobco)
  install-gobco                                  put the pinned gobco on PATH, so measuring needs no network
  mutants <package> [budget] [floor] [ceiling]   mutation-test one package (gremlins)
  spec-conditions [base]                         condition coverage of the action_specs.go a branch changes
`

// osExit is a seam over os.Exit, so a test can observe the exit code main
// passes on without ending the test process.
var osExit = os.Exit

// tools is what the recipes reach outside this process: the condition
// recipe's own tools, which the others share, the clock the mutation recipe
// times its baseline with, the platform, the address-space limits it reads and
// whether it may set one before it sets a ceiling, and the condition recipe as
// the gate over action specs runs it.
type tools struct {
	conditions.Tools
	now      func() time.Time
	goos     string
	limits   func() (soft, hard uint64, err error)
	tryLimit func(maxBytes uint64) error
	measure  func(ctx context.Context, tools conditions.Tools, req conditions.Request) int
}

// session is one run of a subcommand: where it runs, the environment it reads
// and hands on, its streams, and its tools.
type session struct {
	tools

	dir     string
	environ []string
	stdout  io.Writer
	stderr  io.Writer
}

func main() {
	ctx, stop := recipe.Signals(context.Background())
	status := defaultSession().run(ctx, os.Args[1:])
	stop()
	osExit(status)
}

// run dispatches a subcommand and returns the process exit code: 2 for a
// usage error, otherwise whatever the subcommand returns.
func (s *session) run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprint(s.stderr, usage)
		return 2
	}
	switch args[0] {
	case "conditions":
		return s.conditions(ctx, args[1:])
	case "install-gobco":
		return s.installGobco(ctx, args[1:])
	case "mutants":
		return s.mutants(ctx, args[1:])
	case "spec-conditions":
		return s.specConditions(ctx, args[1:])
	default:
		fmt.Fprintf(s.stderr, "%s: unknown subcommand %q\n%s", toolName, args[0], usage)
		return 2
	}
}

// conditions is the conditions subcommand: gobco over one package, as
// cmd/internal/conditions describes it.
func (s *session) conditions(ctx context.Context, args []string) int {
	if len(args) == 0 || len(args) > 2 || args[0] == "" {
		fmt.Fprintf(s.stderr, "%s: conditions takes a package, and build tags after it\n", toolName)
		return conditions.Refused
	}
	return s.measure(ctx, s.Tools, conditions.Request{
		Dir: s.dir, Package: args[0], Tags: strings.Join(args[1:], ""), Environ: s.environ, Stdout: s.stdout, Stderr: s.stderr,
	})
}

// installGobco is the install-gobco subcommand: the pinned gobco put on PATH.
func (s *session) installGobco(ctx context.Context, args []string) int {
	if len(args) > 0 {
		fmt.Fprintf(s.stderr, "%s: install-gobco takes no argument\n", toolName)
		return conditions.Refused
	}
	return conditions.Install(ctx, s.Tools, s.environ, s.stdout, s.stderr)
}

// defaultSession is the session production runs: this process's directory,
// environment and streams, and the real tools.
func defaultSession() *session {
	return &session{
		Tools: conditions.DefaultTools(), now: time.Now, goos: runtime.GOOS,
		limits: recipe.AddressSpace, tryLimit: recipe.TryAddressSpace, measure: conditions.Measure,
		dir: ".", environ: os.Environ(), stdout: os.Stdout, stderr: os.Stderr,
	}
}
