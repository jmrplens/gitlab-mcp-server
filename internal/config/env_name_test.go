package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGetenv_EveryPrefixedName_ResolvesUnderBothSpellings verifies the
// migration contract for each variable individually rather than for a sample.
//
// A spot check would pass while one name in the middle of the list was never
// wired, and that name's operator would find their setting silently ignored
// after an upgrade. The table is generated from the list itself, so a variable
// added to the list without being wired fails here rather than in someone's
// deployment.
func TestGetenv_EveryPrefixedName_ResolvesUnderBothSpellings(t *testing.T) {
	for _, name := range PrefixedEnvNames() {
		t.Run(name, func(t *testing.T) {
			t.Setenv(EnvPrefix+name, "from-prefixed")
			if got := Getenv(name); got != "from-prefixed" {
				t.Errorf("Getenv(%q) = %q with only the prefixed name set, want %q", name, got, "from-prefixed")
			}
		})
	}
}

// TestGetenv_Precedence verifies that the prefixed name wins, that the
// unprefixed one still works, and that neither being set reads as empty.
//
// Precedence is the half of the contract an operator cannot see: with both set
// the server obeys one of them, and picking the new name is what makes a
// migration a migration rather than a coin toss.
func TestGetenv_Precedence(t *testing.T) {
	const name = "LOG_LEVEL"

	for _, tc := range []struct {
		name     string
		prefixed string
		legacy   string
		setBoth  bool
		want     string
	}{
		{name: "only the prefixed name", prefixed: "debug", want: "debug"},
		{name: "only the deprecated name", legacy: "warn", want: "warn"},
		{name: "both, prefixed wins", prefixed: "debug", legacy: "warn", setBoth: true, want: "debug"},
		{name: "neither", want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetDeprecatedEnvUses()
			if tc.prefixed != "" {
				t.Setenv(EnvPrefix+name, tc.prefixed)
			}
			if tc.legacy != "" || tc.setBoth {
				t.Setenv(name, tc.legacy)
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
			resetDeprecatedEnvUses()
			t.Setenv(name, "bare")
			t.Setenv(EnvPrefix+name, "prefixed")

			if got := Getenv(name); got != "bare" {
				t.Errorf("Getenv(%q) = %q, want the bare value; this name is not part of the migration", name, got)
			}
			if warnings := DeprecatedEnvWarnings(); len(warnings) != 0 {
				t.Errorf("reading %q warned about deprecation: %v", name, warnings)
			}
		})
	}
}

// TestDeprecatedEnvWarnings_ReportWhatWasRead verifies that the warning names
// the variable and its replacement, that it distinguishes an old name from a
// redundant pair, and that it is silent about a variable nobody read.
//
// One warning per variable in use, not one per read: several of these are
// consulted more than once during startup, and an operator who set one old name
// should be told once rather than four times.
func TestDeprecatedEnvWarnings_ReportWhatWasRead(t *testing.T) {
	for _, tc := range []struct {
		name        string
		env         map[string]string
		read        []string
		wantCount   int
		wantMention []string
	}{
		{
			name:      "a variable nobody read is not mentioned",
			env:       map[string]string{"AUTH_MODE": "oauth"},
			wantCount: 0,
		},
		{
			name:        "the deprecated name says what to rename it to",
			env:         map[string]string{"AUTH_MODE": "oauth"},
			read:        []string{"AUTH_MODE", "AUTH_MODE"},
			wantCount:   1,
			wantMention: []string{"AUTH_MODE", EnvPrefix + "AUTH_MODE", "v3"},
		},
		{
			name:        "both set says which one is ignored",
			env:         map[string]string{"RATE_LIMIT_RPS": "5", EnvPrefix + "RATE_LIMIT_RPS": "10"},
			read:        []string{"RATE_LIMIT_RPS"},
			wantCount:   1,
			wantMention: []string{"ignored", EnvPrefix + "RATE_LIMIT_RPS"},
		},
		{
			name:      "only the prefixed name warns about nothing",
			env:       map[string]string{EnvPrefix + "TOOL_SURFACE": "meta"},
			read:      []string{"TOOL_SURFACE"},
			wantCount: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetDeprecatedEnvUses()
			t.Cleanup(resetDeprecatedEnvUses)
			for name, value := range tc.env {
				t.Setenv(name, value)
			}
			for _, name := range tc.read {
				_ = Getenv(name)
			}

			warnings := DeprecatedEnvWarnings()
			if len(warnings) != tc.wantCount {
				t.Fatalf("DeprecatedEnvWarnings() = %v, want %d warning(s)", warnings, tc.wantCount)
			}
			for _, want := range tc.wantMention {
				if !strings.Contains(warnings[0], want) {
					t.Errorf("warning %q does not mention %q", warnings[0], want)
				}
			}
		})
	}
}

// TestPrefixedEnvNames_IsACopy verifies that a caller cannot reorder or empty
// the list the whole migration is driven from.
// TestLegacyEnvName_SpellsTheOldNameOfEachSetting pins the two shapes an old
// name takes: the bare suffix for the settings that were generic, and the
// GITLAB_-prefixed name for the switches that already carried one and were
// renamed in 2.8.0 so that every variable of this server starts alike.
func TestLegacyEnvName_SpellsTheOldNameOfEachSetting(t *testing.T) {
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
			if got := LegacyEnvName(tt.name); got != tt.want {
				t.Errorf("LegacyEnvName(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

// TestGetenv_RenamedGitLabSwitch_FallsBackToItsOldSpelling verifies the
// renamed switches through the whole path: the old GITLAB_TIER is read when
// only it is set, the prefixed name wins when both are, and the warning names
// the spelling the operator actually used rather than the bare suffix, since
// "TIER is deprecated" would send them looking for a variable they never set.
func TestGetenv_RenamedGitLabSwitch_FallsBackToItsOldSpelling(t *testing.T) {
	tests := []struct {
		name        string
		old, new    string
		want        string
		wantWarning string
	}{
		{
			name: "only the old spelling set", old: "premium", new: "",
			want:        "premium",
			wantWarning: "GITLAB_TIER is deprecated and will be removed in v3; rename it to GITLAB_MCP_TIER",
		},
		{
			name: "both set, the prefixed one wins", old: "premium", new: "ultimate",
			want:        "ultimate",
			wantWarning: "both GITLAB_MCP_TIER and GITLAB_TIER are set; GITLAB_MCP_TIER is being used and GITLAB_TIER is ignored",
		},
		{
			name: "only the prefixed one set", old: "", new: "free",
			want:        "free",
			wantWarning: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetDeprecatedEnvUses()
			t.Cleanup(resetDeprecatedEnvUses)
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
			warnings := strings.Join(DeprecatedEnvWarnings(), "\n")
			if tt.wantWarning == "" {
				if warnings != "" {
					t.Errorf("warnings = %q, want none", warnings)
				}
				return
			}
			if !strings.Contains(warnings, tt.wantWarning) {
				t.Errorf("warnings = %q, want %q", warnings, tt.wantWarning)
			}
			if strings.Contains(warnings, "TIER is deprecated") && !strings.Contains(warnings, "GITLAB_TIER is deprecated") {
				t.Errorf("the warning names the bare suffix, which the operator never set: %q", warnings)
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
			// Every spelling: a reader left on the old GITLAB_TIER is as
			// wrong as one left on the bare TOOL_SURFACE, and one that reads
			// the prefixed name directly ignores the old one the same way.
			bare := `os.Getenv("` + name + `")`
			legacy := `os.Getenv("` + LegacyEnvName(name) + `")`
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
		body, readErr := os.ReadFile(path) //#nosec G304 -- paths come from walking this module's own checkout
		if readErr != nil {
			t.Fatalf("reading %s: %v", path, readErr)
		}
		sources[path] = string(body)
	}
	return sources
}
