// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// order_test.go covers the order check, the release workflow's decision about
// latest, with every case scripts/release_order_sh_test.py held the script it
// replaces to. Every v* tag used to publish the image as :latest, mark the
// GitHub release Latest, stamp main's manifests, move npm's latest dist-tag
// and update the Homebrew tap, whatever version it named, so a backport
// tagged after a newer release rolled every one of those back, and a
// prerelease tag went out as the default image before the npm job refused it
// (issue 1323). The cases run the check over tag lists written here, so
// nothing reaches a remote.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"
	"testing/iotest"
)

// lsRemote is a tag list as `git ls-remote --tags --refs` prints it: an
// object name, a tab and the ref, for three releases, a prerelease above
// them and an older release out of order.
var lsRemote = func() string {
	var b strings.Builder
	for i, tag := range []string{"v3.0.0", "v3.1.0", "v3.2.0", "v3.3.0-rc.1", "v2.7.5"} {
		fmt.Fprintf(&b, "0123456789abcdef0123456789abcdef0123456%d\trefs/tags/%s\n", i, tag)
	}
	return b.String()
}()

// outputLine is what $GITHUB_OUTPUT accepts as one line of the check's
// stdout: a key, an equals sign and a value with no space or equals sign.
var outputLine = regexp.MustCompile(`^[a-z_]+=[^\s=]+$`)

// orderRun is one run of the order check.
type orderRun struct {
	code           int
	stdout, stderr string
}

// runOrderCheck runs the order check on tag with tags on stdin, as the
// preflight job runs it.
func runOrderCheck(tag, tags string) orderRun {
	var stdout, stderr bytes.Buffer
	code := runMain([]string{"order", tag}, strings.NewReader(tags), &stdout, &stderr)
	return orderRun{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// outputs holds a run to passing and to writing nothing but lines
// $GITHUB_OUTPUT accepts, and returns them by key.
func outputs(t *testing.T, run orderRun) map[string]string {
	t.Helper()

	if run.code != 0 {
		t.Fatalf("the order check exited %d, want 0 (stdout %q, stderr %q)", run.code, run.stdout, run.stderr)
	}
	got := map[string]string{}
	for line := range strings.Lines(run.stdout) {
		line = strings.TrimSuffix(line, "\n")
		if !outputLine.MatchString(line) {
			t.Errorf("stdout line %q is not a key=value pair $GITHUB_OUTPUT accepts", line)
		}
		key, value, _ := strings.Cut(line, "=")
		got[key] = value
	}
	return got
}

// assertOutputs holds the three answers to want.
func assertOutputs(t *testing.T, got, want map[string]string) {
	t.Helper()

	if len(got) != len(want) {
		t.Errorf("outputs = %v, want %v", got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("output %s = %q, want %q (all: %v)", key, got[key], value, got)
		}
	}
}

// TestOrder_TheHighestStableTagIsTheNewest covers the ordinary release: the
// highest stable tag the repository holds is the newest, moves every pointer
// and publishes to npm as latest.
func TestOrder_TheHighestStableTagIsTheNewest(t *testing.T) {
	t.Parallel()

	run := runOrderCheck("v3.2.0", lsRemote)
	assertOutputs(t, outputs(t, run), map[string]string{"stable": "true", "newest": "true", "npm_dist_tag": "latest"})
	if run.stderr != "" {
		t.Errorf("the newest release wrote %q on stderr, want nothing", run.stderr)
	}
}

// TestOrder_ATagTheRemoteDoesNotHoldYetIsComparedAsItself covers a rehearsal,
// which names the version VERSION carries before that tag is cut.
func TestOrder_ATagTheRemoteDoesNotHoldYetIsComparedAsItself(t *testing.T) {
	t.Parallel()

	if got := outputs(t, runOrderCheck("v3.3.0", lsRemote)); got["newest"] != "true" {
		t.Errorf("newest = %q for a tag above every one listed, want true", got["newest"])
	}
}

// TestOrder_ABackportIsNotTheNewestAndKeepsNpmLatest covers a stable tag lower
// than one already released: it is not the newest, publishes to npm under its
// minor's dist-tag, and says on stderr which release keeps the pointers.
func TestOrder_ABackportIsNotTheNewestAndKeepsNpmLatest(t *testing.T) {
	t.Parallel()

	run := runOrderCheck("v3.1.1", lsRemote+"fedcba\trefs/tags/v3.1.1\n")
	assertOutputs(t, outputs(t, run), map[string]string{"stable": "true", "newest": "false", "npm_dist_tag": "release-3.1"})
	if want := "v3.1.1 is a backport: v3.2.0 is newer, so the pointers to the newest release stay on it\n"; run.stderr != want {
		t.Errorf("stderr = %q, want %q", run.stderr, want)
	}
}

// TestOrder_VersionsAreOrderedByNumberNotByText covers the comparison: each
// number is compared as a number, so 10 is above 9 whatever its digits say,
// and two spellings of one number are ordered by their bytes, as sort -V
// ordered them.
func TestOrder_VersionsAreOrderedByNumberNotByText(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		tag  string
		tags string
		want string
	}{
		{name: "v3.10.0 above v3.9.0", tag: "v3.10.0", tags: "v3.9.0\nv3.10.0\n", want: "true"},
		{name: "v3.9.1 below v3.10.0", tag: "v3.9.1", tags: "v3.9.0\nv3.10.0\n", want: "false"},
		{name: "a major above every minor", tag: "v10.0.0", tags: "v9.99.99\n", want: "true"},
		{name: "a patch decides between equal minors", tag: "v3.2.1", tags: "v3.2.2\n", want: "false"},
		{name: "a minor decides before the patch", tag: "v3.3.0", tags: "v3.2.9\n", want: "true"},
		{name: "a lower major whatever its minor", tag: "v2.99.0", tags: "v3.0.0\n", want: "false"},
		{name: "a longer number above a shorter one", tag: "v3.100.0", tags: "v3.99.0\n", want: "true"},
		{name: "leading zeros compare as the number", tag: "v3.010.0", tags: "v3.9.0\n", want: "true"},
		{name: "the tag itself listed", tag: "v3.2.0", tags: "v3.2.0\n", want: "true"},
		{name: "a plain spelling above a zero-padded one", tag: "v3.1.0", tags: "v3.01.0\n", want: "true"},
		{name: "a zero-padded spelling below a plain one", tag: "v3.01.0", tags: "v3.1.0\n", want: "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := outputs(t, runOrderCheck(tc.tag, tc.tags)); got["newest"] != tc.want {
				t.Errorf("newest = %q for %s among %q, want %s", got["newest"], tc.tag, tc.tags, tc.want)
			}
		})
	}
}

// TestOrder_APrereleaseInTheRemoteDoesNotOutrankARelease covers a prerelease
// tag already pushed: v3.3.0-rc.1 sorts above v3.2.0 by version, and must not
// stop v3.2.1 from being the newest stable release.
func TestOrder_APrereleaseInTheRemoteDoesNotOutrankARelease(t *testing.T) {
	t.Parallel()

	if got := outputs(t, runOrderCheck("v3.2.1", lsRemote)); got["newest"] != "true" {
		t.Errorf("newest = %q with only a prerelease above it, want true", got["newest"])
	}
}

// TestOrder_AnEmptyRemoteMakesTheFirstReleaseTheNewest covers a repository
// holding no tag yet.
func TestOrder_AnEmptyRemoteMakesTheFirstReleaseTheNewest(t *testing.T) {
	t.Parallel()

	if got := outputs(t, runOrderCheck("v0.1.0", "")); got["newest"] != "true" {
		t.Errorf("newest = %q with no tag listed, want true", got["newest"])
	}
}

// TestOrder_ReadsEachLineOfTheList covers how a line names a tag: its last
// field, a refs/tags/ prefix dropped, whatever whitespace surrounds it (a
// carriage return included, which the script took for the space before the
// last field, so it read an empty name and passed the tag over), the last
// line read without a newline after it, and a
// line naming no stable tag (a branch, a peeled tag, nothing) passed over.
func TestOrder_ReadsEachLineOfTheList(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		tags string
		want string
	}{
		{name: "a name alone", tags: "v9.0.0\n", want: "false"},
		{name: "an ls-remote line", tags: "fedcba\trefs/tags/v9.0.0\n", want: "false"},
		{name: "spaces around the fields", tags: "  fedcba   refs/tags/v9.0.0  \n", want: "false"},
		{name: "the last line without a newline", tags: "v1.0.0\nv9.0.0", want: "false"},
		{name: "a carriage return before the newline", tags: "v9.0.0\r\n", want: "false"},
		{name: "blank lines", tags: "\n\n  \n", want: "true"},
		{name: "a branch, not a tag", tags: "fedcba\trefs/heads/v9.0.0\n", want: "true"},
		{name: "a peeled tag", tags: "fedcba\trefs/tags/v9.0.0^{}\n", want: "true"},
		{name: "a prefix twice", tags: "refs/tags/refs/tags/v9.0.0\n", want: "true"},
		{name: "a prerelease", tags: "v9.0.0-rc.1\n", want: "true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := outputs(t, runOrderCheck("v3.2.0", tc.tags)); got["newest"] != tc.want {
				t.Errorf("newest = %q for v3.2.0 among %q, want %s", got["newest"], tc.tags, tc.want)
			}
		})
	}
}

// TestOrder_APrereleaseTagIsRefusedBeforeAnythingPublishes covers every tag
// that is not a plain vX.Y.Z: refused with exit 1 and an ::error:: line,
// and nothing on stdout, which is appended to $GITHUB_OUTPUT whole.
func TestOrder_APrereleaseTagIsRefusedBeforeAnythingPublishes(t *testing.T) {
	t.Parallel()

	for _, tag := range []string{"v3.3.0-rc.1", "v3.3.0+build.5", "3.3.0", "v3.3", "v3.3.0.1", "release", "V3.3.0", " v3.3.0", "v3.3.0\n", "v3..0"} {
		t.Run(tag, func(t *testing.T) {
			t.Parallel()

			run := runOrderCheck(tag, lsRemote)
			if run.code != 1 {
				t.Errorf("the order check exited %d for %q, want 1 (stderr %q)", run.code, tag, run.stderr)
			}
			if want := "::error::" + tag + " is not a release version: a release tag is vMAJOR.MINOR.PATCH"; !strings.HasPrefix(run.stderr, want) {
				t.Errorf("stderr = %q, want it to start %q", run.stderr, want)
			}
			if run.stdout != "" {
				t.Errorf("stdout = %q on a refusal, want nothing", run.stdout)
			}
		})
	}
}

// TestOrder_AMissingTagArgumentIsAUsageError covers the arguments: exactly
// one tag, which is not empty, or exit 2 and nothing on stdout.
func TestOrder_AMissingTagArgumentIsAUsageError(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "no tag", args: nil},
		{name: "an empty tag", args: []string{""}},
		{name: "two tags", args: []string{"v3.2.0", "v3.1.0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			code := runMain(append([]string{"order"}, tc.args...), strings.NewReader(lsRemote), &stdout, &stderr)
			if code != 2 || !strings.HasPrefix(stderr.String(), "usage: release_checks order TAG") {
				t.Errorf("order %q = %d with %q on stderr, want 2 and the usage", tc.args, code, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Errorf("order %q wrote %q on stdout, want nothing", tc.args, stdout.String())
			}
		})
	}
}

// TestOrder_AListThatCannotBeReadAnswersNothing covers stdin failing partway:
// the check cannot say the release is the newest from half a list, so it
// fails and writes nothing on stdout, whatever the half it read says.
func TestOrder_AListThatCannotBeReadAnswersNothing(t *testing.T) {
	t.Parallel()

	half := io.MultiReader(strings.NewReader("v1.0.0\n"), iotest.ErrReader(errors.New("the pipe broke")))
	var stdout, stderr bytes.Buffer
	code := runMain([]string{"order", "v3.2.0"}, half, &stdout, &stderr)
	if code != 1 {
		t.Errorf("the order check exited %d on a list it could not read, want 1", code)
	}
	if want := "::error::the tags the repository holds could not be read: the pipe broke\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q on a list it could not read, want nothing", stdout.String())
	}
}
