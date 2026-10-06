package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
)

// releaseTargets are the platforms the release builds: the goos list of
// .goreleaser.yml crossed with its goarch list, which is the set
// cmd/audit_dead_consts reads a platform-constrained package again under. A
// test holds the two lists to each other.
var releaseTargets = []string{
	"linux/amd64",
	"linux/arm64",
	"darwin/amd64",
	"darwin/arm64",
	"windows/amd64",
	"windows/arm64",
}

// listFormat is the row asked of `go list` per package: its import path, its
// module's path, and the four lists of files the go command builds for it,
// which are the lists scripts/coverage-conditions.sh compares a directory
// against. The four lists are compared as one string, so how they are joined
// does not matter as long as it is the same under every target.
const listFormat = `{{.ImportPath}}	{{with .Module}}{{.Path}}{{end}}	{{join .GoFiles " "}}	{{join .CgoFiles " "}}	{{join .TestGoFiles " "}}	{{join .XTestGoFiles " "}}`

// lister runs the go command in dir with env added to the process
// environment, and returns its standard output. It is a parameter so a test
// can answer for the toolchain.
type lister func(ctx context.Context, dir string, env []string, args ...string) ([]byte, error)

// goList is the lister production uses: the toolchain's own go binary, never
// one looked up on PATH.
func goList(ctx context.Context, dir string, env []string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, golist.Executable(), args...) //#nosec G204 -- golist.Executable is the toolchain's own go binary, joined out of GOROOT
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// listedPackage is one row of a listing.
type listedPackage struct {
	path   string
	module string
	files  string
}

// constrainedPackages returns, as ./-relative package paths in sorted order,
// every package of the module in dir whose built files differ between two of
// the release targets.
//
// CGO_ENABLED is set because the go command turns cgo off for a platform
// other than the host's, which would make a cgo file read as constrained to
// the host; the script sets it for its reference listing for the same reason.
func constrainedPackages(ctx context.Context, dir string, list lister) ([]string, error) {
	first := map[string]listedPackage{}
	varies := map[string]bool{}
	for _, target := range releaseTargets {
		goos, goarch, _ := strings.Cut(target, "/")
		out, err := list(ctx, dir, []string{"GOOS=" + goos, "GOARCH=" + goarch, "CGO_ENABLED=1"},
			"list", "-e", "-f", listFormat, "./...")
		if err != nil {
			return nil, fmt.Errorf("go list for %s: %w", target, err)
		}
		rows, err := parseListing(out)
		if err != nil {
			return nil, fmt.Errorf("go list for %s: %w", target, err)
		}
		for _, row := range rows {
			seen, known := first[row.path]
			if !known {
				first[row.path] = row
			} else if seen.files != row.files {
				varies[row.path] = true
			}
		}
	}
	dirs := make([]string, 0, len(varies))
	for path := range varies {
		packageDir, err := relativePackage(first[path])
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, packageDir)
	}
	sort.Strings(dirs)
	return dirs, nil
}

// parseListing reads the rows a `go list -f listFormat` run printed. A blank
// line is skipped, and anything that is not exactly six tab-separated fields
// is refused rather than guessed at.
func parseListing(out []byte) ([]listedPackage, error) {
	var rows []listedPackage
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 6 {
			return nil, fmt.Errorf("unexpected go list row %q", line)
		}
		rows = append(rows, listedPackage{path: fields[0], module: fields[1], files: strings.Join(fields[2:], "\t")})
	}
	return rows, nil
}

// relativePackage names a package the way the script and the workflow take
// it, ./ and its directory below the module root. A package that is not below
// its module's path, the module root itself included, is refused: no package
// of this module sits at its root, and one that did would need a name the
// script reads differently.
func relativePackage(pkg listedPackage) (string, error) {
	rel, below := strings.CutPrefix(pkg.path, pkg.module+"/")
	if !below {
		return "", fmt.Errorf("go list named %q, which is not below its module path %q", pkg.path, pkg.module)
	}
	return "./" + rel, nil
}
