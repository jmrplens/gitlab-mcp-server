// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// requestTimeout bounds one request to SonarCloud, whose answers are small.
const requestTimeout = time.Minute

// errNotPorted is what every step answers until it is ported.
var errNotPorted = errors.New("not ported yet")

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

// main reads the command line and returns the exit code.
func (s *session) main(context.Context, []string) int {
	fmt.Fprintf(s.stderr, "ERROR: %v\n", errNotPorted)
	return 3
}

// property is the value a line of a properties file gives name.
func property([]byte, string) (string, error) {
	return "", errNotPorted
}
