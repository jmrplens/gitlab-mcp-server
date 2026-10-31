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

// readModulePath returns the module path the go.mod file at goMod declares.
//
// It reads the module directive and nothing else, which is all this command
// needs from the file, rather than adding golang.org/x/mod to the module's
// direct requirements for one line.
func readModulePath(goMod string) (string, error) {
	data, err := os.ReadFile(goMod) //#nosec G304 -- the go.mod of the tree the operator's -dir names
	if err != nil {
		return "", err
	}
	for line := range strings.Lines(string(data)) {
		if fields := strings.Fields(line); len(fields) >= 2 && fields[0] == "module" {
			return strings.Trim(fields[1], "\"`"), nil
		}
	}
	return "", fmt.Errorf("%s declares no module", goMod)
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
