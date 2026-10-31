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

// toolName is how the report names itself.
const toolName = "audit_vex"

// defaultDB is the database govulncheck reads unless told otherwise.
const defaultDB = "https://vuln.go.dev"

// defaultDocument is the repository's OpenVEX document, relative to the
// module root. The release attaches it to every GitHub release under its
// base name.
const defaultDocument = "vex/gitlab-mcp-server.openvex.json"

// auditConfig is one configured run: the tree, the document, the database
// and the packages the dependency set is read from.
type auditConfig struct {
	dir      string
	document string
	db       string
	tags     string
	patterns []string
}

// exitProcess is [os.Exit] behind a seam, so the code [runMain] decided is a
// value a test can read rather than the end of the test binary.
var exitProcess = os.Exit

func main() {
	exitProcess(runMain(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

// runMain parses the command line and runs the audit it describes, returning
// the process exit code: 2 for arguments it cannot parse, and otherwise what
// [run] decides. The arguments after the flags are the package patterns the
// scan loads, ./... when there are none.
func runMain(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(toolName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	dir := flags.String("dir", ".", "module root the dependency set is read from")
	document := flags.String("file", defaultDocument, "the OpenVEX document, relative to -dir unless absolute")
	db := flags.String("db", defaultDB, "vulnerability database URL, file:// for a copy on disk")
	tags := flags.String("tags", "", "comma-separated build tags the scan loads the packages with, the ones make govulncheck passes")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	patterns := flags.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	return run(ctx, auditConfig{dir: *dir, document: *document, db: *db, tags: *tags, patterns: patterns}, stdout, stderr)
}

// run audits and reports, returning the process exit code.
//
// Exit 1 covers two different things, told apart by which stream spoke: a run
// that could not be made says so on stderr, and a run that found something
// says it on stdout. Both fail the gate, because a document that could not be
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

// audit reads the module path and the document, validates the document, and
// holds a document that validated to a scan of the dependency set.
func audit(ctx context.Context, cfg auditConfig) (*report, error) {
	module, err := readModulePath(filepath.Join(cfg.dir, "go.mod"))
	if err != nil {
		return nil, err
	}
	path := cfg.document
	if !filepath.IsAbs(path) {
		path = filepath.Join(cfg.dir, path)
	}
	doc, err := readDocument(path)
	if err != nil {
		return nil, err
	}
	rep := &report{document: cfg.document, statements: len(doc.Statements), patterns: cfg.patterns, tags: cfg.tags, invalid: validate(doc, module)}
	if len(rep.invalid) != 0 {
		return rep, nil
	}
	result, err := scanSource(ctx, cfg)
	if err != nil {
		return nil, err
	}
	judge(rep, doc, module, result)
	return rep, nil
}
