// build_test.go covers how each release target is built: from the entry's
// main package, with its env and flags, for the target's platform.
package main

import (
	"context"
	"debug/buildinfo"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// hostTarget is the platform the tests run on. Building for it needs nothing
// the toolchain has not already compiled for the test binary itself, which is
// what keeps these builds to a second or two.
var hostTarget = target{goos: runtime.GOOS, goarch: runtime.GOARCH}

// writeFixtureModule writes a module with one main package and no
// dependencies, and returns its directory.
func writeFixtureModule(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	files := map[string]string{
		"go.mod":  "module example.com/fixture\n\ngo 1.27\n",
		"main.go": "package main\n\nfunc main() {}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	return dir
}

// buildSetting returns the value a binary's build information records under
// key, or "" when it records none.
func buildSetting(t *testing.T, path, key string) string {
	t.Helper()

	info, err := buildinfo.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the build information of %s: %v", path, err)
	}
	for _, s := range info.Settings {
		if s.Key == key {
			return s.Value
		}
	}
	return ""
}

// TestBuildAll_BuildsEachTargetAsTheEntryDescribes covers the three things an
// entry decides about a build, read back out of the binary it produced: the
// flags reached the go command, the entry's env reached it and won over the
// caller's, and the target's platform is the one built for.
func TestBuildAll_BuildsEachTargetAsTheEntryDescribes(t *testing.T) {
	// The caller's shell says the opposite of the entry, which is what shows
	// the entry wins rather than that nothing set it.
	t.Setenv("CGO_ENABLED", "1")

	dir := writeFixtureModule(t)
	out := t.TempDir()
	built, err := buildAll(context.Background(), dir, []build{{
		id:      "fixture",
		main:    ".",
		env:     []string{"CGO_ENABLED=0"},
		flags:   []string{"-trimpath"},
		targets: []target{hostTarget},
	}}, out)
	if err != nil {
		t.Fatalf("buildAll: %v", err)
	}
	if len(built) != 1 {
		t.Fatalf("built %d binaries, want 1", len(built))
	}
	bin := built[0]
	if bin.build != "fixture" || bin.target != hostTarget || filepath.Dir(bin.path) != out {
		t.Errorf("binary = %+v, want the fixture for %s under %s", bin, hostTarget, out)
	}
	for key, want := range map[string]string{
		"-trimpath":   "true",
		"CGO_ENABLED": "0",
		"GOOS":        hostTarget.goos,
		"GOARCH":      hostTarget.goarch,
	} {
		t.Run(key, func(t *testing.T) {
			if got := buildSetting(t, bin.path, key); got != want {
				t.Errorf("the binary records %s=%q, want %q", key, got, want)
			}
		})
	}
}

// TestBuildAll_ABuildThatFails_NamesTheEntryAndTheTarget covers the refusal,
// which has to say which of the release's binaries could not be built and why,
// since the gate stops there.
func TestBuildAll_ABuildThatFails_NamesTheEntryAndTheTarget(t *testing.T) {
	t.Parallel()

	_, err := buildAll(context.Background(), writeFixtureModule(t), []build{{
		id:      "fixture",
		main:    "./absent",
		targets: []target{hostTarget},
	}}, t.TempDir())
	if err == nil {
		t.Fatal("buildAll built a main package that does not exist")
	}
	for _, want := range []string{"building fixture for " + hostTarget.String(), "absent"} {
		t.Run(want, func(t *testing.T) {
			t.Parallel()

			if !strings.Contains(err.Error(), want) {
				t.Errorf("the error %q does not contain %q", err, want)
			}
		})
	}
}
