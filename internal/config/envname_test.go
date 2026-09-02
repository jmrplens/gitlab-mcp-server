package config

import (
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
