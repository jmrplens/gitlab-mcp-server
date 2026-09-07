package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/requestinventory"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

const (
	// defaultShardDir is where `make gen-request-inventory` points the
	// recorder, under the gitignored build directory: a shard is a byproduct
	// of one suite run and only the merged result is worth keeping.
	defaultShardDir = "dist/request-inventory"

	// defaultOutputPath is the committed artifact.
	defaultOutputPath = requestinventory.Path
)

// main parses the flags, merges the shards, and either rewrites the committed
// inventory or reports that it has drifted.
func main() {
	shardDir := flag.String("shards", defaultShardDir, "directory holding the recorded request shards, absolute or relative to the repository root")
	outputPath := flag.String("out", defaultOutputPath, "committed request inventory path")
	check := flag.Bool("check", false, "verify the committed inventory is current without writing it")
	verbose := flag.Bool("v", false, "name every package the catalog owns actions in that recorded no request")
	flag.Parse()

	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		cmdutil.Fatalf("find repository root: %v", err)
	}
	if runErr := run(os.Stderr, root, options{shardDir: *shardDir, outputPath: *outputPath, check: *check, verbose: *verbose}); runErr != nil {
		cmdutil.Fatalf("%v", runErr)
	}
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
	records, err := readShards(underRoot(root, opts.shardDir))
	if err != nil {
		return err
	}
	rows := merge(records)
	summarize(progress, root, rows, opts.verbose)

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
