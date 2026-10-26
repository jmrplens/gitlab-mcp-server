package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// toolName is how every message this command prints names its source.
const toolName = "audit_dco"

// osExit is a seam over os.Exit, so a test can observe the exit code main
// passes on without ending the test process.
var osExit = os.Exit

func main() {
	osExit(runMain(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

// runMain parses the command line, reads the commits of the range from git
// with the merges among them that bring no change of their own, judges each
// and prints the report. It returns 2 for a usage error, 1 when a
// commit is not signed off by its author or git could not answer, and 0
// otherwise.
func runMain(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(toolName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	base := flags.String("base", "origin/main", "the commit the branch grew from; the commits reachable from it are not judged")
	head := flags.String("head", "HEAD", "the commit at the tip of the branch")
	openedBy := flags.String("opened-by", "", "the GitHub login that opened the pull request, which a declared bot's exemption needs")
	dir := flags.String("dir", ".", "the repository to read")
	annotate := flags.Bool("annotate", false, "print a GitHub Actions error command for every commit that fails")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "%s: takes no arguments, and was given %q; name the range with -base and -head\n", toolName, flags.Args())
		return 2
	}
	revisions := []struct{ flag, rev string }{{"base", *base}, {"head", *head}}
	for _, each := range revisions {
		if each.rev == "" || strings.HasPrefix(each.rev, "-") {
			_, _ = fmt.Fprintf(stderr, "%s: -%s %q is not a revision; name a commit, a branch or a tag\n", toolName, each.flag, each.rev)
			return 2
		}
	}
	hashes := make([]string, 0, len(revisions))
	for _, each := range revisions {
		hash, err := resolveCommit(ctx, *dir, each.rev)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: -%s %s names no commit in %s (%v); a shallow clone may lack it, so fetch it or check out with fetch-depth: 0\n", toolName, each.flag, each.rev, *dir, err)
			return 1
		}
		hashes = append(hashes, hash)
	}
	span := fmt.Sprintf("%s..%s (%s..%s)", *base, *head, commit{hash: hashes[0]}.short(), commit{hash: hashes[1]}.short())
	read, err := listCommits(ctx, *dir, hashes[0], hashes[1])
	if err == nil {
		err = markCleanMerges(ctx, *dir, read)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: could not read the commits of %s: %v\n", toolName, span, err)
		return 1
	}
	verdicts := make([]verdict, 0, len(read))
	for _, each := range read {
		verdicts = append(verdicts, judge(each, *openedBy))
	}
	failed := report{span: span, base: *base, openedBy: *openedBy, verdicts: verdicts, annotate: *annotate}.write(stdout)
	if failed {
		return 1
	}
	return 0
}
