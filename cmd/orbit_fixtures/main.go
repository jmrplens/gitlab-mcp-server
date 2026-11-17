// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// toolName is how the messages and the usage name the command.
const toolName = "orbit_fixtures"

// usage is the command line of both subcommands.
const usage = `usage: orbit_fixtures setup [-mirror-cli] [-skip-mirror] [-root dir]
       orbit_fixtures wait-indexer

-root is the repository root that holds test/fixtures/orbit (default ".").

Settings come from the environment: GITLAB_COM_TOKEN (required),
ORBIT_FIXTURES_NAMESPACE (default jmrp-oss/orbit-fixtures),
ORBIT_FIXTURES_GITLAB_URL (default https://gitlab.com),
ORBIT_FIXTURES_VISIBILITY (setup, default public) and
ORBIT_FIXTURES_INDEXER_TIMEOUT (wait-indexer, seconds, default 600).
`

const (
	// defaultNamespace is the canonical fixture group: the subgroup rather
	// than jmrp-oss itself, which holds other projects the path-bounded
	// queries of the live suite must not count.
	defaultNamespace = "jmrp-oss/orbit-fixtures"
	// defaultGitLabURL is the one instance that serves Orbit.
	defaultGitLabURL = "https://gitlab.com"
	// fixturesDir holds the fixture projects, relative to the repository root
	// the make targets run in.
	fixturesDir = "test/fixtures/orbit"
)

// Seams over what main does that a test cannot follow it into: ending the
// process, and the transport every request to GitLab goes through, nil in a
// real run, which is net/http's default.
var (
	exitProcess       = os.Exit
	upstreamTransport http.RoundTripper
)

func main() {
	exitProcess(runMain(context.Background(), os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

// runMain runs the subcommand its first argument names and returns the
// process exit code: 2 for no subcommand, an unknown one or a usage error, and
// otherwise what the subcommand decides.
func runMain(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "setup":
		cfg, code, ok := setupFromArgs(args[1:], getenv, stderr)
		if !ok {
			return code
		}
		return runSetup(ctx, cfg, stdout, stderr)
	case "wait-indexer":
		cfg, err := waitFromEnv(getenv)
		if err != nil {
			fmt.Fprintf(stderr, "%s wait-indexer: %v\n", toolName, err)
			return 2
		}
		return runWait(ctx, cfg, stdout, stderr)
	case "-h", "-help", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "%s: unknown subcommand %q\n%s", toolName, args[0], usage)
	return 2
}

// httpClient is the client every request to GitLab is made with. Each request
// has a deadline of its own, so a connection GitLab accepts and never answers
// ends the run instead of blocking it. A redirect is the answer, as it was to
// the script's curl, and is never followed: net/http would send the token
// header to any host it named, and turn a write into a GET.
func httpClient() *http.Client {
	return &http.Client{
		Timeout:   2 * time.Minute,
		Transport: upstreamTransport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// sleepContext waits for d or until ctx ends, whichever comes first.
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// firstNonEmpty is the first of values that is not empty.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
