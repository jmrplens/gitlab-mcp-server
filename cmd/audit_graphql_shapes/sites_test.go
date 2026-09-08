package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadProgram_TreeThatCannotBeWalked_IsRefused verifies the two refusals
// the loader has beside a directory that is not a module: a package that does
// not type-check, which would fold no constant and type no call, and a pattern
// that matches nothing, which is the shape a mistyped pattern produces and
// must not become a clean run over nothing.
func TestLoadProgram_TreeThatCannotBeWalked_IsRefused(t *testing.T) {
	root := fixtureModule(t, map[string]string{"bad": "package bad\n\nfunc use() { undefinedHelper() }\n"})
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o750); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	cases := []struct {
		name     string
		patterns []string
		want     string
	}{
		{name: "a package that does not type-check", patterns: []string{"./bad/..."}, want: "load fixture/bad: "},
		{name: "a pattern matching nothing", patterns: []string{"./empty/..."}, want: "no packages matched ./empty/..."},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			loaded, err := loadProgram(root, testCase.patterns)

			if err == nil {
				t.Fatalf("loadProgram() error = nil, want one naming %q", testCase.want)
			}
			if loaded != nil {
				t.Errorf("loadProgram() = %v, want nil on failure", loaded)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("loadProgram() error = %q, want it to name %q", err, testCase.want)
			}
		})
	}
}
