// main_test.go covers LobeHub manifest generation: the capability arrays are
// filled from the registered MCP surface, every other manifest field survives a
// regeneration, and a malformed or misspelled manifest fails loudly instead of
// being overwritten.
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// minimalManifest is the smallest input generate accepts: the three required
// fields and nothing else.
const minimalManifest = `{
  "identifier": "jmrplens-gitlab-mcp-server",
  "name": "GitLab MCP Server",
  "version": "9.9.9"
}`

// populatedManifest is a manifest carrying every metadata field the generator
// must leave alone, plus empty capability arrays for it to fill.
const populatedManifest = `{
  "identifier": "jmrplens-gitlab-mcp-server",
  "name": "GitLab MCP Server",
  "version": "9.9.9",
  "description": "desc",
  "author": "Someone",
  "authorUrl": "https://example.com",
  "icon": "https://example.com/icon.png",
  "tags": ["a", "b"],
  "tools": [],
  "prompts": [],
  "resources": []
}`

// generatedManifest runs generate over in and returns the decoded result.
func generatedManifest(t *testing.T, in string) manifest {
	t.Helper()
	out, _, err := generate([]byte(in))
	if err != nil {
		t.Fatalf("generate() error: %v", err)
	}
	var m manifest
	if decodeErr := json.Unmarshal(out, &m); decodeErr != nil {
		t.Fatalf("unmarshal generated manifest: %v", decodeErr)
	}
	return m
}

// TestGenerate_PreservesManifestMetadata verifies the capability arrays are the
// only thing this command owns.
//
// The icon, the tags, and the version are carried over verbatim — the last one
// because the release stamp writes it, and a generator that reset it would
// publish the previous release's number.
func TestGenerate_PreservesManifestMetadata(t *testing.T) {
	m := generatedManifest(t, populatedManifest)

	if m.Identifier != "jmrplens-gitlab-mcp-server" || m.Version != "9.9.9" || m.Description != "desc" {
		t.Fatalf("metadata was not preserved: %+v", m)
	}
	if m.Icon != "https://example.com/icon.png" {
		t.Fatalf("icon = %q, want it preserved", m.Icon)
	}
	if len(m.Tags) != 2 {
		t.Fatalf("tags = %v, want both preserved", m.Tags)
	}
}

// TestGenerate_FillsEveryCapabilityArray verifies each generated entry carries
// the fields the marketplace listing renders.
//
// A tool without a description or an input schema, or a resource without a URI,
// still counts toward the capability badge but shows up blank in the listing.
func TestGenerate_FillsEveryCapabilityArray(t *testing.T) {
	m := generatedManifest(t, populatedManifest)

	if len(m.Tools) == 0 || len(m.Prompts) == 0 || len(m.Resources) == 0 {
		t.Fatalf("got %d tools, %d prompts, %d resources; want all non-zero", len(m.Tools), len(m.Prompts), len(m.Resources))
	}
	for _, tool := range m.Tools {
		if tool.Name == "" || tool.Description == "" || tool.InputSchema == nil {
			t.Fatalf("tool %q is missing name, description, or inputSchema", tool.Name)
		}
	}
	for _, prompt := range m.Prompts {
		if prompt.Name == "" || prompt.Description == "" {
			t.Fatalf("prompt %q is missing name or description", prompt.Name)
		}
	}
	for _, resource := range m.Resources {
		if resource.URI == "" || resource.Name == "" {
			t.Fatalf("resource %q is missing uri or name", resource.Name)
		}
	}
}

// TestGenerate_ReportsCapabilityCounts verifies the summary the command prints
// names all three capability arrays, so a run that silently produced an empty
// one is visible in the output.
func TestGenerate_ReportsCapabilityCounts(t *testing.T) {
	_, counts, err := generate([]byte(minimalManifest))
	if err != nil {
		t.Fatalf("generate() error: %v", err)
	}
	for _, want := range []string{"tools", "prompts", "resources"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(counts, want) {
				t.Fatalf("counts summary = %q, want it to mention %q", counts, want)
			}
		})
	}
}

// TestGenerate_DeclaresDefaultDynamicSurface verifies the manifest describes the
// two-tool dynamic surface a user gets with no configuration.
//
// TOOL_SURFACE is set to a different catalog for the duration of the test, and
// the expected result is unchanged output: reading the environment here would
// make the committed file depend on the machine that generated it.
func TestGenerate_DeclaresDefaultDynamicSurface(t *testing.T) {
	t.Setenv("TOOL_SURFACE", "individual")

	m := generatedManifest(t, minimalManifest)

	if len(m.Tools) != 2 {
		t.Fatalf("len(tools) = %d, want the 2 dynamic tools", len(m.Tools))
	}
	names := []string{m.Tools[0].Name, m.Tools[1].Name}
	want := []string{"gitlab_execute_action", "gitlab_find_action"}
	for i, name := range want {
		t.Run(name, func(t *testing.T) {
			if names[i] != name {
				t.Fatalf("tools = %v, want %v", names, want)
			}
		})
	}
}

// TestGenerate_OmitsOutputSchemaAndIcons verifies the generated tool entries
// carry only the documented marketplace shape.
//
// The output schemas and the base64 icon data URIs are dropped: neither is part
// of the shape LobeHub documents, and the icons alone would triple the file for
// something the listing never renders.
func TestGenerate_OmitsOutputSchemaAndIcons(t *testing.T) {
	out, _, err := generate([]byte(minimalManifest))
	if err != nil {
		t.Fatalf("generate() error: %v", err)
	}

	var raw struct {
		Tools     []map[string]json.RawMessage `json:"tools"`
		Prompts   []map[string]json.RawMessage `json:"prompts"`
		Resources []map[string]json.RawMessage `json:"resources"`
	}
	if decodeErr := json.Unmarshal(out, &raw); decodeErr != nil {
		t.Fatalf("unmarshal generated manifest: %v", decodeErr)
	}
	groups := map[string][]map[string]json.RawMessage{
		"tool": raw.Tools, "prompt": raw.Prompts, "resource": raw.Resources,
	}
	for kind, entries := range groups {
		t.Run(kind, func(t *testing.T) {
			for _, entry := range entries {
				for _, unwanted := range []string{"outputSchema", "icons", "_meta"} {
					if _, found := entry[unwanted]; found {
						t.Fatalf("%s entry carries %q, want it dropped", kind, unwanted)
					}
				}
			}
		})
	}
}

// TestGenerate_Idempotent verifies that re-running generate over its own output
// changes nothing. The --check gate compares bytes, so any instability here
// would fail CI on a machine that merely ran the generator twice.
func TestGenerate_Idempotent(t *testing.T) {
	first, _, err := generate([]byte(minimalManifest))
	if err != nil {
		t.Fatalf("first generate() error: %v", err)
	}
	second, _, err := generate(first)
	if err != nil {
		t.Fatalf("second generate() error: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("generate() is not idempotent: a second run produced different bytes")
	}
}

// TestGenerate_RejectsUnknownField verifies a misspelled key fails loudly.
//
// LobeHub's publish endpoint strips unknown fields silently, so a typo would
// otherwise be dropped here and never noticed in the listing either. Note the
// limit of the guard: encoding/json matches field names case-insensitively, so
// "authorURL" still binds to authorUrl. It catches a wrong name, not wrong
// casing.
func TestGenerate_RejectsUnknownField(t *testing.T) {
	in := []byte(`{"identifier": "x", "name": "y", "version": "1.0.0", "autorUrl": "https://example.com"}`)

	_, _, err := generate(in)
	if err == nil {
		t.Fatal("generate() error = nil, want an error for an unknown field")
	}
	if !strings.Contains(err.Error(), "autorUrl") {
		t.Fatalf("generate() error = %v, want it to name the offending field", err)
	}
}

// TestGenerate_RejectsUnpublishableManifest verifies a manifest the publish
// endpoint would reject fails here instead of being rewritten and certified by
// --check.
//
// Malformed JSON, a manifest missing a required field, and a second JSON value
// after the object all have to fail: the decoder reads one value, so trailing
// content would otherwise be dropped silently on the next rewrite.
func TestGenerate_RejectsUnpublishableManifest(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr string
	}{
		{name: "malformed json", in: "{not json", wantErr: "parse"},
		{name: "missing identifier", in: `{"name": "y", "version": "1.0.0"}`, wantErr: "identifier"},
		{name: "missing name and version", in: `{"identifier": "x"}`, wantErr: "name, version"},
		{name: "blank version", in: `{"identifier": "x", "name": "y", "version": "  "}`, wantErr: "version"},
		{
			name:    "trailing json value",
			in:      `{"identifier": "x", "name": "y", "version": "1.0.0"} {"identifier": "z"}`,
			wantErr: "unexpected content",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := generate([]byte(tt.in))
			if err == nil {
				t.Fatalf("generate(%s) error = nil, want an error", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("generate(%s) error = %v, want it to mention %q", tt.name, err, tt.wantErr)
			}
		})
	}
}

// TestManifestPrompts_SortedByName verifies prompts come out in name order, so
// the file does not churn when registration order changes.
func TestManifestPrompts_SortedByName(t *testing.T) {
	got := manifestPrompts([]*mcp.Prompt{
		{Name: "review_mr"},
		{Name: "audit_project_full"},
		{Name: "my_open_mrs"},
	})

	want := []string{"audit_project_full", "my_open_mrs", "review_mr"}
	for i, name := range want {
		t.Run(name, func(t *testing.T) {
			if got[i].Name != name {
				t.Fatalf("position %d = %q, want %q", i, got[i].Name, name)
			}
		})
	}
}

// TestManifestResources_SortedByName verifies resources are ordered by name and
// keep the URI and annotations the listing displays.
func TestManifestResources_SortedByName(t *testing.T) {
	got := manifestResources([]*mcp.Resource{
		{Name: "groups", URI: "gitlab://groups"},
		{Name: "current_user", URI: "gitlab://user/current", Annotations: &mcp.Annotations{Priority: 0.6}},
	})

	if len(got) != 2 {
		t.Fatalf("len(manifestResources()) = %d, want 2", len(got))
	}
	if got[0].Name != "current_user" || got[1].Name != "groups" {
		t.Fatalf("resources = %q/%q, want current_user before groups", got[0].Name, got[1].Name)
	}
	if got[0].URI != "gitlab://user/current" || got[0].Annotations == nil {
		t.Fatalf("resource fields were dropped: %+v", got[0])
	}
}

// TestRun_CheckModeAcceptsCommittedManifest verifies the committed
// lhm.plugin.json matches the registered surface.
//
// This is the same gate CI runs; failing here means the manifest needs
// regenerating before the marketplace listing goes stale.
func TestRun_CheckModeAcceptsCommittedManifest(t *testing.T) {
	if err := run(true); err != nil {
		t.Fatalf("run(true) error: %v", err)
	}
}

// chdirFixtureProject makes a temporary project root (a directory holding a
// go.mod) the working directory for the rest of the test and returns it, so
// run's project-root walk lands there instead of in this repository.
func chdirFixtureProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	t.Chdir(root)
	return root
}

// TestRun_FixtureProject_Scenarios verifies run against a throwaway project
// root: a missing manifest and an unparsable one are reported by stage, check
// mode rejects a manifest whose capability arrays are stale, and a write run
// rewrites the manifest to exactly what generate produces and then passes
// check mode.
func TestRun_FixtureProject_Scenarios(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		absent   bool
		check    bool
		wantErr  string
	}{
		{name: "missing manifest", absent: true, check: true, wantErr: "read " + manifestFileName},
		{name: "unparsable manifest", manifest: "{", check: true, wantErr: "parse " + manifestFileName},
		{name: "check rejects stale arrays", manifest: minimalManifest, check: true, wantErr: manifestFileName + " is stale"},
		{name: "write rewrites the manifest", manifest: minimalManifest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := chdirFixtureProject(t)
			if !tt.absent {
				if err := os.WriteFile(filepath.Join(root, manifestFileName), []byte(tt.manifest), 0o600); err != nil {
					t.Fatalf("write manifest: %v", err)
				}
			}

			err := run(tt.check)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("run(%v) error = %v, want containing %q", tt.check, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("run(%v) error = %v", tt.check, err)
			}
			assertManifestRewritten(t, root, tt.manifest)
		})
	}
}

// assertManifestRewritten checks that the manifest under root is byte-equal
// to what generate produces from the original content, and that check mode
// then accepts it.
func assertManifestRewritten(t *testing.T, root, original string) {
	t.Helper()
	want, _, genErr := generate([]byte(original))
	if genErr != nil {
		t.Fatalf("generate() error = %v", genErr)
	}
	got, readErr := os.ReadFile(filepath.Join(root, manifestFileName))
	if readErr != nil {
		t.Fatalf("read rewritten manifest: %v", readErr)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("rewritten manifest differs from generate() output:\n%s", got)
	}
	if checkErr := run(true); checkErr != nil {
		t.Fatalf("run(true) after the rewrite error = %v, want the manifest accepted", checkErr)
	}
}

// TestRun_RemovedWorkingDirectory_ReturnsProjectRootError verifies run
// surfaces the project-root lookup failure when the working directory was
// removed from under the process, instead of reading a manifest from nowhere.
func TestRun_RemovedWorkingDirectory_ReturnsProjectRootError(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(gone, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Chdir(gone)
	// Windows refuses to remove a process's working directory, and macOS
	// keeps answering getcwd from the path it remembers, so on neither can
	// the failure be produced this way: both skip rather than report the
	// operating system's design as a defect here.
	if err := os.RemoveAll(gone); err != nil {
		t.Skipf("this platform will not remove the working directory: %v", err)
	}
	if _, err := os.Getwd(); err == nil {
		t.Skip("this platform's getcwd still answers after the working directory is removed")
	}

	err := run(true)
	if err == nil || !strings.Contains(err.Error(), "get working directory") {
		t.Fatalf("run(true) error = %v, want the working-directory error", err)
	}
}
