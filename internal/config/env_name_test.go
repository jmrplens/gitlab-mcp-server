package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGetenv_EveryPrefixedName_ResolvesUnderThePrefixedNameAlone verifies both
// halves of the 3.1.0 contract for each variable individually rather than for
// a sample: the prefixed name is read, and the retired one is not.
//
// A spot check would pass while one name in the middle of the list was never
// wired, and that name's operator would find their setting silently ignored
// after an upgrade. The table is generated from the list itself, so a variable
// added to the list without being wired fails here rather than in someone's
// deployment.
//
// The second half is what this release is: a retired name left readable by one
// setting would be a shim nobody knew was still there, and the only place that
// could be noticed is here.
func TestGetenv_EveryPrefixedName_ResolvesUnderThePrefixedNameAlone(t *testing.T) {
	for _, name := range PrefixedEnvNames() {
		t.Run(name, func(t *testing.T) {
			t.Setenv(EnvPrefix+name, "from-prefixed")
			if got := Getenv(name); got != "from-prefixed" {
				t.Errorf("Getenv(%q) = %q with only the prefixed name set, want %q", name, got, "from-prefixed")
			}

			os.Unsetenv(EnvPrefix + name)
			t.Setenv(RetiredEnvName(name), "from-retired")
			if got := Getenv(name); got != "" {
				t.Errorf("Getenv(%q) = %q with only %s set, want the retired name read by nothing",
					name, got, RetiredEnvName(name))
			}
		})
	}
}

// TestGetenv_ReadsThePrefixedNameAndNothingElse verifies that the retired name
// contributes nothing, whether it is set alone or beside the prefixed one.
//
// There used to be a precedence to verify here, because both spellings were
// read and one had to win. There is no precedence now, and the case worth
// keeping is the one that used to be the interesting half of it: with the
// retired name set and the prefixed one absent, the answer is empty rather
// than the value the operator can plainly see in their environment.
func TestGetenv_ReadsThePrefixedNameAndNothingElse(t *testing.T) {
	const name = "LOG_LEVEL"

	for _, tc := range []struct {
		name     string
		prefixed string
		retired  string
		want     string
	}{
		{name: "only the prefixed name", prefixed: "debug", want: "debug"},
		{name: "only the retired name", retired: "warn", want: ""},
		{name: "both, the retired one contributes nothing", prefixed: "debug", retired: "warn", want: "debug"},
		{name: "neither", want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			os.Unsetenv(EnvPrefix + name)
			os.Unsetenv(name)
			if tc.prefixed != "" {
				t.Setenv(EnvPrefix+name, tc.prefixed)
			}
			if tc.retired != "" {
				t.Setenv(name, tc.retired)
			}

			if got := Getenv(name); got != tc.want {
				t.Errorf("Getenv(%q) = %q, want %q", name, got, tc.want)
			}
		})
	}
}

// TestGetenv_UnlistedName_IsReadVerbatim verifies that a name outside the
// migration is not given a prefixed spelling behind the caller's back.
//
// GITLAB_URL and GITLAB_TOKEN stay bare on purpose, and OTEL_* must stay bare
// because the OpenTelemetry exporters read those names themselves and would
// never see a prefixed one. Reading them through the same helper has to be
// safe, or every call site needs to remember which list its variable is on.
func TestGetenv_UnlistedName_IsReadVerbatim(t *testing.T) {
	for _, name := range []string{"GITLAB_URL", "GITLAB_TOKEN", "OTEL_EXPORTER_OTLP_ENDPOINT"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "bare")
			t.Setenv(EnvPrefix+name, "prefixed")

			if got := Getenv(name); got != "bare" {
				t.Errorf("Getenv(%q) = %q, want the bare value; this name is not part of the migration", name, got)
			}
			refuse, warn := RetiredEnvUses()
			if len(refuse) != 0 || len(warn) != 0 {
				t.Errorf("reading %q reported it as retired: refuse=%v warn=%v", name, refuse, warn)
			}
		})
	}
}

// TestRetiredEnvUses_SplitsByWhatIgnoringOneWouldCost verifies that a retired
// name present in the environment is reported, that it names the replacement,
// and that the two which take capability away are reported apart from the rest.
//
// It reports what is **set**, which is the opposite of the warning it replaces.
// That one could report only what had been read, because reading was still
// happening; nothing reads these now, so a check of what was consulted would
// report nothing at all and an operator would learn of the change by watching
// their deployment behave differently.
func TestRetiredEnvUses_SplitsByWhatIgnoringOneWouldCost(t *testing.T) {
	for _, tc := range []struct {
		name        string
		env         map[string]string
		wantRefuse  int
		wantWarn    int
		wantMention []string
	}{
		{
			name:     "nothing retired is set",
			env:      map[string]string{EnvPrefix + "TOOL_SURFACE": "meta"},
			wantWarn: 0,
		},
		{
			name:        "an ordinary retired name is a warning that says what to rename it to",
			env:         map[string]string{"AUTH_MODE": "oauth"},
			wantWarn:    1,
			wantMention: []string{"AUTH_MODE", EnvPrefix + "AUTH_MODE", "3.1.0"},
		},
		{
			name:        "the renamed switch is named as the operator spelled it",
			env:         map[string]string{"GITLAB_TIER": "premium"},
			wantWarn:    1,
			wantMention: []string{"GITLAB_TIER", EnvPrefix + "TIER"},
		},
		{
			name:        "a retired read-only switch refuses",
			env:         map[string]string{"GITLAB_READ_ONLY": "true"},
			wantRefuse:  1,
			wantMention: []string{"GITLAB_READ_ONLY", EnvPrefix + "READ_ONLY"},
		},
		{
			name:       "a retired safe-mode switch refuses",
			env:        map[string]string{"GITLAB_SAFE_MODE": "true"},
			wantRefuse: 1,
		},
		{
			name:       "the prefixed spelling of a protection is not a retired name",
			env:        map[string]string{EnvPrefix + "READ_ONLY": "true"},
			wantRefuse: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, name := range PrefixedEnvNames() {
				os.Unsetenv(RetiredEnvName(name))
				os.Unsetenv(EnvPrefix + name)
			}
			for name, value := range tc.env {
				t.Setenv(name, value)
			}

			refuse, warn := RetiredEnvUses()

			if len(refuse) != tc.wantRefuse {
				t.Fatalf("RetiredEnvUses() refuse = %v, want %d", refuse, tc.wantRefuse)
			}
			if len(warn) != tc.wantWarn {
				t.Fatalf("RetiredEnvUses() warn = %v, want %d", warn, tc.wantWarn)
			}
			reported := strings.Join(append(append([]string{}, refuse...), warn...), "\n")
			for _, want := range tc.wantMention {
				if !strings.Contains(reported, want) {
					t.Errorf("the report %q does not mention %q", reported, want)
				}
			}
		})
	}
}

// TestPrefixedEnvNames_IsACopy verifies that a caller cannot reorder or empty
// the list the whole migration is driven from.
// TestRetiredEnvName_SpellsTheRemovedNameOfEachSetting pins the two shapes a
// retired name takes: the bare suffix for the settings that were generic, and
// the GITLAB_-prefixed name for the switches that already carried one and were
// renamed in 2.8.0 so that every variable of this server starts alike.
//
// Nothing reads a setting under these any more. They are still spelled here
// because [RetiredEnvUses] looks for them, and a report that named the bare
// suffix of a switch the operator set as GITLAB_TIER would send them looking
// for a variable they never set.
func TestRetiredEnvName_SpellsTheRemovedNameOfEachSetting(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "TOOL_SURFACE", want: "TOOL_SURFACE"},
		{name: "LOG_LEVEL", want: "LOG_LEVEL"},
		{name: "TIER", want: "GITLAB_TIER"},
		{name: "READ_ONLY", want: "GITLAB_READ_ONLY"},
		{name: "SAFE_MODE", want: "GITLAB_SAFE_MODE"},
		{name: "IGNORE_SCOPES", want: "GITLAB_IGNORE_SCOPES"},
		{name: "SKIP_TLS_VERIFY", want: "GITLAB_SKIP_TLS_VERIFY"},
		{name: "YOLO_MODE", want: "YOLO_MODE"},
		{name: "NOT_A_SETTING", want: "NOT_A_SETTING"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RetiredEnvName(tt.name); got != tt.want {
				t.Errorf("RetiredEnvName(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

// TestGetenv_RenamedGitLabSwitch_NoLongerFallsBackToItsOldSpelling verifies
// the renamed switches through the whole path, on the terms 3.1.0 sets: the
// old GITLAB_TIER decides nothing, the prefixed name is what is read, and the
// report names the spelling the operator actually used rather than the bare
// suffix, since "TIER is no longer read" would send them looking for a
// variable they never set.
//
// This is the test that used to assert the fallback. It is kept rather than
// deleted because the behavior it describes is the one an operator upgrading
// into this release is most likely to be relying on, and a removal is only
// really made when something says the old answer is gone.
func TestGetenv_RenamedGitLabSwitch_NoLongerFallsBackToItsOldSpelling(t *testing.T) {
	tests := []struct {
		name       string
		old, new   string
		want       string
		wantReport string
	}{
		{
			name: "only the old spelling set", old: "premium", new: "",
			want:       "",
			wantReport: "GITLAB_TIER is no longer read (removed in 3.1.0): rename it to GITLAB_MCP_TIER",
		},
		{
			name: "both set, the old one contributes nothing", old: "premium", new: "ultimate",
			want:       "ultimate",
			wantReport: "GITLAB_TIER is no longer read",
		},
		{
			name: "only the prefixed one set", old: "", new: "free",
			want:       "free",
			wantReport: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITLAB_TIER", "")
			t.Setenv("GITLAB_MCP_TIER", "")
			os.Unsetenv("GITLAB_TIER")
			os.Unsetenv("GITLAB_MCP_TIER")
			if tt.old != "" {
				t.Setenv("GITLAB_TIER", tt.old)
			}
			if tt.new != "" {
				t.Setenv("GITLAB_MCP_TIER", tt.new)
			}
			if got := Getenv("TIER"); got != tt.want {
				t.Errorf("Getenv(TIER) = %q, want %q", got, tt.want)
			}
			refuse, warn := RetiredEnvUses()
			reported := strings.Join(append(append([]string{}, refuse...), warn...), "\n")
			if tt.wantReport == "" {
				if reported != "" {
					t.Errorf("report = %q, want none", reported)
				}
				return
			}
			if !strings.Contains(reported, tt.wantReport) {
				t.Errorf("report = %q, want %q", reported, tt.wantReport)
			}
			if strings.Contains(reported, "TIER is no longer") && !strings.Contains(reported, "GITLAB_TIER is no longer") {
				t.Errorf("the report names the bare suffix, which the operator never set: %q", reported)
			}
		})
	}
}

func TestPrefixedEnvNames_IsACopy(t *testing.T) {
	names := PrefixedEnvNames()
	if len(names) == 0 {
		t.Fatal("PrefixedEnvNames() is empty")
	}
	names[0] = "MUTATED"

	if PrefixedEnvNames()[0] == "MUTATED" {
		t.Error("PrefixedEnvNames() handed out the backing array")
	}
}

// TestPrefixedEnvNames_NoCallerReadsThemThroughOsGetenv is the guard that makes
// the migration hold for names nobody thinks about again.
//
// [Getenv] only helps a setting whose reader calls it. A call site left on
// os.Getenv keeps working under the deprecated spelling and silently ignores
// the prefixed one, which is the worst of the three possible states: the
// operator migrated, the documentation says the new name is read, and the
// server reads the old one. Scanning the source is the only way to see that,
// because a reader that was never converted behaves correctly in every test
// that sets the old name.
func TestPrefixedEnvNames_NoCallerReadsThemThroughOsGetenv(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving the repository root: %v", err)
	}
	sources := moduleGoSources(t, root)

	for _, name := range PrefixedEnvNames() {
		t.Run(name, func(t *testing.T) {
			// Every spelling: a reader left on the retired GITLAB_TIER is as
			// wrong as one left on the bare TOOL_SURFACE, and one that reads
			// the prefixed name directly bypasses this package the same way.
			bare := `os.Getenv("` + name + `")`
			legacy := `os.Getenv("` + RetiredEnvName(name) + `")`
			prefixed := `os.Getenv("` + EnvPrefix + name + `")`
			for path, body := range sources {
				if !strings.Contains(body, bare) && !strings.Contains(body, legacy) && !strings.Contains(body, prefixed) {
					continue
				}
				rel, relErr := filepath.Rel(root, path)
				if relErr != nil {
					rel = path
				}
				t.Errorf("%s reads %s through os.Getenv; use Getenv or TrimmedGetenv "+
					"so both spellings are honored", rel, EnvPrefix+name)
			}
		})
	}
}

// moduleGoSources reads every non-test Go file of this module, keyed by path.
//
// test/ is excluded: those are separate modules that drive the built binary as
// a client would, so they configure it through whichever spelling they mean to
// exercise, the deprecated one included.
//
// The walk collects paths and the reads happen after it returns, rather than
// inside the callback, because a path handed to a WalkDir callback has already
// been resolved once and reading it there re-resolves it through whatever the
// directory contains by then.
func moduleGoSources(t *testing.T, root string) map[string]string {
	t.Helper()

	// .claude holds agent worktrees, which are other checkouts of this
	// repository at other commits; scanning them would report a reader that
	// was converted here and not there.
	skip := map[string]bool{".git": true, ".claude": true, "node_modules": true, "site": true, "test": true, "dist": true}

	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if skip[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module sources under %s: %v", root, err)
	}
	if len(paths) == 0 {
		t.Fatalf("no Go sources found under %s; the guard would pass by finding nothing", root)
	}

	sources := make(map[string]string, len(paths))
	for _, path := range paths {
		body, readErr := os.ReadFile(path) // paths come from walking this module's own checkout
		if readErr != nil {
			t.Fatalf("reading %s: %v", path, readErr)
		}
		sources[path] = string(body)
	}
	return sources
}
