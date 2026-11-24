// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package golist

import (
	"fmt"
	"strconv"
	"strings"
)

// FilesFormat is the `-f` template a listing of packages with their files
// asks for, and the order [ParseFileRows] reads back. It is asked with -e, so
// a package go cannot load is a row whose last field says why rather than the
// end of the listing. That field is quoted, since an error can span lines,
// and is empty for a package that loads. The two directories are quoted too,
// since a path may hold a tab.
const FilesFormat = `{{printf "%q" .Dir}}	{{with .Module}}{{printf "%q" .Dir}}{{end}}	{{.Name}}	{{.ImportPath}}	{{join .GoFiles " "}}	{{join .CgoFiles " "}}	{{join .TestGoFiles " "}}	{{join .XTestGoFiles " "}}	{{join .Imports " "}}	{{join .TestImports " "}}	{{join .XTestImports " "}}	{{with .Error}}{{printf "%q" .Error}}{{end}}`

// fileRowFields is how many fields a row of [FilesFormat] holds.
const fileRowFields = 12

// PackageFiles is one package with the files the go command builds for it
// under the platform and tags it was asked with, and what each kind of them
// imports.
//
// Dir and ModuleDir are written with slashes whatever the platform, since the
// callers cut one against the other and write the result into patterns the go
// command and gremlins read; go and the os package read either on Windows.
// Error is why go could not load the package, and is empty when it could.
type PackageFiles struct {
	Dir          string
	ModuleDir    string
	Name         string
	ImportPath   string
	GoFiles      []string
	CgoFiles     []string
	TestGoFiles  []string
	XTestGoFiles []string
	Imports      []string
	TestImports  []string
	XTestImports []string
	Error        string
}

// Sources returns the package's non-test files.
func (p PackageFiles) Sources() []string {
	return append(append([]string{}, p.GoFiles...), p.CgoFiles...)
}

// Built returns every file the go command builds for the package, its tests
// of both kinds included.
func (p PackageFiles) Built() []string {
	return append(append(p.Sources(), p.TestGoFiles...), p.XTestGoFiles...)
}

// ParseFileRows reads the output of a `go list -e -f FilesFormat` run into
// one entry per package. A blank line is skipped, and anything that is not
// exactly the fields of the format is refused rather than guessed at, for the
// reason [ParseRows] refuses it.
func ParseFileRows(output []byte) ([]PackageFiles, error) {
	packages := []PackageFiles{}
	for line := range strings.SplitSeq(string(output), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != fileRowFields {
			return nil, fmt.Errorf("unexpected go list row: %q", line)
		}
		for _, quoted := range []*string{&fields[0], &fields[1], &fields[11]} {
			if *quoted == "" {
				continue
			}
			text, err := strconv.Unquote(*quoted)
			if err != nil {
				return nil, fmt.Errorf("unexpected go list row: %q", line)
			}
			*quoted = text
		}
		row := PackageFiles{
			Dir:          slashed(fields[0]),
			ModuleDir:    slashed(fields[1]),
			Name:         fields[2],
			ImportPath:   fields[3],
			GoFiles:      strings.Fields(fields[4]),
			CgoFiles:     strings.Fields(fields[5]),
			TestGoFiles:  strings.Fields(fields[6]),
			XTestGoFiles: strings.Fields(fields[7]),
			Imports:      strings.Fields(fields[8]),
			TestImports:  strings.Fields(fields[9]),
			XTestImports: strings.Fields(fields[10]),
			Error:        fields[11],
		}
		packages = append(packages, row)
	}
	return packages, nil
}

// slashed writes a native path with slashes. It replaces every backslash
// whatever the platform it runs on, rather than the separator of this one, so
// a listing taken from Windows reads the same wherever it is parsed.
func slashed(path string) string {
	return strings.ReplaceAll(path, `\`, "/")
}
