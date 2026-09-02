// Package main tests the gateway character auditor: the per-string scan and
// its excerpt window, the schema walk that skips data keywords, the report
// formatting and exit codes, and one full scan of the served surface, which
// is the CI gate's own assertion that nothing served offends.
package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/mcpsurface"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/gatewaycompat"
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
