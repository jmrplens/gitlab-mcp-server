// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
)

// lister runs the go command in dir and returns its standard output. It is a
// parameter so a test can answer for the toolchain.
type lister func(ctx context.Context, dir string, args ...string) ([]byte, error)

// goList is the lister production uses: the toolchain's own go binary, never
// one looked up on PATH.
func goList(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, golist.Executable(), args...) //#nosec G204 -- golist.Executable is the toolchain's own go binary, joined out of GOROOT
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// listFormat is the row asked of `go list` per package: its import path, its
// module's path, its directory, and the internal and external test files the
// go command builds for it on this platform, which are the only files a fuzz
// target it would run can be in.
const listFormat = `{{.ImportPath}}	{{with .Module}}{{.Path}}{{end}}	{{.Dir}}	{{join .TestGoFiles " "}}	{{join .XTestGoFiles " "}}`

// target is one fuzz target: its package, written ./ and its directory below
// the module root as go test takes it, and its function's name.
type target struct {
	pkg  string
	name string
}

// cell is one run the matrix asks for: one target of one package, and the
// name of the artifact a failing input of it is uploaded under.
type cell struct {
	Package  string `json:"package"`
	Target   string `json:"target"`
	Artifact string `json:"artifact"`
}

// matrix is the strategy matrix the workflow fans out over, in the shape
// `strategy.matrix` reads: every cell is an include entry.
type matrix struct {
	Include []cell `json:"include"`
}

// isFuzzName reports whether the go command reads a top-level function of a
// test file by this name as a fuzz target: Fuzz alone, or Fuzz followed by a
// character that is not a lower-case letter (the rule of isTest in
// cmd/go/internal/load), so FuzzMeterFor is one and Fuzzy is not.
func isFuzzName(name string) bool {
	rest, prefixed := strings.CutPrefix(name, "Fuzz")
	if !prefixed {
		return false
	}
	if rest == "" {
		return true
	}
	next, _ := utf8.DecodeRuneInString(rest)
	return !unicode.IsLower(next)
}

// findTargets returns every fuzz target of the module in dir, package by
// package in the order the listing gives them and by name within a package.
//
// The listing is not asked for errors: a module that does not load cannot be
// fuzzed, and the go command fails the listing for it with its own words.
func findTargets(ctx context.Context, dir string, list lister) ([]target, error) {
	out, err := list(ctx, dir, "list", "-f", listFormat, "./...")
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}
	var found []target
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 5 {
			return nil, fmt.Errorf("unexpected go list row %q", line)
		}
		names, unparsable := fuzzNames(fields[2], strings.Fields(fields[3]+" "+fields[4]))
		if unparsable != nil {
			return nil, unparsable
		}
		if len(names) == 0 {
			continue
		}
		rel, below := strings.CutPrefix(fields[0], fields[1]+"/")
		if !below {
			return nil, fmt.Errorf("go list named %q, which is not below its module path %q", fields[0], fields[1])
		}
		for _, name := range names {
			found = append(found, target{pkg: "./" + rel, name: name})
		}
	}
	return found, nil
}

// fuzzNames parses each named test file of the package in dir and returns the
// names of its fuzz targets, sorted.
func fuzzNames(dir string, files []string) ([]string, error) {
	fset := token.NewFileSet()
	var names []string
	for _, file := range files {
		parsed, err := parser.ParseFile(fset, filepath.Join(dir, file), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("a test file the go command builds cannot be parsed: %w", err)
		}
		for _, decl := range parsed.Decls {
			if fn, isFunc := decl.(*ast.FuncDecl); isFunc && fn.Recv == nil && isFuzzName(fn.Name.Name) {
				names = append(names, fn.Name.Name)
			}
		}
	}
	slices.Sort(names)
	return names, nil
}

// buildMatrix makes a cell of each target, in the order given, and names its
// artifact after its package and its name. Two targets whose artifact names
// would be the same, which only directory names joined differently by a dash
// could make, are refused rather than left to overwrite each other's upload.
func buildMatrix(targets []target) (matrix, error) {
	planned := matrix{Include: make([]cell, 0, len(targets))}
	byArtifact := map[string]cell{}
	for _, each := range targets {
		artifact := "fuzz-" + strings.ReplaceAll(strings.TrimPrefix(each.pkg, "./"), "/", "-") + "-" + each.name
		if other, taken := byArtifact[artifact]; taken {
			return matrix{}, fmt.Errorf("%s in %s and %s in %s would both upload as %s", other.Target, other.Package, each.name, each.pkg, artifact)
		}
		entry := cell{Package: each.pkg, Target: each.name, Artifact: artifact}
		byArtifact[artifact] = entry
		planned.Include = append(planned.Include, entry)
	}
	return planned, nil
}
