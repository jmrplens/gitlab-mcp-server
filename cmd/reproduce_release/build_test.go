// build_test.go covers how one recipe is built: where the binary is written,
// the environment the go command sees, and the refusal when it cannot build.
package main

import (
	"bytes"
	"context"
	"debug/buildinfo"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// hostTarget is the platform the tests run on. Building for it needs nothing
// the toolchain has not already compiled for the test binary itself, which
// keeps these builds to a second or two.
var hostTarget = target{goos: runtime.GOOS, goarch: runtime.GOARCH}

// fixtureModule are the files of a module with one main package that uses
// the two strings the release stamps, so a different stamp makes a different
// binary. The attributes keep its checkout byte for byte what was committed
// on every platform, as the repository's own .gitattributes does.
var fixtureModule = map[string]string{
	"go.mod":         "module example.com/fixture\n\ngo 1.27\n",
	"main.go":        "package main\n\nvar version, commit string\n\nfunc main() { println(version, commit) }\n",
	".gitattributes": "* text=auto eol=lf\n",
}

// writeFiles writes files, named by slash-separated paths, under dir.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("creating the directory of %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
}

// hostRecipe is a recipe building the fixture module's main package for the
// host with flags, the way the fixture configuration's build would.
func hostRecipe(flags ...string) recipe {
	return recipe{
		build:  "fixture",
		target: hostTarget,
		asset:  "fixture-" + hostTarget.goos + "-" + hostTarget.goarch + hostTarget.ext(),
		dir:    hostTarget.distDir("fixture"),
		file:   "fixture" + hostTarget.ext(),
		flags:  flags,
		main:   ".",
		env:    []string{"CGO_ENABLED=0"},
	}
}

// buildSetting returns the value a binary's build information records under
// key, or "" when it records none.
func buildSetting(t *testing.T, data []byte, key string) string {
	t.Helper()

	info, err := buildinfo.Read(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("reading the build information: %v", err)
	}
	return setting(info, key)
}

// TestGoBuild_WritesTheBinaryWhereGoReleaserDoes covers a build end to end:
// the binary lands at GoReleaser's own path for the target, what it returns
// is that file, the recipe's flags and env reached the go command, the
// recipe's env won over the caller's, and the caller's GOFLAGS was cleared,
// since a flag it carried would be a build the release never ran.
func TestGoBuild_WritesTheBinaryWhereGoReleaserDoes(t *testing.T) {
	t.Setenv("CGO_ENABLED", "1")
	t.Setenv("GOFLAGS", "-tags=fromthecaller")

	src := t.TempDir()
	writeFiles(t, src, fixtureModule)
	root := t.TempDir()
	data, err := goBuild(context.Background(), src, hostRecipe("-trimpath"), root)
	if err != nil {
		t.Fatalf("goBuild: %v", err)
	}
	onDisk, err := os.ReadFile(filepath.Join(root, hostTarget.distDir("fixture"), "fixture"+hostTarget.ext()))
	if err != nil || !bytes.Equal(onDisk, data) {
		t.Fatalf("the binary is not at GoReleaser's path, or is not what goBuild returned: %v", err)
	}
	for key, want := range map[string]string{
		"-trimpath":   "true",
		"-tags":       "",
		"CGO_ENABLED": "0",
		"GOOS":        hostTarget.goos,
		"GOARCH":      hostTarget.goarch,
	} {
		t.Run(key, func(t *testing.T) {
			if got := buildSetting(t, data, key); got != want {
				t.Errorf("the binary records %s=%q, want %q", key, got, want)
			}
		})
	}
}

// TestGoBuild_ABuildThatFails_NamesTheAssetAndTheTarget covers the go
// command failing, whose output is part of the error.
func TestGoBuild_ABuildThatFails_NamesTheAssetAndTheTarget(t *testing.T) {
	t.Parallel()

	src := t.TempDir()
	writeFiles(t, src, fixtureModule)
	rc := hostRecipe()
	rc.main = "./absent"
	_, err := goBuild(context.Background(), src, rc, t.TempDir())
	if err == nil {
		t.Fatal("goBuild built a main package that does not exist")
	}
	for _, want := range []string{"building " + rc.asset + " for " + hostTarget.String(), "absent"} {
		t.Run(want, func(t *testing.T) {
			t.Parallel()

			if !strings.Contains(err.Error(), want) {
				t.Errorf("the error %q does not contain %q", err, want)
			}
		})
	}
}

// TestGoBuild_NoDirectoryForTheBinary covers the output directory not being
// creatable, here because a file stands where it would go.
func TestGoBuild_NoDirectoryForTheBinary(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(root, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := goBuild(context.Background(), t.TempDir(), hostRecipe(), root); err == nil {
		t.Error("goBuild wrote a binary under a file")
	}
}

// TestBuildEnv_LaterEntriesWin covers the order of the environment, which is
// what decides a variable set twice: the caller's, then GOFLAGS cleared, then
// the build's env, then the target's, then what the run adds (the toolchain
// or the build cache), which is GoReleaser's order with the two ends this
// command adds.
func TestBuildEnv_LaterEntriesWin(t *testing.T) {
	t.Parallel()

	rc := recipe{target: target{goos: "linux", goarch: "amd64"}, env: []string{"CGO_ENABLED=0"}}
	got := buildEnv([]string{"GOFLAGS=-tags=x", "HOME=/home/a"}, rc, []string{"GOTOOLCHAIN=go1.27.3"})
	want := append(append([]string{"GOFLAGS=-tags=x", "HOME=/home/a", "GOFLAGS=", "CGO_ENABLED=0"}, rc.target.env()...), "GOTOOLCHAIN=go1.27.3")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildEnv =\n%q\nwant\n%q", got, want)
	}
}
