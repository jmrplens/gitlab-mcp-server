// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// toolName is how the report and the errors name the command.
const toolName = "reproduce_release"

// options is one configured run.
type options struct {
	// dir is the module root: with -published, the git repository the
	// release's commit is cloned from; with -twice, the tree built as it
	// stands.
	dir string
	// config is the GoReleaser configuration, relative to the module root.
	config string
	// published is the directory holding a release's checksum file and its
	// binaries.
	published string
	// tag is the release tag the binaries are stamped with.
	tag string
	// ref is the commit a release is rebuilt from.
	ref string
	// shortCommit is what {{.ShortCommit}} renders as, when given.
	shortCommit string
	// out is where the rebuilt binaries are kept, when given.
	out string
	// twice selects the offline check.
	twice bool
}

// exitProcess is [os.Exit] behind a seam, so the code [runMain] decided is a
// value a test can read rather than the end of the test binary.
var exitProcess = os.Exit

func main() {
	exitProcess(runMain(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

// runMain parses the command line and runs what it describes, returning the
// process exit code: 2 for arguments it cannot use, and otherwise what [run]
// decides.
func runMain(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(toolName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	var o options
	flags.StringVar(&o.dir, "dir", ".", "module root: the repository a release is cloned from with -published, the tree built as it stands with -twice")
	flags.StringVar(&o.config, "config", ".goreleaser.yml", "GoReleaser configuration, relative to the module root (with -published, the one at the commit rebuilt)")
	flags.StringVar(&o.published, "published", "", "directory holding a release's checksum file and binaries, as scripts/fetch-release-assets.sh leaves them")
	flags.StringVar(&o.tag, "tag", "", "release tag the binaries are stamped with; with -twice, v followed by VERSION when empty")
	flags.StringVar(&o.ref, "ref", "HEAD", "with -published, the commit to rebuild")
	flags.StringVar(&o.shortCommit, "short-commit", "", "what {{.ShortCommit}} renders as; git's abbreviation of the commit when empty")
	flags.StringVar(&o.out, "out", "", "absolute directory the rebuilt binaries are kept in; a temporary one, removed afterwards, when empty")
	flags.BoolVar(&o.twice, "twice", false, "build every release target of -dir twice, the second time with an empty build cache, and compare the two")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "%s: unexpected arguments %q\n", toolName, flags.Args())
		return 2
	}
	set := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if problem := o.problem(set); problem != "" {
		fmt.Fprintf(stderr, "%s: %s\n", toolName, problem)
		return 2
	}
	return run(ctx, o, stdout, stderr)
}

// problem says what makes the options unusable together, or "" when nothing
// does. set names the flags the command line gave.
func (o options) problem(set map[string]bool) string {
	if o.twice == (o.published != "") {
		return "give exactly one of -published and -twice"
	}
	if !o.twice && o.tag == "" {
		return "-published needs -tag, the tag the release's binaries were stamped as"
	}
	if o.twice && set["ref"] {
		return "-ref names the commit a release is rebuilt from; -twice builds the tree -dir names as it stands"
	}
	if o.out != "" && !filepath.IsAbs(o.out) {
		return "-out must be an absolute path, since the go command runs in another directory"
	}
	return ""
}

// run makes the comparison and reports it, returning the process exit code.
//
// Exit 1 covers two different things, told apart by which stream spoke: a
// run that could not be made says so on stderr, and a run that found a
// binary differing says it on stdout. Both fail, because a reproduction that
// could not be made is not one that passed.
func run(ctx context.Context, o options, stdout, stderr io.Writer) int {
	compare := reproduce
	if o.twice {
		compare = repeat
	}
	rep, err := compare(ctx, o)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return 1
	}
	rep.write(stdout)
	if !rep.ok() {
		return 1
	}
	return 0
}
