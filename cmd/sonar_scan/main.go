// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// requestTimeout bounds one request to SonarCloud, whose answers are small.
const requestTimeout = time.Minute

// totalLine is the line go tool cover ends its report on.
var totalLine = regexp.MustCompile(`(?m)^total:.*[ \t](\d+(?:\.\d+)?)%\r?$`)

// osExit is a seam over os.Exit, so a test can observe the exit code main
// passes on without ending the test process.
var osExit = os.Exit

// session is one run: where it runs, the environment it reads and hands on,
// its streams, and what it reaches outside this process.
type session struct {
	dir      string
	environ  []string
	stdout   io.Writer
	stderr   io.Writer
	goTool   string
	run      recipe.Runner
	lookPath func(name string) (string, error)
	client   *http.Client
	now      func() time.Time
	sleep    func(wait time.Duration)
}

func main() {
	osExit(defaultSession().main(context.Background(), os.Args[1:]))
}

// defaultSession is the session production runs: this process's directory,
// environment and streams, and the real tools.
func defaultSession() *session {
	return &session{
		dir: ".", environ: os.Environ(), stdout: os.Stdout, stderr: os.Stderr,
		goTool: golist.Executable(), run: recipe.Exec, lookPath: exec.LookPath,
		client: &http.Client{Timeout: requestTimeout}, now: time.Now, sleep: time.Sleep,
	}
}

// main reads the command line and returns the exit code: 2 for a usage error,
// a failing program's own, 1 for any other failure and for a gate that is not
// OK, and 0 for one that is.
func (s *session) main(ctx context.Context, args []string) int {
	flags := flag.NewFlagSet("sonar_scan", flag.ContinueOnError)
	flags.SetOutput(s.stderr)
	noScan := flags.Bool("no-scan", false, "fetch the gate of the last analysis, without running the tests or uploading one")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(s.stderr, "sonar_scan: it takes no argument, and was given %q\n", flags.Args())
		return 2
	}
	status, err := s.pipeline(ctx, *noScan)
	if err != nil {
		fmt.Fprintf(s.stderr, "ERROR: %v\n", err)
		return 1
	}
	return status
}

// pipeline runs the steps and returns the exit code. An error is a step that
// could not be done.
func (s *session) pipeline(ctx context.Context, noScan bool) (int, error) {
	read, err := s.readSettings(ctx)
	if err != nil {
		return 0, err
	}
	if !noScan {
		status, scanErr := s.scan(ctx, read)
		if scanErr != nil || status != 0 {
			return status, scanErr
		}
	}
	return s.reportGate(ctx, read)
}

// scan runs the tests, uploads the analysis and waits for it, and returns a
// failing program's status.
func (s *session) scan(ctx context.Context, read settings) (int, error) {
	fmt.Fprintf(s.stdout, "==> [1/4] Running tests with coverage (branch: %s)\n", read.branch)
	if status, err := s.run(ctx, s.call(read, s.goTool, s.environ, "test", "-count=1", "-coverpkg=./cmd/...,./internal/...", "-coverprofile=coverage.out", "./cmd/...", "./internal/...")); err != nil || status != 0 {
		return status, err
	}
	if err := s.holdCoverage(ctx, read); err != nil {
		return 0, err
	}
	fmt.Fprintln(s.stdout, "==> [2/4] Uploading analysis to SonarCloud")
	scanner, err := s.lookPath("sonar-scanner")
	if err != nil {
		return 0, fmt.Errorf("find sonar-scanner: %w", err)
	}
	upload := s.call(read, scanner, slices.Concat(s.environ, read.dotenv), "-Dsonar.token="+read.token, "-Dsonar.host.url="+read.host, "-Dsonar.branch.name="+read.branch)
	status, err := s.run(ctx, upload)
	if err != nil || status != 0 {
		return status, err
	}
	fmt.Fprintln(s.stdout, "==> [3/4] Waiting for the Compute Engine task to finish")
	return 0, s.awaitTask(ctx, read)
}

// call is a program run in the repository's root, its output the caller's.
func (s *session) call(read settings, program string, environ []string, args ...string) recipe.Call {
	return recipe.Call{Program: program, Args: args, Dir: read.root, Env: environ, Stdout: s.stdout, Stderr: s.stderr}
}

// holdCoverage reads the total of the profile the tests wrote, and refuses one
// below the minimum.
func (s *session) holdCoverage(ctx context.Context, read settings) error {
	printed, err := recipe.Output(ctx, s.run, recipe.Call{Program: s.goTool, Args: []string{"tool", "cover", "-func=coverage.out"}, Dir: read.root, Env: s.environ, Stderr: s.stderr})
	if err != nil {
		return err
	}
	found := totalLine.FindStringSubmatch(printed)
	if found == nil {
		return errors.New("go tool cover gave no total")
	}
	// What the line admits always parses, and the minimum was read as a
	// number with the other settings.
	total, _ := strconv.ParseFloat(found[1], 64)
	minimum, _ := strconv.ParseFloat(read.coverageMin, 64)
	fmt.Fprintf(s.stdout, "    total coverage: %s%% (minimum %s%%)\n", found[1], read.coverageMin)
	if total < minimum {
		return fmt.Errorf("coverage %s%% is below minimum %s%%", found[1], read.coverageMin)
	}
	return nil
}

// awaitTask polls the Compute Engine task sonar-scanner's report names until
// it ends or the time it is given is up.
func (s *session) awaitTask(ctx context.Context, read settings) error {
	address, err := s.taskAddress(read)
	if err != nil {
		return err
	}
	deadline := s.now().Add(read.pollTimeout)
	for {
		status, taskErr := s.taskStatus(ctx, read, address)
		if taskErr != nil {
			return taskErr
		}
		switch status {
		case "SUCCESS":
			fmt.Fprintln(s.stdout, "    analysis processed")
			return nil
		case "FAILED", "CANCELED":
			return fmt.Errorf("Compute Engine task %s", status) //nolint:staticcheck // the product's name opens the message a reader of the script knows
		}
		fmt.Fprintf(s.stdout, "    status: %s\r", status)
		if !s.now().Before(deadline) {
			return fmt.Errorf("timed out after %ds waiting for CE task", int(read.pollTimeout/time.Second))
		}
		s.sleep(read.pollInterval)
	}
}

// taskAddress is where the task the upload started answers, as sonar-scanner
// leaves it in its report.
func (s *session) taskAddress(read settings) (string, error) {
	report, err := os.ReadFile(filepath.Join(read.root, ".scannerwork", "report-task.txt"))
	if err != nil {
		return "", err
	}
	return property(report, "ceTaskUrl")
}

// property is the value a line of a properties file gives name.
func property(file []byte, name string) (string, error) {
	for line := range bytes.Lines(file) {
		if value, found := strings.CutPrefix(string(line), name+"="); found {
			return strings.TrimRight(value, "\r\n"), nil
		}
	}
	return "", fmt.Errorf("no %s= line", name)
}
