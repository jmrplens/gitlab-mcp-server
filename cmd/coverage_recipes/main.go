// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/conditions"
)

// tools is what the recipes reach outside this process: the condition
// recipe's own tools, which the others share, the clock the mutation recipe
// times its baseline with, the platform and the address-space limits it reads
// before it sets a ceiling, and the condition recipe as the gate over action
// specs runs it.
type tools struct {
	conditions.Tools
	now     func() time.Time
	goos    string
	limits  func() (soft, hard uint64, err error)
	measure func(ctx context.Context, tools conditions.Tools, req conditions.Request) int
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
	os.Exit((&session{stdout: os.Stdout, stderr: os.Stderr}).run(context.Background(), os.Args[1:]))
}

// run knows no subcommand yet.
func (s *session) run(_ context.Context, args []string) int {
	fmt.Fprintf(s.stderr, "coverage_recipes: unknown subcommand in %q\n", args)
	return 2
}
