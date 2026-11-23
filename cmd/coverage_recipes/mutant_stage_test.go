// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// mutantsFixture is the fixture module of the recorded runs.
func mutantsFixture(t *testing.T) map[string]string {
	t.Helper()
	var rec mutantsRecord
	readRecord(t, "mutants_runs.json", &rec)
	return rec.Files
}

// interruptible is a context a test ends as a signal would, and what ends it.
func interruptible(t *testing.T) (ctx context.Context, interrupt func(os.Signal)) {
	t.Helper()
	ctx, cancel := context.WithCancelCause(t.Context())
	t.Cleanup(func() { cancel(nil) })
	return ctx, func(sig os.Signal) { cancel(recipe.InterruptedError{Signal: sig}) }
}

// windowsListing wraps a bench's runner so go list prints its directories as
// Windows does, with backslashes below the module's root.
func windowsListing(b *mutantsBench) {
	answer := b.session.Run
	b.session.Run = func(ctx context.Context, call recipe.Call) (int, error) {
		if call.Args[0] != "list" || slices.Contains(call.Args, "-m") {
			return answer(ctx, call)
		}
		out, err := recipe.Output(ctx, answer, call)
		if err != nil {
			return 0, err
		}
		for line := range strings.Lines(out) {
			dir, rest, _ := strings.Cut(line, "\t")
			if below, ok := strings.CutPrefix(filepath.ToSlash(dir), filepath.ToSlash(b.root)); ok {
				dir = b.root + strings.ReplaceAll(below, "/", `\`)
			}
			if rest != "" {
				dir += "\t" + rest
			}
			if _, err = call.Stdout.Write([]byte(dir)); err != nil {
				return 0, err
			}
		}
		return 0, nil
	}
}

// TestStage_AWindowsListing_FindsThePackageAndWhatIsBelowIt verifies that go
// list's native paths, which carry backslashes on Windows, are cut against the
// root all the same: the patterns, the staged copy and gremlins' target are
// written with slashes, and the packages below the command are left out of the
// copy rather than copied whole.
func TestStage_AWindowsListing_FindsThePackageAndWhatIsBelowIt(t *testing.T) {
	files := mutantsFixture(t)
	t.Run("where it is", func(t *testing.T) {
		b := newMutantsBench(t, files)
		windowsListing(b)
		if status := b.session.run(t.Context(), []string{"mutants", "./internal/pkg"}); status != 0 {
			t.Fatalf("status = %d, stderr %q", status, b.stderr.String())
		}
		var patterns []string
		for _, call := range b.calls {
			if call.Args[0] == "test" {
				patterns = append(patterns, call.Args[len(call.Args)-1])
				if len(call.Tree) == 0 {
					t.Errorf("go test %q found no file", call.Args)
				}
			}
		}
		if want := []string{"./internal/pkg/...", "./internal/pkg", "./internal/pkg/..."}; !slices.Equal(patterns, want) {
			t.Errorf("patterns = %q, want %q", patterns, want)
		}
	})
	t.Run("a staged copy below it", func(t *testing.T) {
		b := newMutantsBench(t, files)
		writeFile(t, b.root, "cmd/tool/imp.go", "package main\n\nimport _ \"example.com/m/cmd/tool/internal/derive\"\n")
		writeFile(t, b.root, "cmd/tool/internal/derive/derive.go", "package derive\n")
		writeFile(t, b.root, "cmd/tool/internal/NOTES.md", "")
		windowsListing(b)
		if status := b.session.run(t.Context(), []string{"mutants", "./cmd/tool"}); status != 0 {
			t.Fatalf("status = %d, stderr %q", status, b.stderr.String())
		}
		last := b.calls[len(b.calls)-1]
		if got, want := last.Args[len(last.Args)-1], "./cmd/tool/tool.mutants-main"; got != want {
			t.Errorf("gremlins was run over %q, want %q", got, want)
		}
		if want := []string{"doc.go", "imp.go", "internal/NOTES.md", "tool_test.go"}; !slices.Equal(last.Tree, want) {
			t.Errorf("the copy held %q, want %q", last.Tree, want)
		}
	})
}

// TestMutants_AnInterrupt_EndsTheRunAndRemovesTheCopy verifies that a signal
// that arrives while a program runs lets it finish, starts nothing after it,
// ends the run with the status a shell gives the signal and removes the staged
// copy, beside the package or below it, leaving the package as it was: a
// handler that cleaned up and returned once went on to start gremlins on a
// directory that was gone, and let a signal during gremlins exit 0.
func TestMutants_AnInterrupt_EndsTheRunAndRemovesTheCopy(t *testing.T) {
	files := mutantsFixture(t)
	cases := []struct {
		name   string
		signal os.Signal
		during string
		below  bool
		want   int
	}{
		{name: "SIGTERM during the baseline", signal: syscall.SIGTERM, during: "test", want: 143},
		{name: "SIGINT during gremlins", signal: os.Interrupt, during: "run", want: 130},
		{name: "SIGTERM during the baseline of a copy below", signal: syscall.SIGTERM, during: "test", below: true, want: 143},
		{name: "SIGINT during gremlins on a copy below", signal: os.Interrupt, during: "run", below: true, want: 130},
		{name: "SIGINT during the download", signal: os.Interrupt, during: "mod", want: 130},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newMutantsBench(t, files)
			copied := "cmd/tool.mutants-main"
			if tc.below {
				copied = "cmd/tool/tool.mutants-main"
				writeFile(t, b.root, "cmd/tool/imp.go", "package main\n\nimport _ \"example.com/m/cmd/tool/internal/derive\"\n")
				writeFile(t, b.root, "cmd/tool/internal/derive/derive.go", "package derive\n")
			}
			before := fileTree(b.root)
			ctx, interrupt := interruptible(t)
			b.onCall = func(verb string) {
				if verb != tc.during {
					return
				}
				if _, err := os.Stat(filepath.Join(b.root, filepath.FromSlash(copied))); err != nil {
					t.Errorf("no copy while %s ran: %v", verb, err)
				}
				interrupt(tc.signal)
			}
			if status := b.session.run(ctx, []string{"mutants", "./cmd/tool"}); status != tc.want {
				t.Errorf("status = %d, want %d", status, tc.want)
			}
			if last := b.calls[len(b.calls)-1]; last.Args[0] != tc.during {
				t.Errorf("%q was started after the signal", last.Args)
			}
			if b.stderr.Len() != 0 {
				t.Errorf("stderr = %q, want no word of a failure", b.stderr.String())
			}
			if after := fileTree(b.root); !slices.Equal(after, before) {
				t.Errorf("the module holds\n%q\nafter the run, want\n%q", after, before)
			}
		})
	}
}

// TestStage_ACopyThatCannotBeMade_IsRefused verifies that a package whose
// subtree go list cannot name, and one that cannot be copied, are refused with
// the reason and no copy left.
func TestStage_ACopyThatCannotBeMade_IsRefused(t *testing.T) {
	files := mutantsFixture(t)
	planted := errors.New("planted")
	t.Run("the subtree cannot be listed", func(t *testing.T) {
		b := newMutantsBench(t, files)
		refuseCall(b, "", 0, planted, "{{.Dir}}", "./...")
		if status := b.session.run(t.Context(), []string{"mutants", "./cmd/tool"}); status != 1 || b.stderr.String() != "gremlins: go list could not name the packages below ./cmd/tool: planted; refusing to measure\n" {
			t.Errorf("status = %d, stderr = %q", status, b.stderr.String())
		}
	})
	t.Run("the package cannot be copied", func(t *testing.T) {
		b := newMutantsBench(t, files)
		m := &mutation{session: b.session, name: "./cmd/gone", environ: b.session.environ, root: filepath.ToSlash(b.root)}
		m.pkg = golist.PackageFiles{Dir: m.root + "/cmd/gone", Name: "main", ImportPath: "example.com/m/cmd/gone"}
		// The subtree is listed where the bench answers for it, since the
		// directory is not there for a go command to run in.
		m.Run = func(context.Context, recipe.Call) (int, error) { return 0, nil }
		cleanup, err := m.stage(t.Context())
		if err == nil || !strings.HasPrefix(err.Error(), "the staged copy of ./cmd/gone could not be made: ") {
			t.Errorf("stage() = %v, want why the copy could not be made", err)
		}
		cleanup()
		if _, statErr := os.Stat(filepath.Join(b.root, "cmd", "gone.mutants-main")); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("the copy was left behind (%v)", statErr)
		}
	})
}

// TestImportsOwnInternal_EachImport_IsReadWhole verifies which imports are of
// a package's own internal subtree: the subtree's root or a package of it, by
// any of the three kinds of files, and not a longer element, another
// package's, or a path that only ends in it.
func TestImportsOwnInternal_EachImport_IsReadWhole(t *testing.T) {
	t.Parallel()
	const own = "example.com/m/cmd/tool"
	cases := []struct {
		name string
		pkg  golist.PackageFiles
		want bool
	}{
		{"none", golist.PackageFiles{Imports: []string{"fmt"}}, false},
		{"its own files", golist.PackageFiles{Imports: []string{"fmt", own + "/internal/derive"}}, true},
		{"its tests", golist.PackageFiles{TestImports: []string{own + "/internal/join"}}, true},
		{"its external tests", golist.PackageFiles{XTestImports: []string{own + "/internal/join"}}, true},
		{"the subtree's root", golist.PackageFiles{Imports: []string{own + "/internal"}}, true},
		{"a longer element", golist.PackageFiles{Imports: []string{own + "/internalx"}}, false},
		{"another package's", golist.PackageFiles{Imports: []string{"example.com/m/cmd/internal/shared"}}, false},
		{"a path that only ends in it", golist.PackageFiles{Imports: []string{"example.org/fork/" + own + "/internal/derive"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.pkg.ImportPath = own
			if got := importsOwnInternal(tc.pkg); got != tc.want {
				t.Errorf("importsOwnInternal(%+v) = %t, want %t", tc.pkg, got, tc.want)
			}
		})
	}
}
