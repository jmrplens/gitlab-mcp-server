// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package conditions

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// installAttempts is how many times go install is tried.
const installAttempts = 3

// pinnedOnPath returns where PATH finds gobco when that gobco is the pinned
// release, by the version it prints, which is the release's without its v,
// and an empty path when it is not. found describes what PATH finds instead.
func pinnedOnPath(ctx context.Context, tools Tools, environ []string) (pinned, found string) {
	path, err := tools.LookPath("gobco")
	if err != nil {
		return "", "no gobco"
	}
	version := "no version"
	if ctx.Err() == nil {
		if out, runErr := recipe.Output(ctx, tools.Run, recipe.Call{Program: path, Args: []string{"-version"}, Env: environ}); runErr == nil {
			version = strings.TrimSpace(out)
		}
	}
	if version == strings.TrimPrefix(Release, "v") {
		return path, ""
	}
	return "", path + ", which reports " + version
}

// Install puts the pinned gobco on PATH and returns the status of the run, as
// the package comment describes it.
func Install(ctx context.Context, tools Tools, environ []string, stdout, stderr io.Writer) int {
	status := install(ctx, tools, environ, stdout, stderr)
	if interrupted, hit := recipe.Status(ctx); hit {
		return interrupted
	}
	return status
}

// install is [Install] before an interrupt is read.
func install(ctx context.Context, tools Tools, environ []string, stdout, stderr io.Writer) int {
	wait, set := recipe.Getenv(environ, "GOBCO_INSTALL_PAUSE")
	if !set {
		wait = "10"
	}
	seconds, err := strconv.ParseUint(wait, 10, 31)
	if err != nil {
		fmt.Fprintf(stderr, "gobco: GOBCO_INSTALL_PAUSE=%s is not a whole number of seconds; refusing to install\n", wait)
		return Refused
	}
	if path, _ := pinnedOnPath(ctx, tools, environ); path != "" {
		fmt.Fprintf(stdout, "gobco: PATH already finds %s at %s; nothing to install\n", Gobco, path)
		return Measured
	}
	if !goInstall(ctx, tools, environ, stdout, stderr, time.Duration(seconds)*time.Second) {
		return NotMeasured
	}
	path, found := pinnedOnPath(ctx, tools, environ)
	if path == "" {
		fmt.Fprintf(stderr, "gobco: go install %s succeeded, but PATH finds %s: put GOBIN, or GOPATH/bin when GOBIN is unset, on PATH ahead of any other gobco, or every measurement runs gobco through go run and needs the network again\n", Gobco, found)
		return NotMeasured
	}
	fmt.Fprintf(stdout, "gobco: installed %s at %s, which every measurement runs without the network\n", Gobco, path)
	return Measured
}

// goInstall runs go install of the pinned release until an attempt succeeds
// or the last has failed, and reports which.
func goInstall(ctx context.Context, tools Tools, environ []string, stdout, stderr io.Writer, wait time.Duration) bool {
	for attempt := 1; ctx.Err() == nil; attempt++ {
		status, err := tools.Run(ctx, recipe.Call{Program: tools.Go, Args: []string{"install", Gobco}, Env: environ, Stdout: stdout, Stderr: stderr})
		if err == nil && status == 0 {
			return true
		}
		if err != nil {
			fmt.Fprintf(stderr, "gobco: %v\n", err)
		}
		if attempt == installAttempts {
			fmt.Fprintf(stderr, "gobco: go install %s failed %d times; giving up\n", Gobco, installAttempts)
			return false
		}
		fmt.Fprintf(stderr, "gobco: go install %s failed (attempt %d of %d); trying again in %ds\n", Gobco, attempt, installAttempts, int(wait/time.Second))
		tools.Pause(ctx, wait)
	}
	return false
}
