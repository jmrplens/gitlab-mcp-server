// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package conditions

import (
	"context"
	"io"
	"os/exec"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// installCommand is the command line that puts the pinned gobco on PATH, which
// a failed run through go run offers.
const installCommand = "go run ./cmd/coverage_recipes install-gobco"

// Tools is what a recipe reaches outside itself: the go command, how a
// program is run, how gobco is looked up on PATH, and how the pause between
// two attempts at installing it is waited out.
type Tools struct {
	Go       string
	Run      recipe.Runner
	LookPath func(name string) (string, error)
	Pause    func(ctx context.Context, wait time.Duration)
}

// DefaultTools are the tools production measures with: the toolchain's own go
// command and the PATH of this process.
func DefaultTools() Tools {
	return Tools{Go: golist.Executable(), Run: recipe.Exec, LookPath: exec.LookPath, Pause: pause}
}

// Request is one package to measure: the directory the package is named from,
// the package and the build tags it is measured under, the environment the
// settings are read from and every program is given, and where the report and
// the messages go.
type Request struct {
	Dir     string
	Package string
	Tags    string
	Environ []string
	Stdout  io.Writer
	Stderr  io.Writer
}

// pause waits for wait, or until ctx is done.
func pause(ctx context.Context, wait time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(wait):
	}
}

// Measure is not ported yet.
func Measure(_ context.Context, _ Tools, _ Request) int { return -1 }

// Install is not ported yet.
func Install(_ context.Context, _ Tools, _ []string, _, _ io.Writer) int { return -1 }
