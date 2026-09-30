package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
)

// binary is one built release target, and where it was written.
type binary struct {
	build  string
	target target
	path   string
}

// buildAll builds every target of every build into outDir, from dir.
//
// The environment is the process's own with the entry's env and the target's
// GOOS and GOARCH after it, which is the order GoReleaser applies them in, so a
// value the entry sets wins over one the caller's shell happened to export.
func buildAll(ctx context.Context, dir string, builds []build, outDir string) ([]binary, error) {
	var built []binary
	for _, b := range builds {
		for _, t := range b.targets {
			out := filepath.Join(outDir, b.id+"_"+t.goos+"_"+t.goarch)
			args := append([]string{"build", "-o", out}, b.flags...)
			args = append(args, b.main)
			// #nosec G204 G702 -- the program is the Go toolchain joined out of GOROOT, and the arguments come from this repository's own release configuration
			cmd := exec.CommandContext(ctx, golist.Executable(), args...)
			cmd.Dir = dir
			cmd.Env = append(append(os.Environ(), b.env...), "GOOS="+t.goos, "GOARCH="+t.goarch)
			if output, err := cmd.CombinedOutput(); err != nil {
				return nil, fmt.Errorf("building %s for %s: %w\n%s", b.id, t, err, output)
			}
			built = append(built, binary{build: b.id, target: t, path: out})
		}
	}
	return built, nil
}
