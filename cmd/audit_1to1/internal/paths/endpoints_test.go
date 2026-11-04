// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package paths

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

// fixtureRef is the tag the live record [recordIn] writes was taken at, which
// is the only ref an oracle in the same root may have been read at.
const fixtureRef = "v19.3.1-ee"

// oracleRoot is a planted root holding the live record [recordIn] writes and
// the given oracle, as the file commits it.
func oracleRoot(t *testing.T, oracle []byte) string {
	t.Helper()
	root := recordIn(t, nil)
	writeOracle(t, root, oracle)
	return root
}

// writeOracle puts the oracle where the comparison reads it under root.
func writeOracle(t *testing.T, root string, oracle []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(DocEndpointsFile))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("prepare the oracle: %v", err)
	}
	if err := os.WriteFile(path, oracle, 0o600); err != nil {
		t.Fatalf("prepare the oracle: %v", err)
	}
}

// encodeOracle writes an oracle the way the generator does, from its parts,
// for a case that describes one rather than reads pages for it.
func encodeOracle(t *testing.T, ref string, endpoints ...DocumentedEndpoint) []byte {
	t.Helper()
	encoded, err := json.Marshal(DocEndpoints{
		SchemaVersion: docEndpointsSchema,
		Source:        DocSource{Repository: docRepository, Ref: ref, Areas: 1, Endpoints: len(endpoints)},
		Endpoints:     endpoints,
	})
	if err != nil {
		t.Fatalf("encode the oracle: %v", err)
	}
	return encoded
}

// TestCheckEndpoints_RecordedPaths_AreHeldToTheDocumentation verifies the
// comparison end to end, from pages to verdict, over the spellings the real
// documentation uses: a path written without its leading slash, one carrying
// the /api/v4 prefix, a placeholder where our recording carries a fixture
// value, and a glob standing for the rest of the path. The pages are read by
// the generator, written as the oracle and compared from the file, which is
// the only way the comparison reads them now.
func TestCheckEndpoints_RecordedPaths_AreHeldToTheDocumentation(t *testing.T) {
	fetcher := docsServer(t, map[string]string{
		"branches": "## List branches\n\n```plaintext\nGET /projects/:id/repository/branches\n```\n\n" +
			"```plaintext\nDELETE /projects/:id/repository/branches/:branch\n```\n",
		"access_tokens":  "```plaintext\nGET projects/:id/access_tokens\n```\n",
		"model_registry": "```plaintext\nPUT /api/v4/projects/:id/packages/ml_models/:version/files/(*path)\n```\n",
	})
	oracle, err := BuildDocEndpoints(t.Context(), fetcher, fixtureRef)
	if err != nil {
		t.Fatalf("BuildDocEndpoints() error = %v", err)
	}
	root := oracleRoot(t, oracle)
	rows := []requestinventory.Row{
		{Package: "internal/tools/branches", Kind: requestinventory.KindREST, Method: "GET", Path: "/projects/:id/repository/branches"},
		{Package: "internal/tools/branches", Kind: requestinventory.KindREST, Method: "DELETE", Path: "/projects/:id/repository/branches/main"},
		{Package: "internal/tools/accesstokens", Kind: requestinventory.KindREST, Method: "GET", Path: "/projects/:id/access_tokens"},
		{Package: "internal/tools/modelregistry", Kind: requestinventory.KindREST, Method: "PUT", Path: "/projects/:id/packages/ml_models/:iid/files/models/model.bin"},
		{Package: "internal/tools/integrations", Kind: requestinventory.KindREST, Method: "DELETE", Path: "/projects/:id/services/slack"},
		{Package: "internal/tools/epics", Kind: requestinventory.KindGraphQL, Method: "POST", Path: "/graphql", Operation: "query epics"},
	}

	check, err := checkEndpoints(root, rows)
	if err != nil {
		t.Fatalf("checkEndpoints() error = %v, want nil", err)
	}
	if !check.Ran || check.Oracle != DocEndpointsFile || check.Ref != fixtureRef || check.Areas != 3 || check.DocumentedEndpoints != 4 {
		t.Fatalf("check = %+v, want the oracle of three pages and four endpoints read at %s", check, fixtureRef)
	}
	if check.RecordedEndpoints != 5 {
		t.Errorf("recorded endpoints = %d, want the five REST rows and not the GraphQL one", check.RecordedEndpoints)
	}
	if len(check.Undocumented) != 1 || check.Undocumented[0].Path != "/projects/:id/services/slack" {
		t.Errorf("undocumented = %+v, want only the deprecated services endpoint", check.Undocumented)
	}
}

// TestCheckEndpoints_AnOracleItCannotJudgeBy_Fails verifies that every way the
// comparison cannot hold a request to the documentation stops the run rather
// than reporting findings about the oracle. A missing or unreadable file and a
// shape this build was not written for say nothing about an endpoint; and an
// oracle read at another release than the live record's, or beside a record
// that names none, would hold the requests to a GitLab the other oracles do
// not speak for. The last is what a re-pin that forgets the regeneration
// meets.
func TestCheckEndpoints_AnOracleItCannotJudgeBy_Fails(t *testing.T) {
	cases := []struct {
		name    string
		arrange func(t *testing.T) string
		want    string
	}{
		{
			name:    "no oracle",
			arrange: func(t *testing.T) string { t.Helper(); return recordIn(t, nil) },
			want:    "make gen-api-doc-endpoints writes it",
		},
		{
			name: "an oracle that is not JSON",
			arrange: func(t *testing.T) string {
				t.Helper()
				return oracleRoot(t, []byte("{"))
			},
			want: "decode " + DocEndpointsFile,
		},
		{
			name: "an oracle of another shape",
			arrange: func(t *testing.T) string {
				t.Helper()
				return oracleRoot(t, []byte(`{"schema_version": 2, "source": {"ref": "v19.3.1-ee"}}`))
			},
			want: "schema version 2 and this build reads version 1",
		},
		{
			name: "an oracle read at another release",
			arrange: func(t *testing.T) string {
				t.Helper()
				return oracleRoot(t, encodeOracle(t, "v19.2.0-ee"))
			},
			want: "was read at v19.2.0-ee and the live API record is of v19.3.1-ee",
		},
		{
			name: "no live record to name the release",
			arrange: func(t *testing.T) string {
				t.Helper()
				root := t.TempDir()
				writeOracle(t, root, encodeOracle(t, fixtureRef))
				return root
			},
			want: "read the release the documentation oracle must have been read at",
		},
		{
			name: "a live record that names no release",
			arrange: func(t *testing.T) string {
				t.Helper()
				root := t.TempDir()
				record := fixtureDocument(nil)
				record.Source.Version = ""
				if err := apilive.Write(recordDir(root), record); err != nil {
					t.Fatalf("prepare the record: %v", err)
				}
				writeOracle(t, root, encodeOracle(t, fixtureRef))
				return root
			},
			want: "names no release",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := testCase.arrange(t)

			check, err := checkEndpoints(root, nil)

			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("checkEndpoints() error = %v, want one naming %q", err, testCase.want)
			}
			if check.Ran {
				t.Errorf("check = %+v, want nothing compared", check)
			}
		})
	}
}

// TestMatchesShape_TheToleranceIsOneSided verifies the comparison itself. A
// documented placeholder matches whatever we put there, which is what makes a
// recorded fixture value comparable at all; nothing else is tolerated.
func TestMatchesShape_TheToleranceIsOneSided(t *testing.T) {
	cases := []struct {
		name       string
		recorded   string
		documented string
		want       bool
	}{
		{name: "the same path", recorded: "/projects/:id/issues", documented: "/projects/:id/issues", want: true},
		{name: "a fixture value under a placeholder", recorded: "/projects/:id/repository/branches/main", documented: "/projects/:id/repository/branches/:branch", want: true},
		{name: "a worked example in the documentation", recorded: "/projects/:id/issues/:iid", documented: "/projects/1/issues/80", want: true},
		{name: "an angle-bracket placeholder", recorded: "/projects/:id/foo", documented: "/projects/<id>/foo", want: true},
		{name: "a glob swallowing the rest", recorded: "/packages/:id/files/models/model.bin", documented: "/packages/:id/files/(*path)", want: true},
		{name: "a glob with nothing left to swallow", recorded: "/packages/:id/files", documented: "/packages/:id/files/(*path)", want: false},
		{name: "a glob written without its parentheses", recorded: "/packages/:id/files/models/model.bin", documented: "/packages/:id/files/*path", want: true},
		// An identifier we sent empty is not a value a placeholder stands for.
		// Letting one match is how the eight requests this server makes with an
		// empty identifier were read as endpoints GitLab documents.
		{name: "an empty identifier under a placeholder", recorded: "/projects//statistics", documented: "/projects/:id/statistics", want: false},

		{name: "a different literal", recorded: "/projects/:id/services/slack", documented: "/projects/:id/integrations/:slug", want: false},
		{name: "one segment too many", recorded: "/projects/:id/issues/:iid", documented: "/projects/:id/issues", want: false},
		{name: "one segment too few", recorded: "/projects/:id", documented: "/projects/:id/issues", want: false},
		{name: "a placeholder of ours over a literal of theirs", recorded: "/projects/:id/:iid", documented: "/projects/:id/issues", want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := matchesShape(normalizePath(testCase.recorded), normalizePath(testCase.documented))

			if got != testCase.want {
				t.Errorf("matchesShape(%q, %q) = %v, want %v", testCase.recorded, testCase.documented, got, testCase.want)
			}
		})
	}
}

// TestNormalizePath_TheSpellingsTheDocumentationUses_AreAllTheSamePath
// verifies the normalization, since GitLab writes the same endpoint four ways
// and each difference would otherwise be a false finding.
func TestNormalizePath_TheSpellingsTheDocumentationUses_AreAllTheSamePath(t *testing.T) {
	cases := []struct {
		name string
		path string
		want []string
	}{
		{name: "as recorded", path: "/projects/:id/issues", want: []string{"projects", ":id", "issues"}},
		{name: "without the leading slash", path: "projects/:id/issues", want: []string{"projects", ":id", "issues"}},
		{name: "with the api prefix", path: "/api/v4/projects/:id/issues", want: []string{"projects", ":id", "issues"}},
		{name: "with a trailing slash", path: "/projects/:id/issues/", want: []string{"projects", ":id", "issues"}},
		{name: "with a query", path: "/projects/:id/issues?state=opened", want: []string{"projects", ":id", "issues"}},
		{name: "with a fragment", path: "/projects/:id/issues#notes", want: []string{"projects", ":id", "issues"}},
		{name: "the root", path: "/", want: nil},
		{name: "nothing at all", path: "", want: nil},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := normalizePath(testCase.path); !slices.Equal(got, testCase.want) {
				t.Errorf("normalizePath(%q) = %v, want %v", testCase.path, got, testCase.want)
			}
		})
	}
}

// TestRecordedEndpoints_OneEntryPerMethodAndPath verifies the folding, since a
// list that named one of the packages issuing an endpoint would send a reader
// to the wrong file.
func TestRecordedEndpoints_OneEntryPerMethodAndPath(t *testing.T) {
	endpoints := recordedEndpoints([]requestinventory.Row{
		{Package: "internal/tools/second", Kind: requestinventory.KindREST, Method: "GET", Path: "/projects/:id"},
		{Package: "internal/tools/first", Kind: requestinventory.KindREST, Method: "GET", Path: "/projects/:id"},
		{Package: "internal/tools/first", Kind: requestinventory.KindREST, Method: "DELETE", Path: "/projects/:id"},
		{Package: "internal/tools/first", Kind: requestinventory.KindREST, Method: "PUT", Path: "/projects/:id"},
		{Package: "internal/tools/first", Kind: requestinventory.KindREST, Method: "POST", Path: "/projects/:id"},
		{Package: "internal/tools/first", Kind: requestinventory.KindREST, Method: "GET", Path: "/groups/:id"},
		{Package: "internal/tools/epics", Kind: requestinventory.KindGraphQL, Method: "POST", Path: "/graphql"},
	})

	if len(endpoints) != 5 {
		t.Fatalf("recordedEndpoints() = %+v, want five REST endpoints", endpoints)
	}
	// The four methods on one path are what makes the second key visible: the
	// order has to be the same between two runs of the same inventory, and the
	// map the fold is built in has no order of its own.
	order := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		order = append(order, endpoint.Method+" "+endpoint.Path)
	}
	want := []string{
		"GET /groups/:id",
		http.MethodDelete + " /projects/:id",
		"GET /projects/:id",
		http.MethodPost + " /projects/:id",
		http.MethodPut + " /projects/:id",
	}
	if !slices.Equal(order, want) {
		t.Errorf("recordedEndpoints() = %v, want them ordered by path then method: %v", order, want)
	}
	if !slices.Equal(endpoints[2].Packages, []string{"internal/tools/first", "internal/tools/second"}) {
		t.Errorf("packages = %v, want every package that issues it, sorted", endpoints[2].Packages)
	}
}

// TestEndpointString_ReadsAsALineAReportPrints verifies the rendering a text
// summary uses.
func TestEndpointString_ReadsAsALineAReportPrints(t *testing.T) {
	line := Endpoint{Method: "GET", Path: "/projects/:id", Packages: []string{"a", "b"}}.String()

	if line != "GET /projects/:id (a, b)" {
		t.Errorf("String() = %q, want the method, the path and the packages", line)
	}
}
