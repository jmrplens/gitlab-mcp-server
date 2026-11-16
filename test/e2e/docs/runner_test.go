// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// runner_test.go is the runner: it takes a page's marked blocks in order and
// runs each in a reader, a shell it writes files to and runs scripts in,
// which is the container of reader_test.go in the live run and a local bash
// in the controls.
package docse2e

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/docblocks"
)

// shell is a reader's terminal: a home directory the runner writes a block's
// text into, and a bash that runs scripts there.
type shell interface {
	// write puts content at name, a path relative to the home directory.
	write(ctx context.Context, name string, content []byte) error
	// run runs script with bash in the home directory and returns what it
	// printed, standard error included.
	run(ctx context.Context, script string) (string, error)
}

// runnerDir is where, under the reader's home, the runner keeps each block's
// file, and a background block's log and process group.
const runnerDir = ".docs-runner"

// The verdicts a page can fail with, matched with errors.Is.
var (
	// errBlockFailed is a block that ended with a nonzero status, or a probe
	// that had not passed when the readiness window closed.
	errBlockFailed = errors.New("the block failed")
	// errBlockTimedOut is a block that did not end within the block timeout.
	errBlockTimedOut = errors.New("the block did not end")
	// errBackgroundStopped is a background block no longer running when the
	// page's next step needed it.
	errBackgroundStopped = errors.New("the background block stopped")
)

// timeoutStatus is the status GNU timeout ends with when it stopped the
// command it ran.
const timeoutStatus = 124

// substitutions are the values a block's placeholders become: the release
// the site renders %%VERSION%% as, the GitLab the reader reaches in place of
// the instances the pages name, and the token in place of the token
// placeholders.
type substitutions struct {
	version   string
	gitlabURL string
	token     string
}

// The placeholders the pages use. An instance is matched whole, so
// https://gitlab.company.com and https://gitlab.internal.example.com, which a
// page writes to mean some other instance, stay as printed; a token
// placeholder is glpat-your-token or glpat- followed by four x or more.
var (
	instancePlaceholder = regexp.MustCompile(`https://gitlab\.(?:example\.)?com\b`)
	tokenPlaceholder    = regexp.MustCompile(`glpat-(?:your-token|x{4,})`)
)

// versionPlaceholder is the token the site replaces with the release.
const versionPlaceholder = "%%VERSION%%"

// apply returns body with every placeholder replaced.
func (s substitutions) apply(body string) string {
	body = strings.ReplaceAll(body, versionPlaceholder, s.version)
	body = instancePlaceholder.ReplaceAllLiteralString(body, s.gitlabURL)
	return tokenPlaceholder.ReplaceAllLiteralString(body, s.token)
}

// redact returns text with the token replaced, for a report.
func (s substitutions) redact(text string) string {
	if s.token == "" {
		return text
	}
	return strings.ReplaceAll(text, s.token, "[token]")
}

// runner runs the marked blocks of a page in a reader.
type runner struct {
	shell shell
	subs  substitutions
	// blockTimeout bounds one run of a foreground block.
	blockTimeout time.Duration
	// settle is how long a background block runs before the runner checks
	// that it is still running.
	settle time.Duration
	// readiness is how long the first foreground block after a background
	// one is retried for.
	readiness time.Duration
	// poll is the wait between two tries of that block.
	poll time.Duration
	// logf, when set, is handed what each block that passed printed, so a
	// run's log shows what the pages' commands answered.
	logf func(format string, args ...any)
}

// background is a background block the runner started.
type background struct {
	line int
	pid  string
	log  string
}

// runPage runs a page's marked blocks in page order and returns the first
// failure, naming the page and the line. A background block still running
// when the page ends is stopped whatever the verdict.
func (r *runner) runPage(ctx context.Context, page string, blocks []docblocks.Block) error {
	var bg *background
	defer func() {
		if bg != nil {
			r.stop(ctx, bg)
		}
	}()
	probe := false
	for _, block := range blocks {
		where := fmt.Sprintf("%s:%d", page, block.Line)
		file := fmt.Sprintf("%s/block-%d.sh", runnerDir, block.Line)
		if err := r.shell.write(ctx, file, []byte(r.subs.apply(block.Body))); err != nil {
			return fmt.Errorf("%s: writing the block into the reader: %w", where, err)
		}
		if bg != nil {
			if err := r.stillRunning(ctx, page, bg, "before the block at "+where); err != nil {
				return err
			}
		}
		if block.Run == docblocks.RunBackground {
			if bg != nil {
				r.stop(ctx, bg)
			}
			started, err := r.start(ctx, page, block.Line, file)
			bg = started
			if err != nil {
				return err
			}
			probe = true
			continue
		}
		if err := r.foreground(ctx, where, file, bg, page, probe); err != nil {
			return err
		}
		probe = false
	}
	if bg != nil {
		return r.stillRunning(ctx, page, bg, "at the end of the page")
	}
	return nil
}

// foreground runs one block to its end. A probe, the first block after a
// background one, is tried again until it passes or the readiness window
// closes, as long as the background block keeps running; a try that hangs is
// a hang, not a server still starting, and is never tried again.
func (r *runner) foreground(ctx context.Context, where, file string, bg *background, page string, probe bool) error {
	deadline := time.Now().Add(r.readiness)
	for {
		out, err := r.shell.run(ctx, fmt.Sprintf("timeout -k 5 %s bash -e %s", seconds(r.blockTimeout), file))
		if err == nil {
			if r.logf != nil {
				r.logf("%s: passed:\n%s", where, r.subs.redact(out))
			}
			return nil
		}
		if status(err) == timeoutStatus {
			return fmt.Errorf("%s: did not end within %s:\n%s%w", where, r.blockTimeout, r.subs.redact(out), errBlockTimedOut)
		}
		if !probe || time.Now().After(deadline) {
			var waited string
			if probe {
				waited = fmt.Sprintf(", still failing when the readiness window of %s closed", r.readiness)
			}
			return fmt.Errorf("%s: %w%s:\n%s%w", where, err, waited, r.subs.redact(out), errBlockFailed)
		}
		if stopped := r.stillRunning(ctx, page, bg, "while the block at "+where+" waited for it"); stopped != nil {
			return stopped
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s: %w", where, ctx.Err())
		case <-time.After(r.poll):
		}
	}
}

// start starts a background block in a process group of its own and checks,
// after the settle time, that it is still running.
func (r *runner) start(ctx context.Context, page string, line int, file string) (*background, error) {
	bg := &background{
		line: line,
		pid:  fmt.Sprintf("%s/block-%d.pid", runnerDir, line),
		log:  fmt.Sprintf("%s/block-%d.log", runnerDir, line),
	}
	// setsid gives the block a process group whose id the pid file holds,
	// written by the block's own shell so it names the group leader whether
	// or not setsid had to fork, and stop signals the whole group.
	script := fmt.Sprintf(`setsid bash -c 'echo $$ >%[1]s; exec bash -e %[2]s' >%[3]s 2>&1 </dev/null &
for _ in $(seq 1 100); do [ -s %[1]s ] && exit 0; sleep 0.05; done
echo "the block wrote no pid file" >&2; exit 1`, bg.pid, file, bg.log)
	if out, err := r.shell.run(ctx, script); err != nil {
		return nil, fmt.Errorf("%s:%d: starting the block: %w:\n%s%w", page, line, err, r.subs.redact(out), errBlockFailed)
	}
	select {
	case <-ctx.Done():
		r.stop(ctx, bg)
		return nil, fmt.Errorf("%s:%d: %w", page, line, ctx.Err())
	case <-time.After(r.settle):
	}
	if err := r.stillRunning(ctx, page, bg, fmt.Sprintf("%s after it started", r.settle)); err != nil {
		// What the block started before it ended may still be running.
		r.stop(ctx, bg)
		return nil, err
	}
	return bg, nil
}

// stillRunning returns errBackgroundStopped, with the block's output, when the
// background block's process is gone; when says when the runner looked.
func (r *runner) stillRunning(ctx context.Context, page string, bg *background, when string) error {
	if _, err := r.shell.run(ctx, fmt.Sprintf(`kill -0 "$(cat %s)" 2>/dev/null`, bg.pid)); err == nil {
		return nil
	}
	log, _ := r.shell.run(ctx, "cat "+bg.log)
	return fmt.Errorf("%s:%d: stopped, found %s; its output:\n%s%w", page, bg.line, when, r.subs.redact(log), errBackgroundStopped)
}

// stop ends a background block's process group, politely first.
func (r *runner) stop(ctx context.Context, bg *background) {
	_, _ = r.shell.run(context.WithoutCancel(ctx), fmt.Sprintf(`pid=$(cat %s)
kill -TERM -- -"$pid" 2>/dev/null
for _ in $(seq 1 50); do kill -0 -- -"$pid" 2>/dev/null || exit 0; sleep 0.1; done
kill -KILL -- -"$pid" 2>/dev/null
exit 0`, bg.pid))
}

// seconds spells a duration the way GNU timeout reads one.
func seconds(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', -1, 64) + "s"
}

// status returns the exit status an error from a reader's run carries, or -1
// when it carries none.
func status(err error) int {
	var exit interface{ ExitCode() int }
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return -1
}

// markedPage is a page of the site that marks blocks to run.
type markedPage struct {
	// rel is the page's path under the content root, slash-separated.
	rel    string
	blocks []docblocks.Block
}

// markedPages walks the site's English pages under root, every .md and .mdx
// file outside es/, and returns those that mark blocks to run, in path order,
// with their blocks. A page that cannot be read with its twin, or whose twin
// marks other blocks, fails the walk: a page the runner skipped would pass.
func markedPages(root string) ([]markedPage, error) {
	var pages []markedPage
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "es" {
				return fs.SkipDir
			}
			return nil
		}
		if ext := filepath.Ext(rel); ext != ".md" && ext != ".mdx" {
			return nil
		}
		page, err := docblocks.Open(root, rel)
		if err != nil {
			return err
		}
		blocks, err := page.RunBlocks()
		if err != nil {
			return err
		}
		if len(blocks) > 0 {
			pages = append(pages, markedPage{rel: rel, blocks: blocks})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return pages, nil
}

// releaseVersion reads the release the site renders %%VERSION%% as from the
// tree's VERSION file.
func releaseVersion(repo string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(repo, "VERSION")) //#nosec G304 -- the repository's own VERSION file
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(raw))
	if version == "" {
		return "", errors.New("the VERSION file is empty")
	}
	return version, nil
}
