// main_test.go drives the llms generator end to end through run and covers
// each renderer, validator and helper on its own.
//
// Two surfaces feed the tests. The canned one (cannedCatalog) is a handful of
// tools, resources and prompts with known shapes, so the rendered text can be
// asserted line by line and check mode can be driven in milliseconds. The real
// one (defaultLLMSSurface) is built once, in
// TestRun_RealSurfaceReproducesCommittedFiles, whose output must match the
// committed files byte for byte: that is the same gate make check-llms runs.
package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/mcpsurface"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// cannedVersion is what the VERSION file of a canned project root holds.
const cannedVersion = "9.9.9-test"

// cannedHeader is the one-line summary every canned long-form file opens with.
const cannedHeader = "> Version 9.9.9-test | up to 3 tools | 2 base meta-tools; 3 self-managed enterprise meta-tools; " +
	"4 GitLab.com Enterprise meta-tools | 2 dynamic tools | 2 resources | 1 prompts\n"

// generatedFileNames lists every file run writes, in the order it writes them.
var generatedFileNames = []string{
	llmsFileName, llmsFullFileName, llmsMediumFileName,
	llmsFullMetaFileName, llmsFullIndividualFileName, llmsFullCapabilityFileName,
}

// cannedCatalog returns a small catalog whose every field exercises one
// rendering path: a GitLab.com individual surface one tool larger than the
// self-managed one, a meta surface that grows by one tool on self-managed
// Enterprise and by two on GitLab.com, action maps with and without output
// schemas, a static resource plus a template, and a prompt with a required and
// an undocumented argument.
func cannedCatalog() llmsCatalog {
	issueList := &mcp.Tool{
		Name:        "gitlab_issue_list",
		Description: "List issues in a project. Supports filtering by state and labels.\n\nSecond paragraph the full reference drops.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id":  map[string]any{"type": "string", "description": "Project ID or path,required"},
				"labels":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Label names"},
				"assignee_id": map[string]any{"type": []any{"integer", "null"}},
				"malformed":   "not a schema",
			},
			"required": []any{"project_id", 42},
		},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)},
	}
	issueGet := &mcp.Tool{Name: "gitlab_issue_get", Description: "Get one issue.", InputSchema: map[string]any{"type": "object"}}
	projectGet := &mcp.Tool{Name: "gitlab_project_get", Description: "Get one project.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}

	issue := &mcp.Tool{
		Name: "gitlab_issue", Title: "Issues", Description: "Manage issues. Covers notes and links.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(true), OpenWorldHint: new(false)},
	}
	project := &mcp.Tool{Name: "gitlab_project", Description: "Manage projects."}
	epic := &mcp.Tool{Name: "gitlab_epic", Title: "Epics", Description: "Manage epics."}
	orbit := &mcp.Tool{Name: "gitlab_orbit", Description: "Query the Knowledge Graph."}

	find := &mcp.Tool{
		Name: mcpsurface.DynamicFindToolName, Title: "Find action",
		Description: "Search the local GitLab action catalog. Returns the matching action IDs.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"query": map[string]any{"type": "string", "description": "Search text"}},
			"required":   []any{"query"},
		},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(false)},
	}
	execute := &mcp.Tool{Name: mcpsurface.DynamicExecuteActionToolName, Description: "Execute one canonical action."}

	return llmsCatalog{
		Individual:              []*mcp.Tool{issueList, issueGet, projectGet},
		IndividualSelfManaged:   []*mcp.Tool{issueList, issueGet},
		MetaBase:                []*mcp.Tool{issue, project},
		MetaEnterprise:          []*mcp.Tool{issue, project, epic},
		MetaGitLabComEnterprise: []*mcp.Tool{issue, project, epic, orbit},
		Dynamic:                 []*mcp.Tool{find, execute},
		MetaRoutes: map[string]toolutil.ActionMap{
			"gitlab_issue": {
				"list": {OutputSchema: map[string]any{"type": "object", "properties": map[string]any{"issues": map[string]any{"type": "array"}}}},
				"get":  {},
			},
			"gitlab_epic": {"list": {OutputSchema: map[string]any{"type": "object"}}},
		},
		Resources: []*mcp.Resource{
			{URI: "gitlab://server/info", Name: "Server info", MIMEType: "application/json", Description: "Server metadata"},
		},
		ResourceTemplates: []*mcp.ResourceTemplate{
			{URITemplate: "gitlab://projects/{id}", Name: "Project", MIMEType: "application/json", Description: "One project"},
		},
		Prompts: []*mcp.Prompt{{
			Name: "review_mr", Description: "Review one merge request diff. Summarizes the changes.",
			Arguments: []*mcp.PromptArgument{{Name: "project_id", Description: "Project ID", Required: true}, {Name: "mr_iid"}},
		}},
	}
}

// canned bundles a canned surface with the two client identities its
// functions route on, and records whether run released the stub client.
//
// The clients are distinct zero values: none of the functions dereferences
// them, but run must hand the GitLab.com one to the calls that describe
// GitLab.com, and routing on identity is how a mix-up shows up in the counts.
type canned struct {
	surface   llmsSurface
	stub      *gitlabclient.Client
	gitLabCom *gitlabclient.Client
	closed    bool
}

// newCanned wraps catalog in the shape run introspects.
func newCanned(catalog llmsCatalog) *canned {
	c := &canned{stub: new(gitlabclient.Client), gitLabCom: new(gitlabclient.Client)}
	c.surface = llmsSurface{
		newStubClient: func() (*gitlabclient.Client, func()) {
			return c.stub, func() { c.closed = true }
		},
		newGitLabComClient: func() *gitlabclient.Client { return c.gitLabCom },
		resources: func(*gitlabclient.Client) ([]*mcp.Resource, []*mcp.ResourceTemplate) {
			return catalog.Resources, catalog.ResourceTemplates
		},
		listTools: func(client *gitlabclient.Client, meta bool) []*mcp.Tool {
			switch {
			case meta:
				return catalog.MetaBase
			case client == c.gitLabCom:
				return catalog.Individual
			default:
				return catalog.IndividualSelfManaged
			}
		},
		listToolsEnterprise: func(client *gitlabclient.Client) []*mcp.Tool {
			if client == c.gitLabCom {
				return catalog.MetaGitLabComEnterprise
			}
			return catalog.MetaEnterprise
		},
		dynamicTools: func(*gitlabclient.Client) []*mcp.Tool { return catalog.Dynamic },
		actionCatalog: func(*gitlabclient.Client) *actioncatalog.Catalog {
			return actioncatalog.FromActionMaps(catalog.MetaRoutes)
		},
		prompts: func(*gitlabclient.Client) []*mcp.Prompt { return catalog.Prompts },
	}
	return c
}

// writeFile writes one file under dir or fails the test.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// readGenerated reads one generated file under dir or fails the test.
func readGenerated(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

// projectRootWithVersion creates a project root holding go.mod and the given
// VERSION, makes it the working directory, and returns it.
func projectRootWithVersion(t *testing.T, version string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example\n")
	writeFile(t, dir, "VERSION", version+"\n")
	t.Chdir(dir)
	return dir
}

// generateCanned runs the generator over the canned surface into a fresh
// project root and returns that root.
func generateCanned(t *testing.T) string {
	t.Helper()
	dir := projectRootWithVersion(t, cannedVersion)
	if err := run(newCanned(cannedCatalog()).surface, false); err != nil {
		t.Fatalf("run() error: %v", err)
	}
	return dir
}

// requireFragments opens one subtest per fragment and fails those absent
// from got.
func requireFragments(t *testing.T, got string, fragments []string) {
	t.Helper()
	for _, want := range fragments {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(got, want) {
				t.Errorf("output missing %q\ngot:\n%s", want, got)
			}
		})
	}
}

// rejectFragments opens one subtest per fragment and fails those present
// in got.
func rejectFragments(t *testing.T, got string, fragments []string) {
	t.Helper()
	for _, unwanted := range fragments {
		t.Run(unwanted, func(t *testing.T) {
			if strings.Contains(got, unwanted) {
				t.Errorf("output must not contain %q\ngot:\n%s", unwanted, got)
			}
		})
	}
}

// twoCounts extracts the two integers a pattern captures from text.
func twoCounts(t *testing.T, text, pattern string) (int, int) {
	t.Helper()
	match := regexp.MustCompile(pattern).FindStringSubmatch(text)
	if len(match) != 3 {
		t.Fatalf("pattern %q not found in output", pattern)
	}
	first, err := strconv.Atoi(match[1])
	if err != nil {
		t.Fatalf("parse %q: %v", match[1], err)
	}
	second, err := strconv.Atoi(match[2])
	if err != nil {
		t.Fatalf("parse %q: %v", match[2], err)
	}
	return first, second
}

// TestRun_WritesLLMSTxt verifies the concise index run writes for the canned
// surface: the version and every population count in the summary paragraph,
// the meta-tool domains, the dynamic, meta, resource and prompt listings with
// first-sentence descriptions, and absolute documentation links. The result
// must also pass the llms.txt validator the generator applies to itself.
func TestRun_WritesLLMSTxt(t *testing.T) {
	dir := generateCanned(t)
	got := readGenerated(t, dir, llmsFileName)

	if err := validateLLMSTxt(got); err != nil {
		t.Fatalf("generated llms.txt fails validation: %v", err)
	}
	requireFragments(t, got, []string{
		"# gitlab-mcp-server\n\n> A Model Context Protocol (MCP) server",
		"gitlab-mcp-server v9.9.9-test is a single static binary",
		"up to 3 individual MCP tools across 2 GitLab API domains, 2 base meta-tools, 3 self-managed enterprise meta-tools, 4 GitLab.com Enterprise meta-tools,\n",
		"a default 2-tool dynamic find/execute surface, 2 resources, 1 prompts, and 4 MCP capabilities",
		"Tool domains:\n\nIssue, Project.\n\n",
		"- gitlab_find_action: Search the local GitLab action catalog.\n- gitlab_execute_action: Execute one canonical action.\n\n",
		"When GITLAB_MCP_TOOL_SURFACE=meta, 2 domain meta-tools are registered instead of\nup to 3 individual tools. " +
			"Enterprise/Premium entries register 3 meta-tools on self-managed GitLab,\nor 4 on GitLab.com when Orbit is available.",
		"- gitlab_issue: Manage issues.\n- gitlab_project: Manage projects.\n\n",
		"2 read-only resources (26 resource kinds are subscribable",
		"- gitlab://server/info: Server info\n- gitlab://projects/{id}: Project\n\n",
		"1 prompts:\n\n- review_mr: Review one merge request diff.\n\n",
		"## Documentation\n\n- [Getting started](https://github.com/jmrplens/gitlab-mcp-server/blob/main/docs/getting-started.md): Installation and first-run guide\n",
		"## Optional\n\n- [Medium LLM reference](https://jmrp.io/docs/gitlab-mcp-server/llms-medium.txt): ",
		"- [Full LLM reference](https://jmrp.io/docs/gitlab-mcp-server/llms-full.txt): ",
	})
}

// TestRun_WritesLLMSFullTxt verifies the long-form reference for the canned
// surface: the header counts, the dynamic tool with its parameter table and
// annotations, a meta-tool with its per-action output schema (only the action
// that declares one), the enterprise-only section listing exactly the tools
// the base surface lacks, individual tools grouped by domain with singular and
// plural headings and first-paragraph descriptions, resources with MIME type
// and description, and prompt arguments with the required marker and the
// name fallback for an undocumented argument.
func TestRun_WritesLLMSFullTxt(t *testing.T) {
	dir := generateCanned(t)
	got := readGenerated(t, dir, llmsFullFileName)

	if err := validateLLMSFullTxt(got); err != nil {
		t.Fatalf("generated llms-full.txt fails validation: %v", err)
	}
	requireFragments(t, got, []string{
		"# gitlab-mcp-server. Full Reference\n\n" + cannedHeader,
		"## Dynamic Toolset\n\nDynamic mode is the default",
		"### gitlab_find_action\n\n**Find action**\n\nSearch the local GitLab action catalog. Returns the matching action IDs.\n\n" +
			"**Parameters:**\n\n- `query` (string) (required): Search text\n\n" +
			"Annotations: readOnly=true, destructive=false, idempotent=true, openWorld=false\n\n",
		"### gitlab_execute_action\n\nExecute one canonical action.\n\n\n## Meta-Tools\n\n",
		"### gitlab_issue\n\n**Issues**\n\nManage issues. Covers notes and links.\n\n" +
			"Annotations: readOnly=false, destructive=true, idempotent=false, openWorld=false\n\n" +
			"**Action Output Schemas:**\n\n<details><summary>list</summary>\n\n```json\n" +
			`{"properties":{"issues":{"type":"array"}},"type":"object"}` + "\n```\n\n</details>\n\n",
		"### gitlab_project\n\nManage projects.\n\n\n## Enterprise-Only Meta-Tools\n\n" +
			"These 2 tools require GITLAB_TIER=premium or GITLAB_TIER=ultimate (or a detected Premium/Ultimate license). " +
			"GitLab.com-only tools, including Orbit, also require GITLAB_URL=https://gitlab.com.\n\n" +
			"### gitlab_epic\n\n**Epics**\n\nManage epics.\n\n\n**Action Output Schemas:**\n\n<details><summary>list</summary>",
		"### gitlab_orbit\n\nQuery the Knowledge Graph.\n\n\n## Individual Tools\n\n" +
			"When `GITLAB_MCP_TOOL_SURFACE=individual`, up to 3 individual tools are registered on GitLab.com Enterprise/Premium; " +
			"self-managed Enterprise/Premium registers 2.\nGrouped by domain:\n\n",
		"### issue (2 tools)\n\n#### gitlab_issue_list\n\nList issues in a project. Supports filtering by state and labels.\n\n" +
			"**Parameters:**\n\n- `assignee_id` (integer)\n- `labels` (array of strings): Label names\n- `project_id` (string) (required): Project ID or path\n\n" +
			"Annotations: readOnly=true, destructive=false, idempotent=true, openWorld=true\n\n" +
			"#### gitlab_issue_get\n\nGet one issue.\n\n\n",
		"### project (1 tool)\n\n#### gitlab_project_get\n\nGet one project.\n\n" +
			"Annotations: readOnly=true, destructive=false, idempotent=false, openWorld=true\n\n",
		"## Resources\n\n2 resources: read-only GitLab data",
		"### Server info\n\n- **URI**: `gitlab://server/info`\n- **MIME**: application/json\n- **Description**: Server metadata\n\n" +
			"### Project\n\n- **URI Template**: `gitlab://projects/{id}`\n- **MIME**: application/json\n- **Description**: One project\n\n",
		"## Prompts\n\n1 prompt templates for AI-assisted GitLab workflows.\n\n### review_mr\n\nReview one merge request diff. Summarizes the changes.\n\n" +
			"**Arguments:**\n\n- `project_id` (required): Project ID\n- `mr_iid`: mr_iid\n\n",
	})
	rejectFragments(t, got, []string{"Second paragraph the full reference drops", "`malformed`"})
}

// TestRun_WritesCompanionFiles verifies the four companions of llms-full.txt:
// each opens with the shared header under its own subtitle, the medium
// reference lists every meta-tool's actions and every individual tool without
// any per-action schema, and each split carries only its own sections.
func TestRun_WritesCompanionFiles(t *testing.T) {
	dir := generateCanned(t)

	tests := []struct {
		name   string
		file   string
		want   []string
		reject []string
	}{
		{
			name: "medium reference",
			file: llmsMediumFileName,
			want: []string{
				"# gitlab-mcp-server. Medium Reference\n\n" + cannedHeader,
				"Fetch the exact schema for one action from `gitlab://tools/{id}`, or read llms-full.txt for the complete schemas.\n\n## Dynamic Toolset\n\n",
				"## Meta-Tools\n\nEnabled with `GITLAB_MCP_TOOL_SURFACE=meta`.",
				"### gitlab_issue\n\n**Issues**\n\nManage issues.\n\nActions (2): get, list\n\n### gitlab_project\n\nManage projects.\n\n### gitlab_epic\n\n**Epics**\n\nManage epics.\n\nActions (1): list\n\n### gitlab_orbit\n\nQuery the Knowledge Graph.\n\n",
				"## Individual Tools\n\nEnabled with `GITLAB_MCP_TOOL_SURFACE=individual`: 3 tools, one per operation.\n\n" +
					"- gitlab_issue_list: List issues in a project.\n- gitlab_issue_get: Get one issue.\n- gitlab_project_get: Get one project.\n\n",
				"## Resources\n\n2 resources",
				"## Prompts\n\n1 prompt templates",
			},
			reject: []string{"<details>", "Action Output Schemas", "#### gitlab_issue_list"},
		},
		{
			name:   "meta-tools split",
			file:   llmsFullMetaFileName,
			want:   []string{"# gitlab-mcp-server. Meta-Tools Reference\n\n" + cannedHeader, "## Dynamic Toolset\n", "## Meta-Tools\n", "## Enterprise-Only Meta-Tools\n", "### gitlab_orbit\n"},
			reject: []string{"## Individual Tools", "## Resources", "## Prompts"},
		},
		{
			name:   "individual tools split",
			file:   llmsFullIndividualFileName,
			want:   []string{"# gitlab-mcp-server. Individual Tools Reference\n\n" + cannedHeader, "## Individual Tools\n", "#### gitlab_issue_list\n"},
			reject: []string{"## Dynamic Toolset", "## Meta-Tools", "## Resources", "## Prompts"},
		},
		{
			name:   "resources and prompts split",
			file:   llmsFullCapabilityFileName,
			want:   []string{"# gitlab-mcp-server. Resources and Prompts Reference\n\n" + cannedHeader, "## Resources\n", "## Prompts\n", "### review_mr\n"},
			reject: []string{"## Dynamic Toolset", "## Meta-Tools", "## Individual Tools"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := readGenerated(t, dir, tt.file)
			requireFragments(t, got, tt.want)
			rejectFragments(t, got, tt.reject)
		})
	}
}

// TestRun_CheckModeAcceptsFreshOutput verifies check mode passes right after
// generation, releases the stub client, and writes nothing: llms.txt is
// rewritten with CRLF endings first, which check mode tolerates, and still has
// them afterwards.
func TestRun_CheckModeAcceptsFreshOutput(t *testing.T) {
	dir := generateCanned(t)
	crlf := strings.ReplaceAll(readGenerated(t, dir, llmsFileName), "\n", "\r\n")
	writeFile(t, dir, llmsFileName, crlf)

	c := newCanned(cannedCatalog())
	if err := run(c.surface, true); err != nil {
		t.Fatalf("run(check) error: %v", err)
	}
	if !c.closed {
		t.Error("run(check) did not release the stub client")
	}
	if readGenerated(t, dir, llmsFileName) != crlf {
		t.Error("run(check) rewrote llms.txt; check mode must not write")
	}
}

// TestRun_CheckModeReportsDrift verifies check mode names the first stale or
// missing file and leaves it as it found it, for the two primary files and
// for the companions, whose failures carry the file name as context.
func TestRun_CheckModeReportsDrift(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		remove  bool
		wantErr string
	}{
		{name: "stale llms.txt", file: llmsFileName, wantErr: "write llms.txt: llms.txt is out of date; run go run ./cmd/gen_llms/"},
		{name: "stale llms-full.txt", file: llmsFullFileName, wantErr: "write llms-full.txt: llms-full.txt is out of date"},
		{name: "stale medium companion", file: llmsMediumFileName, wantErr: "write llms-medium.txt: llms-medium.txt is out of date"},
		{name: "missing individual companion", file: llmsFullIndividualFileName, remove: true, wantErr: "write llms-full-individual-tools.txt: "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := generateCanned(t)
			stillDisturbed := disturbGenerated(t, dir, tt.file, tt.remove)

			err := run(newCanned(cannedCatalog()).surface, true)
			if err == nil {
				t.Fatal("run(check) error = nil, want drift error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("run(check) error = %q, want it to contain %q", err, tt.wantErr)
			}
			if tt.remove && !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("run(check) error = %v, want fs.ErrNotExist", err)
			}
			stillDisturbed(t)
		})
	}
}

// disturbGenerated makes one generated file stale, or removes it, and returns
// the check that it is still in that state, which is how a check-mode run
// proves it wrote nothing.
func disturbGenerated(t *testing.T, dir, name string, remove bool) func(t *testing.T) {
	t.Helper()
	path := filepath.Join(dir, name)
	if remove {
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove %s: %v", name, err)
		}
		return func(t *testing.T) {
			t.Helper()
			if _, statErr := os.Stat(path); !errors.Is(statErr, fs.ErrNotExist) {
				t.Error("run(check) recreated the missing file; check mode must not write")
			}
		}
	}
	writeFile(t, dir, name, "# stale\n")
	return func(t *testing.T) {
		t.Helper()
		if readGenerated(t, dir, name) != "# stale\n" {
			t.Error("run(check) rewrote the stale file; check mode must not write")
		}
	}
}

// TestRun_RequiresProjectRoot verifies run refuses to guess where the files
// live: with no go.mod above the working directory it reports the missing root
// before opening any client.
func TestRun_RequiresProjectRoot(t *testing.T) {
	t.Chdir(t.TempDir())
	c := newCanned(cannedCatalog())

	err := run(c.surface, false)
	if err == nil || !strings.Contains(err.Error(), "project root") {
		t.Fatalf("run() error = %v, want the missing project root", err)
	}
	if c.closed {
		t.Error("run() opened the stub client before locating the project root")
	}
}

// TestRun_RealSurfaceReproducesCommittedFiles builds the real surface once and
// generates into a scratch project root carrying the repository's VERSION,
// then holds the six outputs to the committed files byte for byte (line
// endings normalized): the same gate make check-llms runs, so a surface change
// that forgot to regenerate fails here too. It then checks two facts only the
// real surface can show: the GitLab.com Enterprise meta surface exceeds the
// self-managed one, with Orbit in the enterprise-only section, and the
// individual surface is likewise larger on GitLab.com.
func TestRun_RealSurfaceReproducesCommittedFiles(t *testing.T) {
	repoRoot, err := mcpsurface.ProjectRoot()
	if err != nil {
		t.Fatalf("ProjectRoot() error: %v", err)
	}
	committed := map[string]string{}
	for _, name := range generatedFileNames {
		data, readErr := os.ReadFile(filepath.Join(repoRoot, name))
		if readErr != nil {
			t.Fatalf("read committed %s: %v", name, readErr)
		}
		committed[name] = normalizeLineEndings(string(data))
	}

	dir := projectRootWithVersion(t, readVersion(repoRoot))
	if runErr := run(defaultLLMSSurface(), false); runErr != nil {
		t.Fatalf("run() error: %v", runErr)
	}

	for _, name := range generatedFileNames {
		t.Run(name, func(t *testing.T) {
			got := normalizeLineEndings(readGenerated(t, dir, name))
			if got != committed[name] {
				t.Errorf("%s differs from the committed file (%d vs %d bytes); run go run ./cmd/gen_llms/", name, len(got), len(committed[name]))
			}
		})
	}
	t.Run("gitlab.com enterprise meta surface adds orbit", func(t *testing.T) {
		full := readGenerated(t, dir, llmsFullFileName)
		start := strings.Index(full, "## Enterprise-Only Meta-Tools\n")
		end := strings.Index(full, "## Individual Tools\n")
		if start < 0 || end < start {
			t.Fatal("llms-full.txt lacks the enterprise-only section before the individual tools")
		}
		if !strings.Contains(full[start:end], "### gitlab_orbit\n") {
			t.Error("enterprise-only section does not list gitlab_orbit")
		}
		selfManaged, gitLabCom := twoCounts(t, readGenerated(t, dir, llmsFileName),
			`(\d+) self-managed enterprise meta-tools, (\d+) GitLab\.com Enterprise meta-tools`)
		if gitLabCom <= selfManaged {
			t.Errorf("GitLab.com enterprise meta-tools = %d, want more than the %d self-managed ones", gitLabCom, selfManaged)
		}
	})
	t.Run("individual surface is larger on gitlab.com", func(t *testing.T) {
		gitLabCom, selfManaged := twoCounts(t, readGenerated(t, dir, llmsFullFileName),
			`up to (\d+) individual tools are registered on GitLab\.com Enterprise/Premium; self-managed Enterprise/Premium registers (\d+)\.`)
		if gitLabCom <= selfManaged {
			t.Errorf("GitLab.com individual tools = %d, want more than the %d self-managed ones", gitLabCom, selfManaged)
		}
	})
}

// TestReadVersion_UsesProjectRoot verifies readVersion reads VERSION from the
// supplied project root and trims trailing whitespace.
//
// The test writes a temporary VERSION file and expects the exact semantic
// version string. This prevents generation from depending on the process working
// directory when a root is supplied.
func TestReadVersion_UsesProjectRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte("2.1.0\n"), 0o600); err != nil {
		t.Fatalf("write VERSION: %v", err)
	}

	if got := readVersion(dir); got != "2.1.0" {
		t.Fatalf("readVersion() = %q, want 2.1.0", got)
	}
}

// TestReadVersion_FallsBackToUnknown verifies readVersion reports "unknown"
// rather than failing when the root cannot be opened or holds no VERSION file,
// so a generation run in an unusual checkout still produces files.
func TestReadVersion_FallsBackToUnknown(t *testing.T) {
	tests := []struct {
		name string
		dir  func(t *testing.T) string
	}{
		{name: "root without VERSION", dir: func(t *testing.T) string { t.Helper(); return t.TempDir() }},
		{name: "root that does not exist", dir: func(t *testing.T) string {
			t.Helper()
			return filepath.Join(t.TempDir(), "missing")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := readVersion(tt.dir(t)); got != "unknown" {
				t.Errorf("readVersion() = %q, want unknown", got)
			}
		})
	}
}

// TestValidateLLMSTxt_AcceptsSpecFileListSections verifies llms.txt validation
// accepts H2 sections made of Markdown file-list entries.
//
// The content includes prose before the generated sections and both linked docs
// with and without descriptions. The expected result is no error, matching the
// public llms.txt shape documented by the generator.
func TestValidateLLMSTxt_AcceptsSpecFileListSections(t *testing.T) {
	content := strings.Join([]string{
		"# Example",
		"",
		"> Short project summary.",
		"",
		"Details before H2 sections can use normal Markdown lists.",
		"",
		"- key: value",
		"",
		"## Docs",
		"",
		"- [Guide](https://example.test/docs/guide.md): Short guide",
		"- [Reference](https://example.test/docs/reference.md)",
		"",
		"## Optional",
		"",
		"- [Full reference](https://example.test/llms-full.txt): Expanded context",
		"",
	}, "\n")

	if err := validateLLMSTxt(content); err != nil {
		t.Fatalf("validateLLMSTxt() error: %v", err)
	}
}

// TestValidateLLMSTxt_RejectsRelativeFileListTarget guards the GEO regression
// where every llms.txt link was a repository-relative path. llms.txt is served
// from the docs domain, so "docs/getting-started.md" resolved against that host
// and 404'd — 17 of 18 links were dead. Only absolute URLs work for every
// consumer, so generation must fail rather than publish dead links again.
func TestValidateLLMSTxt_RejectsRelativeFileListTarget(t *testing.T) {
	content := strings.Join([]string{
		"# Example",
		"",
		"> Short project summary.",
		"",
		"## Docs",
		"",
		"- [Guide](docs/guide.md): Short guide",
		"",
	}, "\n")

	err := validateLLMSTxt(content)
	if err == nil {
		t.Fatal("validateLLMSTxt() error = nil, want error for a relative link target")
	}
	if !strings.Contains(err.Error(), "absolute URL") {
		t.Fatalf("validateLLMSTxt() error = %v, want it to mention the absolute-URL requirement", err)
	}
}

// TestValidateLLMSTxt_ReportsEachStructuralDefect verifies the validator names
// the first defect it meets: a missing or non-H1 first line, a missing
// blockquote summary, a heading level llms.txt does not allow, a section with
// no links whether another section or the end of the file follows it, and a
// file-list entry with an unterminated, empty, or note-less link. CRLF content
// is accepted as-is, since check mode compares it normalized.
func TestValidateLLMSTxt_ReportsEachStructuralDefect(t *testing.T) {
	const head = "# Title\n\n> Summary.\n\n"
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "empty content", content: "", wantErr: "missing H1 title"},
		{name: "first line is prose", content: "Title\n", wantErr: "first line must be an H1 title"},
		{name: "first line is an H2", content: "## Title\n", wantErr: "first line must be an H1 title"},
		{name: "missing blockquote summary", content: "# Title\n\n## Docs\n\n- [A](https://x/a)\n", wantErr: "missing blockquote summary"},
		{name: "H3 heading", content: head + "### Sub\n", wantErr: "line 5: llms.txt only allows H1 plus H2 file-list sections"},
		{name: "section without links followed by another", content: head + "## A\n\n## B\n\n- [B](https://x/b)\n", wantErr: `section "A" has no file links`},
		{name: "trailing section without links", content: head + "## A\n", wantErr: `section "A" has no file links`},
		{name: "unterminated link target", content: head + "## A\n\n- [A](https://x/a\n", wantErr: "line 7: file-list entry is missing markdown link target"},
		{name: "empty link target", content: head + "## A\n\n- [A]( )\n", wantErr: "file-list entry has empty markdown link target"},
		{name: "notes without a colon", content: head + "## A\n\n- [A](https://x/a) note\n", wantErr: "file-list entry notes must follow ':'"},
		{name: "CRLF content is accepted", content: "# Title\r\n\r\n> Summary.\r\n\r\n## A\r\n\r\n- [A](https://x/a): note\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLLMSTxt(tt.content)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateLLMSTxt() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateLLMSTxt() error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// TestValidateHeading_RejectsEmptyTitle verifies the heading step refuses an
// H2 with nothing after the marker. validateLLMSTxt trims each line first, so
// such a heading reaches it as "##" and fails the level check instead; the
// method still guards its own contract for a caller that does not trim.
func TestValidateHeading_RejectsEmptyTitle(t *testing.T) {
	state := &llmsTxtValidationState{}

	err := state.validateHeading(7, "## ")
	if err == nil || !strings.Contains(err.Error(), "line 7: H2 section title is empty") {
		t.Fatalf("validateHeading() error = %v, want the empty-title error", err)
	}
}

// TestAbsoluteLLMSTarget_ResolvesRepoRelativeAndPreservesAbsolute verifies both
// branches of the link resolver: repository-relative documentation paths gain
// the blob prefix, while already-absolute targets are emitted untouched.
func TestAbsoluteLLMSTarget_ResolvesRepoRelativeAndPreservesAbsolute(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   string
	}{
		{
			name:   "repo relative doc path gains the blob prefix",
			target: "docs/getting-started.md",
			want:   repoBlobBaseURL + "docs/getting-started.md",
		},
		{
			name:   "repo root file gains the blob prefix",
			target: "PRIVACY.md",
			want:   repoBlobBaseURL + "PRIVACY.md",
		},
		{
			name:   "absolute https target is preserved",
			target: siteBaseURL + "llms-full.txt",
			want:   siteBaseURL + "llms-full.txt",
		},
		{
			name:   "absolute http target is preserved",
			target: "http://example.test/a.md",
			want:   "http://example.test/a.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := absoluteLLMSTarget(tt.target); got != tt.want {
				t.Errorf("absoluteLLMSTarget(%q) = %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}

// TestValidateLLMSTxt_AcceptsPreambleCodeBlock verifies that a fenced code block
// in the preamble (before the first H2) is accepted. The generated llms.txt uses
// this for the headless AI-assistant install snippet (the mcpServers JSON), so a
// stricter validator that rejected code fences would silently break that section.
func TestValidateLLMSTxt_AcceptsPreambleCodeBlock(t *testing.T) {
	content := strings.Join([]string{
		"# Example",
		"",
		"> Short project summary.",
		"",
		"Installing for an AI assistant (headless, no wizard):",
		"",
		"```json",
		"{",
		"  \"mcpServers\": {",
		"    \"gitlab\": { \"command\": \"docker\" }",
		"  }",
		"}",
		"```",
		"",
		"## Docs",
		"",
		"- [Guide](https://example.test/docs/guide.md): Short guide",
		"",
	}, "\n")

	if err := validateLLMSTxt(content); err != nil {
		t.Fatalf("validateLLMSTxt() rejected a preamble code block: %v", err)
	}
}

// TestValidateLLMSTxt_RejectsNonLinkH2Content verifies llms.txt H2 sections must
// contain file-list link entries rather than arbitrary prose.
//
// The test places plain text under a Docs section and expects validation to
// fail, keeping generated discovery files machine-readable for model consumers.
func TestValidateLLMSTxt_RejectsNonLinkH2Content(t *testing.T) {
	content := strings.Join([]string{
		"# Example",
		"",
		"> Short project summary.",
		"",
		"## Docs",
		"",
		"Plain text is not a file-list entry.",
		"",
	}, "\n")

	if err := validateLLMSTxt(content); err == nil {
		t.Fatal("validateLLMSTxt() error = nil, want error")
	}
}

// TestValidateLLMSTxt_RejectsEmptyFileListLinkLabel verifies llms.txt validation
// rejects Markdown links without visible labels.
//
// Empty labels produce poor LLM context and broken human navigation, so the test
// expects a validation error for a file-list entry using [](...).
func TestValidateLLMSTxt_RejectsEmptyFileListLinkLabel(t *testing.T) {
	content := strings.Join([]string{
		"# Example",
		"",
		"> Short project summary.",
		"",
		"## Docs",
		"",
		"- [](docs/guide.md)",
		"",
	}, "\n")

	if err := validateLLMSTxt(content); err == nil {
		t.Fatal("validateLLMSTxt() error = nil, want error")
	}
}

// TestValidateLLMSFullTxt_RequiresGeneratedSections verifies llms-full.txt
// validation requires all generated catalog sections.
//
// The first fixture includes Dynamic Toolset, Meta-Tools, Individual Tools,
// Resources, and Prompts and should pass. The second fixture omits most sections
// and should fail so partial generated files are caught before writing.
func TestValidateLLMSFullTxt_RequiresGeneratedSections(t *testing.T) {
	content := strings.Join([]string{
		"# Example Full Reference",
		"",
		"## Dynamic Toolset",
		"",
		"## Meta-Tools",
		"",
		"## Individual Tools",
		"",
		"## Resources",
		"",
		"## Prompts",
		"",
	}, "\n")

	if err := validateLLMSFullTxt(content); err != nil {
		t.Fatalf("validateLLMSFullTxt() error: %v", err)
	}
	if err := validateLLMSFullTxt("# Example\n\n## Dynamic Toolset\n"); err == nil {
		t.Fatal("validateLLMSFullTxt() error = nil, want error")
	}
}

// TestValidateLLMSFullTxt_NamesTheMissingPart verifies the long-form validator
// reports a missing H1 for empty content and names the first absent section
// otherwise, so a truncated generation is diagnosed rather than written.
func TestValidateLLMSFullTxt_NamesTheMissingPart(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "empty content", content: "", wantErr: "missing H1 title"},
		{name: "prose first line", content: "Reference\n\n## Dynamic Toolset\n", wantErr: "missing H1 title"},
		{name: "missing prompts section", content: "# R\n\n## Dynamic Toolset\n## Meta-Tools\n## Individual Tools\n## Resources\n", wantErr: `missing "## Prompts" section`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLLMSFullTxt(tt.content)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateLLMSFullTxt() error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// TestWriteGeneratedFile_RejectsUnexpectedFileName verifies generated llms files
// can only be written to the supported top-level artifact names.
//
// The test attempts README.md, a parent-directory escape, and a docs path in
// check mode. Each should fail to prevent accidental writes outside the intended
// llms.txt and llms-full.txt outputs.
func TestWriteGeneratedFile_RejectsUnexpectedFileName(t *testing.T) {
	for _, name := range []string{"README.md", "../llms.txt", "docs/llms.txt"} {
		t.Run(name, func(t *testing.T) {
			if err := writeGeneratedFile(name, "content", true); err == nil {
				t.Fatal("writeGeneratedFile() error = nil, want error")
			}
		})
	}
}

// TestWriteGeneratedFile_CheckModeAcceptsCRLFLineEndings verifies check mode
// treats CRLF and LF generated files as equivalent.
//
// The test writes llms.txt with Windows line endings, then checks the same
// content with LF endings. A nil error prevents cross-platform line ending
// differences from causing false generation drift.
func TestWriteGeneratedFile_CheckModeAcceptsCRLFLineEndings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	content := "# Example\n\n"
	if err := os.WriteFile(filepath.Join(dir, llmsFileName), []byte("# Example\r\n\r\n"), 0o600); err != nil {
		t.Fatalf("write llms.txt: %v", err)
	}
	t.Chdir(dir)

	if err := writeGeneratedFile(llmsFileName, content, true); err != nil {
		t.Fatalf("writeGeneratedFile() error = %v", err)
	}
}

// TestWriteGeneratedFile_WritesIntoProjectRoot verifies write mode places the
// file at the project root found from the working directory, with exactly the
// given content, and overwrites a previous generation.
func TestWriteGeneratedFile_WritesIntoProjectRoot(t *testing.T) {
	dir := projectRootWithVersion(t, cannedVersion)
	writeFile(t, dir, llmsFullFileName, "# previous\n")
	nested := filepath.Join(dir, "cmd", "gen_llms")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Chdir(nested)

	if err := writeGeneratedFile(llmsFullFileName, "# fresh\n", false); err != nil {
		t.Fatalf("writeGeneratedFile() error = %v", err)
	}
	if got := readGenerated(t, dir, llmsFullFileName); got != "# fresh\n" {
		t.Errorf("llms-full.txt = %q, want the fresh content", got)
	}
}

// TestWriteGeneratedFile_CheckModeReportsMissingAndStaleFiles verifies check
// mode distinguishes a file that is not there, whose error keeps
// fs.ErrNotExist, from one that is stale, whose error tells the reader how to
// regenerate.
func TestWriteGeneratedFile_CheckModeReportsMissingAndStaleFiles(t *testing.T) {
	tests := []struct {
		name         string
		existing     string
		wantErr      string
		wantNotExist bool
	}{
		{name: "missing file", wantNotExist: true},
		{name: "stale file", existing: "# stale\n", wantErr: "llms.txt is out of date; run go run ./cmd/gen_llms/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := projectRootWithVersion(t, cannedVersion)
			if tt.existing != "" {
				writeFile(t, dir, llmsFileName, tt.existing)
			}

			err := writeGeneratedFile(llmsFileName, "# fresh\n", true)
			if err == nil {
				t.Fatal("writeGeneratedFile() error = nil, want error")
			}
			if errors.Is(err, fs.ErrNotExist) != tt.wantNotExist {
				t.Errorf("errors.Is(err, fs.ErrNotExist) = %v, want %v (err %v)", !tt.wantNotExist, tt.wantNotExist, err)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("writeGeneratedFile() error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// TestWriteGeneratedFile_RequiresProjectRoot verifies the writer reports the
// missing project root, in either mode, rather than writing next to the
// working directory.
func TestWriteGeneratedFile_RequiresProjectRoot(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	for _, checkOnly := range []bool{false, true} {
		t.Run(strconv.FormatBool(checkOnly), func(t *testing.T) {
			err := writeGeneratedFile(llmsFileName, "# x\n", checkOnly)
			if err == nil || !strings.Contains(err.Error(), "project root") {
				t.Fatalf("writeGeneratedFile() error = %v, want the missing project root", err)
			}
			if _, statErr := os.Stat(filepath.Join(dir, llmsFileName)); !errors.Is(statErr, fs.ErrNotExist) {
				t.Error("writeGeneratedFile() wrote into the working directory")
			}
		})
	}
}

// TestSchemaTypeLabel_ArrayAndNullableTypes verifies schemaTypeLabel summarizes
// nullable, array, nested-array, object, and untyped schemas.
//
// The table covers common JSON Schema shapes emitted for tool inputs. Expected
// labels are human-readable phrases used in generated llms-full.txt parameter
// references.
func TestSchemaTypeLabel_ArrayAndNullableTypes(t *testing.T) {
	tests := []struct {
		name   string
		schema map[string]any
		want   string
	}{
		{
			name:   "nullable string",
			schema: map[string]any{"type": []any{"null", "string"}},
			want:   "string",
		},
		{
			name: "nullable integer array",
			schema: map[string]any{
				"type":  []any{"null", "array"},
				"items": map[string]any{"type": "integer"},
			},
			want: "array of integers",
		},
		{
			name: "object array",
			schema: map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "object"},
			},
			want: "array of objects",
		},
		{
			name:   "untyped any value",
			schema: map[string]any{},
			want:   "any",
		},
		{
			name: "nested string array",
			schema: map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "string"},
				},
			},
			want: "array of arrays of strings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := schemaTypeLabel(tt.schema); got != tt.want {
				t.Fatalf("schemaTypeLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSchemaTypeLabel_InfersUntypedAndUnionShapes verifies the label falls back
// to the shape of a schema with no type keyword, joins a union of types, keeps a
// bare array bare, calls an array of unions an array of values, pluralizes any
// item type, and ignores blank or non-string entries in a type list.
func TestSchemaTypeLabel_InfersUntypedAndUnionShapes(t *testing.T) {
	tests := []struct {
		name   string
		schema map[string]any
		want   string
	}{
		{name: "items without type read as array", schema: map[string]any{"items": map[string]any{}}, want: "array"},
		{name: "properties without type read as object", schema: map[string]any{"properties": map[string]any{}}, want: "object"},
		{name: "union of two types is joined", schema: map[string]any{"type": []any{"string", "integer"}}, want: "string or integer"},
		{name: "array without items stays bare", schema: map[string]any{"type": "array"}, want: "array"},
		{name: "array of union items", schema: map[string]any{"type": "array", "items": map[string]any{"type": []any{"string", "integer"}}}, want: "array of values"},
		{name: "array of booleans", schema: map[string]any{"type": "array", "items": map[string]any{"type": "boolean"}}, want: "array of booleans"},
		{name: "array of numbers", schema: map[string]any{"type": "array", "items": map[string]any{"type": "number"}}, want: "array of numbers"},
		{name: "array of a custom type", schema: map[string]any{"type": "array", "items": map[string]any{"type": "date"}}, want: "array of dates"},
		{name: "blank and non-string entries are ignored", schema: map[string]any{"type": []any{"", " ", 3, "string"}}, want: "string"},
		{name: "unrecognized type value reads as any", schema: map[string]any{"type": 42}, want: "any"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := schemaTypeLabel(tt.schema); got != tt.want {
				t.Errorf("schemaTypeLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPluralSchemaType_PluralizesEachLabel verifies the plural forms used
// after "array of": each JSON Schema primitive, a nested array label, a union
// label that becomes "values", and the default "s" suffix.
func TestPluralSchemaType_PluralizesEachLabel(t *testing.T) {
	tests := []struct {
		name string
		typ  string
		want string
	}{
		{name: "integer", typ: "integer", want: "integers"},
		{name: "number", typ: "number", want: "numbers"},
		{name: "string", typ: "string", want: "strings"},
		{name: "boolean", typ: "boolean", want: "booleans"},
		{name: "object", typ: "object", want: "objects"},
		{name: "nested array", typ: "array of strings", want: "arrays of strings"},
		{name: "union", typ: "string or null", want: "values"},
		{name: "default suffix", typ: "date", want: "dates"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pluralSchemaType(tt.typ); got != tt.want {
				t.Errorf("pluralSchemaType(%q) = %q, want %q", tt.typ, got, tt.want)
			}
		})
	}
}

// TestWriteInputSchema_RendersParameterTable verifies the parameter table:
// nothing for a non-map schema or one without properties, otherwise one sorted
// line per property with its type label, the required marker taken from the
// schema's required list (non-string entries ignored), the description with the
// legacy ",required" suffix stripped and no colon when it is empty, and
// properties that are not schemas skipped.
func TestWriteInputSchema_RendersParameterTable(t *testing.T) {
	tests := []struct {
		name   string
		schema any
		want   string
	}{
		{name: "non-map schema writes nothing", schema: "string", want: ""},
		{name: "schema without properties writes nothing", schema: map[string]any{"type": "object"}, want: ""},
		{name: "empty properties write nothing", schema: map[string]any{"properties": map[string]any{}}, want: ""},
		{
			name: "sorted, typed, required and described",
			schema: map[string]any{
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
					"id":   map[string]any{"type": "integer", "description": "Issue IID"},
				},
				"required": []any{"id"},
			},
			want: "**Parameters:**\n\n- `id` (integer) (required): Issue IID\n- `name` (string)\n\n",
		},
		{
			name: "legacy required suffix stripped and non-schema properties skipped",
			schema: map[string]any{
				"properties": map[string]any{
					"a": map[string]any{"type": "string", "description": "A,required"},
					"b": 7,
				},
				"required": []any{"a", 1},
			},
			want: "**Parameters:**\n\n- `a` (string) (required): A\n\n",
		},
		{
			name: "required of the wrong shape marks nothing",
			schema: map[string]any{
				"properties": map[string]any{"id": map[string]any{"type": "integer", "description": "Issue IID"}},
				"required":   "id",
			},
			want: "**Parameters:**\n\n- `id` (integer): Issue IID\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			writeInputSchema(&b, tt.schema)
			if got := b.String(); got != tt.want {
				t.Errorf("writeInputSchema() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestWriteAnnotations_RendersEveryHint verifies the annotation line and its
// defaults for the pointer hints: a missing destructive hint reads as false
// and a missing open-world hint as true, which is what the MCP specification
// tells a client to assume, and nil annotations write nothing.
func TestWriteAnnotations_RendersEveryHint(t *testing.T) {
	tests := []struct {
		name string
		ann  *mcp.ToolAnnotations
		want string
	}{
		{name: "nil annotations write nothing", ann: nil, want: ""},
		{
			name: "unset pointers take the specification defaults",
			ann:  &mcp.ToolAnnotations{ReadOnlyHint: true},
			want: "Annotations: readOnly=true, destructive=false, idempotent=false, openWorld=true\n",
		},
		{
			name: "explicit pointers are honored",
			ann:  &mcp.ToolAnnotations{DestructiveHint: new(true), IdempotentHint: true, OpenWorldHint: new(false)},
			want: "Annotations: readOnly=false, destructive=true, idempotent=true, openWorld=false\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			writeAnnotations(&b, tt.ann)
			if got := b.String(); got != tt.want {
				t.Errorf("writeAnnotations() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestWriteActionOutputSchemas_EmbedsOnlyDeclaredSchemas verifies the
// collapsible schema blocks: nothing for an empty map or for actions without
// an output schema, otherwise one block per schema in action-name order, and a
// schema that cannot be marshaled is skipped rather than aborting the file.
func TestWriteActionOutputSchemas_EmbedsOnlyDeclaredSchemas(t *testing.T) {
	tests := []struct {
		name   string
		routes toolutil.ActionMap
		want   string
	}{
		{name: "no routes", routes: toolutil.ActionMap{}, want: ""},
		{name: "routes without output schemas", routes: toolutil.ActionMap{"get": {}, "list": {}}, want: ""},
		{
			name: "schemas sorted by action and unmarshalable ones skipped",
			routes: toolutil.ActionMap{
				"list":   {OutputSchema: map[string]any{"type": "array"}},
				"create": {OutputSchema: map[string]any{"type": "object"}},
				"broken": {OutputSchema: map[string]any{"ch": make(chan int)}},
				"get":    {},
			},
			want: "**Action Output Schemas:**\n\n" +
				"<details><summary>create</summary>\n\n```json\n{\"type\":\"object\"}\n```\n\n</details>\n\n" +
				"<details><summary>list</summary>\n\n```json\n{\"type\":\"array\"}\n```\n\n</details>\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			writeActionOutputSchemas(&b, "gitlab_issue", tt.routes)
			if got := b.String(); got != tt.want {
				t.Errorf("writeActionOutputSchemas() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestWriteLLMSResource_OmitsEmptyFields verifies a resource entry drops the
// MIME and description lines when they are empty, keeping the URI line, so a
// template with no metadata still renders as a well-formed entry.
func TestWriteLLMSResource_OmitsEmptyFields(t *testing.T) {
	var b strings.Builder
	writeLLMSResource(&b, "Guide", "gitlab://guide", "URI", "", "")

	if got, want := b.String(), "### Guide\n\n- **URI**: `gitlab://guide`\n\n"; got != want {
		t.Errorf("writeLLMSResource() = %q, want %q", got, want)
	}
}

// TestWriteLLMSFullPrompts_OmitsEmptyDescriptionAndArguments verifies a prompt
// without description or arguments renders as just its heading under the
// section intro, with no dangling Arguments header.
func TestWriteLLMSFullPrompts_OmitsEmptyDescriptionAndArguments(t *testing.T) {
	var b strings.Builder
	writeLLMSFullPrompts(&b, []*mcp.Prompt{{Name: "bare"}})

	if got, want := b.String(), "## Prompts\n\n1 prompt templates for AI-assisted GitLab workflows.\n\n### bare\n\n"; got != want {
		t.Errorf("writeLLMSFullPrompts() = %q, want %q", got, want)
	}
}

// TestWriteLLMSFullEnterpriseOnlyMetaTools_SkipsSectionWhenNoneExist verifies
// no Enterprise-Only heading is written when the GitLab.com surface adds
// nothing to the base one, so a Free-only catalog does not advertise an empty
// section.
func TestWriteLLMSFullEnterpriseOnlyMetaTools_SkipsSectionWhenNoneExist(t *testing.T) {
	base := []*mcp.Tool{{Name: "gitlab_issue", Description: "Manage issues."}}
	var b strings.Builder
	writeLLMSFullEnterpriseOnlyMetaTools(&b, llmsCatalog{MetaBase: base, MetaGitLabComEnterprise: base})

	if got := b.String(); got != "" {
		t.Errorf("writeLLMSFullEnterpriseOnlyMetaTools() = %q, want nothing", got)
	}
}

// TestEnterpriseOnlyMetaTools_KeepsToolsAbsentFromBase verifies the difference
// is taken by name in GitLab.com order and is empty, not nil, when the surfaces
// coincide.
func TestEnterpriseOnlyMetaTools_KeepsToolsAbsentFromBase(t *testing.T) {
	issue := &mcp.Tool{Name: "gitlab_issue"}
	epic := &mcp.Tool{Name: "gitlab_epic"}
	orbit := &mcp.Tool{Name: "gitlab_orbit"}
	tests := []struct {
		name      string
		base      []*mcp.Tool
		gitLabCom []*mcp.Tool
		want      []string
	}{
		{name: "two additions in gitlab.com order", base: []*mcp.Tool{issue}, gitLabCom: []*mcp.Tool{orbit, issue, epic}, want: []string{"gitlab_orbit", "gitlab_epic"}},
		{name: "identical surfaces", base: []*mcp.Tool{issue}, gitLabCom: []*mcp.Tool{issue}, want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := enterpriseOnlyMetaTools(tt.base, tt.gitLabCom)
			if got == nil {
				t.Fatal("enterpriseOnlyMetaTools() = nil, want an empty slice at least")
			}
			names := make([]string, 0, len(got))
			for _, tool := range got {
				names = append(names, tool.Name)
			}
			if strings.Join(names, ",") != strings.Join(tt.want, ",") {
				t.Errorf("enterpriseOnlyMetaTools() = %v, want %v", names, tt.want)
			}
		})
	}
}

// TestCompactToolDescription_KeepsFirstParagraphWithinLimit verifies the
// description shown for an individual tool: the first paragraph when it fits,
// its first sentence when the paragraph is too long, and a hard truncation
// when even that sentence, or a paragraph with no sentence break, is too long.
func TestCompactToolDescription_KeepsFirstParagraphWithinLimit(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        string
	}{
		{name: "short first paragraph is kept whole", description: "One. Two.\n\nThree.", want: "One. Two."},
		{name: "long paragraph falls back to its first sentence", description: "Short lead sentence. " + strings.Repeat("x", 700), want: "Short lead sentence."},
		{name: "long paragraph without a sentence break is truncated", description: strings.Repeat("y", 700), want: strings.Repeat("y", maxFullDescRunes) + "..."},
		{name: "long first sentence is truncated", description: strings.Repeat("z", 650) + ". Next.", want: strings.Repeat("z", maxFullDescRunes) + "..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compactToolDescription(tt.description); got != tt.want {
				t.Errorf("compactToolDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestTruncateRunes_CountsRunesNotBytes verifies truncation measures runes, so
// a multi-byte character is never split, and leaves text within the limit
// untouched without an ellipsis.
func TestTruncateRunes_CountsRunesNotBytes(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		maxRunes int
		want     string
	}{
		{name: "multi-byte runes are kept whole", s: "héllo wörld", maxRunes: 5, want: "héllo..."},
		{name: "text within the limit is unchanged", s: "abc", maxRunes: 5, want: "abc"},
		{name: "text at the limit is unchanged", s: "abcde", maxRunes: 5, want: "abcde"},
		{name: "empty text", s: "", maxRunes: 3, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateRunes(tt.s, tt.maxRunes); got != tt.want {
				t.Errorf("truncateRunes(%q, %d) = %q, want %q", tt.s, tt.maxRunes, got, tt.want)
			}
		})
	}
}

// TestFirstParagraph_CutsAtBlankLine verifies the first paragraph ends at the
// first blank line, is trimmed, and that text without a blank line is returned
// whole, single newlines included.
func TestFirstParagraph_CutsAtBlankLine(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want string
	}{
		{name: "blank line ends the paragraph", s: "  a b \n\nc", want: "a b"},
		{name: "single newline is kept", s: "a\nb", want: "a\nb"},
		{name: "empty text", s: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstParagraph(tt.s); got != tt.want {
				t.Errorf("firstParagraph(%q) = %q, want %q", tt.s, got, tt.want)
			}
		})
	}
}

// TestFirstSentence_SkipsAbbreviations verifies the first sentence ends at the
// first ". " that is not part of a known abbreviation, stops at a newline, and
// is the whole text when no boundary exists. The abbreviation check compares
// the characters before the period, not a whole word, so any word ending in
// one of them ("request." ends in "est.") defers the cut; the case pins that
// behavior so a fix is a deliberate change to the generated files.
func TestFirstSentence_SkipsAbbreviations(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want string
	}{
		{name: "period and space end the sentence", s: "First. Second.", want: "First."},
		{name: "newline ends the sentence", s: "Line one\nLine two.", want: "Line one"},
		{name: "abbreviation is not a boundary", s: "See e.g. this. Then that.", want: "See e.g. this."},
		{name: "abbreviations match by suffix, so request. reads as est.", s: "Open a request. Then wait.", want: "Open a request. Then wait."},
		{name: "leading abbreviation is skipped", s: "vs. a. b", want: "vs. a."},
		{name: "only an abbreviation", s: "i.e. only", want: "i.e. only"},
		{name: "no terminator", s: "No terminator", want: "No terminator"},
		{name: "empty text", s: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstSentence(tt.s); got != tt.want {
				t.Errorf("firstSentence(%q) = %q, want %q", tt.s, got, tt.want)
			}
		})
	}
}

// TestFindSentenceEnd_ReturnsBoundaryIndex verifies the index of the sentence
// boundary, -1 when there is none, and that every listed abbreviation is
// skipped even when it starts the text.
func TestFindSentenceEnd_ReturnsBoundaryIndex(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want int
	}{
		{name: "plain boundary", s: "a. b", want: 1},
		{name: "no boundary", s: "a.b", want: -1},
		{name: "abbreviation only", s: "e.g. x", want: -1},
		{name: "boundary after an abbreviation", s: "vs. a. b", want: 5},
		{name: "empty text", s: "", want: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findSentenceEnd(tt.s); got != tt.want {
				t.Errorf("findSentenceEnd(%q) = %d, want %d", tt.s, got, tt.want)
			}
		})
	}
}

// TestCountDomains_CountsSecondNameSegment verifies domains are the second
// underscore-separated segment of a tool name, counted once each, and that a
// name with no second segment counts for nothing.
func TestCountDomains_CountsSecondNameSegment(t *testing.T) {
	tls := []*mcp.Tool{
		{Name: "gitlab_issue_list"},
		{Name: "gitlab_issue_get"},
		{Name: "gitlab_project_get"},
		{Name: "gitlab_orbit"},
		{Name: "gitlab"},
	}

	if got := countDomains(tls); got != 3 {
		t.Errorf("countDomains() = %d, want 3 (issue, project, orbit)", got)
	}
}

// TestGroupByDomain_BucketsBySecondSegment verifies grouping keeps each tool
// under its second name segment, in input order, and files a name with no
// second segment under "other".
func TestGroupByDomain_BucketsBySecondSegment(t *testing.T) {
	tls := []*mcp.Tool{{Name: "gitlab_issue_list"}, {Name: "gitlab"}, {Name: "gitlab_issue_get"}, {Name: "x_y"}}

	got := groupByDomain(tls)
	tests := []struct {
		domain string
		want   []string
	}{
		{domain: "issue", want: []string{"gitlab_issue_list", "gitlab_issue_get"}},
		{domain: "other", want: []string{"gitlab"}},
		{domain: "y", want: []string{"x_y"}},
	}
	if len(got) != len(tests) {
		t.Errorf("groupByDomain() has %d domains, want %d", len(got), len(tests))
	}
	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			names := make([]string, 0, len(got[tt.domain]))
			for _, tool := range got[tt.domain] {
				names = append(names, tool.Name)
			}
			if strings.Join(names, ",") != strings.Join(tt.want, ",") {
				t.Errorf("domain %q = %v, want %v", tt.domain, names, tt.want)
			}
		})
	}
}

// TestSortedDomains_SortsAlphabetically verifies the domain headings of the
// individual section come out sorted regardless of registration order.
func TestSortedDomains_SortsAlphabetically(t *testing.T) {
	tls := []*mcp.Tool{{Name: "gitlab_project_get"}, {Name: "gitlab_issue_list"}, {Name: "gitlab_branch_create"}, {Name: "gitlab_issue_get"}}

	if got := strings.Join(sortedDomains(tls), ","); got != "branch,issue,project" {
		t.Errorf("sortedDomains() = %q, want branch,issue,project", got)
	}
}

// TestPluralizeTools_SingularForOne verifies the domain heading reads "1 tool"
// and "N tools" otherwise, zero included.
func TestPluralizeTools_SingularForOne(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want string
	}{
		{name: "zero", n: 0, want: "tools"},
		{name: "one", n: 1, want: "tool"},
		{name: "many", n: 2, want: "tools"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pluralizeTools(tt.n); got != tt.want {
				t.Errorf("pluralizeTools(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

// TestClassifyMetaDomains_CapitalizesAndSorts verifies the domain list of
// llms.txt strips the gitlab_ prefix, renders known acronyms in upper case,
// capitalizes the remaining words, and sorts the result.
func TestClassifyMetaDomains_CapitalizesAndSorts(t *testing.T) {
	metaTools := []*mcp.Tool{{Name: "gitlab_mr_approval"}, {Name: "gitlab_ci_variable"}, {Name: "gitlab_dora_metrics"}, {Name: "gitlab_issue"}}

	if got := strings.Join(classifyMetaDomains(metaTools), ", "); got != "CI Variable, DORA Metrics, Issue, MR Approval" {
		t.Errorf("classifyMetaDomains() = %q", got)
	}
}

// TestCapitalizeWords_HandlesAcronymsAndEmptyParts verifies each acronym in the
// table is upper-cased, other words are title-cased, an empty part between
// consecutive underscores is preserved as an empty word, and empty input stays
// empty.
func TestCapitalizeWords_HandlesAcronymsAndEmptyParts(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want string
	}{
		{name: "acronym and word", s: "group_scim", want: "Group SCIM"},
		{name: "every acronym", s: "ci_mr_dora_scim_ssh_gpg_api", want: "CI MR DORA SCIM SSH GPG API"},
		{name: "empty part is kept", s: "a__b", want: "A  B"},
		{name: "empty input", s: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := capitalizeWords(tt.s); got != tt.want {
				t.Errorf("capitalizeWords(%q) = %q, want %q", tt.s, got, tt.want)
			}
		})
	}
}

// TestIsGeneratedLLMSFile_CoversEveryCompanion pins the write allowlist. The
// companion files exist because llms-full.txt is ~750K tokens — larger than any
// production context window — so a new companion silently failing to write
// would quietly undo that fix.
func TestIsGeneratedLLMSFile_CoversEveryCompanion(t *testing.T) {
	for _, name := range []string{
		llmsFileName, llmsFullFileName, llmsMediumFileName,
		llmsFullMetaFileName, llmsFullIndividualFileName, llmsFullCapabilityFileName,
	} {
		t.Run(name, func(t *testing.T) {
			if !isGeneratedLLMSFile(name) {
				t.Errorf("isGeneratedLLMSFile(%q) = false, want true", name)
			}
		})
	}
	for _, name := range []string{"README.md", "llms.txt.bak", "../llms.txt", ""} {
		t.Run(name, func(t *testing.T) {
			if isGeneratedLLMSFile(name) {
				t.Errorf("isGeneratedLLMSFile(%q) = true, want false", name)
			}
		})
	}
}

// TestWriteLLMSMediumMetaTools_OmitsSchemas verifies the medium reference keeps
// the action inventory but drops the per-action JSON schemas. Those schemas are
// what make the full file unloadable, so their absence here is the whole point.
func TestWriteLLMSMediumMetaTools_OmitsSchemas(t *testing.T) {
	catalog := llmsCatalog{
		MetaBase: []*mcp.Tool{{
			Name:        "gitlab_issue",
			Title:       "Issue",
			Description: "Manage issues. Use {\"action\":\"list\"}.",
		}},
		MetaRoutes: map[string]toolutil.ActionMap{
			"gitlab_issue": {"list": {}, "create": {}, "get": {}},
		},
	}

	var b strings.Builder
	writeLLMSMediumMetaTools(&b, catalog)
	got := b.String()

	for _, want := range []string{"### gitlab_issue", "**Issue**", "Actions (3):", "create, get, list"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(got, want) {
				t.Errorf("medium meta-tools output missing %q\ngot:\n%s", want, got)
			}
		})
	}
	// Actions must be sorted so regeneration is deterministic.
	if strings.Index(got, "create") > strings.Index(got, "get") {
		t.Error("actions are not sorted alphabetically")
	}
	for _, unwanted := range []string{"inputSchema", "\"properties\"", "Input Schema"} {
		t.Run(unwanted, func(t *testing.T) {
			if strings.Contains(got, unwanted) {
				t.Errorf("medium meta-tools output must not embed schemas, found %q", unwanted)
			}
		})
	}
}
