// main_test.go covers LobeHub manifest generation: the capability arrays are
// filled from the registered MCP surface, every other manifest field survives a
// regeneration, and a malformed or misspelled manifest fails loudly instead of
// being overwritten.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/freshness"
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

// TestGenerate_CountsSummaryMatchesTheArrays verifies the three numbers in the
// summary are the lengths of the three arrays the manifest carries, each under
// its own word.
//
// The summary is the only thing a run prints, and the three figures are
// interchangeable integers: exchanging two of them reports a surface the file
// does not hold, and no caller of this command reads anything else. The test
// first establishes that the three counts really differ, since against equal
// counts a crossing would be invisible here too.
func TestGenerate_CountsSummaryMatchesTheArrays(t *testing.T) {
	out, counts, err := generate([]byte(minimalManifest))
	if err != nil {
		t.Fatalf("generate() error: %v", err)
	}
	var m manifest
	if decodeErr := json.Unmarshal(out, &m); decodeErr != nil {
		t.Fatalf("unmarshal generated manifest: %v", decodeErr)
	}

	tools, prompts, resources := len(m.Tools), len(m.Prompts), len(m.Resources)
	if tools == prompts || prompts == resources || tools == resources {
		t.Fatalf("the registered surface has %d tools, %d prompts and %d resources; two counts that agree cannot witness a crossing", tools, prompts, resources)
	}
	want := fmt.Sprintf("%d tools, %d prompts, %d resources", tools, prompts, resources)
	if counts != want {
		t.Fatalf("counts summary = %q, want %q", counts, want)
	}
}

// TestGenerate_CarriesLocalizationsAndLeavesMarkupUnescaped verifies a
// localization block survives the rewrite and that the encoder does not escape
// the markup characters a description may carry.
//
// Two properties meet here. Localizations are held as raw JSON, so this is what
// pins that everything the decoder accepts can be written back out, which is why
// the encode failure the generator guards against cannot happen for a manifest
// that parsed. And HTML escaping is off, so a description carrying an angle
// bracket or an ampersand stays readable in the committed file instead of
// arriving at the marketplace as a run of numeric escapes.
func TestGenerate_CarriesLocalizationsAndLeavesMarkupUnescaped(t *testing.T) {
	in := `{
  "identifier": "jmrplens-gitlab-mcp-server",
  "name": "GitLab MCP Server",
  "version": "9.9.9",
  "localizations": [{"locale": "es-ES", "description": "Servidor MCP <GitLab> & compania"}]
}`

	out, _, err := generate([]byte(in))
	if err != nil {
		t.Fatalf("generate() error: %v", err)
	}

	if !strings.Contains(string(out), "<GitLab> & compania") {
		t.Errorf("generated manifest escaped the markup characters:\n%s", out)
	}
	var m manifest
	if decodeErr := json.Unmarshal(out, &m); decodeErr != nil {
		t.Fatalf("unmarshal generated manifest: %v", decodeErr)
	}
	if len(m.Localizations) != 1 {
		t.Fatalf("len(localizations) = %d, want the one it was given", len(m.Localizations))
	}
	var localization struct {
		Locale      string `json:"locale"`
		Description string `json:"description"`
	}
	if decodeErr := json.Unmarshal(m.Localizations[0], &localization); decodeErr != nil {
		t.Fatalf("unmarshal the localization: %v", decodeErr)
	}
	if localization.Locale != "es-ES" || localization.Description == "" {
		t.Errorf("localization = %+v, want it carried through unchanged", localization)
	}
}

// TestGenerate_DeclaresDefaultDynamicSurface verifies the manifest describes the
// two-tool dynamic surface a user gets with no configuration.
//
// TOOL_SURFACE is set to a different catalog for the duration of the test, and
// the expected result is unchanged output: reading the environment here would
// make the committed file depend on the machine that generated it.
func TestGenerate_DeclaresDefaultDynamicSurface(t *testing.T) {
	t.Setenv("GITLAB_MCP_TOOL_SURFACE", "individual")

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

// TestManifestTools_SortedByName verifies tools come out in name order.
//
// The registered surface is the two dynamic tools, which arrive find-then-
// execute and are therefore always reordered here; nothing drove the converter
// with a list already in order, so the comparator was only ever asked a
// question it answered yes to.
func TestManifestTools_SortedByName(t *testing.T) {
	got := manifestTools([]*mcp.Tool{
		{Name: "gitlab_execute_action"},
		{Name: "gitlab_find_action"},
		{Name: "gitlab_discover_project"},
	})

	want := []string{"gitlab_discover_project", "gitlab_execute_action", "gitlab_find_action"}
	for i, name := range want {
		t.Run(name, func(t *testing.T) {
			if got[i].Name != name {
				t.Fatalf("position %d = %q, want %q", i, got[i].Name, name)
			}
		})
	}
}

// TestManifestEntries_CarryEachFieldToItsOwnKey verifies each converter copies
// every field to the manifest key of the same meaning.
//
// Two of these fields are a pair no other test can tell apart: a title and a
// description are both free text, so exchanging them changes no type, no
// ordering and no count, and the listing would render the one-line title as the
// blurb and the blurb as the title. Each entry is therefore built with a value
// no sibling field shares and compared whole.
func TestManifestEntries_CarryEachFieldToItsOwnKey(t *testing.T) {
	t.Run("tool", func(t *testing.T) {
		annotations := &mcp.ToolAnnotations{ReadOnlyHint: true}
		schema := map[string]any{"type": "object"}
		got := manifestTools([]*mcp.Tool{{
			Name:        "tool-name",
			Title:       "tool-title",
			Description: "tool-description",
			InputSchema: schema,
			Annotations: annotations,
		}})

		want := []manifestTool{{
			Name:        "tool-name",
			Title:       "tool-title",
			Description: "tool-description",
			InputSchema: schema,
			Annotations: annotations,
		}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("manifestTools() = %+v, want %+v", got, want)
		}
	})

	t.Run("prompt", func(t *testing.T) {
		arguments := []*mcp.PromptArgument{{Name: "argument-name"}}
		got := manifestPrompts([]*mcp.Prompt{{
			Name:        "prompt-name",
			Title:       "prompt-title",
			Description: "prompt-description",
			Arguments:   arguments,
		}})

		want := []manifestPrompt{{
			Name:        "prompt-name",
			Title:       "prompt-title",
			Description: "prompt-description",
			Arguments:   arguments,
		}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("manifestPrompts() = %+v, want %+v", got, want)
		}
	})

	t.Run("resource", func(t *testing.T) {
		annotations := &mcp.Annotations{Priority: 0.6}
		got := manifestResources([]*mcp.Resource{{
			URI:         "gitlab://resource-uri",
			Name:        "resource-name",
			Title:       "resource-title",
			Description: "resource-description",
			MIMEType:    "application/resource-mime",
			Annotations: annotations,
		}})

		want := []manifestResource{{
			URI:         "gitlab://resource-uri",
			Name:        "resource-name",
			Title:       "resource-title",
			Description: "resource-description",
			MIMEType:    "application/resource-mime",
			Annotations: annotations,
		}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("manifestResources() = %+v, want %+v", got, want)
		}
	})
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
// regenerating before the marketplace listing goes stale. Being the same gate,
// it is deferred the same way: ci.yml skips its "Check LobeHub manifest" step
// below the top of a stack, where the manifest is stale on purpose, and this
// test used to fail there instead (issue 644). The comparison is not lost, it
// runs where the manifest is refreshed. Every other test in this file uses a
// throwaway project root and is not deferred, so a manifest the generator
// would rewrite still fails check mode on every run.
func TestRun_CheckModeAcceptsCommittedManifest(t *testing.T) {
	freshness.SkipIfDeferred(t)
	if err := run(io.Discard, true); err != nil {
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
//
// The stale case also holds the command a reader is told to run. That string is
// the only actionable half of the refusal, and it reaches the message through
// an argument no other assertion here reads.
func TestRun_FixtureProject_Scenarios(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		absent   bool
		check    bool
		wantErr  []string
	}{
		{name: "missing manifest", absent: true, check: true, wantErr: []string{"read " + manifestFileName}},
		{name: "unparsable manifest", manifest: "{", check: true, wantErr: []string{"parse " + manifestFileName}},
		{
			name:     "check rejects stale arrays",
			manifest: minimalManifest,
			check:    true,
			wantErr:  []string{manifestFileName + " is stale", regenerateHint},
		},
		{name: "write rewrites the manifest", manifest: minimalManifest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := chdirFixtureProject(t)
			if !tt.absent {
				writeManifest(t, root, tt.manifest)
			}

			var stdout bytes.Buffer
			err := run(&stdout, tt.check)
			if len(tt.wantErr) > 0 {
				assertErrorMentions(t, err, tt.wantErr)
				if stdout.Len() != 0 {
					t.Errorf("stdout = %q, want nothing printed for a failed run", stdout.String())
				}
				return
			}
			if err != nil {
				t.Fatalf("run(%v) error = %v", tt.check, err)
			}
			assertSummary(t, stdout.String(), "Generated "+manifestFileName)
			assertManifestRewritten(t, root, tt.manifest)
		})
	}
}

// writeManifest lays content down as the manifest of the fixture project under
// root.
func writeManifest(t *testing.T, root, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, manifestFileName), []byte(content), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

// assertErrorMentions checks that err was returned and that it names each of
// want, so a refusal is held to every part of it a reader acts on rather than
// to the first phrase alone.
func assertErrorMentions(t *testing.T, err error, want []string) {
	t.Helper()
	for _, phrase := range want {
		if err == nil || !strings.Contains(err.Error(), phrase) {
			t.Fatalf("error = %v, want containing %q", err, phrase)
		}
	}
}

// assertSummary checks that a run's one line of output opens with prefix and
// closes with the capability counts in parentheses.
//
// The two summaries differ only in that prefix: one says the file was written,
// the other that the committed one is already current, so a test reading only
// the counts could not tell a write run from a check run.
func assertSummary(t *testing.T, got, prefix string) {
	t.Helper()
	if !strings.HasPrefix(got, prefix) {
		t.Errorf("summary = %q, want it to start with %q", got, prefix)
	}
	wantCounts := "(" + registeredCounts(t) + ")\n"
	if !strings.HasSuffix(got, wantCounts) {
		t.Errorf("summary = %q, want it to end with %q", got, wantCounts)
	}
}

// registeredCounts is the counts summary generate reports for the registered
// surface, read back through generate itself so the expectation is not a second
// copy of the same numbers.
func registeredCounts(t *testing.T) string {
	t.Helper()
	_, counts, err := generate([]byte(minimalManifest))
	if err != nil {
		t.Fatalf("generate() error = %v", err)
	}
	return counts
}

// assertManifestRewritten checks that the manifest under root is byte-equal
// to what generate produces from the original content, and that check mode
// then accepts it and says so.
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
	var stdout bytes.Buffer
	if checkErr := run(&stdout, true); checkErr != nil {
		t.Fatalf("run(true) after the rewrite error = %v, want the manifest accepted", checkErr)
	}
	assertSummary(t, stdout.String(), manifestFileName+" is current")
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

	err := run(io.Discard, true)
	if err == nil || !strings.Contains(err.Error(), "get working directory") {
		t.Fatalf("run(true) error = %v, want the working-directory error", err)
	}
}

// TestRun_UnopenableProjectRoot_NamesTheStage verifies the arm that reports a
// project root it could not open says which stage failed, so it is not read as
// the manifest read below it, which is fixed by a different thing.
//
// The directory handed to the open is the one the project-root walk has just
// found a go.mod in, so no fixture can make that open fail; the seam is
// replaced for the length of this test instead, which is the only way the arm
// is reachable at all.
func TestRun_UnopenableProjectRoot_NamesTheStage(t *testing.T) {
	writeManifest(t, chdirFixtureProject(t), minimalManifest)
	original := openRoot
	openRoot = func(string) (*os.Root, error) { return nil, errors.New("the root went away") }
	t.Cleanup(func() { openRoot = original })

	err := run(io.Discard, true)

	if err == nil || !strings.Contains(err.Error(), "open project root: the root went away") {
		t.Fatalf("run(true) error = %v, want the open stage named", err)
	}
}

// TestRunMain_FlagParsing_ReturnsTheExitCode verifies a parse failure is an exit
// code this function returns rather than an os.Exit inside the flag package: an
// unknown flag is the usage exit, 2, and -h is the one parse failure that exits
// clean, which is what the package-level ExitOnError flag set would have done
// for both. Neither reaches the run, which the absent manifest witnesses.
func TestRunMain_FlagParsing_ReturnsTheExitCode(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
		text string
	}{
		{name: "an unknown flag is a usage error", args: []string{"-bogus"}, want: 2, text: "flag provided but not defined: -bogus"},
		{name: "asking for help exits clean", args: []string{"-h"}, want: 0, text: "-check"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := chdirFixtureProject(t)

			var stdout, stderr bytes.Buffer
			if code := runMain(tt.args, &stdout, &stderr); code != tt.want {
				t.Fatalf("runMain(%v) = %d, want %d (stderr %q)", tt.args, code, tt.want, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.text) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.text)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing on it past a failed parse", stdout.String())
			}
			if _, err := os.Stat(filepath.Join(root, manifestFileName)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("os.Stat(manifest) = %v, want nothing written past a failed parse", err)
			}
		})
	}
}

// TestRunMain_FixtureProject_ExitCodeAndStream verifies the two endings a run
// has, and that each writes to the stream a caller reads it from: a write run
// exits 0 with its summary on stdout, and a stale manifest exits 1 with the
// refusal on stderr and nothing on stdout.
func TestRunMain_FixtureProject_ExitCodeAndStream(t *testing.T) {
	t.Run("a write run exits zero and reports on stdout", func(t *testing.T) {
		writeManifest(t, chdirFixtureProject(t), minimalManifest)

		var stdout, stderr bytes.Buffer
		if code := runMain(nil, &stdout, &stderr); code != 0 {
			t.Fatalf("runMain() = %d, want 0 (stderr %q)", code, stderr.String())
		}
		assertSummary(t, stdout.String(), "Generated "+manifestFileName)
		if stderr.Len() != 0 {
			t.Errorf("stderr = %q, want nothing on it for a run that worked", stderr.String())
		}
	})

	t.Run("a stale manifest exits one and refuses on stderr", func(t *testing.T) {
		root := chdirFixtureProject(t)
		writeManifest(t, root, minimalManifest)

		var stdout, stderr bytes.Buffer
		if code := runMain([]string{"-check"}, &stdout, &stderr); code != 1 {
			t.Fatalf("runMain(-check) = %d, want 1 (stderr %q)", code, stderr.String())
		}
		if !strings.Contains(stderr.String(), manifestFileName+" is stale") {
			t.Errorf("stderr = %q, want it to say the manifest is stale", stderr.String())
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout = %q, want nothing on it for a refused run", stdout.String())
		}
		got, err := os.ReadFile(filepath.Join(root, manifestFileName))
		if err != nil {
			t.Fatalf("read manifest: %v", err)
		}
		if string(got) != minimalManifest {
			t.Errorf("check mode rewrote the manifest:\n%s", got)
		}
	})
}

// TestMain_HandsTheExitCodeToTheSeam verifies main wires runMain's result to the
// exit seam and reads its flags from os.Args, which is the only thing main does
// and the one line no other test here reaches. Both endings are driven, since a
// main that exited a constant would satisfy either one alone.
func TestMain_HandsTheExitCodeToTheSeam(t *testing.T) {
	tests := []struct {
		name          string
		writeManifest bool
		want          int
	}{
		{name: "a manifest it can rewrite exits zero", writeManifest: true, want: 0},
		{name: "no manifest at all exits one", want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := chdirFixtureProject(t)
			if tt.writeManifest {
				writeManifest(t, root, minimalManifest)
			}
			originalArgs := os.Args
			os.Args = []string{toolName}
			t.Cleanup(func() { os.Args = originalArgs })
			code := -1
			originalExit := osExit
			osExit = func(got int) { code = got }
			t.Cleanup(func() { osExit = originalExit })

			main()

			if code != tt.want {
				t.Fatalf("main() exited %d, want %d", code, tt.want)
			}
		})
	}
}
