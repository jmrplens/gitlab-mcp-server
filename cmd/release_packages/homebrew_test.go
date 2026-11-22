// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// tapRun is one recorded run of update-homebrew-tap.sh.
type tapRun struct {
	Name           string   `json:"name"`
	Args           []string `json:"args"`
	FormulaVersion *string  `json:"formula_version"`
	Checksums      string   `json:"checksums"`
	Clone          bool     `json:"clone"`
	Exit           int      `json:"exit"`
	Stdout         string   `json:"stdout"`
	Stderr         string   `json:"stderr"`
	Formula        *string  `json:"formula"`
	PushedLog      string   `json:"pushed_log"`
	LocalLog       string   `json:"local_log"`
}

// tapGit runs git for the replay with the record's environment, failing the
// test on an error, and returns its stdout.
func tapGit(t *testing.T, env []string, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir, cmd.Env = dir, env
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return string(out)
}

// layOutTap builds what one recorded tap run started from: LICENSE, a bare
// tap with the recorder's first commit and, unless the run clones it, a
// clone holding the formula of the version it names, rendered by this
// command as the recorder rendered it with the script.
func layOutTap(t *testing.T, record stampRecord, run tapRun, env []string) string {
	t.Helper()
	tree := t.TempDir()
	writeTree(t, tree, map[string][]byte{"LICENSE": []byte(record.License), "dist/checksums.txt": []byte(run.Checksums)})
	bare := filepath.Join(tree, "tap.git")
	tapGit(t, env, tree, "init", "-q", "--bare", "-b", "main", bare)
	seed := filepath.Join(tree, "seed")
	tapGit(t, env, tree, "clone", "-q", bare, seed)
	writeTree(t, seed, map[string][]byte{"README.md": []byte("tap\n")})
	tapGit(t, env, seed, "add", "README.md")
	tapGit(t, env, seed, "-c", "user.name=Tap", "-c", "user.email=tap@example.com", "commit", "-q", "-m", "init")
	tapGit(t, env, seed, "push", "-q", "origin", "HEAD:main")
	if err := os.RemoveAll(seed); err != nil {
		t.Fatal(err)
	}
	if run.Clone {
		return tree
	}
	tapGit(t, env, tree, "clone", "-q", bare, "tap")
	if run.FormulaVersion != nil {
		seedSums := filepath.Join("dist", "seed.txt")
		writeTree(t, tree, map[string][]byte{seedSums: []byte(record.Stamps[0].Files["dist/checksums.txt"])})
		var out bytes.Buffer
		if code := runHomebrew(tree, []string{seedSums, *run.FormulaVersion, "tap"}, &out, &out, env); code != 0 {
			t.Fatalf("seeding the tap with %s exited %d: %s", *run.FormulaVersion, code, out.String())
		}
		if err := os.Remove(filepath.Join(tree, seedSums)); err != nil {
			t.Fatal(err)
		}
	}
	return tree
}

// TestHomebrew_RecordedRuns_MatchTheScript replays every recorded tap run
// against a local bare tap, with the record's git environment, and holds the
// exit code, stdout, stderr, the formula and both logs to the script's.
func TestHomebrew_RecordedRuns_MatchTheScript(t *testing.T) {
	for _, tool := range []string{"git", "ruby"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed: %v", tool, err)
		}
	}
	record := readStampRecord(t)
	for _, run := range record.Taps {
		t.Run(run.Name, func(t *testing.T) {
			env := os.Environ()
			for key, value := range record.GitEnv {
				env = append(env, key+"="+value)
			}
			tree := layOutTap(t, record, run, env)
			env = append(env, "TAP_REPO="+filepath.Join(tree, "tap.git"))
			var stdout, stderr bytes.Buffer
			code := runHomebrew(tree, run.Args, &stdout, &stderr, env)
			placeholder := strings.NewReplacer(tree, "<tree>")
			if code != run.Exit {
				t.Errorf("exit %d, the script exited %d (stderr %q)", code, run.Exit, stderr.String())
			}
			if got := placeholder.Replace(stdout.String()); got != run.Stdout {
				t.Errorf("stdout differs from the script's:\n--- got\n%s\n--- want\n%s", got, run.Stdout)
			}
			if got := placeholder.Replace(stderr.String()); got != run.Stderr {
				t.Errorf("stderr differs from the script's:\n--- got\n%s\n--- want\n%s", got, run.Stderr)
			}
			clone := filepath.Join(tree, run.Args[2])
			formula, err := os.ReadFile(filepath.Join(clone, "Formula", "gitlab-mcp-server.rb"))
			switch {
			case run.Formula == nil && err == nil:
				t.Errorf("a formula was written where the script wrote none")
			case run.Formula != nil && string(formula) != *run.Formula:
				t.Errorf("the formula differs from the script's (%v):\n--- got\n%s\n--- want\n%s", err, formula, *run.Formula)
			}
			if got := tapGit(t, env, tree, "--git-dir", filepath.Join(tree, "tap.git"), "log", "--format=%H %an <%ae> %s", "main"); got != run.PushedLog {
				t.Errorf("the tap holds:\n%s\nthe script left:\n%s", got, run.PushedLog)
			}
			if _, statErr := os.Stat(clone); statErr == nil {
				if got := tapGit(t, env, clone, "log", "--format=%H %an <%ae> %s", "HEAD"); got != run.LocalLog {
					t.Errorf("the clone holds:\n%s\nthe script left:\n%s", got, run.LocalLog)
				}
			}
		})
	}
}
