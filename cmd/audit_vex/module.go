// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
)

// majorSuffix is the last element of a module path that names a major
// version from v2 on, which is not part of the project's name.
var majorSuffix = regexp.MustCompile(`^v([2-9]|[1-9]\d+)$`)

// goModule is what this command reads from a go.mod file.
type goModule struct {
	// path is the module path the module directive declares.
	path string
	// requires holds the version every require directive names, by module
	// path, which is the set a scanner reading go.mod alone (osv-scanner,
	// OpenSSF Scorecard) reports advisories against.
	requires map[string]string
}

// readGoMod reads the module directive and the require directives of the
// go.mod file at goMod.
//
// It reads those two and nothing else, which is all this command needs from
// the file, rather than adding golang.org/x/mod to the module's direct
// requirements for them. A comment is cut from every line first, so the
// // indirect mark and a trailing note are not read as fields, and a
// directive in a block of another kind (replace, exclude, retract) is never
// read as a requirement.
func readGoMod(goMod string) (goModule, error) {
	data, err := os.ReadFile(goMod) //#nosec G304 -- the go.mod of the tree the operator's -dir names
	if err != nil {
		return goModule{}, err
	}
	mod := goModule{requires: map[string]string{}}
	block := ""
	for line := range strings.Lines(string(data)) {
		code, _, _ := strings.Cut(line, "//")
		fields := strings.Fields(code)
		switch {
		case len(fields) == 0:
		case block != "":
			if fields[0] == ")" {
				block = ""
			} else if block == "require" && len(fields) >= 2 {
				mod.requires[unquote(fields[0])] = unquote(fields[1])
			}
		case len(fields) == 2 && fields[1] == "(":
			block = fields[0]
		case fields[0] == "module" && len(fields) >= 2:
			mod.path = unquote(fields[1])
		case fields[0] == "require" && len(fields) >= 3:
			mod.requires[unquote(fields[1])] = unquote(fields[2])
		}
	}
	if mod.path == "" {
		return goModule{}, fmt.Errorf("%s declares no module", goMod)
	}
	return mod, nil
}

// unquote takes off the quotes go.mod allows around a path or a version.
func unquote(s string) string {
	return strings.Trim(s, "\"`")
}

// imageName is the name the project's image is published under, which is the
// module path's last element once a major version suffix is set aside:
// github.com/jmrplens/gitlab-mcp-server/v3 publishes gitlab-mcp-server.
func imageName(module string) string {
	name := path.Base(module)
	if majorSuffix.MatchString(name) {
		name = path.Base(path.Dir(module))
	}
	return name
}
