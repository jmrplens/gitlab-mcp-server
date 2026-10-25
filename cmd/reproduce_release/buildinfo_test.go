// buildinfo_test.go covers what the build information of a published binary
// is held to before it is rebuilt, and how two binaries that differ are told
// apart.
package main

import (
	"os"
	"reflect"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
)

// TestSetting_ReadsOneKey covers a key that is there and one that is not.
func TestSetting_ReadsOneKey(t *testing.T) {
	t.Parallel()

	info := &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "GOOS", Value: "linux"}}}
	if got := setting(info, "GOOS"); got != "linux" {
		t.Errorf("setting(GOOS) = %q", got)
	}
	if got := setting(info, "GOARCH"); got != "" {
		t.Errorf("setting(GOARCH) = %q, want empty", got)
	}
}

// TestProvenance_HoldsThePublishedBinaryToTheCommit covers what a published
// binary has to say before a rebuild can mean anything: that it was built
// from the commit being rebuilt, from a tree nobody had changed, by a release
// toolchain GOTOOLCHAIN can name.
func TestProvenance_HoldsThePublishedBinaryToTheCommit(t *testing.T) {
	t.Parallel()

	built := func(goVersion string, pairs ...[2]string) *debug.BuildInfo {
		info := &debug.BuildInfo{GoVersion: goVersion}
		for _, pair := range pairs {
			info.Settings = append(info.Settings, debug.BuildSetting{Key: pair[0], Value: pair[1]})
		}
		return info
	}
	revision := func(value string) [2]string { return [2]string{"vcs.revision", value} }
	modified := func(value string) [2]string { return [2]string{"vcs.modified", value} }
	toolchain, err := provenance(built("go1.27.2 X:nocoverageredesign", revision("abc"), modified("false")), "abc")
	if err != nil || toolchain != "go1.27.2" {
		t.Errorf("provenance = %q, %v, want go1.27.2", toolchain, err)
	}
	for _, tc := range []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{name: "no commit", info: built("go1.27.2", modified("false")), want: "records no commit"},
		{name: "another commit", info: built("go1.27.2", revision("def"), modified("false")), want: "was built from def, and the commit rebuilt is abc"},
		{name: "a modified tree", info: built("go1.27.2", revision("abc"), modified("true")), want: "vcs.modified=true"},
		{name: "no word on the tree", info: built("go1.27.2", revision("abc")), want: "vcs.modified="},
		{name: "a development toolchain", info: built("devel go1.28-abc", revision("abc"), modified("false")), want: `records the toolchain "devel go1.28-abc"`},
		{name: "no toolchain", info: built("", revision("abc"), modified("false")), want: `records the toolchain ""`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, refused := provenance(tc.info, "abc"); refused == nil || !strings.Contains(refused.Error(), tc.want) {
				t.Errorf("provenance = %v, want an error containing %q", refused, tc.want)
			}
		})
	}
}

// TestDifferences_NamesEachFieldThatDiffers covers every part of the build
// information two binaries can disagree on: the toolchain, the main package
// and module, a module only one links, a replaced module, and a setting one
// records and the other does not or records otherwise. What agrees is left
// out, and the lines come in a fixed order.
func TestDifferences_NamesEachFieldThatDiffers(t *testing.T) {
	t.Parallel()

	published := &debug.BuildInfo{
		GoVersion: "go1.27.2", Path: "example.com/m/cmd/x", Main: debug.Module{Path: "example.com/m", Version: "v1.2.3"},
		Deps: []*debug.Module{{Path: "a.example/x", Version: "v1.0.0", Sum: "h1:a"}, {Path: "b.example/y", Version: "v1.0.0", Sum: "h1:b"}},
		Settings: []debug.BuildSetting{
			{Key: "GOOS", Value: "linux"}, {Key: "vcs.revision", Value: "1111"},
		},
	}
	rebuilt := &debug.BuildInfo{
		GoVersion: "go1.27.3", Path: "example.com/m/cmd/x", Main: debug.Module{Path: "example.com/m", Version: "v1.2.4"},
		Deps: []*debug.Module{
			{Path: "a.example/x", Version: "v1.0.0", Sum: "h1:a"},
			{Path: "c.example/z", Version: "v0.1.0", Sum: "h1:c", Replace: &debug.Module{Path: "../z"}},
		},
		Settings: []debug.BuildSetting{
			{Key: "GOOS", Value: "linux"}, {Key: "vcs.revision", Value: "2222"}, {Key: "-tags", Value: "x"},
		},
	}
	want := []string{
		`go version: published "go1.27.2", rebuilt "go1.27.3"`,
		`main module: published "example.com/m@v1.2.3", rebuilt "example.com/m@v1.2.4"`,
		`module b.example/y: published "b.example/y@v1.0.0 h1:b", rebuilt ""`,
		`module c.example/z: published "", rebuilt "c.example/z@v0.1.0 h1:c => ../z"`,
		`setting -tags: published "", rebuilt "x"`,
		`setting vcs.revision: published "1111", rebuilt "2222"`,
	}
	if got := differences(published, rebuilt, againstRelease); !reflect.DeepEqual(got, want) {
		t.Errorf("differences =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	other := *rebuilt
	other.Path = "example.com/m/cmd/y"
	if got := differences(rebuilt, &other, againstItself); !reflect.DeepEqual(got, []string{`main package: first build "example.com/m/cmd/x", second build "example.com/m/cmd/y"`}) {
		t.Errorf("differences of the main package = %q", got)
	}
}

// TestExplain_SaysWhereTwoBinariesPart covers the three answers: two Go
// binaries whose build information agrees, which points at what it does not
// record, two whose information differs, and a file with none, where only
// the bytes can be compared. Each ends with where the bytes first differ.
func TestExplain_SaysWhereTwoBinariesPart(t *testing.T) {
	t.Parallel()

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	test, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	toolchain, err := os.ReadFile(golist.Executable())
	if err != nil {
		t.Fatal(err)
	}
	longer := append(append([]byte(nil), test...), 0)
	for _, tc := range []struct {
		name          string
		before, after []byte
		want, avoid   string
	}{
		{name: "the same build information", before: test, after: longer, want: "the build information is the same"},
		{name: "different build information", before: test, after: toolchain, want: "main package: published", avoid: "the build information is the same"},
		{name: "no build information before", before: []byte("not a binary"), after: test, avoid: "build information"},
		{name: "no build information after", before: test, after: []byte("not a binary"), avoid: "build information"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			notes := explain(tc.before, tc.after, againstRelease)
			text := strings.Join(notes, "\n")
			if !strings.Contains(notes[len(notes)-1], "the bytes first differ at offset") {
				t.Errorf("the last note is not where the bytes differ:\n%s", text)
			}
			if tc.want != "" && !strings.Contains(text, tc.want) {
				t.Errorf("the notes lack %q:\n%s", tc.want, text)
			}
			if tc.avoid != "" && strings.Contains(text, tc.avoid) {
				t.Errorf("the notes say %q:\n%s", tc.avoid, text)
			}
		})
	}
}

// TestFirstDifference_TheOffsetAndBothSizes covers a byte that differs, one
// file that is a prefix of the other, and two that agree, where the offset is
// the end of both.
func TestFirstDifference_TheOffsetAndBothSizes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		before, after, want string
	}{
		{before: "abcd", after: "abxd", want: "the bytes first differ at offset 2; the published file is 4 bytes and the rebuilt one 4"},
		{before: "abc", after: "abcd", want: "the bytes first differ at offset 3; the published file is 3 bytes and the rebuilt one 4"},
		{before: "ab", after: "ab", want: "the bytes first differ at offset 2; the published file is 2 bytes and the rebuilt one 2"},
	} {
		t.Run(tc.before+" "+tc.after, func(t *testing.T) {
			t.Parallel()

			if got := firstDifference([]byte(tc.before), []byte(tc.after), againstRelease); got != tc.want {
				t.Errorf("firstDifference = %q, want %q", got, tc.want)
			}
		})
	}
}
