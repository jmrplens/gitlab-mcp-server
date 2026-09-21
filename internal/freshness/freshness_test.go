// freshness_test.go verifies the harness switch in both directions, and holds
// its two ends to the workflow that drives it.
//
// Both halves are load-bearing and neither substitutes for the other. If the
// deferral stopped working, the tests that compare a committed artifact would
// fail on every layer of a stack below its top, which is the cost this package
// exists to remove. If it started applying when nothing asked for it, those
// same comparisons would be skipped where the artifact lands, and a stale
// artifact would reach main unnoticed. So every value the variable can carry
// is asserted, not just the one that defers.
//
// Neither half says anything about the strings themselves, though, and both
// ends of this switch are strings agreed with a file in another language: CI
// writes a name and a value, and this package reads them. So the last two
// tests read .github/workflows/ci.yml and hold the agreement, because a rename
// on one side alone leaves a green run whose deferral does nothing.
package freshness

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestDeferred_OnlyTheDeferredValueDefers verifies exactly one spelling of the
// variable defers and every other reading compares: the value CI passes when
// the artifacts are checked, an unset variable (a developer's machine, and the
// jobs that deliberately set nothing), and the near misses a hand-typed value
// produces.
func TestDeferred_OnlyTheDeferredValueDefers(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "deferred", value: "deferred", want: true},
		{name: "checked", value: "checked", want: false},
		{name: "empty", value: "", want: false},
		{name: "capitalized", value: "Deferred", want: false},
		{name: "padded", value: " deferred ", want: false},
		{name: "truthy", value: "true", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(EnvVar, tt.value)
			if got := Deferred(); got != tt.want {
				t.Errorf("Deferred() with %s=%q = %t, want %t", EnvVar, tt.value, got, tt.want)
			}
		})
	}

	t.Run("unset", func(t *testing.T) {
		// t.Setenv first so its cleanup restores whatever the process had,
		// however the variable is left below.
		t.Setenv(EnvVar, "deferred")
		if err := os.Unsetenv(EnvVar); err != nil {
			t.Fatalf("unset %s: %v", EnvVar, err)
		}
		if Deferred() {
			t.Errorf("Deferred() with %s unset = true, want false: an unset variable must compare", EnvVar)
		}
	})
}

// TestSkipIfDeferred_SkipsOnlyWhenDeferred verifies the one call the tests
// make behaves both ways: it skips when the harness deferred, and it returns
// so the comparison runs when it did not. The skip is observed through
// [testing.T.Skipped] from a deferred function, because [testing.T.Skip] ends
// the subtest's goroutine and nothing after the call would run.
func TestSkipIfDeferred_SkipsOnlyWhenDeferred(t *testing.T) {
	var skipped bool
	t.Run("deferred", func(t *testing.T) {
		t.Setenv(EnvVar, "deferred")
		defer func() { skipped = t.Skipped() }()
		SkipIfDeferred(t)
	})
	if !skipped {
		t.Errorf("SkipIfDeferred() ran the comparison the harness deferred through %s", EnvVar)
	}

	skipped = true
	t.Run("checked", func(t *testing.T) {
		t.Setenv(EnvVar, "checked")
		defer func() { skipped = t.Skipped() }()
		SkipIfDeferred(t)
	})
	if skipped {
		t.Errorf("SkipIfDeferred() skipped a comparison %s=checked asked for", EnvVar)
	}
}

// recordingTB stands in for the [testing.T] a deferred test hands
// [SkipIfDeferred], so what that call passes the testing package can be read
// back. It embeds [testing.TB] because that interface carries an unexported
// method and cannot be implemented from outside the testing package; every
// method the function under test calls is overridden here, so the embedded
// value answers nothing and no real test is skipped.
type recordingTB struct {
	testing.TB
	helpers     int
	skips       int
	skipMessage string
}

func (r *recordingTB) Helper()          { r.helpers++ }
func (r *recordingTB) Skip(args ...any) { r.skips++; r.skipMessage = fmt.Sprint(args...) }

func (r *recordingTB) Skipf(format string, args ...any) {
	r.skips++
	r.skipMessage = fmt.Sprintf(format, args...)
}
func (r *recordingTB) SkipNow() { r.skips++ }

// TestSkipIfDeferred_TheSkipCarriesTheReason verifies the skip a deferred run
// makes names [SkipReason], and that a run which was not deferred skips
// nothing. Observing the skip through [testing.T.Skipped] cannot tell those
// apart from a bare SkipNow, so the whole point of the constant, that a reader
// of the log sees a decision rather than a gap, held on nothing: a
// SkipIfDeferred that dropped the message passed every test in this file.
func TestSkipIfDeferred_TheSkipCarriesTheReason(t *testing.T) {
	t.Run("deferred", func(t *testing.T) {
		t.Setenv(EnvVar, deferredValue)
		rec := &recordingTB{TB: t}
		SkipIfDeferred(rec)
		if rec.skips != 1 {
			t.Errorf("SkipIfDeferred() skipped %d times with %s=%q, want 1", rec.skips, EnvVar, deferredValue)
		}
		if rec.skipMessage != SkipReason {
			t.Errorf("SkipIfDeferred() skipped with %q, want SkipReason %q", rec.skipMessage, SkipReason)
		}
		if rec.helpers == 0 {
			t.Errorf("SkipIfDeferred() called no Helper(), so the skip is attributed to this package rather than to the deferred test")
		}
	})

	t.Run("checked", func(t *testing.T) {
		t.Setenv(EnvVar, "checked")
		rec := &recordingTB{TB: t}
		SkipIfDeferred(rec)
		if rec.skips != 0 {
			t.Errorf("SkipIfDeferred() skipped with %s=checked, reporting %q", EnvVar, rec.skipMessage)
		}
	})
}

// TestEnvVar_IsTheNameCIWrites verifies the spelling this package reads is the
// one ci.yml hands the unit suite. Nothing else held it: every test here sets
// the variable through [EnvVar] itself, and TestSkipReason_NamesTheSettingAndTheValue
// only asks that [SkipReason] contain the name, which a truncated one still
// does. A rename on either side alone leaves CI setting a variable nothing
// reads, so every layer of a stack compares artifacts its top refreshes and
// the run stays green while the deferral it advertises does nothing.
func TestEnvVar_IsTheNameCIWrites(t *testing.T) {
	const handoff = "${{ env.FRESHNESS }}"
	// Keyed by where it was written, because both handoffs name the same
	// variable and a subtest per name would collide.
	handoffs := map[string]string{}
	for n, line := range strings.Split(readCIWorkflow(t), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || !strings.HasSuffix(trimmed, handoff) {
			continue
		}
		if name, _, ok := strings.Cut(trimmed, ":"); ok {
			handoffs[fmt.Sprintf("ci.yml:%d", n+1)] = name
		}
	}
	if len(handoffs) == 0 {
		t.Fatalf("ci.yml hands %s to no environment variable, so the unit suite is never told whether to compare", handoff)
	}
	for where, name := range handoffs {
		t.Run(where, func(t *testing.T) {
			if name != EnvVar {
				t.Errorf("%s hands %s to %q, but this package reads %q", where, handoff, name, EnvVar)
			}
		})
	}
}

// TestDeferredValue_IsTheValueCIComputes verifies the one spelling that defers
// here is one the FRESHNESS expression can produce. The value is a literal in
// a YAML expression and a constant in Go, agreed between them and checked by
// nothing: were CI to compute another word, every stacked layer would compare
// the artifacts its top refreshes, which is the cost this package removes.
func TestDeferredValue_IsTheValueCIComputes(t *testing.T) {
	var definition string
	for line := range strings.SplitSeq(readCIWorkflow(t), "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "FRESHNESS:") {
			definition = trimmed
			break
		}
	}
	if definition == "" {
		t.Fatalf("ci.yml defines no FRESHNESS, so %s is handed a value from nowhere", EnvVar)
	}
	if want := "'" + deferredValue + "'"; !strings.Contains(definition, want) {
		t.Errorf("ci.yml computes FRESHNESS as %s, which never yields %s: this package would defer for a value CI cannot produce", definition, want)
	}
}

// TestDoc_NamesEveryPackageThatDefers verifies the package comment's account of
// who defers is the tree's, in both directions. That list is the only statement
// anywhere of what deferring reaches, so a reader sizes the blast radius by it,
// and it is prose about other packages: no compiler, no gate and neither
// coverage tool can be wrong about it, which is how it came to name four while
// six called.
func TestDoc_NamesEveryPackageThatDefers(t *testing.T) {
	callers := packagesCallingFreshness(t)
	named := packagesNamedInDoc(t)

	for caller := range callers {
		t.Run("calls/"+caller, func(t *testing.T) {
			if !named[caller] {
				t.Errorf("%s defers through this package and doc.go does not name it", caller)
			}
		})
	}
	for pkg := range named {
		t.Run("named/"+pkg, func(t *testing.T) {
			if !callers[pkg] {
				t.Errorf("doc.go names %s as a package that defers, and it imports nothing from here", pkg)
			}
		})
	}
}

// packagesCallingFreshness asks the toolchain which packages of this module
// import this one, from a test file or otherwise. It is asked rather than
// scanned because a scan of the tree would read the hundred-odd worktrees the
// parallel-agent tooling keeps under .claude and fold another branch's callers
// into the answer. Paths are returned the way this repository writes them,
// with the module prefix off.
func packagesCallingFreshness(t *testing.T) map[string]bool {
	t.Helper()
	const format = `{{.ImportPath}}{{range .Imports}} {{.}}{{end}}{{range .TestImports}} {{.}}{{end}}{{range .XTestImports}} {{.}}{{end}}`
	list := exec.CommandContext(t.Context(), "go", "list", "-f", format, "./cmd/...", "./internal/...")
	// From the module root, because a test runs in its own package directory
	// and the patterns are relative.
	list.Dir = moduleRoot(t)
	var stderr strings.Builder
	list.Stderr = &stderr
	listed, err := list.Output()
	if err != nil {
		t.Fatalf("go list in %s: %v: %s", list.Dir, err, stderr.String())
	}

	const self = "internal/freshness"
	var prefix string
	rows := strings.Split(string(listed), "\n")
	for _, row := range rows {
		if pkg, _, _ := strings.Cut(strings.TrimSpace(row), " "); strings.HasSuffix(pkg, "/"+self) {
			prefix = strings.TrimSuffix(pkg, self)
			break
		}
	}
	if prefix == "" {
		t.Fatalf("go list named %d row(s) and not %s, so it is not listing this module", len(rows), self)
	}

	callers := map[string]bool{}
	for _, row := range rows {
		fields := strings.Fields(row)
		if len(fields) == 0 || fields[0] == prefix+self {
			continue
		}
		if slices.Contains(fields[1:], prefix+self) {
			callers[strings.TrimPrefix(fields[0], prefix)] = true
		}
	}
	return callers
}

// packagesNamedInDoc reads the package comment's list back. The list is a
// godoc bullet list rather than a sentence so that what it claims can be read
// without parsing prose, and a comment naming a package anywhere else, such as
// the cmd/server test named below it, is not mistaken for a member.
func packagesNamedInDoc(t *testing.T) map[string]bool {
	t.Helper()
	source, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatalf("read doc.go: %v", err)
	}
	named := map[string]bool{}
	for line := range strings.SplitSeq(string(source), "\n") {
		if pkg, ok := strings.CutPrefix(line, "//   - "); ok {
			named[strings.TrimSpace(pkg)] = true
		}
	}
	if len(named) == 0 {
		t.Fatalf("doc.go lists no package that defers, so its account of what deferring reaches is gone")
	}
	return named
}

// readCIWorkflow returns the workflow that computes FRESHNESS and hands it to
// the unit suite.
func readCIWorkflow(t *testing.T) string {
	t.Helper()
	path := filepath.Join(moduleRoot(t), ".github", "workflows", "ci.yml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

// moduleRoot walks up from the package directory to the go.mod above it,
// rather than counting the ".." a fixed relative path would need, so a test
// that cannot find what it judges says so instead of reading nothing.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s, so the module root could not be found", dir)
			return ""
		}
		dir = parent
	}
}

// TestSkipReason_NamesTheSettingAndTheValue verifies the message a skipped run
// prints identifies the decision rather than only announcing one: the variable
// that made it, the value it carried, and where the comparison does happen.
func TestSkipReason_NamesTheSettingAndTheValue(t *testing.T) {
	for _, want := range []string{EnvVar, deferredValue, "top of a stack", "push to main"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(SkipReason, want) {
				t.Errorf("SkipReason = %q, want it to mention %q", SkipReason, want)
			}
		})
	}
}
