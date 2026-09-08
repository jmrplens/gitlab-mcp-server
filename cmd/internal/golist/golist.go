package golist

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

// Format is the `-f` template a listing asks for, and the order [ParseRows]
// reads back.
//
// The three fields are what both callers need and all they need: the directory
// to read the package's files out of, the import path to name it by, and the
// package clause's own name, which is not derivable from either (a directory
// and its package disagree whenever a package is renamed or a directory holds
// package main).
const Format = "{{.Dir}}\t{{.ImportPath}}\t{{.Name}}"

// runtimeGOOS is the operating system [Executable] appends a suffix for. It is
// a seam because the tests run on one host and the Windows branch has to be
// reachable from it; production reads runtime.GOOS once, here.
var runtimeGOOS = runtime.GOOS

// PackageInfo identifies one Go package returned by go list.
//
// Dir is the absolute directory of the package; ImportPath is its module
// path; Name is the short package identifier from the package clause.
type PackageInfo struct {
	Dir        string `json:"dir"`
	ImportPath string `json:"import_path"`
	Name       string `json:"name"`
}

// ParseRows reads the output of a `go list -f Format` run into one entry per
// package.
//
// A blank line is skipped, because the toolchain ends its output with a
// newline and a caller that merges stderr can pick up an empty one. Anything
// else that is not exactly three tab-separated fields is refused rather than
// guessed at: a caller that captures combined output gets the toolchain's
// warnings in the same stream as its rows, and a warning silently parsed as a
// package is a package audited or documented under a name nothing has.
func ParseRows(output []byte) ([]PackageInfo, error) {
	packages := []PackageInfo{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			return nil, fmt.Errorf("unexpected go list row: %q", line)
		}
		packages = append(packages, PackageInfo{Dir: fields[0], ImportPath: fields[1], Name: fields[2]})
	}
	return packages, nil
}

// Executable returns the absolute Go tool path from the running toolchain.
//
// It is joined out of GOROOT rather than looked up on PATH so that a command
// this repository runs on a developer's machine cannot be pointed at another
// program by the environment, which is Sonar's go:S4036; the callers' exec
// sites cite this function in their `#nosec G204` notes for the same reason.
func Executable() string {
	name := "go"
	if runtimeGOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(runtime.GOROOT(), "bin", name) //nolint:staticcheck // Avoid PATH lookup for Sonar go:S4036.
}
