// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/fileheader"
)

// toolName is how every message this command prints names its source.
const toolName = "audit_file_headers"

// The findings a run reports, after the file's path and, for a notice, its
// line.
const (
	missingMessage = "does not open with the copyright and license header"
	noticeMessage  = "carries a copyright or license notice that is not the header at its top"
)

// remedy is what a failing run prints after the count, with the two lines of
// the header in place of its verbs.
const remedy = `
Every source file opens with these two lines, written as comments of its
language, after a shebang, an encoding declaration, a Dockerfile parser
directive or a CSS @charset rule, and inside an Astro component's
frontmatter:

    %s
    %s

make fix-file-headers writes them into every file reported as not opening
with them. A file reported for another notice is left as it is: a file this
repository did not write keeps its own notice and is not stamped with this
one, and one it did write carries the header once, at its top, which is a
change to make by hand.
`

// osExit is a seam over os.Exit, so a test can observe the exit code main
// passes on without ending the test process.
var osExit = os.Exit

func main() {
	osExit(runMain(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

// runMain parses the command line, judges every source file of the
// repository and, under -fix, writes the header into each file missing it
// that carries no other notice. It prints a line per finding and per file
// written, then a summary, and returns 2 for a usage error, 1 when a file
// still fails or could not be listed, read or written, and 0 otherwise.
func runMain(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(toolName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	dir := flags.String("dir", ".", "the repository whose source files are judged")
	fixMode := flags.Bool("fix", false, "write the header into every file that does not open with it and carries no other notice")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "%s: takes no arguments, and was given %q; name the repository with -dir\n", toolName, flags.Args())
		return 2
	}
	names, err := listFiles(ctx, *dir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: could not list the files of %s: %v; the list is git's, so run this in a checkout\n", toolName, *dir, err)
		return 1
	}
	outcomes := map[outcome]int{}
	for _, name := range names {
		if k, ok := kindOf(name); ok {
			outcomes[checkFile(filepath.Join(*dir, filepath.FromSlash(name)), name, k, *fixMode, stdout, stderr)]++
		}
	}
	judged := outcomes[passed] + outcomes[failed] + outcomes[written]
	if outcomes[failed] > 0 {
		_, _ = fmt.Fprintf(stdout, "file headers: %d of %d source files fail\n", outcomes[failed], judged)
		_, _ = fmt.Fprintf(stdout, remedy, fileheader.CopyrightStatement, fileheader.LicenseStatement)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "file headers: %d source files, every one opening with the copyright and license header", judged)
	if outcomes[written] > 0 {
		_, _ = fmt.Fprintf(stdout, ", %d of them written by this run", outcomes[written])
	}
	_, _ = fmt.Fprintln(stdout)
	return 0
}

// outcome is what became of one source file in a run.
type outcome int

// The outcomes a source file can have. A file skipped is not counted as
// judged.
const (
	// skipped is a path whose working tree holds no regular file: one deleted
	// since it was added, a symbolic link or a nested checkout's directory.
	skipped outcome = iota
	// passed opens with the header and carries no other notice.
	passed
	// failed still fails, or could not be stated, read or written.
	failed
	// written lacked the header alone, and -fix wrote it.
	written
)

// checkFile judges the source file of kind k at path, which the report calls
// name, and under fixMode writes the header into it when the header is all it
// lacks. It prints each finding and each write on stdout, an error on stderr,
// and returns what became of the file.
func checkFile(path, name string, k kind, fixMode bool, stdout, stderr io.Writer) outcome {
	info, err := lstat(path)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && !info.Mode().IsRegular()) {
		return skipped
	}
	var data []byte
	if err == nil {
		data, err = readFile(path)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s: %v\n", toolName, name, err)
		return failed
	}
	v := judge(k, string(data))
	if !v.missing && len(v.notices) == 0 {
		return passed
	}
	if fixMode && len(v.notices) == 0 {
		if err = writeFile(path, []byte(fix(k, string(data))), info.Mode().Perm()); err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %s: %v\n", toolName, name, err)
			return failed
		}
		_, _ = fmt.Fprintf(stdout, "%s: header written\n", name)
		return written
	}
	if v.missing {
		_, _ = fmt.Fprintf(stdout, "%s: %s\n", name, missingMessage)
	}
	for _, line := range v.notices {
		_, _ = fmt.Fprintf(stdout, "%s:%d: %s\n", name, line, noticeMessage)
	}
	return failed
}
