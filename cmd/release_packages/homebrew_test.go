// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
// exit code, stdout, stderr, the formula and both logs to the script's. The
// release checks the formula with ruby -c, so on CI, where GitHub sets CI on
// every step, a missing git or ruby fails the test rather than skipping
// every recorded run, as the script's suite did.
func TestHomebrew_RecordedRuns_MatchTheScript(t *testing.T) {
	var missing error
	for _, tool := range []string{"git", "ruby"} {
		if _, err := exec.LookPath(tool); err != nil && missing == nil {
			missing = err
		}
	}
	if missing != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("the tap update needs git and ruby, and CI must run these cases rather than skip them: %v", missing)
		}
		t.Skipf("the tap update needs git and ruby: %v", missing)
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
			// git prints the tap's path with the host's separator, a
			// backslash on Windows, where the script ran on a slash.
			placeholder := strings.NewReplacer(tree+string(filepath.Separator), "<tree>/", tree, "<tree>")
			if code != run.Exit {
				t.Errorf("exit %d, the script exited %d (stderr %q)", code, run.Exit, stderr.String())
			}
			if got := placeholder.Replace(stdout.String()); got != run.Stdout {
				t.Errorf("stdout differs from the script's:\n--- got\n%s\n--- want\n%s", got, run.Stdout)
			}
			if got := placeholder.Replace(stderr.String()); got != run.Stderr {
				t.Errorf("stderr differs from the script's:\n--- got\n%s\n--- want\n%s", got, run.Stderr)
			}
			compareTap(t, run, tree, env)
		})
	}
}

// compareTap holds the formula a run left and the logs of the tap and of
// its clone to what the script left.
func compareTap(t *testing.T, run tapRun, tree string, env []string) {
	t.Helper()
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
}

// TestSortVersionCompare_OrdersAsSortV holds the downgrade check's order to
// GNU sort -V on the versions a release names, prereleases and leading
// zeros included, and compares with the sort on the host when it has one.
func TestSortVersionCompare_OrdersAsSortV(t *testing.T) {
	ordered := []string{"1.2", "1.2.0", "1.2.0-rc.1", "1.2.0-rc.2", "1.2.0-rc.10", "1.2.1", "1.10.0", "1.010.1", "2.0.0~beta", "2.0.0", "2.0.0.tar", "10.0.0"}
	for i := range len(ordered) - 1 {
		a, b := ordered[i], ordered[i+1]
		t.Run(a+" before "+b, func(t *testing.T) {
			if sortVersionCompare(a, b) >= 0 || sortVersionCompare(b, a) <= 0 {
				t.Errorf("%s does not sort before %s", a, b)
			}
		})
	}
	sortV, err := exec.LookPath("sort")
	if err != nil {
		return
	}
	shuffled := slices.Clone(ordered)
	slices.Reverse(shuffled)
	cmd := exec.CommandContext(t.Context(), sortV, "-V")
	cmd.Stdin = strings.NewReader(strings.Join(shuffled, "\n") + "\n")
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("sort -V is not available: %v", err)
	}
	if got := strings.Fields(string(out)); !slices.Equal(got, ordered) {
		t.Errorf("sort -V orders them %q, the table %q", got, ordered)
	}
	if sortVersionCompare("", "1") >= 0 || sortVersionCompare("1", "") <= 0 || sortVersionCompare("1", "1") != 0 {
		t.Error("an empty version does not sort first")
	}
}

// TestHomebrew_RefusesWhatItCannotUse holds the refusals that end a run
// before the tap is touched: a usage error, a checksums.txt it cannot read
// and a tree without LICENSE.
func TestHomebrew_RefusesWhatItCannotUse(t *testing.T) {
	record := readStampRecord(t)
	tree := t.TempDir()
	writeTree(t, tree, map[string][]byte{"dist/checksums.txt": []byte(record.Stamps[0].Files["dist/checksums.txt"])})
	cases := []struct {
		name string
		args []string
		code int
		want string
	}{
		{"usage", []string{"dist/checksums.txt"}, 2, "homebrew takes"},
		{"no checksums.txt", []string{"dist/none.txt", "1.0.0"}, 1, "none.txt"},
		{"no LICENSE", []string{"dist/checksums.txt", "1.0.0"}, 1, "no LICENSE at"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := runIn(tree, append([]string{"homebrew"}, tc.args...), &stdout, &stderr, os.Getenv); code != tc.code || !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("exit %d, stderr %q; want %d naming %q", code, stderr.String(), tc.code, tc.want)
			}
		})
	}
}
