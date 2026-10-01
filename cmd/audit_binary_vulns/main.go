package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

// toolName is how the report names itself.
const toolName = "audit_binary_vulns"

// defaultDB is the database govulncheck reads unless told otherwise.
const defaultDB = "https://vuln.go.dev"

// auditConfig is one configured run: where the release is built from, what
// it builds, the database it is held to and the declarations it may rely on.
type auditConfig struct {
	dir      string
	config   string
	db       string
	declared map[string]declaration
}

// exitProcess is [os.Exit] behind a seam, so the code [runMain] decided is a
// value a test can read rather than the end of the test binary.
var exitProcess = os.Exit

func main() {
	exitProcess(runMain(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

// runMain parses the command line and runs the audit it describes, returning
// the process exit code: 2 for arguments it cannot parse, and otherwise what
// [run] decides.
func runMain(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(toolName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	dir := flags.String("dir", ".", "module root the release binaries are built from")
	config := flags.String("config", ".goreleaser.yml", "GoReleaser configuration that names the release targets")
	db := flags.String("db", defaultDB, "vulnerability database URL, file:// for a copy on disk")
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
	return run(ctx, auditConfig{dir: *dir, config: *config, db: *db, declared: acceptedAdvisories}, stdout, stderr)
}

// run audits and reports, returning the process exit code.
//
// Exit 1 covers two different things, told apart by which stream spoke: a run
// that could not be made says so on stderr, and a run that found something
// says it on stdout. Both fail the gate, because a release that could not be
// checked is not one that passed.
func run(ctx context.Context, cfg auditConfig, stdout, stderr io.Writer) int {
	rep, err := audit(ctx, cfg)
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

// audit builds every release target into a directory of its own, scans each
// binary and judges the findings.
func audit(ctx context.Context, cfg auditConfig) (report, error) {
	builds, err := readBuilds(cfg.config)
	if err != nil {
		return report{}, err
	}
	outDir, err := os.MkdirTemp("", toolName+"-")
	if err != nil {
		return report{}, err
	}
	defer func() { _ = os.RemoveAll(outDir) }()

	binaries, err := buildAll(ctx, cfg.dir, builds, outDir)
	if err != nil {
		return report{}, err
	}
	scans := make([]scanned, 0, len(binaries))
	for _, bin := range binaries {
		result, scanErr := scanBinary(ctx, cfg.db, bin.path)
		if scanErr != nil {
			return report{}, scanErr
		}
		scans = append(scans, scanned{binary: bin, result: result})
	}
	return judge(scans, cfg.declared), nil
}
