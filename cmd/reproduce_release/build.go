package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
)

// goBuild builds one recipe from the module at src into root, at the path
// GoReleaser writes it to, and returns the binary. extra is added to the
// environment last, so what the run decides (the toolchain, the build cache)
// wins over anything before it. root has to be absolute, since the go command
// runs in src.
func goBuild(ctx context.Context, src string, rc recipe, root string, extra ...string) ([]byte, error) {
	out := filepath.Join(root, rc.dir, rc.file)
	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		return nil, err
	}
	// #nosec G204 G702 -- the program is the Go toolchain joined out of GOROOT, and the arguments come from the release configuration of the tree being rebuilt
	cmd := exec.CommandContext(ctx, golist.Executable(), rc.arguments(out)...)
	cmd.Dir = src
	cmd.Env = buildEnv(os.Environ(), rc, extra)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("building %s for %s: %w\n%s", rc.asset, rc.target, err, output)
	}
	return os.ReadFile(out) //#nosec G304 -- the binary the go command just wrote under this run's own output directory
}

// buildEnv is the environment a recipe is built in, in GoReleaser's order
// (the process's own, then the build's env, then the target's), so a value
// the build or the target sets wins over one the caller's shell exported.
// GOFLAGS is cleared before the build's env, since the release job sets none
// and every flag a release passes is on its command line, and extra comes
// last.
func buildEnv(base []string, rc recipe, extra []string) []string {
	return slices.Concat(base, []string{"GOFLAGS="}, rc.env, rc.target.env(), extra)
}
