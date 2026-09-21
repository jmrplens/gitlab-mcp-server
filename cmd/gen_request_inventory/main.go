package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

const (
	// toolName is the command's own name, used as the flag set's name so a
	// usage message names the command rather than the test binary that drove
	// it.
	toolName = "gen_request_inventory"

	// defaultShardDir is where `make gen-request-inventory` points the
	// recorder, under the gitignored build directory: a shard is a byproduct
	// of one suite run and only the merged result is worth keeping.
	defaultShardDir = "dist/request-inventory"

	// defaultOutputPath is the committed artifact.
	defaultOutputPath = requestinventory.Path
)

// osExit is os.Exit behind a variable, so the one line main carries is
// reachable from a test rather than only from a process.
var osExit = os.Exit

// main merges the shards and either rewrites the committed inventory or
// reports that it has drifted.
func main() {
	osExit(runMain(os.Args[1:], os.Stderr))
}

// runMain parses args, the command line with the program name already removed,
// and returns the process exit code.
//
// The flag set is ContinueOnError rather than the package-level ExitOnError
// one, so a bad flag is an exit code this function returns instead of an
// os.Exit the seam above never sees; -h is the one parse failure that exits
// clean, as ExitOnError would. The two failures below both exit 1 and are
// fixed differently, so each names its own stage: one means this is not a
// checkout, the other that the merge or the comparison refused.
func runMain(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet(toolName, flag.ContinueOnError)
	fs.SetOutput(stderr)
	shardDir := fs.String("shards", defaultShardDir, "directory holding the recorded request shards, absolute or relative to the repository root")
	outputPath := fs.String("out", defaultOutputPath, "committed request inventory path")
	check := fs.Bool("check", false, "verify the committed inventory is current without writing it")
	verbose := fs.Bool("v", false, "name every package the catalog owns actions in that recorded no request")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		fmt.Fprintf(stderr, "find repository root: %v\n", err)
		return 1
	}
	if runErr := run(stderr, root, options{shardDir: *shardDir, outputPath: *outputPath, check: *check, verbose: *verbose}); runErr != nil {
		fmt.Fprintf(stderr, "%v\n", runErr)
		return 1
	}
	return 0
}

// options carries what the flags decided.
type options struct {
	shardDir   string
	outputPath string
	check      bool
	verbose    bool
}

// run is the testable entry point. Every error names the stage that failed,
// which is the text main reports.
func run(progress io.Writer, root string, opts options) error {
	recorded, err := readShards(underRoot(root, opts.shardDir))
	if err != nil {
		return err
	}
	rows := merge(recorded.records)
	summarize(progress, root, rows, recorded.written, opts.verbose)

	target := underRoot(root, opts.outputPath)
	content := render(rows)
	if opts.check {
		return checkInventory(target, content, rows)
	}
	if writeErr := os.WriteFile(target, content, 0o600); writeErr != nil {
		return fmt.Errorf("write %s: %w", target, writeErr)
	}
	return nil
}

// underRoot resolves a path given on the command line, which is relative to
// the repository root unless it is already absolute.
//
// The recorder demands an absolute directory (a test binary runs in its own
// package directory, so a relative one scatters a shard under each of them),
// and filepath.Join treats an absolute second element as relative. Without
// this, recording into the temporary directory the recorder insists on and
// then merging it was impossible: the same path the suite wrote to could not
// be read back.
func underRoot(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

// checkInventory compares the committed artifact with what the shards say it
// should be.
//
// The failure names the requests that differ rather than only saying the file
// is stale. The whole reason to commit the artifact is that a handler which
// starts calling a different endpoint stops being invisible, and a gate that
// answers "300 kilobytes of JSON changed" hands that visibility back.
func checkInventory(path string, want []byte, rows []row) error {
	got, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("%s is out of date: run `make gen-request-inventory`%s", path, differences(got, rows))
	}
	return nil
}
