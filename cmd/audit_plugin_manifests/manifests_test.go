// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// manifests_test.go holds the check to what scripts/check-openplugin.sh
// accepted and refused, and covers the rules it keeps in code.
//
// testdata/check_openplugin.json is the record. Each case laid out the
// tree's plugin.json, .plugin/plugin.json, .plugin/plugin.schema.json and
// mcp.json in a scratch directory, with one change to one of them, ran the
// script at d03f4b2f0 (unchanged since #1563) under jq 1.6, and kept its exit
// status and output beside the four files it read. The cases are every check
// the script made, each passing and each refused, plus three shapes its key
// sets let through and the schemas refuse.
package main

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// openpluginRecord is testdata/check_openplugin.json.
type openpluginRecord struct {
	RecordedFrom string `json:"recorded_from"`
	Cases        []struct {
		Name   string            `json:"name"`
		Files  map[string]string `json:"files"`
		Exit   int               `json:"exit"`
		Stdout string            `json:"stdout"`
		Stderr string            `json:"stderr"`
	} `json:"cases"`
}

// The schema each manifest declares, as a problem of the command names it.
const (
	agentSchemaProblem  = "ERROR: plugin.json does not match the schema it declares, " + pluginSchemaURL + ": "
	mcpSchemaProblem    = "ERROR: mcp.json does not match the schema it declares, " + mcpSchemaURL + ": "
	legacySchemaProblem = "ERROR: .plugin/plugin.json does not match the schema it declares, ./plugin.schema.json: "
)

// openpluginDifferences are the recorded cases the command answers in its
// own words: the key sets the script checked by hand, which the schemas now
// hold, the files jq could not parse, and the three shapes only the schemas
// refuse. Each maps to the line the command's refusal opens with.
var openpluginDifferences = map[string]string{
	"plugin.json carrying logo":              agentSchemaProblem,
	"plugin.json carrying mcpServers":        agentSchemaProblem,
	"an author key the schema forbids":       agentSchemaProblem,
	"plugin.json that is not JSON":           "ERROR: plugin.json could not be read as a JSON object: ",
	"mcp.json that is not JSON":              "ERROR: mcp.json could not be read as a JSON object: ",
	"the legacy manifest that is not JSON":   "ERROR: .plugin/plugin.json could not be read as a JSON object: ",
	"keywords that are a string":             agentSchemaProblem,
	"a server field the schema forbids":      mcpSchemaProblem,
	"a legacy manifest without a license":    legacySchemaProblem,
	"the legacy manifest without mcpServers": legacySchemaProblem,
}

// layOut writes files below a new directory and returns it.
func layOut(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, text := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil { //#nosec G703 -- a scratch file below the test's own directory
			t.Fatal(err)
		}
	}
	return root
}

// readOpenpluginRecord decodes the record.
func readOpenpluginRecord(t *testing.T) openpluginRecord {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "check_openplugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record openpluginRecord
	if decodeErr := json.Unmarshal(data, &record); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if len(record.Cases) == 0 {
		t.Fatal("the record holds no case")
	}
	return record
}

// TestRun_CheckOpenpluginRecord_AcceptsAndRefusesWhatTheScriptDid verifies
// every recorded case: a tree the script accepted passes with its line, and
// one it refused is refused, with the script's own line for every rule kept
// in code, or with the line of the command's own words that
// openpluginDifferences names.
func TestRun_CheckOpenpluginRecord_AcceptsAndRefusesWhatTheScriptDid(t *testing.T) {
	t.Parallel()

	record := readOpenpluginRecord(t)
	for _, tc := range record.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			code := run(layOut(t, tc.Files), &stdout, &stderr)
			if opening, differs := openpluginDifferences[tc.Name]; differs {
				if code != 1 || !hasLineStarting(stderr.String(), opening) {
					t.Errorf("exit %d and stderr\n%s\nwant 1 and a line opening with %q", code, stderr.String(), opening)
				}
				return
			}
			if tc.Exit == 0 {
				if code != 0 || stdout.String() != tc.Stdout || stderr.Len() != 0 {
					t.Errorf("exit %d, stdout %q, stderr %q, want 0 and %q", code, stdout.String(), stderr.String(), tc.Stdout)
				}
				return
			}
			want := strings.TrimSuffix(strings.SplitN(tc.Stderr, "\n", 2)[0], "\n")
			if code != 1 || !hasLineStarting(stderr.String(), want) || stdout.Len() != 0 {
				t.Errorf("exit %d and stderr\n%s\nwant 1 and the script's line %q", code, stderr.String(), want)
			}
		})
	}
}

// hasLineStarting reports whether a line of text opens with prefix.
func hasLineStarting(text, prefix string) bool {
	for line := range strings.Lines(text) {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// TestOpenpluginDifferences_EachNamesARecordedCase holds the table of
// differences to the record: an entry naming no case excuses nothing.
func TestOpenpluginDifferences_EachNamesARecordedCase(t *testing.T) {
	t.Parallel()

	names := map[string]bool{}
	for _, tc := range readOpenpluginRecord(t).Cases {
		names[tc.Name] = true
	}
	for name := range openpluginDifferences {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if !names[name] {
				t.Errorf("no recorded case is named %q", name)
			}
		})
	}
}

// TestRun_TheRepositorysManifests_Pass verifies the manifests the tree holds
// now, rather than the recorded copy.
func TestRun_TheRepositorysManifests_Pass(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run(filepath.Join("..", ".."), &stdout, &stderr); code != 0 || stdout.String() != valid+"\n" {
		t.Errorf("exit %d, stdout %q, stderr %q, want the manifests valid", code, stdout.String(), stderr.String())
	}
}

// TestCheckManifests_SingleFaults_AreReportedOnceInTheScriptsWords verifies
// the exact output of the rules kept in code, one fault at a time on the
// committed manifests, and that each is reported alone where nothing else
// is wrong, and every problem where several are.
func TestCheckManifests_SingleFaults_AreReportedOnceInTheScriptsWords(t *testing.T) {
	t.Parallel()

	record := readOpenpluginRecord(t)
	base := record.Cases[0].Files
	for _, tc := range []struct {
		name, file, from, to string
		want                 []string
	}{
		{
			name: "a name the schema refuses", file: "plugin.json", from: `"name": "gitlab-mcp-server"`, to: `"name": "Bad--Name"`,
			want: []string{
				`plugin.json does not match the schema it declares, ` + pluginSchemaURL + `: name "Bad--Name" does not match the pattern ` + namePattern,
				"plugin.json name must be gitlab-mcp-server",
			},
		},
		{
			name: "no server at all", file: "mcp.json", from: `"mcpServers": {`, to: `"mcpServers": {}, "x": {`,
			want: nil,
		},
		{
			name: "two images", file: "mcp.json", from: `"--rm",`, to: `"--rm", "ghcr.io/jmrplens/gitlab-mcp-server:1.0.0",`,
			want: []string{"Docker image must use latest or a concrete semantic version tag (got: ghcr.io/jmrplens/gitlab-mcp-server:1.0.0\nghcr.io/jmrplens/gitlab-mcp-server:latest)"},
		},
		{
			name: "a PLUGIN_ROOT and then a HOME", file: "mcp.json", from: `"--rm",`, to: `"${PLUGIN_ROOT}/${HOME}",`,
			want: []string{"mcp.json uses ${...} interpolation Agent Plugins does not expand"},
		},
		{
			name: "a legacy manifest declaring a schema nobody carries", file: ".plugin/plugin.json", from: `"$schema": "./plugin.schema.json"`, to: `"$schema": "https://example.com/x.json"`,
			want: []string{`.plugin/plugin.json: it declares $schema "https://example.com/x.json", which is neither a schema this check carries nor a file beside it`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			files := maps.Clone(base)
			if !strings.Contains(files[tc.file], tc.from) {
				t.Fatalf("%s holds no %q", tc.file, tc.from)
			}
			files[tc.file] = strings.Replace(files[tc.file], tc.from, tc.to, 1)
			got := checkManifests(layOut(t, files))
			if tc.want == nil {
				if len(got) == 0 {
					t.Error("the check passed")
				}
				return
			}
			if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Errorf("problems\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
			}
		})
	}
}
