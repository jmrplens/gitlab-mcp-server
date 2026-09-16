// Package main tests the gateway character auditor: the per-string scan and
// its excerpt window, the schema walk that skips data keywords, the report
// formatting and exit codes, the command line main assembles out of its three
// flags, and one full scan of the served surface, which is the CI gate's own
// assertion that nothing served offends.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/mcpsurface"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/gatewaycompat"
)

// captureOutput redirects the command's stdout and stderr into buffers for
// the duration of the test and returns them.
func captureOutput(t *testing.T) (out, errOut *bytes.Buffer) {
	t.Helper()
	out, errOut = &bytes.Buffer{}, &bytes.Buffer{}
	prevOut, prevErr := stdout, stderr
	stdout, stderr = out, errOut
	t.Cleanup(func() { stdout, stderr = prevOut, prevErr })
	return out, errOut
}

// callMain runs main with args as the process command line and returns the
// code it handed the exit seam along with everything it printed.
//
// The flag set is a fresh one for the duration, because main registers its
// three flags on flag.CommandLine, where the test binary's own flags already
// live and where a second call would panic on the redefinition.
func callMain(t *testing.T, args ...string) (code int, out, errOut string) {
	t.Helper()
	setScanMode(t, false, nil)
	outBuf, errBuf := captureOutput(t)

	prevArgs, prevFlags, prevExit := os.Args, flag.CommandLine, osExit
	t.Cleanup(func() { os.Args, flag.CommandLine, osExit = prevArgs, prevFlags, prevExit })

	os.Args = append([]string{"audit_gateway_chars"}, args...)
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)

	exits := 0
	osExit = func(c int) { code, exits = c, exits+1 }
	main()
	if exits != 1 {
		t.Fatalf("main() reached the exit seam %d times, want exactly once", exits)
	}
	return code, outBuf.String(), errBuf.String()
}

// sortedLines splits a report into its rows and orders them, so two reports
// can be compared as the multisets of rows they are.
func sortedLines(report string) []string {
	lines := strings.Split(strings.TrimSuffix(report, "\n"), "\n")
	slices.Sort(lines)
	return lines
}

// firstDifference names where two ordered row lists part company, so a failure
// points at one row instead of printing two reports of tens of thousands of
// bytes.
func firstDifference(left, right []string) string {
	for i := range min(len(left), len(right)) {
		if left[i] != right[i] {
			return fmt.Sprintf("row %d: %q vs %q", i, left[i], right[i])
		}
	}
	return fmt.Sprintf("row %d, where the shorter list ends", min(len(left), len(right)))
}

// setScanMode pins the two scan knobs for one test and restores them after.
func setScanMode(t *testing.T, full bool, subs []gatewaycompat.Substitution) {
	t.Helper()
	prevFull, prevSubs := fullStrings, appliedSubstitutions
	fullStrings, appliedSubstitutions = full, subs
	t.Cleanup(func() { fullStrings, appliedSubstitutions = prevFull, prevSubs })
}

// TestScanText_CharacterClasses_ReportsOnlyOffenders verifies the served-text
// policy: pure ASCII prose passes, a listed ASCII character or any rune above
// U+007F is reported once per string with an excerpt centered on the first
// hit, and the applied substitutions run before the judgement.
func TestScanText_CharacterClasses_ReportsOnlyOffenders(t *testing.T) {
	long := strings.Repeat("a", 50) + ";" + strings.Repeat("b", 50)
	cases := []struct {
		name    string
		text    string
		full    bool
		subs    []gatewaycompat.Substitution
		want    bool
		excerpt string
	}{
		{name: "ascii_prose_is_clean", text: "Lists issues. Returns: a page of issues.", want: false},
		{name: "empty_is_clean", text: "", want: false},
		{name: "semicolon_is_reported", text: "one; two", want: true, excerpt: "one; two"},
		{name: "hit_at_start_clips_window", text: ";abc", want: true, excerpt: ";abc"},
		{name: "em_dash_is_reported", text: "a \u2014 b", want: true, excerpt: "a \u2014 b"},
		{name: "accented_letter_is_reported", text: "caf\u00e9", want: true, excerpt: "caf\u00e9"},
		{name: "excerpt_centers_on_first_hit", text: long, want: true, excerpt: long[20:80]},
		{name: "newline_becomes_space_in_excerpt", text: "line one\nline; two", want: true, excerpt: "line one line; two"},
		{name: "full_mode_prints_whole_string", text: "a\nb;", full: true, want: true, excerpt: "a\\nb;"},
		{
			name: "substitution_clears_the_hit", text: "one; two",
			subs: []gatewaycompat.Substitution{{Old: ";", New: ","}}, want: false,
		},
		{
			name: "substitution_can_introduce_a_hit", text: "axb",
			subs: []gatewaycompat.Substitution{{Old: "x", New: ";"}}, want: true, excerpt: "a;b",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setScanMode(t, tc.full, tc.subs)
			got := scanText("meta", "tool gitlab_issue description", tc.text)
			if !tc.want {
				if len(got) != 0 {
					t.Fatalf("scanText(%q) = %+v, want no offender", tc.text, got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("scanText(%q) = %+v, want exactly one offender", tc.text, got)
			}
			if got[0].surface != "meta" || got[0].where != "tool gitlab_issue description" {
				t.Errorf("offender location = %q/%q, want meta/tool gitlab_issue description", got[0].surface, got[0].where)
			}
			if got[0].excerpt != tc.excerpt {
				t.Errorf("excerpt = %q, want %q", got[0].excerpt, tc.excerpt)
			}
		})
	}
}

// TestScanSchema_ProseKeywords_SkipsDataAndNames verifies the schema walk
// judges only description and title values, however deeply nested under
// properties, and never the data keywords (pattern, enum, default, const) or
// property names, since a validator rejecting a regex is not the reported
// problem.
//
// The last two rows are the two ways a schema can refuse to serialize into the
// form the walk descends. One cannot be marshaled at all (a func value); the
// other marshals and cannot be read back, because a raw default outside
// float64's range is valid JSON text that no `any` can hold. Both carry the
// offending prose of top_level_description, which is reported when the schema
// does serialize, so what these two rows assert is that a schema the walk
// cannot read reports nothing at all.
//
// That is a claim about the guard rather than about an empty value: the failed
// read leaves a map behind holding the description beside a +Inf default, so
// without the guard the walk descends the half that survived and reports it.
func TestScanSchema_ProseKeywords_SkipsDataAndNames(t *testing.T) {
	cases := []struct {
		name   string
		schema any
		want   int
	}{
		{name: "nil_schema", schema: nil, want: 0},
		{name: "clean_schema", schema: map[string]any{"type": "object", "description": "A thing."}, want: 0},
		{name: "top_level_description", schema: map[string]any{"description": "one; two"}, want: 1},
		{name: "top_level_title", schema: map[string]any{"title": "caf\u00e9"}, want: 1},
		{
			name: "nested_property_description",
			schema: map[string]any{"properties": map[string]any{
				"id": map[string]any{"type": "integer", "description": "The id; required."},
			}},
			want: 1,
		},
		{
			name: "two_prose_strings_are_two_offenders",
			schema: map[string]any{"description": "a; b", "properties": map[string]any{
				"x": map[string]any{"title": "x; y"},
			}},
			want: 2,
		},
		{name: "pattern_is_data", schema: map[string]any{"pattern": "^[a-z;]+$"}, want: 0},
		{name: "enum_is_data", schema: map[string]any{"enum": []any{"a;b", "\u00e9"}}, want: 0},
		{name: "default_is_data", schema: map[string]any{"default": "x;y"}, want: 0},
		{name: "property_name_is_not_prose", schema: map[string]any{"properties": map[string]any{"a;b": map[string]any{}}}, want: 0},
		{name: "typed_schema_description", schema: &jsonschema.Schema{Type: "object", Description: "typed; prose"}, want: 1},
		{name: "unmarshalable_schema_is_skipped", schema: map[string]any{"description": func() {}}, want: 0},
		{
			name: "schema_that_cannot_be_read_back_is_skipped",
			schema: map[string]any{
				"description": "one; two",
				"default":     json.RawMessage(`1e400`),
			},
			want: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scanSchema("individual", "tool gitlab_issue_get input schema", tc.schema)
			if len(got) != tc.want {
				t.Fatalf("scanSchema = %+v, want %d offender(s)", got, tc.want)
			}
			for _, f := range got {
				if f.surface != "individual" || f.where != "tool gitlab_issue_get input schema" {
					t.Errorf("offender location = %q/%q, want the schema location", f.surface, f.where)
				}
			}
		})
	}
}

// TestReport_Offenders_PrintsSortedAndReturnsExitCode verifies the report:
// a clean scan prints the all-clear and exits 0 whatever -check says, an
// offending scan prints every row sorted by surface then location in the
// padded excerpt form (or tab-separated with -full) followed by the count, and
// exits 1 only under -check.
func TestReport_Offenders_PrintsSortedAndReturnsExitCode(t *testing.T) {
	offenders := []offender{
		{surface: "resources", where: "resource b description", excerpt: "x; y"},
		{surface: "dynamic", where: "tool gitlab_find_action description", excerpt: "a; b"},
		{surface: "resources", where: "resource a description", excerpt: "c; d"},
	}
	cases := []struct {
		name     string
		found    []offender
		check    bool
		full     bool
		wantCode int
		wantOut  []string
	}{
		{
			name: "clean_without_check", found: nil, check: false, wantCode: 0,
			wantOut: []string{"gateway character audit: nothing served carries an offending character\n"},
		},
		{
			name: "clean_with_check", found: nil, check: true, wantCode: 0,
			wantOut: []string{"nothing served carries an offending character"},
		},
		{
			name: "offenders_without_check", found: offenders, check: false, wantCode: 0,
			wantOut: []string{
				"dynamic     tool gitlab_find_action description                  a; b\n" +
					"resources   resource a description                               c; d\n" +
					"resources   resource b description                               x; y\n" +
					"gateway character audit: 3 served string(s) carry an offending character\n",
			},
		},
		{
			name: "offenders_with_check", found: offenders, check: true, wantCode: 1,
			wantOut: []string{"gateway character audit: 3 served string(s) carry an offending character\n"},
		},
		{
			name: "full_mode_is_tab_separated", found: offenders[:1], check: false, full: true, wantCode: 0,
			wantOut: []string{"resources\tresource b description\tx; y\n", "1 served string(s)"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setScanMode(t, tc.full, nil)
			out, errOut := captureOutput(t)
			found := append([]offender(nil), tc.found...)
			if got := report(found, tc.check); got != tc.wantCode {
				t.Errorf("report returned %d, want %d", got, tc.wantCode)
			}
			for _, want := range tc.wantOut {
				if !strings.Contains(out.String(), want) {
					t.Errorf("stdout lacks %q:\n%s", want, out.String())
				}
			}
			if errOut.Len() != 0 {
				t.Errorf("stderr should stay empty, got %q", errOut.String())
			}
		})
	}
}

// TestRun_ApplyWithoutUsableSubstitutions_Fails verifies the -apply
// preconditions: a malformed GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS refuses the
// run with the parse error, and an empty one refuses it because there is
// nothing to verify; both exit 1 before any surface is listed.
func TestRun_ApplyWithoutUsableSubstitutions_Fails(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		wantErr string
	}{
		{name: "malformed_pair", env: "no-separator", wantErr: gatewaycompat.EnvVar},
		{name: "empty_variable", env: "", wantErr: "-apply: " + gatewaycompat.EnvVar + " is empty, nothing to apply"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(gatewaycompat.EnvVar, tc.env)
			setScanMode(t, false, nil)
			out, errOut := captureOutput(t)
			if got := run(false, true); got != 1 {
				t.Errorf("run(check=false, apply=true) = %d, want 1", got)
			}
			if !strings.Contains(errOut.String(), tc.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", errOut.String(), tc.wantErr)
			}
			if out.Len() != 0 {
				t.Errorf("stdout should stay empty when -apply is refused, got %q", out.String())
			}
			if appliedSubstitutions != nil {
				t.Errorf("appliedSubstitutions = %+v, want none applied after a refused run", appliedSubstitutions)
			}
		})
	}
}

// TestListSurface_Surfaces_ReturnTheirRegisteredTools verifies the listing
// helper publishes each named surface over a real tools/list round-trip: the
// dynamic pair, the meta domain tools, and nothing for a surface name the
// server does not know.
func TestListSurface_Surfaces_ReturnTheirRegisteredTools(t *testing.T) {
	client, cleanup := mcpsurface.NewStubClient()
	t.Cleanup(cleanup)

	cases := []struct {
		name      string
		surface   string
		wantTools []string
		wantEmpty bool
	}{
		{name: "dynamic_pair", surface: config.ToolSurfaceDynamic, wantTools: []string{"gitlab_find_action", "gitlab_execute_action"}},
		{name: "meta_domains", surface: config.ToolSurfaceMeta, wantTools: []string{"gitlab_issue", "gitlab_project", "gitlab_group"}},
		{name: "unknown_surface_registers_nothing", surface: "bogus", wantEmpty: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			listed := listSurface(client, tc.surface)
			if tc.wantEmpty {
				if len(listed) != 0 {
					t.Fatalf("listSurface(%q) returned %d tools, want none", tc.surface, len(listed))
				}
				return
			}
			names := map[string]bool{}
			for _, tool := range listed {
				names[tool.Name] = true
			}
			for _, want := range tc.wantTools {
				if !names[want] {
					t.Errorf("surface %q does not list %s (got %d tools)", tc.surface, want, len(listed))
				}
			}
		})
	}
}

// TestRun_ServedSurfaceWithApply_IsClean is the gate itself: every tool
// surface at the widest tier plus prompts and resources is scanned through
// -apply with a substitution that matches nothing served, so the judgement is
// of the unmodified text, and under -check the run must exit 0 with the
// all-clear. A semicolon or non-ASCII character reaching any listed string
// fails this test before it fails at a gateway's door.
func TestRun_ServedSurfaceWithApply_IsClean(t *testing.T) {
	t.Setenv(gatewaycompat.EnvVar, "zzqx-never-served=zzqy")
	setScanMode(t, false, nil)
	out, errOut := captureOutput(t)

	if got := run(true, true); got != 0 {
		t.Fatalf("run(check=true, apply=true) = %d, want 0\nstdout:\n%s\nstderr:\n%s", got, out.String(), errOut.String())
	}
	if want := "gateway character audit: nothing served carries an offending character\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
	if len(appliedSubstitutions) != 1 || appliedSubstitutions[0].Old != "zzqx-never-served" || appliedSubstitutions[0].New != "zzqy" {
		t.Errorf("appliedSubstitutions = %+v, want the parsed environment pair", appliedSubstitutions)
	}
}

// TestMain_ApplyWithAnEmptyConfiguration_RefusesAndHasAlreadyTakenFull
// verifies the half of the command line main owns and the half it passes on.
// -full is main's own: it is transferred to the scan knob before run is
// called, and it is still set when the run refuses. -apply is passed on, and
// with an empty GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS the run refuses on
// stderr and exits 1 before any surface is listed, so nothing reaches stdout.
func TestMain_ApplyWithAnEmptyConfiguration_RefusesAndHasAlreadyTakenFull(t *testing.T) {
	t.Setenv(gatewaycompat.EnvVar, "")

	code, out, errOut := callMain(t, "-full", "-apply")

	if code != 1 {
		t.Errorf("main() exited %d, want 1", code)
	}
	if want := "-apply: " + gatewaycompat.EnvVar + " is empty, nothing to apply"; !strings.Contains(errOut, want) {
		t.Errorf("stderr = %q, want it to contain %q", errOut, want)
	}
	if out != "" {
		t.Errorf("stdout = %q, want nothing printed when -apply is refused", out)
	}
	if !fullStrings {
		t.Error("-full did not reach the scan knob: fullStrings is false after main()")
	}
}

// TestMain_CheckFlag_TurnsTheSameFindingsIntoAFailure verifies that -check
// decides the exit code and nothing else. Both runs scan the served surface
// under a substitution that puts a semicolon into every served string naming
// GitLab, so both find the same offenders and print the same report; only the
// run carrying -check exits non-zero. Asserting the two reports carry the same
// rows is what separates "check gates" from "check also scans differently",
// and the report being an offender list at all is what proves -apply reached
// the scan rather than being parsed and dropped.
//
// The rows are compared as a multiset rather than as text. The report sorts by
// surface and location only, and one schema holding several offending strings
// contributes several rows under the one location; their relative order is
// whatever gatewaycompat.RewriteSchemaProse's walk over the decoded map
// produced, and Go randomizes map iteration per run.
func TestMain_CheckFlag_TurnsTheSameFindingsIntoAFailure(t *testing.T) {
	t.Setenv(gatewaycompat.EnvVar, "GitLab=GitLab;")
	const offenders = "carry an offending character"

	reportCode, reportOut, reportErr := callMain(t, "-apply")
	gateCode, gateOut, gateErr := callMain(t, "-apply", "-check")

	if !strings.Contains(reportOut, offenders) {
		t.Fatalf("the substitution introduced no offender, so nothing about -check is under test; stdout:\n%s", reportOut)
	}
	if reportCode != 0 {
		t.Errorf("main() without -check exited %d, want 0: findings alone are not a failure", reportCode)
	}
	if gateCode != 1 {
		t.Errorf("main() with -check exited %d, want 1", gateCode)
	}
	reported, gated := sortedLines(reportOut), sortedLines(gateOut)
	if !slices.Equal(reported, gated) {
		t.Errorf("-check changed the report: %d rows without it, %d rows with it, first difference at %s",
			len(reported), len(gated), firstDifference(reported, gated))
	}
	if reportErr != "" || gateErr != "" {
		t.Errorf("stderr should stay empty, got %q and %q", reportErr, gateErr)
	}
}
