// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
)

// toolName is how every message this command prints names its source.
const toolName = "merge_coverage"

// osExit is a seam over os.Exit, so a test can observe the exit code main
// passes on without ending the test process.
var osExit = os.Exit

func main() {
	osExit(runMain(os.Args[1:], os.Stdout, os.Stderr))
}

// profileList is a flag that may be given more than once, each time naming
// one profile.
type profileList []string

// String lists the profiles named so far, as flag.Value requires.
func (l *profileList) String() string { return strings.Join(*l, ",") }

// Set adds one profile.
func (l *profileList) Set(profile string) error {
	*l = append(*l, profile)
	return nil
}

// options is a command line the command can use.
type options struct {
	packages     []string
	sources      []source
	out, summary string
}

// runMain parses the command line, merges the profiles and reports. It
// returns 2 for a command line it cannot use, 1 when the profiles could not
// be read or merged, a package could not be measured or a file could not be
// written, and 0 otherwise.
func runMain(args []string, stdout, stderr io.Writer) int {
	opts, ok := parseArgs(args, stderr)
	if !ok {
		return 2
	}
	if err := run(opts, stdout); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return 1
	}
	return 0
}

// parseArgs reads the command line, printing why it cannot be used when it
// cannot.
func parseArgs(args []string, stderr io.Writer) (options, bool) {
	flags := flag.NewFlagSet(toolName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	var offline, online profileList
	flags.Var(&offline, "offline", "a profile the packages' offline tests wrote; give it once per profile")
	flags.Var(&online, "online", "a profile a run against GitLab wrote; give it once per profile")
	packages := flags.String("packages", "", "the packages to report, comma-separated as go test -coverpkg takes them")
	out := flags.String("o", "", "where to write the merged profile")
	summary := flags.String("summary", "", "a Markdown file to append the report to, such as $GITHUB_STEP_SUMMARY")
	if flags.Parse(args) != nil {
		return options{}, false
	}
	pkgs, err := parsePackages(*packages)
	if problem := commandLineProblem(flags.Args(), offline, online); problem != nil {
		err = problem
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return options{}, false
	}
	opts := options{packages: pkgs, out: *out, summary: *summary}
	for _, p := range offline {
		opts.sources = append(opts.sources, source{path: p, half: offlineHalf})
	}
	for _, p := range online {
		opts.sources = append(opts.sources, source{path: p, half: onlineHalf})
	}
	return opts, true
}

// commandLineProblem says why a parsed command line cannot be used: an
// argument beside the flags, or a half no profile was named for. It returns
// nil for one that can be.
func commandLineProblem(args []string, offline, online profileList) error {
	if len(args) > 0 {
		return fmt.Errorf("takes no arguments, and was given %q; name each profile with -offline or -online", args)
	}
	if len(offline) == 0 {
		return errors.New("needs at least one -offline profile, which the packages' offline tests write")
	}
	if len(online) == 0 {
		return errors.New("needs at least one -online profile, which a run against GitLab writes")
	}
	return nil
}

// parsePackages reads -packages: comma-separated, each a directory below the
// module root spelled ./dir or an import path, as go test's -coverpkg takes
// them, and each returned as the import path a profile names its files by.
func parsePackages(list string) ([]string, error) {
	if list == "" {
		return nil, errors.New("-packages names no package; give them comma-separated, as go test -coverpkg takes them")
	}
	var pkgs []string
	for spelling := range strings.SplitSeq(list, ",") {
		if spelling == "" {
			return nil, fmt.Errorf("-packages %q names an empty package", list)
		}
		pkgs = append(pkgs, importPath(spelling))
	}
	return pkgs, nil
}

// importPath reads one package spelling: a directory below the module root
// joined onto the module path, an import path cleaned of a trailing slash.
func importPath(spelling string) string {
	if spelling == "." || strings.HasPrefix(spelling, "./") {
		return path.Join(goprogram.ModulePath, spelling)
	}
	return path.Clean(spelling)
}

// run merges the profiles, writes the merged profile when asked, prints the
// report and appends it to the summary when asked.
func run(opts options, stdout io.Writer) error {
	cov, err := read(opts.sources)
	if err != nil {
		return err
	}
	rows, err := cov.rows(opts.packages)
	if err != nil {
		return err
	}
	if opts.out != "" {
		if err = os.WriteFile(opts.out, cov.profile(), 0o600); err != nil {
			return fmt.Errorf("writing the merged profile: %w", err)
		}
	}
	report := render(opts.sources, rows)
	_, _ = io.WriteString(stdout, report)
	if opts.summary != "" {
		if err = appendSummary(opts.summary, report); err != nil {
			return fmt.Errorf("appending the report to %s: %w", opts.summary, err)
		}
	}
	return nil
}
