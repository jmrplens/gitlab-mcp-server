// Tests for audit_doc_coverage. The unit tests cover the small,
// pure helpers (README row parser, doc-tool parser, tier badge
// matcher) and one end-to-end buildReport smoke test that exercises
// the full comparison loop against a synthetic catalog and a
// synthetic doc tree. The real README/catalog integration test lives
// in TestBuildReport_LiveBaseline.
package main

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestParseDomainsTable verifies the README table parser extracts the
// four canonical columns and rejects malformed rows.
func TestParse_DomainsTable(t *testing.T) {
	tmp := t.TempDir()
	readme := filepath.Join(tmp, "README.md")
	content := "## Domains\n\n| Domain | Tools | Meta-tool | Document |\n| --- | ---: | --- | --- |\n| Projects | 50 | `gitlab_project` | [projects.md](projects.md) |\n| Access & Tokens | 68 | various | [access.md](access.md) |\n| Branch Rules | 1 | `gitlab_branch` (routed) | [branch-rules.md](branch-rules.md) |\n"
	if err := os.WriteFile(readme, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	rows, err := parseDomainsTable(readme)
	if err != nil {
		t.Fatalf("parseDomainsTable: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if rows[0].Domain != "Projects" || rows[0].ExpectedCount != 50 {
		t.Errorf("row[0] = %+v, want Projects/50", rows[0])
	}
	if rows[1].MetaToolsCSV != "various" {
		t.Errorf("row[1] MetaToolsCSV = %q, want %q", rows[1].MetaToolsCSV, "various")
	}
	if rows[2].ExpectedCount != 1 {
		t.Errorf("row[2] count = %d, want 1", rows[2].ExpectedCount)
	}
}

// TestExpectedGroupsForRow verifies the README→group mapping for the
// most common shapes: simple comma-separated tool names, parenthesised
// annotations, and the "various" rows that fall through to the
// hardcoded specialGroups table.
func TestExpected_GroupsForRow(t *testing.T) {
	tests := []struct {
		name string
		row  docMappingRow
		want []string
	}{
		{
			name: "single tool",
			row:  docMappingRow{DocLink: "branches.md", MetaToolsCSV: "`gitlab_branch`"},
			want: []string{"gitlab_branch"},
		},
		{
			name: "csv with parens",
			row: docMappingRow{
				DocLink:      "merge-requests.md",
				MetaToolsCSV: "`gitlab_pipeline`, `gitlab_job`, etc.",
			},
			want: []string{"gitlab_pipeline", "gitlab_job"},
		},
		{
			name: "routed branch rules returns nil",
			row: docMappingRow{
				DocLink:      "branch-rules.md",
				MetaToolsCSV: "`gitlab_branch` (routed)",
			},
			want: nil,
		},
		{
			name: "various returns nil (no group claims)",
			row:  docMappingRow{DocLink: "access.md", MetaToolsCSV: "various"},
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := expectedGroupsForRow(tc.row)
			sort.Strings(got)
			want := append([]string(nil), tc.want...)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("expectedGroupsForRow = %v, want %v", got, want)
			}
		})
	}
}

// TestParseDocTools verifies the three Markdown patterns the auditor
// accepts as tool references. Each test case exercises one pattern
// alone, then a doc combining all three is checked for the union.
func TestParse_DocTools(t *testing.T) {
	tmp := t.TempDir()
	cases := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name: "heading with backticks",
			content: `## Tools

### ` + "`gitlab_branch_get`" + `

Body.

### ` + "`gitlab_branch_list`" + `

Body.
`,
			want: []string{"gitlab_branch_get", "gitlab_branch_list"},
		},
		{
			name: "heading without backticks",
			content: `## Geo Site Tools

### gitlab_create_geo_site

### gitlab_list_geo_sites
`,
			want: []string{"gitlab_create_geo_site", "gitlab_list_geo_sites"},
		},
		{
			name: "pipe table rows",
			content: `## Tools

| Tool | Description |
| --- | --- |
| ` + "`gitlab_get_recently_created_issues_count`" + ` | Count |
| ` + "`gitlab_get_compliance_policy_settings`" + ` | Settings |
`,
			want: []string{
				"gitlab_get_compliance_policy_settings",
				"gitlab_get_recently_created_issues_count",
			},
		},
		{
			name: "mixed patterns are unioned",
			content: `## Overview

### ` + "`gitlab_branch_get`" + `

### gitlab_list_geo_sites

| Tool | Description |
| --- | --- |
| ` + "`gitlab_get_recently_created_issues_count`" + ` | Count |
`,
			want: []string{
				"gitlab_branch_get",
				"gitlab_get_recently_created_issues_count",
				"gitlab_list_geo_sites",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := filepath.Join(tmp, tc.name+".md")
			if err := os.WriteFile(doc, []byte(tc.content), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			got, err := parseDocTools(doc)
			if err != nil {
				t.Fatalf("parseDocTools: %v", err)
			}
			sort.Strings(got)
			want := append([]string(nil), tc.want...)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("parseDocTools = %v, want %v", got, want)
			}
		})
	}
}

// TestParseTierBadge verifies the tier badge detector picks up the
// three documented badge styles (table cell, prose "> **Tier**:", and
// inline annotation) and ignores tool headings whose body is free.
func TestParse_TierBadge(t *testing.T) {
	tmp := t.TempDir()
	cases := []struct {
		name    string
		content string
		tool    string
		want    string
		ok      bool
	}{
		{
			name: "table cell premium",
			content: `### ` + "`gitlab_branch_protect`" + `

Body.

| Annotation | **Update** |
| Tier | **Premium** |
`,
			tool: "gitlab_branch_protect",
			want: "Premium",
			ok:   true,
		},
		{
			name: "prose premium",
			content: `### ` + "`gitlab_branch_protect`" + `

> **Tier**: Premium

Body.
`,
			tool: "gitlab_branch_protect",
			want: "Premium",
			ok:   true,
		},
		{
			name: "ultimate wins over premium when both present",
			content: `### ` + "`gitlab_compliance_policy`" + `

| Tier | **Ultimate** |
| Notes | Premium only |
`,
			tool: "gitlab_compliance_policy",
			want: "Ultimate",
			ok:   true,
		},
		{
			name: "no badge",
			content: `### ` + "`gitlab_branch_get`" + `

Body.
`,
			tool: "gitlab_branch_get",
			want: "",
			ok:   false,
		},
		{
			name: "different tool heading has no effect",
			content: `### ` + "`gitlab_other_tool`" + `

| Tier | **Premium** |
`,
			tool: "gitlab_branch_protect",
			want: "",
			ok:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := filepath.Join(tmp, tc.name+".md")
			if err := os.WriteFile(doc, []byte(tc.content), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			badge, ok := parseTierBadge(doc, tc.tool)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if badge != tc.want {
				t.Errorf("badge = %q, want %q", badge, tc.want)
			}
		})
	}
}

// TestTierBadgeMatches is the comparison table: catalog tier (Free /
// Premium / Ultimate) versus doc badge ("", "Premium", "Ultimate").
// Only Premium/Ultimate badges that disagree with the catalog should
// trip the auditor.
func TestTier_Badge_Matches(t *testing.T) {
	tests := []struct {
		name    string
		edition string
		badge   string
		want    bool
	}{
		{"free_without_badge", "", "", true},
		{"free_tolerates_any_badge", "", "Premium", true}, // no badge is expected on a free tool
		{"premium_matches", "premium", "Premium", true},
		{"premium_badged_ultimate", "premium", "Ultimate", false},
		{"ultimate_matches", "ultimate", "Ultimate", true},
		{"ultimate_badged_premium", "ultimate", "Premium", false},
		{"ultimate_missing_badge", "ultimate", "", true}, // a missing badge on Ultimate is not flagged here
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tierBadgeMatches(tc.badge, tc.edition)
			if got != tc.want {
				t.Errorf("tierBadgeMatches(%q, %q) = %v, want %v", tc.badge, tc.edition, got, tc.want)
			}
		})
	}
}

// TestBuildReport_SyntheticFlow exercises the full comparison loop
// against a synthetic catalog and two synthetic docs (one with
// missing tools, one with orphans). It validates the structural
// invariants of fileFinding without requiring the live catalog.
func TestBuildReport_SyntheticFlow(t *testing.T) {
	rep := buildSyntheticFlowReport(t)

	if len(rep.Files) != 2 {
		t.Fatalf("Files = %d, want 2", len(rep.Files))
	}

	byDoc := map[string]fileFinding{}
	for _, f := range rep.Files {
		byDoc[f.DocPath] = f
	}

	assertBranchesSynthetic(t, byDoc["docs/branches.md"])
	assertTagsSynthetic(t, byDoc["docs/tags.md"])

	if rep.Summary.MissingTotal != 1 || rep.Summary.OrphanTotal != 1 {
		t.Errorf("summary counts wrong: missing=%d orphan=%d", rep.Summary.MissingTotal, rep.Summary.OrphanTotal)
	}
	if rep.Summary.DocsWithFindings != 2 {
		t.Errorf("DocsWithFindings = %d, want 2", rep.Summary.DocsWithFindings)
	}
}

// buildSyntheticFlowReport constructs the synthetic fixture (one
// README mapping two docs, a catalog with one missing and one
// orphan tool) and returns the resulting report. Used by
// TestBuildReport_SyntheticFlow.
func buildSyntheticFlowReport(t *testing.T) report {
	t.Helper()
	tmp := t.TempDir()
	readme := filepath.Join(tmp, "README.md")
	docsRoot := filepath.Join(tmp, "docs")

	if err := os.MkdirAll(docsRoot, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	readmeContent := "## Domains\n\n| Domain | Tools | Meta-tool | Document |\n| --- | ---: | --- | --- |\n| Branches | 3 | `gitlab_branch` | [branches.md](branches.md) |\n| Tags | 2 | `gitlab_tag` | [tags.md](tags.md) |\n"
	if err := os.WriteFile(readme, []byte(readmeContent), 0o600); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	// branches.md is missing gitlab_branch_third.
	branchesContent := "## Tools\n\n### `gitlab_branch_get`\n### `gitlab_branch_list`\n"
	if err := os.WriteFile(filepath.Join(docsRoot, "branches.md"), []byte(branchesContent), 0o600); err != nil {
		t.Fatalf("write branches.md: %v", err)
	}

	// tags.md has an orphan tool not in the catalog.
	tagsContent := "## Tools\n\n### `gitlab_tag_get`\n### `gitlab_tag_orphan`\n"
	if err := os.WriteFile(filepath.Join(docsRoot, "tags.md"), []byte(tagsContent), 0o600); err != nil {
		t.Fatalf("write tags.md: %v", err)
	}

	catalog := &catalogSnapshot{Tools: map[string]catalogTool{
		"gitlab_branch_get":   {Name: "gitlab_branch_get", Group: "gitlab_branch"},
		"gitlab_branch_list":  {Name: "gitlab_branch_list", Group: "gitlab_branch"},
		"gitlab_branch_third": {Name: "gitlab_branch_third", Group: "gitlab_branch"},
		"gitlab_tag_get":      {Name: "gitlab_tag_get", Group: "gitlab_tag"},
	}}

	rep, err := buildReport(tmp, docsRoot, readme, catalog)
	if err != nil {
		t.Fatalf("buildReport: %v", err)
	}
	return rep
}

// assertBranchesSynthetic verifies the expected/per-file counts and
// finding lists for the synthetic branches.md fixture. branches.md
// has 3 catalog tools and 2 documented — so one Missing, no
// Orphans, and CountMatches must be false.
func assertBranchesSynthetic(t *testing.T, branches fileFinding) {
	t.Helper()
	if branches.ExpectedCount != 3 {
		t.Errorf("branches.ExpectedCount = %d, want 3", branches.ExpectedCount)
	}
	if branches.DocumentedCount != 2 {
		t.Errorf("branches.DocumentedCount = %d, want 2", branches.DocumentedCount)
	}
	if !reflect.DeepEqual(branches.Missing, []string{"gitlab_branch_third"}) {
		t.Errorf("branches.Missing = %v, want [gitlab_branch_third]", branches.Missing)
	}
	if len(branches.Orphan) != 0 {
		t.Errorf("branches.Orphan = %v, want []", branches.Orphan)
	}
	if branches.CountMatches {
		t.Errorf("branches.CountMatches = true, want false (doc 2 != catalog 3)")
	}
	if branches.ReadmeCount != 3 || !branches.ReadmeCountMatches {
		t.Errorf("branches readme check failed: count=%d matches=%v", branches.ReadmeCount, branches.ReadmeCountMatches)
	}
}

// assertTagsSynthetic verifies the synthetic tags.md fixture. The
// doc has 2 tools but only 1 is in the catalog, so one Orphan and
// CountMatches must be false.
func assertTagsSynthetic(t *testing.T, tags fileFinding) {
	t.Helper()
	if tags.ExpectedCount != 1 {
		t.Errorf("tags.ExpectedCount = %d, want 1", tags.ExpectedCount)
	}
	if !reflect.DeepEqual(tags.Orphan, []string{"gitlab_tag_orphan"}) {
		t.Errorf("tags.Orphan = %v, want [gitlab_tag_orphan]", tags.Orphan)
	}
	if tags.CountMatches {
		t.Errorf("tags.CountMatches = true, want false (doc 2 != catalog 1)")
	}
}

// TestBuildReport_RoutedTool verifies that hardcoded overrides
// correctly move routed tools (gitlab_list_branch_rules) from the
// group-owner's doc (branches.md) to their dedicated doc
// (branch-rules.md).
func TestBuildReport_RoutedTool(t *testing.T) {
	tmp := t.TempDir()
	readme := filepath.Join(tmp, "README.md")
	docsRoot := filepath.Join(tmp, "docs")

	if err := os.MkdirAll(docsRoot, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	readmeContent := `## Domains

| Domain | Tools | Meta-tool | Document |
| --- | ---: | --- | --- |
| Branches | 1 | ` + "`gitlab_branch`" + ` | [branches.md](branches.md) |
| Branch Rules | 1 | ` + "`gitlab_branch` (routed)`" + ` | [branch-rules.md](branch-rules.md) |
`
	if err := os.WriteFile(readme, []byte(readmeContent), 0o600); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	if err := os.WriteFile(filepath.Join(docsRoot, "branches.md"), []byte(""), 0o600); err != nil {
		t.Fatalf("write branches.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsRoot, "branch-rules.md"), []byte(""), 0o600); err != nil {
		t.Fatalf("write branch-rules.md: %v", err)
	}

	catalog := &catalogSnapshot{Tools: map[string]catalogTool{
		"gitlab_branch_get":        {Name: "gitlab_branch_get", Group: "gitlab_branch"},
		"gitlab_list_branch_rules": {Name: "gitlab_list_branch_rules", Group: "gitlab_branch"},
	}}

	rep, err := buildReport(tmp, docsRoot, readme, catalog)
	if err != nil {
		t.Fatalf("buildReport: %v", err)
	}

	byDoc := map[string]fileFinding{}
	for _, f := range rep.Files {
		byDoc[f.DocPath] = f
	}

	// gitlab_list_branch_rules is routed via the override to
	// branch-rules.md. It must not be expected in branches.md even
	// though its owning group is gitlab_branch.
	branches := byDoc["docs/branches.md"]
	for _, m := range branches.Missing {
		if m == "gitlab_list_branch_rules" {
			t.Errorf("gitlab_list_branch_rules should not be expected in branches.md (missing=%v)", branches.Missing)
		}
	}
	for _, o := range branches.Orphan {
		if o == "gitlab_list_branch_rules" {
			t.Errorf("gitlab_list_branch_rules should not appear as orphan in branches.md (orphan=%v)", branches.Orphan)
		}
	}

	branchRules := byDoc["docs/branch-rules.md"]
	if branchRules.ExpectedCount != 1 {
		t.Errorf("branch-rules.md ExpectedCount = %d, want 1", branchRules.ExpectedCount)
	}
	if len(branchRules.Missing) != 1 || branchRules.Missing[0] != "gitlab_list_branch_rules" {
		t.Errorf("branch-rules.md Missing = %v, want [gitlab_list_branch_rules]", branchRules.Missing)
	}
}

// TestCheck_FailsWhenFindingsPresent locks the -check gate behavior:
// any missing/orphan/tier_mismatch in the summary produces a non-empty
// diagnostic; clean reports pass.
func TestCheck_FailsWhenFindingsPresent(t *testing.T) {
	r := report{
		Summary: reportSummary{MissingTotal: 1},
	}
	if r.check() == "" {
		t.Error("check() = empty, want non-empty")
	}
	// Unassigned tools (catalog gained a new group before the README
	// Domains table was updated) must also block -check so the
	// orchestrator routes them explicitly instead of slipping past.
	r = report{
		Summary: reportSummary{UnassignedTotal: 1},
	}
	if r.check() == "" {
		t.Error("check() = empty with UnassignedTotal=1, want non-empty")
	}
	r = report{}
	if r.check() != "" {
		t.Errorf("check() = %q, want empty", r.check())
	}
}

// TestRelativeDocPath verifies README link targets resolve to the
// canonical docs/reference/tools/<name>.md path used everywhere else.
func TestRelative_DocPath(t *testing.T) {
	tests := []struct {
		name string
		link string
		want string
	}{
		{"plain_doc", "projects.md", "docs/reference/tools/projects.md"},
		{"hyphenated_doc", "branch-rules.md", "docs/reference/tools/branch-rules.md"},
		{"sibling_doc", "branches.md", "docs/reference/tools/branches.md"},
		{"empty_link", "", "docs/reference/tools/"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := relativeDocPath(tc.link)
			if got != tc.want {
				t.Errorf("relativeDocPath(%q) = %q, want %q", tc.link, got, tc.want)
			}
		})
	}
}

// TestSortedDiff ensures the sorted set difference utility returns
// only the elements in want that are absent from got, in lex order.
func TestSorted_Diff(t *testing.T) {
	got := sortedDiff([]string{"b", "a", "c"}, []string{"a", "x"})
	if !reflect.DeepEqual(got, []string{"b", "c"}) {
		t.Errorf("sortedDiff = %v, want [b c]", got)
	}
	if r := sortedDiff(nil, nil); r != nil {
		t.Errorf("sortedDiff(nil, nil) = %v, want nil", r)
	}
}

// docFixture is a synthetic README plus docs tree for buildReport tests. The
// README claims every row in rows (Domain, count, meta-tool, doc link); docs
// maps a doc basename to its content and is written under docsRoot.
type docFixture struct {
	rows      string
	docs      map[string]string
	ownership string
}

// writeDocFixture materializes the fixture and returns the repo root, the
// docs root and the README path.
func writeDocFixture(t *testing.T, fx docFixture) (repoRoot, docsRoot, readme string) {
	t.Helper()
	repoRoot = t.TempDir()
	docsRoot = filepath.Join(repoRoot, "docs")
	readme = filepath.Join(repoRoot, "README.md")
	if err := os.MkdirAll(docsRoot, 0o750); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	content := "## Domains\n\n| Domain | Tools | Meta-tool | Document |\n| --- | ---: | --- | --- |\n" + fx.rows
	if err := os.WriteFile(readme, []byte(content), 0o600); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	for name, body := range fx.docs {
		if err := os.WriteFile(filepath.Join(docsRoot, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if fx.ownership != "" {
		if err := os.WriteFile(filepath.Join(repoRoot, "doc-ownership.json"), []byte(fx.ownership), 0o600); err != nil {
			t.Fatalf("write ownership: %v", err)
		}
	}
	return repoRoot, docsRoot, readme
}

// overlongLine is longer than bufio.Scanner's default token limit, so any
// line-scanning parser that meets it reports a scan error.
var overlongLine = strings.Repeat("x", 70_000)

// TestBuildReport_UnusableInputs_ReturnsError verifies each input failure
// of buildReport is reported with the stage that failed: a README that is
// missing, an ownership file that does not parse, a docs directory that is
// missing, and a doc whose line the scanner cannot buffer.
func TestBuildReport_UnusableInputs_ReturnsError(t *testing.T) {
	branchRow := "| Branches | 1 | `gitlab_branch` | [branches.md](branches.md) |\n"
	catalog := &catalogSnapshot{Tools: map[string]catalogTool{
		"gitlab_branch_get": {Name: "gitlab_branch_get", Group: "gitlab_branch"},
	}}
	tests := []struct {
		name    string
		fixture docFixture
		mutate  func(t *testing.T, repoRoot, docsRoot, readme string) (docs, readmePath string)
		wantErr string
	}{
		{
			name:    "readme missing",
			fixture: docFixture{rows: branchRow, docs: map[string]string{"branches.md": ""}},
			mutate: func(_ *testing.T, repoRoot, docsRoot, _ string) (string, string) {
				return docsRoot, filepath.Join(repoRoot, "absent.md")
			},
			wantErr: "load doc mapping from",
		},
		{
			name:    "ownership rules do not parse",
			fixture: docFixture{rows: branchRow, docs: map[string]string{"branches.md": ""}, ownership: "{"},
			wantErr: "load ownership rules",
		},
		{
			name:    "docs directory missing",
			fixture: docFixture{rows: branchRow},
			mutate: func(_ *testing.T, repoRoot, _, readme string) (string, string) {
				return filepath.Join(repoRoot, "absent"), readme
			},
			wantErr: "discover docs under",
		},
		{
			name:    "doc line exceeds the scanner buffer",
			fixture: docFixture{rows: branchRow, docs: map[string]string{"branches.md": "## Tools\n" + overlongLine + "\n"}},
			wantErr: "parse docs/branches.md: scan doc",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoRoot, docsRoot, readme := writeDocFixture(t, tt.fixture)
			if tt.mutate != nil {
				docsRoot, readme = tt.mutate(t, repoRoot, docsRoot, readme)
			}
			_, err := buildReport(repoRoot, docsRoot, readme, catalog)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("buildReport() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

// TestBuildReport_ReadmeRowWithoutDoc_ReportsEveryExpectedToolMissing
// verifies a README row whose doc file does not exist still produces an
// entry, under its canonical path, with every expected tool missing and
// nothing documented.
func TestBuildReport_ReadmeRowWithoutDoc_ReportsEveryExpectedToolMissing(t *testing.T) {
	repoRoot, docsRoot, readme := writeDocFixture(t, docFixture{
		rows: "| Branches | 1 | `gitlab_branch` | [branches.md](branches.md) |\n| Tags | 1 | `gitlab_tag` | [tags.md](tags.md) |\n",
		docs: map[string]string{"branches.md": "### `gitlab_branch_get`\n"},
	})
	catalog := &catalogSnapshot{Tools: map[string]catalogTool{
		"gitlab_branch_get": {Name: "gitlab_branch_get", Group: "gitlab_branch"},
		"gitlab_tag_get":    {Name: "gitlab_tag_get", Group: "gitlab_tag"},
	}}

	rep, err := buildReport(repoRoot, docsRoot, readme, catalog)
	if err != nil {
		t.Fatalf("buildReport() error = %v", err)
	}
	if len(rep.Files) != 2 {
		t.Fatalf("Files = %d, want 2", len(rep.Files))
	}
	byDoc := map[string]fileFinding{}
	for _, f := range rep.Files {
		byDoc[f.DocPath] = f
	}
	tags, ok := byDoc["docs/reference/tools/tags.md"]
	if !ok {
		t.Fatalf("Files = %+v, want an entry under the README row's canonical path", rep.Files)
	}
	if tags.Domain != "Tags" {
		t.Errorf("tags entry = %+v, want the canonical path of the README row", tags)
	}
	if !reflect.DeepEqual(tags.Missing, []string{"gitlab_tag_get"}) || tags.DocumentedCount != 0 || tags.Orphan != nil {
		t.Errorf("tags findings = %+v, want every expected tool missing", tags)
	}
	if tags.CountMatches || !tags.ReadmeCountMatches {
		t.Errorf("tags counts = %+v, want doc count mismatch and README count match", tags)
	}
	if rep.Summary.MissingTotal != 1 || rep.Summary.CleanDocs != 1 {
		t.Errorf("summary = %+v, want one missing tool and one clean doc", rep.Summary)
	}
}

// TestBuildReport_TierBadgeDisagrees_ReportsMismatch verifies a doc badge
// that contradicts the catalog tier surfaces as a tier_mismatch finding with
// both labels.
func TestBuildReport_TierBadgeDisagrees_ReportsMismatch(t *testing.T) {
	repoRoot, docsRoot, readme := writeDocFixture(t, docFixture{
		rows: "| Branches | 2 | `gitlab_branch` | [branches.md](branches.md) |\n",
		docs: map[string]string{"branches.md": "### `gitlab_branch_list`\n\n**Read** | **Ultimate**\n\n### `gitlab_branch_get`\n\n**Read** | **Premium**\n"},
	})
	catalog := &catalogSnapshot{Tools: map[string]catalogTool{
		"gitlab_branch_get":  {Name: "gitlab_branch_get", Group: "gitlab_branch", Tier: "ultimate"},
		"gitlab_branch_list": {Name: "gitlab_branch_list", Group: "gitlab_branch", Tier: "premium"},
	}}

	rep, err := buildReport(repoRoot, docsRoot, readme, catalog)
	if err != nil {
		t.Fatalf("buildReport() error = %v", err)
	}
	want := []tierMismatch{
		{Tool: "gitlab_branch_get", Catalog: "Ultimate", Documented: "Premium"},
		{Tool: "gitlab_branch_list", Catalog: "Premium", Documented: "Ultimate"},
	}
	if len(rep.Files) != 1 || !reflect.DeepEqual(rep.Files[0].TierMismatch, want) {
		t.Fatalf("Files = %+v, want the two tier mismatches sorted by tool %+v", rep.Files, want)
	}
	if rep.Summary.TierMismatchTotal != 2 || rep.Summary.DocsWithFindings != 1 {
		t.Errorf("summary = %+v, want two tier mismatch findings", rep.Summary)
	}
}

// TestBuildReport_UnclaimedCatalogTool_ReportsUnassignedEntry verifies a
// catalog tool no README row claims lands in the "(unassigned)" pseudo-entry,
// counts as a finding, and blocks the check gate.
func TestBuildReport_UnclaimedCatalogTool_ReportsUnassignedEntry(t *testing.T) {
	repoRoot, docsRoot, readme := writeDocFixture(t, docFixture{
		rows: "| Branches | 1 | `gitlab_branch` | [branches.md](branches.md) |\n",
		docs: map[string]string{"branches.md": "### `gitlab_branch_get`\n"},
	})
	catalog := &catalogSnapshot{Tools: map[string]catalogTool{
		"gitlab_branch_get":  {Name: "gitlab_branch_get", Group: "gitlab_branch"},
		"gitlab_zzz_list":    {Name: "gitlab_zzz_list", Group: "gitlab_zzz"},
		"gitlab_zzz_archive": {Name: "gitlab_zzz_archive", Group: "gitlab_zzz"},
	}}

	rep, err := buildReport(repoRoot, docsRoot, readme, catalog)
	if err != nil {
		t.Fatalf("buildReport() error = %v", err)
	}
	if len(rep.Files) != 2 {
		t.Fatalf("Files = %d, want the doc entry plus the unassigned entry", len(rep.Files))
	}
	unassigned := rep.Files[1]
	if unassigned.DocPath != unassignedDocPath || !reflect.DeepEqual(unassigned.UnassignedExpected, []string{"gitlab_zzz_archive", "gitlab_zzz_list"}) {
		t.Errorf("unassigned entry = %+v, want both gitlab_zzz tools sorted", unassigned)
	}
	if rep.Summary.UnassignedTotal != 2 || rep.Summary.DocsWithFindings != 1 || rep.Summary.CleanDocs != 1 {
		t.Errorf("summary = %+v, want two unassigned tools counted as one finding doc", rep.Summary)
	}
	if msg := rep.check(); !strings.Contains(msg, "2 unassigned") {
		t.Errorf("check() = %q, want the unassigned count", msg)
	}
}

// TestFilterGapsOnly_MixedFiles_KeepsOnlyFindings verifies the gaps-only
// filter drops clean files, keeps each kind of finding, and recomputes the
// summary over what remains.
func TestFilterGapsOnly_MixedFiles_KeepsOnlyFindings(t *testing.T) {
	rep := report{SchemaVersion: schemaVersion, Files: []fileFinding{
		{DocPath: "docs/clean.md", ReadmeCountMatches: true},
		{DocPath: "docs/missing.md", Missing: []string{"gitlab_a"}, ReadmeCountMatches: true},
		{DocPath: "docs/orphan.md", Orphan: []string{"gitlab_b"}},
		{DocPath: "docs/tier.md", TierMismatch: []tierMismatch{{Tool: "gitlab_c"}}, ReadmeCountMatches: true},
	}}

	got := rep.filterGapsOnly()
	paths := make([]string, 0, len(got.Files))
	for _, f := range got.Files {
		paths = append(paths, f.DocPath)
	}
	if !reflect.DeepEqual(paths, []string{"docs/missing.md", "docs/orphan.md", "docs/tier.md"}) {
		t.Errorf("filtered paths = %v, want the three files with findings", paths)
	}
	want := reportSummary{Docs: 3, DocsWithFindings: 3, MissingTotal: 1, OrphanTotal: 1, TierMismatchTotal: 1, ReadmeCountDriftDocs: 1}
	if got.Summary != want || got.SchemaVersion != schemaVersion {
		t.Errorf("filtered report = %+v, want summary %+v", got, want)
	}
}

// TestBuildDocEntries_UnrelatablePath_KeepsAbsolutePath verifies a doc path
// that cannot be expressed relative to the repo root is keyed as-is.
func TestBuildDocEntries_UnrelatablePath_KeepsAbsolutePath(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "doc.md")
	entries, absByRel, relByBase := buildDocEntries("relative-root", []string{abs})
	if entries[abs] == nil || absByRel[abs] != abs || relByBase["doc.md"] != abs {
		t.Fatalf("entries = %v, absByRel = %v, relByBase = %v; want the absolute path kept", entries, absByRel, relByBase)
	}
}

// captureStdout swaps os.Stdout for a temporary file until the test ends and
// returns a reader for what was written, so the "-" output path can be
// observed.
func captureStdout(t *testing.T) func() string {
	t.Helper()
	file, err := os.Create(filepath.Join(t.TempDir(), "stdout"))
	if err != nil {
		t.Fatalf("create stdout capture: %v", err)
	}
	previous := os.Stdout
	os.Stdout = file
	t.Cleanup(func() {
		os.Stdout = previous
		_ = file.Close()
	})
	return func() string {
		data, readErr := os.ReadFile(file.Name())
		if readErr != nil {
			t.Fatalf("read stdout capture: %v", readErr)
		}
		return string(data)
	}
}

// TestWriteReport_Scenarios_WritesStdoutOrFile verifies the "-" sentinel
// writes to stdout, a nested output path gets its directories created, and a
// parent that is a file is reported as an error.
func TestWriteReport_Scenarios_WritesStdoutOrFile(t *testing.T) {
	t.Run("stdout sentinel", func(t *testing.T) {
		stdout := captureStdout(t)
		if err := writeReport("-", []byte("{}\n")); err != nil {
			t.Fatalf("writeReport(-) error = %v", err)
		}
		if got := stdout(); got != "{}\n" {
			t.Errorf("stdout = %q, want the report", got)
		}
	})
	t.Run("nested file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "plan", "nested", "backlog.json")
		if err := writeReport(path, []byte("{}\n")); err != nil {
			t.Fatalf("writeReport() error = %v", err)
		}
		if data, err := os.ReadFile(path); err != nil || string(data) != "{}\n" {
			t.Errorf("written report = %q, %v; want the content", data, err)
		}
	})
	t.Run("parent is a file", func(t *testing.T) {
		blocker := filepath.Join(t.TempDir(), "blocker")
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatalf("write blocker: %v", err)
		}
		if err := writeReport(filepath.Join(blocker, "backlog.json"), []byte("{}\n")); err == nil {
			t.Fatal("writeReport() error = nil, want the directory creation failure")
		}
	})
}

// TestParseDocTools_UnreadableDoc_ReturnsError verifies the doc parser
// reports a doc it cannot open and a doc whose line the scanner cannot
// buffer.
func TestParseDocTools_UnreadableDoc_ReturnsError(t *testing.T) {
	tests := []struct {
		name    string
		content string
		absent  bool
		wantErr string
	}{
		{name: "missing doc", absent: true, wantErr: "open doc"},
		{name: "overlong line", content: overlongLine + "\n", wantErr: "scan doc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "doc.md")
			if !tt.absent {
				if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
					t.Fatalf("write doc: %v", err)
				}
			}
			_, err := parseDocTools(path)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("parseDocTools() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

// TestParseTierBadge_MissingDoc_ReportsNoBadge verifies a doc that cannot be
// opened yields no badge rather than an error, matching the best-effort
// contract of the badge lookup.
func TestParseTierBadge_MissingDoc_ReportsNoBadge(t *testing.T) {
	badge, found := parseTierBadge(filepath.Join(t.TempDir(), "absent.md"), "gitlab_branch_get")
	if found || badge != "" {
		t.Fatalf("parseTierBadge() = %q, %v; want no badge", badge, found)
	}
}

// writeEmptyDoc creates an empty file at path for the discovery fixtures.
func writeEmptyDoc(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestDiscoverDocFiles_Scenarios_ListsMarkdownOnly verifies discovery skips
// subdirectories, non-Markdown files and the README, and reports a missing
// docs root.
func TestDiscoverDocFiles_Scenarios_ListsMarkdownOnly(t *testing.T) {
	t.Run("missing root", func(t *testing.T) {
		if _, err := discoverDocFiles(filepath.Join(t.TempDir(), "absent")); err == nil || !strings.Contains(err.Error(), "read docs root") {
			t.Fatalf("discoverDocFiles() error = %v, want read docs root", err)
		}
	})
	t.Run("mixed entries", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, "nested"), 0o750); err != nil {
			t.Fatalf("mkdir nested: %v", err)
		}
		writeEmptyDoc(t, filepath.Join(root, "README.md"))
		writeEmptyDoc(t, filepath.Join(root, "notes.txt"))
		writeEmptyDoc(t, filepath.Join(root, "branches.md"))
		got, err := discoverDocFiles(root)
		if err != nil {
			t.Fatalf("discoverDocFiles() error = %v", err)
		}
		if !reflect.DeepEqual(got, []string{filepath.Join(root, "branches.md")}) {
			t.Errorf("discoverDocFiles() = %v, want branches.md only", got)
		}
	})
}

// TestLoadOwnershipRules_PathIsDirectory_ReturnsReadError verifies a read
// failure other than "not found" is reported instead of being treated as an
// absent rule file.
func TestLoadOwnershipRules_PathIsDirectory_ReturnsReadError(t *testing.T) {
	_, err := loadOwnershipRules(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "read ") {
		t.Fatalf("loadOwnershipRules(directory) error = %v, want a read error", err)
	}
}

// TestParseDomainsTable_MalformedTables_ReturnError verifies the README
// parser reports a README it cannot open, a tool count that does not fit an
// int, a row the scanner cannot buffer, and a README without Domains rows.
func TestParseDomainsTable_MalformedTables_ReturnError(t *testing.T) {
	tests := []struct {
		name    string
		content string
		absent  bool
		wantErr string
	}{
		{name: "missing readme", absent: true, wantErr: "open readme"},
		{name: "tool count overflows", content: "## Domains\n| Branches | 99999999999999999999 | `gitlab_branch` | [branches.md](branches.md) |\n", wantErr: "invalid tool count"},
		{name: "overlong row", content: "## Domains\n| " + overlongLine + " |\n", wantErr: "scan readme"},
		{name: "no rows", content: "## Domains\n\nNothing tabulated.\n", wantErr: "no Domains rows parsed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "README.md")
			if !tt.absent {
				if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
					t.Fatalf("write readme: %v", err)
				}
			}
			_, err := parseDomainsTable(path)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("parseDomainsTable() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

// TestSortedUnique_Scenarios_DedupesAndSorts verifies duplicates collapse,
// the result is sorted, and an empty input yields nil.
func TestSortedUnique_Scenarios_DedupesAndSorts(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   []string
	}{
		{name: "empty", values: nil, want: nil},
		{name: "duplicates", values: []string{"b", "a", "b"}, want: []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sortedUnique(tt.values); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("sortedUnique(%v) = %v, want %v", tt.values, got, tt.want)
			}
		})
	}
}

// TestResolveOutputPath_ByPathKind verifies the -output resolution rules:
// relative paths anchor to the repo root, while the "-" stdout sentinel and
// absolute paths pass through verbatim (the absolute case is the guard that
// stops an absolute -output from being joined onto repoRoot and writing inside
// the repo).
func TestResolveOutputPath_ByPathKind(t *testing.T) {
	root := filepath.Join("home", "user", "repo")
	abs := filepath.Join(string(filepath.Separator)+"tmp", "out.json")
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"relative anchors to repo root", "plan/backlog.json", filepath.Join(root, "plan/backlog.json")},
		{"dash passes through as stdout", "-", "-"},
		{"absolute passes through unchanged", abs, abs},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveOutputPath(root, c.input); got != c.want {
				t.Errorf("resolveOutputPath(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}

// TestLoadOwnershipRules_ByFileState verifies the doc-ownership.json loader
// across its three input states: a valid file (groups/prefixes parsed, the
// human-only "note" field ignored), a missing file (empty non-error ruleset so
// the auditor still runs on a bare checkout), and malformed JSON (an error
// rather than partial rules).
func TestLoadOwnershipRules_ByFileState(t *testing.T) {
	dir := t.TempDir()

	t.Run("valid file parses groups and prefixes and ignores note", func(t *testing.T) {
		valid := filepath.Join(dir, "doc-ownership.json")
		const body = `{
  "ci-cd.md": {
    "note": "rationale that the parser must ignore",
    "groups": ["gitlab_ci_variable"],
    "prefixes": ["gitlab_branch_"]
  }
}`
		if err := os.WriteFile(valid, []byte(body), 0o600); err != nil {
			t.Fatalf("write valid file: %v", err)
		}
		rules, err := loadOwnershipRules(valid)
		if err != nil {
			t.Fatalf("loadOwnershipRules(valid) error: %v", err)
		}
		got, ok := rules["ci-cd.md"]
		if !ok {
			t.Fatalf("ci-cd.md missing from parsed rules: %v", rules)
		}
		if !reflect.DeepEqual(got.Groups, []string{"gitlab_ci_variable"}) {
			t.Errorf("Groups = %v, want [gitlab_ci_variable]", got.Groups)
		}
		if !reflect.DeepEqual(got.Prefixes, []string{"gitlab_branch_"}) {
			t.Errorf("Prefixes = %v, want [gitlab_branch_]", got.Prefixes)
		}
	})

	t.Run("missing file yields empty ruleset without error", func(t *testing.T) {
		rules, err := loadOwnershipRules(filepath.Join(dir, "does-not-exist.json"))
		if err != nil {
			t.Fatalf("loadOwnershipRules(missing) error: %v", err)
		}
		if len(rules) != 0 {
			t.Errorf("missing file produced %d rules, want 0", len(rules))
		}
	})

	t.Run("malformed JSON returns a parse error", func(t *testing.T) {
		bad := filepath.Join(dir, "bad.json")
		if werr := os.WriteFile(bad, []byte("{not json"), 0o600); werr != nil {
			t.Fatalf("write bad file: %v", werr)
		}
		if _, derr := loadOwnershipRules(bad); derr == nil {
			t.Error("loadOwnershipRules(malformed) returned nil error, want parse error")
		}
	})
}

// TestBuildReport_LiveBaseline is the integration smoke test against
// the real catalog and the real docs/reference/tools tree. It runs the full
// pipeline and asserts the report shape, not the exact counts (which
// drift as the catalog grows). The point of this test is to catch
// regressions in the catalog walk, the README parser, and the doc
// parser when any of them changes shape.
func TestBuildReport_LiveBaseline(t *testing.T) {
	repoRoot, err := repoRootFromTest()
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}
	readme := filepath.Join(repoRoot, "docs/reference/tools/README.md")
	docsRoot := filepath.Join(repoRoot, "docs/reference/tools")

	catalog, err := loadCatalog(repoRoot)
	if err != nil {
		t.Fatalf("loadCatalog: %v", err)
	}
	if len(catalog.Tools) < 1000 {
		t.Errorf("catalog tool count = %d, expected > 1000", len(catalog.Tools))
	}

	rep, err := buildReport(repoRoot, docsRoot, readme, catalog)
	if err != nil {
		t.Fatalf("buildReport: %v", err)
	}
	if rep.SchemaVersion != schemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", rep.SchemaVersion, schemaVersion)
	}
	if len(rep.Files) < 30 {
		t.Errorf("Files = %d, want >= 30", len(rep.Files))
	}
	if rep.Summary.Docs != len(rep.Files) {
		t.Errorf("Summary.Docs = %d, Files = %d (must match)", rep.Summary.Docs, len(rep.Files))
	}

	// Every README row must have produced an entry in Files (even if
	// its doc file does not yet exist on disk). The "(unassigned)"
	// pseudo-entry surfaces catalog tools that no README row claims;
	// it is allowed but only when there is actually something to
	// assign.
	for _, f := range rep.Files {
		if strings.HasPrefix(f.DocPath, "docs/reference/tools/") {
			continue
		}
		if f.DocPath == "(unassigned)" {
			continue
		}
		t.Errorf("file %q has unexpected doc path prefix", f.DocPath)
	}

	// Sanity: at least one routed-tool override should be in effect,
	// so verify the branch-rules.md expected set is non-empty.
	var branchRules *fileFinding
	for i := range rep.Files {
		if rep.Files[i].DocPath == "docs/reference/tools/branch-rules.md" {
			branchRules = &rep.Files[i]
			break
		}
	}
	if branchRules == nil {
		t.Fatal("branch-rules.md entry missing from report")
	}
	if branchRules.ExpectedCount == 0 {
		t.Errorf("branch-rules.md ExpectedCount = 0, want > 0 (override should have assigned gitlab_list_branch_rules)")
	}
}

// repoRootFromTest walks upward from the test working directory to
// find the module root (where go.mod lives). Used by the live
// baseline test so it runs from any cwd.
func repoRootFromTest() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	current := cwd
	for {
		if _, statErr := os.Stat(filepath.Join(current, "go.mod")); statErr == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", os.ErrNotExist
		}
		current = parent
	}
}
