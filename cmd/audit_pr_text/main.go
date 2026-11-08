// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"os"
)

// usage is the command line, with what each half judges and where it reads
// it from.
const usage = `Usage:
  audit_pr_text diff [BASE_REF]
  audit_pr_text description

diff         Fail when a line this branch adds carries an em dash (U+2014).
             BASE_REF defaults to $EM_DASH_BASE, then to origin/main. The
             comparison is the three-dot form, so only what this branch added
             on top of their merge base is judged.

description  Fail when the pull request title or body carries an em dash, a
             command that makes GitHub skip workflows or a reference GitLab
             resolves to another project's merge request or issue, or when
             the body carries a skip-checks trailer or a generated block a
             review bot injected.
             The pull request is $PR_NUMBER, or the one open for the current
             branch. Set PR_TITLE_FILE and PR_BODY_FILE to judge text from
             disk instead, which is how the gate is rehearsed without a pull
             request.
`

// exitProcess is [os.Exit] behind a seam, so the code [run] decided is a
// value a test can read rather than the end of the test binary.
var exitProcess = os.Exit

func main() {
	exitProcess(run(context.Background(), os.Args[1:], ".", os.Getenv, os.Stdout, os.Stderr))
}

// run runs the subcommand args name over the repository at dir, with env
// answering the variables each half reads, and returns the exit code.
func run(ctx context.Context, args []string, dir string, env func(string) string, stdout, stderr io.Writer) int {
	subcommand := ""
	if len(args) > 0 {
		subcommand = args[0]
	}
	if subcommand == "diff" {
		return runDiff(ctx, dir, args[1:], env, stdout, stderr)
	}
	if subcommand == "description" && len(args) == 1 {
		return runDescription(ctx, env, stdout, stderr)
	}
	if subcommand == "help" || subcommand == "-h" || subcommand == "--help" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprint(stderr, usage)
	return 2
}
