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

// The two values -freshness takes, the ones ci.yml's FRESHNESS decides
// between: checked judges a record that predates the tree like any other,
// which is what a stack's top and main are held to, and deferred makes the
// excess such a record explains a note, below a stack's top.
const (
	freshnessChecked  = "checked"
	freshnessDeferred = "deferred"
)

// options is a command line the command can use.
type options struct {
	packages     []string
	sources      []source
	out, summary string
	// writeRecord is where the record of the runs' half is written.
	writeRecord string
	// checkRecord is the record the gate reads the runs' half from.
	checkRecord string
	// deferred is -freshness deferred.
	deferred bool
	// stamp is where the stamp of the packages' sources is written.
	stamp string
}

// modeFlags is what the command line said about the mode the command runs
// in, as parsed and before it is judged.
type modeFlags struct {
	out, summary, writeRecord, checkRecord, freshness, stamp string
}

// runMain parses the command line, merges the profiles and reports, writing
// the record when asked, judges the offline half against a record, or
// stamps the packages' sources. It returns 2 for a command line it cannot
// use, 1 when the profiles or the record could not be read or merged, a run
// profile a record was asked of is not stamped as a complete run of the
// tree, a package could not be measured or stamped, the report could not be
// printed, a file could not be written or the gate failed, and 0 otherwise.
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
	var mode modeFlags
	flags.StringVar(&mode.out, "o", "", "where to write the merged profile")
	flags.StringVar(&mode.summary, "summary", "", "a Markdown file to append the report to, such as $GITHUB_STEP_SUMMARY")
	flags.StringVar(&mode.writeRecord, "write-record", "", "where to write the record of what the runs reached, beside the report")
	flags.StringVar(&mode.checkRecord, "check-record", "", "the record to read the runs' half from, holding each package to it")
	flags.StringVar(&mode.freshness, "freshness", "", "how -check-record judges a record that predates the tree: checked (the default) or deferred")
	flags.StringVar(&mode.stamp, "stamp", "", "where to write the stamp of the packages' sources, which a complete run leaves beside its profile")
	if flags.Parse(args) != nil {
		return options{}, false
	}
	pkgs, err := parsePackages(*packages)
	if problem := commandLineProblem(flags.Args(), offline, online, mode); problem != nil {
		err = problem
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return options{}, false
	}
	opts := options{
		packages: pkgs, out: mode.out, summary: mode.summary,
		writeRecord: mode.writeRecord, checkRecord: mode.checkRecord, deferred: mode.freshness == freshnessDeferred,
		stamp: mode.stamp,
	}
	for _, p := range offline {
		opts.sources = append(opts.sources, source{path: p, half: offlineHalf})
	}
	for _, p := range online {
		opts.sources = append(opts.sources, source{path: p, half: onlineHalf})
	}
	return opts, true
}

// commandLineProblem says why a parsed command line cannot be used: an
// argument beside the flags, no offline profile, or flags the mode they
// name cannot take. It returns nil for one that can be.
func commandLineProblem(args []string, offline, online profileList, mode modeFlags) error {
	if len(args) > 0 {
		return fmt.Errorf("takes no arguments, and was given %q; name each profile with -offline or -online", args)
	}
	if mode.stamp != "" {
		return stampProblem(offline, online, mode)
	}
	if len(offline) == 0 {
		return errors.New("needs at least one -offline profile, which the packages' offline tests write")
	}
	if mode.checkRecord != "" {
		return checkProblem(online, mode)
	}
	if len(online) == 0 {
		return errors.New("needs at least one -online profile, which a run against GitLab writes, unless -check-record reads the runs' half from a record")
	}
	if mode.freshness != "" {
		return errors.New("-freshness says how -check-record judges a record that predates the tree, and -check-record is not given")
	}
	return nil
}

// checkProblem says why a command line that names -check-record cannot be
// used: the record is the runs' half, so a run profile, a record to write and
// a merged profile have no place beside it, and -freshness takes one of its
// two values.
func checkProblem(online profileList, mode modeFlags) error {
	if len(online) > 0 {
		return errors.New("-check-record reads the runs' half from the record, so it takes no -online profile")
	}
	if mode.writeRecord != "" {
		return errors.New("-check-record judges a record and -write-record writes one; give one of them")
	}
	if mode.out != "" {
		return errors.New("-o writes the merge of profiles, which -check-record does not make")
	}
	if mode.freshness != "" && mode.freshness != freshnessChecked && mode.freshness != freshnessDeferred {
		return fmt.Errorf("-freshness %q is neither %s nor %s", mode.freshness, freshnessChecked, freshnessDeferred)
	}
	return nil
}

// stampProblem says why a command line that names -stamp cannot be used: the
// stamp is of the packages' sources alone, so a profile, another output and a
// record have no place beside it.
func stampProblem(offline, online profileList, mode modeFlags) error {
	if len(offline) > 0 || len(online) > 0 {
		return errors.New("-stamp writes the stamp of the packages' sources and reads no profile")
	}
	for _, other := range []struct{ name, value string }{
		{"-o", mode.out},
		{"-write-record", mode.writeRecord},
		{"-check-record", mode.checkRecord},
		{"-freshness", mode.freshness},
		{"-summary", mode.summary},
	} {
		if other.value != "" {
			return fmt.Errorf("-stamp writes the stamp alone, so it takes no %s", other.name)
		}
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

// run writes the stamp of the packages' sources when -stamp names a file,
// and judges the offline half against a record when -check-record names one.
// Otherwise it merges the profiles, writes the merged profile and the record
// when asked, prints the report and appends it to the summary when asked. A
// record is taken only of run profiles stamped by a complete run of the
// packages' sources as this tree holds them, which is settled before a
// profile is read, and a record that cannot be taken of the merge is refused
// before the merged profile is written.
func run(opts options, stdout io.Writer) error {
	if opts.stamp != "" {
		return writeStamp(opts.stamp, opts.packages)
	}
	if opts.checkRecord != "" {
		return check(opts, stdout)
	}
	if opts.writeRecord != "" {
		if err := checkStamps(opts.sources, opts.packages); err != nil {
			return err
		}
	}
	cov, err := read(opts.sources)
	if err != nil {
		return err
	}
	rows, err := cov.rows(opts.packages)
	if err != nil {
		return err
	}
	var rec *record
	if opts.writeRecord != "" {
		if rec, err = cov.newRecord(rows, opts.sources); err != nil {
			return err
		}
	}
	if opts.out != "" {
		if err = os.WriteFile(opts.out, cov.profile(), 0o600); err != nil {
			return fmt.Errorf("writing the merged profile: %w", err)
		}
	}
	if rec != nil {
		if writeErr := writeRecord(opts.writeRecord, rec); writeErr != nil {
			return writeErr
		}
	}
	return publish(render(opts.sources, rows), opts.summary, stdout)
}
