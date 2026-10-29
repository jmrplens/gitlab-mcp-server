// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package golist

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ModuleFormat is the `-f` template [ModuleDir] asks `go list -m` for: the
// directory the module's source sits in.
const ModuleFormat = "{{.Dir}}"

// listModule runs `go list -m` for one module from dir and returns its
// standard output. It is a seam because the one answer a test cannot arrange
// from outside is a module the build list names and the module cache does not
// hold, which the go command answers with an empty directory.
var listModule = func(ctx context.Context, dir, module string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, Executable(), "list", "-m", "-f", ModuleFormat, module) //#nosec G204 -- Executable is the toolchain's own go binary, joined out of GOROOT, and module is a constant of the caller's
	cmd.Dir = dir
	return cmd.Output()
}

// ModuleDir returns the directory of module in the build list of the module
// at dir, which is where its pinned source sits in the module cache, without
// loading or type-checking any package of it.
//
// R-ROUTE (cmd/audit_1to1 -scope=routes) asks it for client-go's directory,
// to read which routes the pinned SDK sends without the typed load of the
// tool packages the other scopes find it through. A failure of the go command
// is returned with what it wrote on its standard error, which says why (a
// module the build list does not know), and a module it answers with no
// directory, one the cache does not hold, is an error rather than an empty
// path a reader would parse as a module with no source.
func ModuleDir(ctx context.Context, dir, module string) (string, error) {
	out, err := listModule(ctx, dir, module)
	if err != nil {
		var exited *exec.ExitError
		if errors.As(err, &exited) && len(exited.Stderr) > 0 {
			return "", fmt.Errorf("go list -m %s: %w: %s", module, err, strings.TrimSpace(string(exited.Stderr)))
		}
		return "", fmt.Errorf("go list -m %s: %w", module, err)
	}
	moduleDir := strings.TrimSpace(string(out))
	if moduleDir == "" {
		return "", fmt.Errorf("go list -m %s: the module has no directory; run go mod download", module)
	}
	return moduleDir, nil
}
