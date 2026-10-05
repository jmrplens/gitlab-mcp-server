package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/orbitrecord"
)

// committedRevision names the record as HEAD holds it. The path is relative to
// the directory git runs in, which is the record's own, so it names the same
// file whichever directory -dir points at inside the repository.
func committedRevision() string { return "HEAD:./" + orbitrecord.FileName }

// committedRecord reads the record as HEAD holds it in the repository cfg.dir
// belongs to. A directory outside any repository, a record HEAD does not hold
// yet, and a machine with no git all come back as the error they are, which
// the caller reports as having nothing to compare with.
func committedRecord(ctx context.Context, cfg genRun) (orbitrecord.Document, error) {
	raw, err := cfg.git(ctx, cfg.dir, "show", committedRevision())
	if err != nil {
		return orbitrecord.Document{}, err
	}
	return orbitrecord.Decode(raw)
}

// runGit runs git in dir and returns what it wrote to standard output.
//
// The program is resolved to an absolute path first, which is how every other
// command here runs a tool it did not build (cmd/gen_model_results resolves
// git this way): the lookup is the one exec
// would do, decided here rather than by whatever PATH holds when the command
// starts. A git that ran and refused says why on standard error, which is
// what the error carries, since "exit status 128" alone tells a reader
// nothing.
func runGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	bin, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("find git: %w", err)
	}
	// #nosec G204 -- the program is the absolute path just resolved, and the
	// arguments are this command's own literals: no value read from a file or
	// from the network reaches them.
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	output, err := cmd.Output()
	if exitErr, isExit := errors.AsType[*exec.ExitError](err); isExit {
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exitErr.Stderr)))
	}
	if err != nil {
		return nil, fmt.Errorf("run git: %w", err)
	}
	return output, nil
}
