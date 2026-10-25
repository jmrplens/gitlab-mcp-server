// repeat_test.go drives the offline check end to end: every release target
// of a tree built twice, the second time from an empty build cache, and the
// two builds compared.
package main

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestRun_BuildsEachTargetTwiceToTheSameBytes is the offline check doing its
// job on a tree that is not a repository, which is what a copy synced
// without .git is: the version comes from VERSION, the commit is "none" as
// GoReleaser renders it outside a repository, and both builds are kept where
// -out says and are the same bytes.
func TestRun_BuildsEachTargetTwiceToTheSameBytes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	files := maps.Clone(fixtureModule)
	files[".goreleaser.yml"] = hostConfig("")
	files["VERSION"] = "1.2.3\n"
	writeFiles(t, dir, files)
	out := t.TempDir()
	code, stdout, stderr := runRelease(options{twice: true, dir: dir, config: ".goreleaser.yml", out: out})
	if code != 0 {
		t.Fatalf("exit %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	for _, want := range []string{
		"built every release target of v1.2.3 at none twice",
		"the second time with an empty build cache",
		"repeated " + hostAsset() + " (" + hostTarget.String() + ")",
		"1 of 1 binaries built twice to the same bytes",
	} {
		t.Run(want, func(t *testing.T) {
			t.Parallel()

			if !strings.Contains(stdout, want) {
				t.Errorf("the report lacks %q:\n%s", want, stdout)
			}
		})
	}
	kept := func(which string) string {
		return filepath.Join(out, which, hostTarget.distDir("fixture"), "fixture"+hostTarget.ext())
	}
	first, errFirst := os.ReadFile(kept("first"))
	second, errSecond := os.ReadFile(kept("second"))
	if errFirst != nil || errSecond != nil || !bytes.Equal(first, second) {
		t.Errorf("the two builds are not kept under -out as the same bytes: %v, %v", errFirst, errSecond)
	}
}

// TestRun_TwiceRefusesWhatItCannotBuild covers every step that can stop the
// offline check: no version to stamp, no configuration, a template it cannot
// evaluate, and a build that fails, here in a repository so the commit is
// read from git on the way.
func TestRun_TwiceRefusesWhatItCannotBuild(t *testing.T) {
	t.Parallel()

	tree := func(config string, version bool) string {
		dir := t.TempDir()
		files := maps.Clone(fixtureModule)
		files[".goreleaser.yml"] = config
		if version {
			files["VERSION"] = "1.2.3\n"
		}
		writeFiles(t, dir, files)
		return dir
	}
	broken, _ := releaseRepo(t, strings.Replace(hostConfig(""), "main: .", "main: ./absent", 1), nil)
	for _, tc := range []struct {
		name string
		dir  string
		want string
	}{
		{name: "no version", dir: tree(hostConfig(""), false), want: "no -tag given"},
		{name: "no configuration", dir: t.TempDir(), want: ".goreleaser.yml"},
		{name: "an archive name it cannot evaluate", dir: tree(strings.Replace(hostConfig(""), `"{{ .Binary }}-{{ .Os }}-{{ .Arch }}"`, `"{{ .Nope }}"`, 1), true), want: "archive name_template"},
		{name: "a main package that does not build", dir: broken, want: "building " + hostAsset()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			o := options{twice: true, dir: tc.dir, config: ".goreleaser.yml"}
			if tc.name != "no version" {
				o.tag = "v1.2.3"
			}
			code, stdout, stderr := runRelease(o)
			if code != 1 || stdout != "" || !strings.Contains(stderr, tc.want) {
				t.Errorf("exit %d, want 1, no report and %q on stderr\nstdout:\n%s\nstderr:\n%s", code, tc.want, stdout, stderr)
			}
		})
	}
}

// TestTagOf_TheFlagOrTheVersionFile covers where the offline check takes the
// tag it stamps: -tag when given, and v followed by VERSION otherwise.
func TestTagOf_TheFlagOrTheVersionFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"VERSION": " 4.5.6\n"})
	for _, tc := range []struct {
		name, tag, want string
	}{
		{name: "given", tag: "v9.9.9", want: "v9.9.9"},
		{name: "from VERSION", want: "v4.5.6"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got, err := tagOf(dir, tc.tag); err != nil || got != tc.want {
				t.Errorf("tagOf = %q, %v, want %q", got, err, tc.want)
			}
		})
	}
}

// TestFatNote_NamesWhatIsNotBuiltTwice covers the note on the universal
// binaries, which the offline check does not build since only a release's own
// header says in which order their slices go.
func TestFatNote_NamesWhatIsNotBuiltTwice(t *testing.T) {
	t.Parallel()

	if got := fatNote(nil); got != nil {
		t.Errorf("fatNote of no universal binary = %q, want none", got)
	}
	got := fatNote([]fatRecipe{{asset: "a-darwin-all"}, {asset: "b-darwin-all"}})
	if len(got) != 1 || !strings.Contains(got[0], "a-darwin-all, b-darwin-all") || !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "compared against a release only") }) {
		t.Errorf("fatNote = %q", got)
	}
}
